package sync_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tally/internal/sync"
)

func TestWebDAVClientInsecureCheck(t *testing.T) {
	// 非局域网公网 HTTP 应该被拦截
	_, err := sync.NewClient("http://example.com/dav", "user", "pass")
	if err != sync.ErrInsecureHTTP {
		t.Fatalf("expected ErrInsecureHTTP, got %v", err)
	}

	// 本地 loopback HTTP 应该被允许
	c, err := sync.NewClient("http://127.0.0.1:8080/dav", "user", "pass")
	if err != nil {
		t.Fatalf("expected loopback http allowed, got %v", err)
	}
	if c == nil {
		t.Fatal("client is nil")
	}
}

func TestWebDAVClientOperations(t *testing.T) {
	files := make(map[string][]byte)
	dirs := map[string]bool{"/": true}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "testuser" || pass != "testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		path := r.URL.Path

		switch r.Method {
		case "PROPFIND", "OPTIONS":
			if path == "/" || dirs[path] || dirs[strings.TrimSuffix(path, "/")] {
				w.WriteHeader(http.StatusMultiStatus)
				return
			}
			if _, ok := files[path]; ok {
				w.WriteHeader(http.StatusMultiStatus)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case "MKCOL":
			clean := strings.TrimSuffix(path, "/")
			if dirs[clean] {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			dirs[clean] = true
			w.WriteHeader(http.StatusCreated)
		case "PUT":
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			files[path] = buf
			w.WriteHeader(http.StatusCreated)
		case "GET":
			content, ok := files[path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(content)
		case "MOVE":
			dest := r.Header.Get("Destination")
			// 提取路径部分
			idx := strings.Index(dest, "://")
			destPath := dest
			if idx != -1 {
				slashIdx := strings.Index(dest[idx+3:], "/")
				if slashIdx != -1 {
					destPath = dest[idx+3+slashIdx:]
				}
			}
			content, ok := files[path]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			delete(files, path)
			files[destPath] = content
			w.WriteHeader(http.StatusCreated)
		case "DELETE":
			delete(files, path)
			delete(dirs, strings.TrimSuffix(path, "/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// 1. 凭据错误测试
	badClient, err := sync.NewClient(server.URL, "wrong", "pass")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if err := badClient.Ping(ctx); err != sync.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}

	// 2. 正常连接测试
	client, err := sync.NewClient(server.URL, "testuser", "testpass")
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// 3. 递归建目录
	if err := client.EnsureDir(ctx, "Tally/packages"); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// 4. 上传文件与读取
	testData := []byte("Hello WebDAV content")
	if err := client.Put(ctx, "Tally/test.txt", testData); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 5. Stat 检查存在
	exists, err := client.Stat(ctx, "Tally/test.txt")
	if err != nil || !exists {
		t.Fatalf("Stat expected true, got %v (err: %v)", exists, err)
	}

	// 6. Get 下载
	got, err := client.Get(ctx, "Tally/test.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(got) != string(testData) {
		t.Fatalf("expected %s, got %s", string(testData), string(got))
	}

	// 7. Move 原子重命名
	if err := client.Move(ctx, "Tally/test.txt", "Tally/test.renamed.txt"); err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	got2, err := client.Get(ctx, "Tally/test.renamed.txt")
	if err != nil || string(got2) != string(testData) {
		t.Fatalf("Get after Move failed: %v", err)
	}

	// 8. Delete 删除
	if err := client.Delete(ctx, "Tally/test.renamed.txt"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	exists2, err := client.Stat(ctx, "Tally/test.renamed.txt")
	if err != nil || exists2 {
		t.Fatalf("Stat expected false after delete, got %v (err: %v)", exists2, err)
	}
}
