package sync

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"strings"
	"sync"
)

// recordedRequest 记录测试服务端收到的请求信息，用于断言协议头与调用顺序。
type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
}

// rfc4918Server 是严格模拟 RFC 4918 行为的内存 WebDAV 测试服务端。
type rfc4918Server struct {
	mu                       sync.Mutex
	dirs                     map[string]bool
	files                    map[string][]byte
	username                 string
	password                 string
	forbiddenPaths           map[string]bool
	insufficientStorage      bool
	propfindMethodNotAllowed bool
	optionsRequiresAuth      bool
	requests                 []recordedRequest

	// 可选钩子，用于注入特定故障（如回读损坏、MOVE 失败等）。
	onPut  func(reqPath string, data []byte) (override []byte, status int)
	onGet  func(reqPath string, data []byte) (override []byte, status int)
	onMove func(srcPath, dstPath string) (status int)

	srv *httptest.Server
}

func newRFC4918Server(username, password string) *rfc4918Server {
	s := &rfc4918Server{
		dirs:           map[string]bool{"/": true},
		files:          make(map[string][]byte),
		username:       username,
		password:       password,
		forbiddenPaths: make(map[string]bool),
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	return s
}

func (s *rfc4918Server) URL() string {
	return s.srv.URL
}

func (s *rfc4918Server) Close() {
	s.srv.Close()
}

func (s *rfc4918Server) snapshotRequests() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

func (s *rfc4918Server) hasFile(relPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	clean := cleanDAVPath(relPath)
	_, ok := s.files[clean]
	return ok
}

func (s *rfc4918Server) hasDir(relPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	clean := cleanDAVPath(relPath)
	return s.dirs[clean]
}

func (s *rfc4918Server) getRawFile(relPath string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clean := cleanDAVPath(relPath)
	b, ok := s.files[clean]
	if !ok {
		return nil, false
	}
	cp := append([]byte(nil), b...)
	return cp, true
}

func (s *rfc4918Server) setRawFile(relPath string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	clean := cleanDAVPath(relPath)
	s.files[clean] = append([]byte(nil), data...)
}

func cleanDAVPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	return cleaned
}

func (s *rfc4918Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests = append(s.requests, recordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Header: r.Header.Clone(),
	})

	// 鉴权检查（部分服务器对 OPTIONS 免鉴权，可通过 optionsRequiresAuth 控制）。
	if r.Method != http.MethodOptions || s.optionsRequiresAuth {
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.username || pass != s.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="TallyWebDAV"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	reqPath := cleanDAVPath(r.URL.Path)
	if s.forbiddenPaths[reqPath] {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodOptions:
		w.Header().Set("DAV", "1, 2")
		w.Header().Set("Allow", "OPTIONS, PROPFIND, GET, PUT, MKCOL, DELETE, MOVE")
		w.WriteHeader(http.StatusOK)

	case "PROPFIND":
		if s.propfindMethodNotAllowed && reqPath == "/" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.dirs[reqPath] {
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>%s</D:href><D:propstat><D:prop><D:resourcetype><D:collection/></D:resourcetype></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`, reqPath)
			return
		}
		if data, ok := s.files[reqPath]; ok {
			w.Header().Set("Content-Type", "application/xml; charset=utf-8")
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:"><D:response><D:href>%s</D:href><D:propstat><D:prop><D:getcontentlength>%d</D:getcontentlength></D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`, reqPath, len(data))
			return
		}
		w.WriteHeader(http.StatusNotFound)

	case "MKCOL":
		if s.insufficientStorage {
			w.WriteHeader(http.StatusInsufficientStorage)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(body) > 0 {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		if reqPath == "/" || s.dirs[reqPath] || s.files[reqPath] != nil {
			// RFC 4918 Section 9.3.1: 目标已存在时返回 405 Method Not Allowed。
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		parent := path.Dir(reqPath)
		if !s.dirs[parent] {
			// RFC 4918 Section 9.3.1: 父集合不存在时返回 409 Conflict。
			w.WriteHeader(http.StatusConflict)
			return
		}
		s.dirs[reqPath] = true
		w.WriteHeader(http.StatusCreated)

	case http.MethodPut:
		if s.insufficientStorage {
			w.WriteHeader(http.StatusInsufficientStorage)
			return
		}
		if s.dirs[reqPath] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		parent := path.Dir(reqPath)
		if !s.dirs[parent] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.onPut != nil {
			override, status := s.onPut(reqPath, body)
			if status != 0 {
				w.WriteHeader(status)
				return
			}
			if override != nil {
				body = override
			}
		}
		_, existed := s.files[reqPath]
		s.files[reqPath] = body
		if existed {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusCreated)
		}

	case http.MethodGet:
		if s.dirs[reqPath] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		data, ok := s.files[reqPath]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if s.onGet != nil {
			override, status := s.onGet(reqPath, data)
			if status != 0 {
				w.WriteHeader(status)
				return
			}
			if override != nil {
				data = override
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)

	case "MOVE":
		destRaw := r.Header.Get("Destination")
		if destRaw == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		destURL, err := url.Parse(destRaw)
		if err != nil || destURL.Host != r.Host {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		dstPath := cleanDAVPath(destURL.Path)
		if s.forbiddenPaths[dstPath] {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		overwrite := r.Header.Get("Overwrite")
		if overwrite == "" {
			overwrite = "T"
		}
		if overwrite != "T" && overwrite != "F" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.onMove != nil {
			if status := s.onMove(reqPath, dstPath); status != 0 {
				w.WriteHeader(status)
				return
			}
		}
		data, ok := s.files[reqPath]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		dstParent := path.Dir(dstPath)
		if !s.dirs[dstParent] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		_, dstExisted := s.files[dstPath]
		if dstExisted && overwrite == "F" {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		if s.dirs[dstPath] {
			w.WriteHeader(http.StatusConflict)
			return
		}
		delete(s.files, reqPath)
		s.files[dstPath] = data
		if dstExisted {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusCreated)
		}

	case http.MethodDelete:
		if reqPath == "/" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if _, ok := s.files[reqPath]; ok {
			delete(s.files, reqPath)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if s.dirs[reqPath] {
			prefix := reqPath + "/"
			for f := range s.files {
				if strings.HasPrefix(f, prefix) {
					delete(s.files, f)
				}
			}
			for d := range s.dirs {
				if d == reqPath || strings.HasPrefix(d, prefix) {
					delete(s.dirs, d)
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
