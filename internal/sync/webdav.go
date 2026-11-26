package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnauthorized        = errors.New("WebDAV 用户名或密码错误 (401)")
	ErrForbidden           = errors.New("WebDAV 目录权限不足 (403)")
	ErrNotFound            = errors.New("WebDAV 目录或文件不存在 (404)")
	ErrInsufficientStorage = errors.New("WebDAV 服务器空间不足 (507)")
	ErrInsecureHTTP        = errors.New("仅允许 HTTPS 或局域网内网 HTTP 连接")
)

// Client 是连接 WebDAV 服务器的轻量级客户端。
type Client struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client
}

// NewClient 初始化 WebDAV 客户端并校验 URL 协议安全性。
func NewClient(rawURL, username, password string) (*Client, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("WebDAV 服务器地址不能为空")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("无效的 URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("URL 必须使用 https 或 http 协议")
	}
	if parsed.Scheme == "http" && !isPrivateOrLoopback(parsed.Hostname()) {
		return nil, ErrInsecureHTTP
	}
	if !strings.HasSuffix(rawURL, "/") {
		rawURL += "/"
	}
	return &Client{
		BaseURL:  rawURL,
		Username: username,
		Password: password,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func isPrivateOrLoopback(host string) bool {
	host = strings.Trim(host, "[]")
	if host == "localhost" || strings.HasSuffix(host, ".local") {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func (c *Client) resolveURL(relPath string) string {
	relPath = strings.TrimPrefix(relPath, "/")
	return c.BaseURL + relPath
}

func (c *Client) do(ctx context.Context, method, relPath string, body []byte, headers map[string]string) (*http.Response, error) {
	reqURL := c.resolveURL(relPath)
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	if c.Username != "" || c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		_ = resp.Body.Close()
		return nil, ErrUnauthorized
	case http.StatusForbidden:
		_ = resp.Body.Close()
		return nil, ErrForbidden
	case http.StatusNotFound:
		_ = resp.Body.Close()
		return nil, ErrNotFound
	case 507: // Insufficient Storage
		_ = resp.Body.Close()
		return nil, ErrInsufficientStorage
	}
	return resp, nil
}

// Ping 测试 WebDAV 连接连通性与凭据正确性。
func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.do(ctx, "PROPFIND", "", nil, map[string]string{"Depth": "0"})
	if err != nil {
		return err
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return fmt.Errorf("关闭连接测试响应失败: %w", closeErr)
	}
	if (resp.StatusCode >= 200 && resp.StatusCode < 300) || resp.StatusCode == http.StatusMultiStatus {
		return nil
	}
	if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		optResp, optErr := c.do(ctx, "OPTIONS", "", nil, nil)
		if optErr != nil {
			return optErr
		}
		if closeErr := optResp.Body.Close(); closeErr != nil {
			return fmt.Errorf("关闭 OPTIONS 响应失败: %w", closeErr)
		}
		if optResp.StatusCode >= 200 && optResp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("连接测试返回非成功状态码: %d", optResp.StatusCode)
	}
	return fmt.Errorf("连接测试返回非成功状态码: %d", resp.StatusCode)
}

// EnsureDir 递归确保远端相对路径的各层目录已创建。
func (c *Client) EnsureDir(ctx context.Context, relPath string) error {
	relPath = strings.Trim(relPath, "/")
	if relPath == "" {
		return nil
	}
	parts := strings.Split(relPath, "/")
	current := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		resp, err := c.do(ctx, "MKCOL", current+"/", nil, nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusCreated, http.StatusOK, http.StatusNoContent, http.StatusMethodNotAllowed:
			// 405 Method Not Allowed 在 RFC 4918 中表示集合已存在
			continue
		default:
			return fmt.Errorf("创建远端目录 %s 失败，状态码: %d", current, resp.StatusCode)
		}
	}
	return nil
}

// Put 上传单个文件到指定远端相对路径。
func (c *Client) Put(ctx context.Context, relPath string, data []byte) error {
	resp, err := c.do(ctx, "PUT", relPath, data, map[string]string{
		"Content-Type": "application/octet-stream",
	})
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("上传文件失败，状态码: %d", resp.StatusCode)
}

// Get 下载指定远端相对路径的文件。
func (c *Client) Get(ctx context.Context, relPath string) ([]byte, error) {
	resp, err := c.do(ctx, "GET", relPath, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载文件失败，状态码: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Move 在远端原子重命名或移动文件。
func (c *Client) Move(ctx context.Context, srcRelPath, dstRelPath string) error {
	destURL := c.resolveURL(dstRelPath)
	resp, err := c.do(ctx, "MOVE", srcRelPath, nil, map[string]string{
		"Destination": destURL,
		"Overwrite":   "T",
	})
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("移动文件失败，状态码: %d", resp.StatusCode)
}

// Delete 删除远端相对路径的文件或目录。
func (c *Client) Delete(ctx context.Context, relPath string) error {
	resp, err := c.do(ctx, "DELETE", relPath, nil, nil)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("删除文件失败，状态码: %d", resp.StatusCode)
}

// Stat 检查远端相对路径的文件或目录是否存在。
func (c *Client) Stat(ctx context.Context, relPath string) (bool, error) {
	resp, err := c.do(ctx, "PROPFIND", relPath, nil, map[string]string{
		"Depth": "0",
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 || resp.StatusCode == 207 {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("检查文件状态失败，状态码: %d", resp.StatusCode)
}
