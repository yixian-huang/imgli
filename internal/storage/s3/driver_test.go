package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/storage"
)

// mockS3 内存 S3 桩：按 URL path 存对象，支持 PUT/GET(Range)/HEAD/DELETE。
type mockS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	// last 记录最近一次请求，供断言 Host / Path
	lastHost      string
	lastPath      string
	lastContentLn int64 // 最近一次请求的 Content-Length(codex F1:验 *os.File 不走 chunked)
	// forceStatus: 若非 0，对该 method 强制返回该状态码（用于 Exists 403 等）
	forceStatus  map[string]int
	ignoreRange  bool // codex F3:GET 无视 Range 恒返 200 全量(模拟不合规服务端)
	headNoLength bool // HEAD 200 但不带 Content-Length（部分网关/MinIO 代理）
}

func newMockS3() *mockS3 {
	return &mockS3{
		objects:     make(map[string][]byte),
		forceStatus: make(map[string]int),
	}
}

func (m *mockS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastHost = r.Host
	m.lastPath = r.URL.Path
	m.lastContentLn = r.ContentLength

	if code, ok := m.forceStatus[r.Method]; ok && code != 0 {
		w.WriteHeader(code)
		return
	}

	// path-style: /bucket/key... ; virtual-host: /key...
	// 统一用 r.URL.Path 作为存储键，兼容两种风格。
	storeKey := r.URL.Path

	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		m.objects[storeKey] = body
		w.WriteHeader(http.StatusOK)
	case http.MethodGet:
		data, ok := m.objects[storeKey]
		if !ok {
			http.NotFound(w, r)
			return
		}
		rangeHdr := r.Header.Get("Range")
		if strings.HasPrefix(rangeHdr, "bytes=") && !m.ignoreRange {
			// bytes=N- 或 bytes=N-M
			spec := strings.TrimPrefix(rangeHdr, "bytes=")
			parts := strings.SplitN(spec, "-", 2)
			start, _ := strconv.ParseInt(parts[0], 10, 64)
			end := int64(len(data) - 1)
			if len(parts) > 1 && parts[1] != "" {
				end, _ = strconv.ParseInt(parts[1], 10, 64)
			}
			if start < 0 || start >= int64(len(data)) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if end >= int64(len(data)) {
				end = int64(len(data) - 1)
			}
			chunk := data[start : end+1]
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
			w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(chunk)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	case http.MethodHead:
		data, ok := m.objects[storeKey]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !m.headNoLength {
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		delete(m.objects, storeKey)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// redirectHTTPS 把 https://host/path 请求改写到桩 server（scheme→http，host→桩）。
type redirectHTTPS struct {
	base   string // e.g. http://127.0.0.1:12345
	inner  http.RoundTripper
	server *httptest.Server
}

func (r *redirectHTTPS) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := http.NewRequestWithContext(req.Context(), req.Method, r.base+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	u.Header = req.Header.Clone()
	u.ContentLength = req.ContentLength // 重建请求须承接 ContentLength(否则桩看到 -1,遮蔽 F1 验证)
	// 保留原 Host（virtual-host 断言需要）；Go 的 Host 字段在 URL 改写后会被覆盖，
	// 桩通过 r.Host 读到的是 u.Host。显式设为原 req.Host。
	u.Host = req.Host
	if u.Host == "" {
		u.Host = req.URL.Host
	}
	if r.inner == nil {
		r.inner = http.DefaultTransport
	}
	return r.inner.RoundTrip(u)
}

func testDriver(t *testing.T, mock *mockS3, cfg map[string]string) *Driver {
	t.Helper()
	srv := httptest.NewServer(mock)
	t.Cleanup(srv.Close)

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	d.Client = &http.Client{
		Transport: &redirectHTTPS{base: srv.URL},
	}
	d.now = func() time.Time {
		return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return d
}

func baseCfg(pathStyle string) map[string]string {
	return map[string]string{
		"endpoint":          "s3.example.com",
		"region":            "us-east-1",
		"bucket":            "mybucket",
		"access_key_id":     "AKIDEXAMPLE0000",
		"secret_access_key": "SECRETKEY0000000000000000",
		"path_style":        pathStyle,
	}
}

func TestS3PutOpenDelete(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()

	payload := []byte("hello-s3-png")
	if err := d.Put(ctx, "a/b.png", bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// 桩应收到 path-style PUT
	mock.mu.Lock()
	path := mock.lastPath
	mock.mu.Unlock()
	if path != "/mybucket/a/b.png" {
		t.Errorf("PUT path = %q, want /mybucket/a/b.png", path)
	}

	// 用自定义 handler 再验一次 Authorization / x-amz-content-sha256
	var sawAuth, sawUnsigned bool
	checkMock := newMockS3()
	// wrap: 记录头
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			sawAuth = true
		}
		if r.Header.Get("x-amz-content-sha256") == "UNSIGNED-PAYLOAD" {
			sawUnsigned = true
		}
		checkMock.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	d2, err := New(baseCfg("true"))
	if err != nil {
		t.Fatal(err)
	}
	d2.Client = &http.Client{Transport: &redirectHTTPS{base: srv.URL}}
	d2.now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }
	if err := d2.Put(ctx, "a/b.png", bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put2: %v", err)
	}
	if !sawAuth {
		t.Error("Authorization 应以 AWS4-HMAC-SHA256 开头")
	}
	if !sawUnsigned {
		t.Error("x-amz-content-sha256 应为 UNSIGNED-PAYLOAD")
	}

	rsc, err := d2.Open(ctx, "a/b.png")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rsc)
	rsc.Close()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("Open body = %q, want %q", got, payload)
	}

	if err := d2.Delete(ctx, "a/b.png"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err = d2.Open(ctx, "a/b.png")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("Open after Delete: err=%v, want ErrNotFound", err)
	}
}

func TestS3RangeSeek(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()

	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	if err := d.Put(ctx, "range.bin", bytes.NewReader(data)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rsc, err := d.Open(ctx, "range.bin")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rsc.Close()

	end, err := rsc.Seek(0, io.SeekEnd)
	if err != nil || end != 100 {
		t.Fatalf("SeekEnd = %d, %v; want 100", end, err)
	}
	if _, err := rsc.Seek(10, io.SeekStart); err != nil {
		t.Fatalf("SeekStart(10): %v", err)
	}
	got, err := io.ReadAll(rsc)
	if err != nil {
		t.Fatalf("ReadAll from 10: %v", err)
	}
	if len(got) != 90 {
		t.Fatalf("len(got)=%d, want 90", len(got))
	}
	if got[0] != 10 {
		t.Errorf("got[0]=%d, want 10", got[0])
	}
	if !bytes.Equal(got, data[10:]) {
		t.Errorf("range content mismatch")
	}

	// 新 Open 全量
	rsc2, err := d.Open(ctx, "range.bin")
	if err != nil {
		t.Fatalf("Open2: %v", err)
	}
	defer rsc2.Close()
	all, err := io.ReadAll(rsc2)
	if err != nil {
		t.Fatalf("ReadAll full: %v", err)
	}
	if !bytes.Equal(all, data) {
		t.Errorf("full read len=%d, want 100", len(all))
	}
}

func TestS3VirtualHostVsPathStyle(t *testing.T) {
	ctx := context.Background()
	payload := []byte("x")

	// path_style false → Host 含 bucket.
	mockVH := newMockS3()
	dVH := testDriver(t, mockVH, baseCfg("false"))
	if err := dVH.Put(ctx, "obj", bytes.NewReader(payload)); err != nil {
		t.Fatalf("virtual Put: %v", err)
	}
	mockVH.mu.Lock()
	host := mockVH.lastHost
	path := mockVH.lastPath
	mockVH.mu.Unlock()
	if !strings.Contains(host, "mybucket.") {
		t.Errorf("virtual-host Host=%q, want contain mybucket.", host)
	}
	if strings.HasPrefix(path, "/mybucket/") {
		t.Errorf("virtual-host path should not start with /bucket/: %q", path)
	}

	// path_style true → URI 含 /bucket/
	mockPS := newMockS3()
	dPS := testDriver(t, mockPS, baseCfg("true"))
	if err := dPS.Put(ctx, "obj", bytes.NewReader(payload)); err != nil {
		t.Fatalf("path-style Put: %v", err)
	}
	mockPS.mu.Lock()
	pathPS := mockPS.lastPath
	mockPS.mu.Unlock()
	if !strings.Contains(pathPS, "/mybucket/") {
		t.Errorf("path-style path=%q, want contain /mybucket/", pathPS)
	}
}

func TestS3Prefix(t *testing.T) {
	mock := newMockS3()
	cfg := baseCfg("true")
	cfg["prefix"] = "imgli/"
	d := testDriver(t, mock, cfg)
	ctx := context.Background()

	if err := d.Put(ctx, "a/b.png", bytes.NewReader([]byte("p"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	mock.mu.Lock()
	path := mock.lastPath
	// 对象是否落在 imgli/ 下
	_, ok := mock.objects["/mybucket/imgli/a/b.png"]
	mock.mu.Unlock()
	if path != "/mybucket/imgli/a/b.png" {
		t.Errorf("path=%q, want /mybucket/imgli/a/b.png", path)
	}
	if !ok {
		t.Errorf("object not stored at imgli/a/b.png; objects=%v", mock.objects)
	}
}

func TestS3Exists404(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()

	ok, err := d.Exists(ctx, "missing")
	if err != nil || ok {
		t.Errorf("Exists missing: ok=%v err=%v; want false,nil", ok, err)
	}

	if err := d.Put(ctx, "hit", bytes.NewReader([]byte("y"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ok, err = d.Exists(ctx, "hit")
	if err != nil || !ok {
		t.Errorf("Exists hit: ok=%v err=%v; want true,nil", ok, err)
	}

	// 403 → error
	mock.forceStatus["HEAD"] = 403
	_, err = d.Exists(ctx, "hit")
	if err == nil {
		t.Error("Exists with 403: want error")
	}
}

// codex F1:*os.File 上传须显式 Content-Length(不走 chunked,否则真 S3 MissingContentLength)。
func TestS3PutOsFileContentLength(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	f, err := os.CreateTemp(t.TempDir(), "up-*")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("file-body-1234567890")
	f.Write(payload)
	f.Seek(0, io.SeekStart)
	if err := d.Put(context.Background(), "f.png", f); err != nil {
		t.Fatalf("Put os.File: %v", err)
	}
	mock.mu.Lock()
	cl := mock.lastContentLn
	mock.mu.Unlock()
	if cl != int64(len(payload)) {
		t.Errorf("Content-Length=%d, want %d(不应为 -1/chunked)", cl, len(payload))
	}
}

// codex F2:含特殊字符的对象键须 S3 规范编码,签名与请求路径一致,端到端往返正确。
func TestS3SpecialCharKey(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()
	key := "dir/a b+c&d=e.png" // 含 space/+/&/= 等 url.PathEscape 会漏编的字符
	payload := []byte("special")
	if err := d.Put(ctx, key, bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put special: %v", err)
	}
	mock.mu.Lock()
	path := mock.lastPath // 服务端解码后的 path
	mock.mu.Unlock()
	if path != "/mybucket/"+key {
		t.Errorf("解码 path=%q, want /mybucket/%s(编码往返一致)", path, key)
	}
	rsc, err := d.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open special: %v", err)
	}
	got, _ := io.ReadAll(rsc)
	rsc.Close()
	if !bytes.Equal(got, payload) {
		t.Errorf("special key 往返内容不符")
	}
}

// codex F3:offset>0 却收 200(服务端忽略 Range)须报错,不能读到错字节。
func TestS3RangeIgnoredBy200Errors(t *testing.T) {
	mock := newMockS3()
	mock.ignoreRange = true
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()
	data := make([]byte, 50)
	for i := range data {
		data[i] = byte(i)
	}
	if err := d.Put(ctx, "r.bin", bytes.NewReader(data)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rsc, err := d.Open(ctx, "r.bin")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rsc.Close()
	if _, err := rsc.Seek(10, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 10)
	if _, err := rsc.Read(buf); err == nil {
		t.Error("offset>0 收 200 应报错(防读错字节),got nil")
	}
}

func TestS3OpenFallsBackToGETWhenHEADForbidden(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()
	payload := []byte("minio-head-403")
	if err := d.Put(ctx, "soon.png", bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	mock.forceStatus["HEAD"] = http.StatusForbidden
	rsc, err := d.Open(ctx, "soon.png")
	if err != nil {
		t.Fatalf("HEAD 403 应回落 GET, err=%v", err)
	}
	defer rsc.Close()
	got, err := io.ReadAll(rsc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("body=%q want %q", got, payload)
	}
}

func TestS3OpenFallsBackToGETWhenHEADOmitsContentLength(t *testing.T) {
	mock := newMockS3()
	mock.headNoLength = true
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()
	payload := []byte("no-cl")
	if err := d.Put(ctx, "ncl.png", bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	rsc, err := d.Open(ctx, "ncl.png")
	if err != nil {
		t.Fatalf("HEAD 无 Content-Length 应回落 GET, err=%v", err)
	}
	defer rsc.Close()
	got, err := io.ReadAll(rsc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("body=%q want %q", got, payload)
	}
}

func TestS3OpenFallsBackToGETWhenHEADUnavailable(t *testing.T) {
	mock := newMockS3()
	d := testDriver(t, mock, baseCfg("true"))
	ctx := context.Background()
	payload := []byte("head-503")
	if err := d.Put(ctx, "w.png", bytes.NewReader(payload)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	mock.forceStatus["HEAD"] = http.StatusServiceUnavailable
	rsc, err := d.Open(ctx, "w.png")
	if err != nil {
		t.Fatalf("HEAD 503 应回落 GET, err=%v", err)
	}
	defer rsc.Close()
	got, _ := io.ReadAll(rsc)
	if !bytes.Equal(got, payload) {
		t.Errorf("body=%q", got)
	}
}

func TestS3OpenStillNotFoundWhenMissing(t *testing.T) {
	d := testDriver(t, newMockS3(), baseCfg("true"))
	_, err := d.Open(context.Background(), "nope.png")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("err=%v want ErrNotFound", err)
	}
}

func TestNewMissingConfig(t *testing.T) {
	_, err := New(map[string]string{})
	if err == nil {
		t.Fatal("want error for empty cfg")
	}
	if !strings.Contains(err.Error(), "缺少必填配置") {
		t.Errorf("err=%v", err)
	}
}

// endpoint 含 http:// 前缀 → 明文端点(MinIO/on-prem);缺省 https。
func TestS3EndpointScheme(t *testing.T) {
	d, err := New(map[string]string{
		"endpoint": "http://localhost:9000", "region": "us-east-1", "bucket": "b",
		"access_key_id": "k", "secret_access_key": "s", "path_style": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if d.scheme != "http" || d.endpoint != "localhost:9000" {
		t.Errorf("scheme=%q endpoint=%q, want http/localhost:9000", d.scheme, d.endpoint)
	}
	d2, _ := New(map[string]string{
		"endpoint": "s3.us-east-1.amazonaws.com", "region": "us-east-1", "bucket": "b",
		"access_key_id": "k", "secret_access_key": "s",
	})
	if d2.scheme != "https" {
		t.Errorf("缺省 scheme=%q, want https", d2.scheme)
	}
}
