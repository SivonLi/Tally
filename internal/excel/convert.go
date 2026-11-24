package excel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrSofficeNotFound 表示没有找到 LibreOffice 的 soffice 可执行文件。
var ErrSofficeNotFound = errors.New("没有找到 LibreOffice 的 soffice")

// SofficeStatus 描述本机 LibreOffice 可执行文件的解析状态，供设置页与导入前检测展示。
type SofficeStatus struct {
	Path   string
	Source string
	Found  bool
	Error  string
}

// ResolveSoffice 按「用户设置路径 -> TALLY_SOFFICE 环境变量 -> 系统自动检测」顺序解析 soffice 状态。
func ResolveSoffice(configured string) SofficeStatus {
	cleaned := strings.TrimSpace(configured)
	if cleaned != "" {
		info, err := os.Stat(cleaned)
		if err != nil || info.IsDir() {
			return SofficeStatus{
				Path:   cleaned,
				Source: "configured",
				Found:  false,
				Error:  fmt.Sprintf("%v: %s", ErrSofficeNotFound, cleaned),
			}
		}
		return SofficeStatus{
			Path:   cleaned,
			Source: "configured",
			Found:  true,
		}
	}
	if explicit := strings.TrimSpace(os.Getenv("TALLY_SOFFICE")); explicit != "" {
		info, err := os.Stat(explicit)
		if err != nil || info.IsDir() {
			return SofficeStatus{
				Path:   explicit,
				Source: "env",
				Found:  false,
				Error:  fmt.Sprintf("%v: %s", ErrSofficeNotFound, explicit),
			}
		}
		return SofficeStatus{
			Path:   explicit,
			Source: "env",
			Found:  true,
		}
	}
	path, err := FindSoffice()
	if err != nil {
		return SofficeStatus{
			Source: "auto",
			Found:  false,
			Error:  err.Error(),
		}
	}
	return SofficeStatus{
		Path:   path,
		Source: "auto",
		Found:  true,
	}
}

// FindSoffice 查找本机的 LibreOffice soffice 可执行文件。
func FindSoffice() (string, error) {
	if explicit := os.Getenv("TALLY_SOFFICE"); explicit != "" {
		info, err := os.Stat(explicit)
		if err != nil || info.IsDir() {
			return "", fmt.Errorf("%w: %s", ErrSofficeNotFound, explicit)
		}
		return explicit, nil
	}
	if path, err := exec.LookPath("soffice"); err == nil {
		return path, nil
	}
	candidates := []string{
		"/opt/homebrew/bin/soffice",
		"/usr/local/bin/soffice",
		`C:\Program Files\LibreOffice\program\soffice.exe`,
		`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w，请安装 LibreOffice 或者手工另存为 xlsx", ErrSofficeNotFound)
}

// Converter 调用 LibreOffice 转换旧版 Excel 文件。
type Converter struct {
	SofficePath string
	Timeout     time.Duration
}

// Convert 把旧版工作簿转换为 xlsx，返回转换后的文件路径。
func (c Converter) Convert(inputPath, outputDir string) (string, error) {
	if strings.EqualFold(filepath.Ext(inputPath), ".xlsx") {
		return inputPath, nil
	}
	soffice := c.SofficePath
	if soffice == "" {
		var err error
		soffice, err = FindSoffice()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("创建转换目录: %w", err)
	}
	profileDir, err := os.MkdirTemp(outputDir, ".soffice-profile-")
	if err != nil {
		return "", fmt.Errorf("创建独立配置目录: %w", err)
	}
	defer os.RemoveAll(profileDir)

	base := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(outputDir, base+".xlsx")
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("清理旧转换文件: %w", err)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	profileURL := (&url.URL{Scheme: "file", Path: profileDir}).String()
	command := exec.CommandContext(ctx, soffice,
		"--headless",
		"-env:UserInstallation="+profileURL,
		"--convert-to", "xlsx",
		"--outdir", outputDir,
		inputPath,
	)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("转换超过 %s，输出: %s", timeout, strings.TrimSpace(string(output)))
	}
	if err != nil {
		return "", fmt.Errorf("转换失败: %w，输出: %s", err, strings.TrimSpace(string(output)))
	}
	if _, err := os.Stat(outputPath); err != nil {
		return "", fmt.Errorf("转换后没有生成文件: %s，输出: %s", outputPath, strings.TrimSpace(string(output)))
	}
	if _, err := InspectWorkbook(outputPath); err != nil {
		return "", fmt.Errorf("转换后的工作簿无法读取: %w", err)
	}
	return outputPath, nil
}
