package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yixian-huang/imgli/internal/storage"
)

// Driver 是 S3 兼容存储驱动（手写 SigV4，零 SDK）。
type Driver struct {
	endpoint      string
	region        string
	bucket        string
	accessKey     string
	secretKey     string
	pathStyle     bool
	prefix        string
	presignScheme string // 预签名目标域的 scheme(https/http)
	presignHost   string // 预签名目标域的 host(不含 scheme),空=该策略不启用预签名
	scheme        string // http 或 https(endpoint 可含 http://前缀,缺省 https)
	Client        *http.Client
	now           func() time.Time
}

// New 从配置 map 构造 Driver。必填: endpoint, region, bucket, access_key_id, secret_access_key。
// 可选: path_style ("true"/"false"/""), prefix。endpoint 可含 http://或 https://前缀
// (缺省 https;http 供 MinIO/on-prem 明文端点)。
func New(cfg map[string]string) (*Driver, error) {
	endpoint := strings.TrimSpace(cfg["endpoint"])
	scheme := "https"
	if s := strings.ToLower(endpoint); strings.HasPrefix(s, "http://") {
		scheme = "http"
		endpoint = endpoint[len("http://"):]
	} else if strings.HasPrefix(s, "https://") {
		endpoint = endpoint[len("https://"):]
	}
	endpoint = strings.TrimRight(endpoint, "/")
	region := strings.TrimSpace(cfg["region"])
	bucket := strings.TrimSpace(cfg["bucket"])
	accessKey := strings.TrimSpace(cfg["access_key_id"])
	secretKey := strings.TrimSpace(cfg["secret_access_key"])
	if endpoint == "" {
		return nil, fmt.Errorf("s3: 缺少必填配置 endpoint")
	}
	if region == "" {
		return nil, fmt.Errorf("s3: 缺少必填配置 region")
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3: 缺少必填配置 bucket")
	}
	if accessKey == "" {
		return nil, fmt.Errorf("s3: 缺少必填配置 access_key_id")
	}
	if secretKey == "" {
		return nil, fmt.Errorf("s3: 缺少必填配置 secret_access_key")
	}

	pathStyleStr := cfg["path_style"]
	var pathStyle bool
	switch pathStyleStr {
	case "true":
		pathStyle = true
	case "", "false":
		pathStyle = false
	default:
		return nil, fmt.Errorf("s3: path_style 无效 %q", pathStyleStr)
	}

	// presign_domain 可选:私密图 302 的签名目标(如 https://s3.img.li)。必须是
	// 不经 CDN 缓存、不重写 path 的直连域——SigV4 签名覆盖 Host 与 URI path,
	// 经 CDN 或路径重写都会致校验失败,且缓存签名响应会跨用户串号。
	// 须为纯 origin(无 userinfo/path/query/fragment);host 按浏览器 authority
	// 规范化(小写、剥默认端口),签名与返回 URL 共用同一规范值,否则会静默 403。
	presignScheme, presignHost := "", ""
	if pd := strings.TrimSpace(cfg["presign_domain"]); pd != "" {
		u, err := url.Parse(pd)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("s3: presign_domain 非法 URL")
		}
		// 拒非纯 origin:签名只用 scheme+host,其余组件会被静默丢弃;userinfo
		// 明文凭据还会经策略 config 回显(presign_domain 不在掩码字段之列)。
		if u.User != nil {
			return nil, fmt.Errorf("s3: presign_domain 不得含用户名密码")
		}
		if u.Path != "" && u.Path != "/" {
			return nil, fmt.Errorf("s3: presign_domain 不得含 path")
		}
		if u.RawQuery != "" {
			return nil, fmt.Errorf("s3: presign_domain 不得含 query")
		}
		if u.Fragment != "" {
			return nil, fmt.Errorf("s3: presign_domain 不得含 fragment")
		}
		// 规范化 host:小写 + 剥与 scheme 匹配的默认端口。按最后一个 ':' 切分
		// 去端口,IPv6 字面量 [::1]:443 安全(无端口时 [::1] 无末尾冒号)。
		host := strings.ToLower(u.Host)
		// 非 ASCII 主机名拒绝:浏览器会转 punycode,我们无转换能力必致 403。
		for i := 0; i < len(host); i++ {
			if host[i] >= 0x80 {
				return nil, fmt.Errorf("s3: presign_domain 主机名不得含非 ASCII 字符")
			}
		}
		if i := strings.LastIndex(host, ":"); i >= 0 {
			port := host[i+1:]
			if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
				host = host[:i]
			}
		}
		presignScheme, presignHost = u.Scheme, host
	}

	return &Driver{
		endpoint:      endpoint,
		region:        region,
		bucket:        bucket,
		accessKey:     accessKey,
		secretKey:     secretKey,
		pathStyle:     pathStyle,
		prefix:        cfg["prefix"],
		presignScheme: presignScheme,
		presignHost:   presignHost,
		scheme:        scheme,
	}, nil
}

func (d *Driver) httpClient() *http.Client {
	if d.Client != nil {
		return d.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (d *Driver) nowOr() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

// s3URIEncode 按 S3 SigV4 规范编码对象键:除 unreserved(A-Za-z0-9-._~)外全部
// %HH 大写编码,保留 `/` 作路径分隔(codex 评审 F2:url.PathEscape 漏编 +:@&=$ 等,
// 会致特殊键签名不匹配)。请求 URL path 与 canonical URI 用同一编码,保签名一致。
func s3URIEncode(objectKey string) string {
	var b strings.Builder
	for i := 0; i < len(objectKey); i++ {
		c := objectKey[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '.', c == '_', c == '~', c == '/':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func (d *Driver) hostAndURI(objectKey string) (host, uri string) {
	escaped := s3URIEncode(objectKey)
	if d.pathStyle {
		return d.endpoint, "/" + d.bucket + "/" + escaped
	}
	return d.bucket + "." + d.endpoint, "/" + escaped
}

// bodyLen 探测 body 长度以显式设 Content-Length:S3 PUT 不接受未知长度 chunked
// (会 MissingContentLength;codex 评审 F1)。*os.File(上传管线)与 Len() 类
// (bytes.Reader 等)可测;未知类型返回 -1(退回 http 默认,可能 chunked)。
func bodyLen(r io.Reader) int64 {
	switch v := r.(type) {
	case nil:
		return 0
	case *os.File:
		if fi, err := v.Stat(); err == nil {
			return fi.Size()
		}
	case interface{ Len() int }:
		return int64(v.Len())
	}
	return -1
}

func (d *Driver) do(ctx context.Context, method, objectKey string, body io.Reader, payloadHash string, contentLength int64, extraHeaders map[string]string) (*http.Response, error) {
	host, uri := d.hostAndURI(d.prefix + objectKey)
	t := d.nowOr().UTC()
	amzDate := t.Format("20060102T150405Z")
	date := t.Format("20060102")

	headers := map[string]string{
		"host":                 host,
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	for k, v := range extraHeaders {
		headers[k] = v
	}

	auth := SignV4(method, uri, "", d.accessKey, d.secretKey, d.region, "s3", amzDate, date, payloadHash, headers)

	scheme := d.scheme
	if scheme == "" {
		scheme = "https"
	}
	req, err := http.NewRequestWithContext(ctx, method, scheme+"://"+host+uri, body)
	if err != nil {
		return nil, err
	}
	req.Host = host
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	req.Header.Set("x-amz-content-sha256", payloadHash)
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("Authorization", auth)
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}

	return d.httpClient().Do(req)
}

func (d *Driver) Put(ctx context.Context, key string, r io.Reader) error {
	resp, err := d.do(ctx, "PUT", key, r, "UNSIGNED-PAYLOAD", bodyLen(r), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("s3: PUT %d: %s", resp.StatusCode, snippet)
	}
	return nil
}

func (d *Driver) Open(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	resp, err := d.do(ctx, "HEAD", key, nil, "UNSIGNED-PAYLOAD", -1, nil)
	if err != nil {
		// 部分 MinIO/网关 HEAD 直接断连；回落 GET。
		return d.openViaGET(ctx, key)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, storage.ErrNotFound
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		size, perr := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if perr == nil && size >= 0 {
			return &rangeReadSeekCloser{d: d, ctx: ctx, key: key, size: size, offset: 0}, nil
		}
	}
	// HEAD 403/5xx 或无 Content-Length：MinIO/网关刚 PUT 完常见；改 GET。
	return d.openViaGET(ctx, key)
}

const maxOpenGETFallback = 64 << 20 // 64MiB：HEAD 不可用时整对象入内存，超限仍报错

func (d *Driver) openViaGET(ctx context.Context, key string) (io.ReadSeekCloser, error) {
	resp, err := d.do(ctx, "GET", key, nil, "UNSIGNED-PAYLOAD", -1, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, storage.ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("s3: GET %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxOpenGETFallback+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxOpenGETFallback {
		return nil, fmt.Errorf("s3: GET fallback 超过 %d 字节", maxOpenGETFallback)
	}
	return byteRSC{bytes.NewReader(data)}, nil
}

type byteRSC struct{ *bytes.Reader }

func (byteRSC) Close() error { return nil }

func (d *Driver) Delete(ctx context.Context, key string) error {
	resp, err := d.do(ctx, "DELETE", key, nil, "UNSIGNED-PAYLOAD", -1, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode < 300 {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("s3: DELETE %d: %s", resp.StatusCode, snippet)
}

func (d *Driver) Exists(ctx context.Context, key string) (bool, error) {
	resp, err := d.do(ctx, "HEAD", key, nil, "UNSIGNED-PAYLOAD", -1, nil)
	if err != nil {
		return false, err
	}
	resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("s3: HEAD %d", resp.StatusCode)
	}
}

type rangeReadSeekCloser struct {
	d      *Driver
	ctx    context.Context // Open 的 ctx,惰性 Range GET 继承之(codex 评审 F4)
	key    string
	size   int64
	offset int64
	body   io.ReadCloser // 惰性,当前 offset 起的 GET 流;nil 表示需重开
}

func (r *rangeReadSeekCloser) Read(p []byte) (int, error) {
	if r.offset >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		startOffset := r.offset
		resp, err := r.d.do(r.ctx, "GET", r.key, nil, "UNSIGNED-PAYLOAD", -1,
			map[string]string{"range": fmt.Sprintf("bytes=%d-", startOffset)})
		if err != nil {
			return 0, err
		}
		// offset>0 却收 200(服务端忽略 Range)会从第 0 字节返回,读到错字节——必须拒
		// (codex 评审 F3)。offset==0 时 200/206 皆可。
		if startOffset > 0 && resp.StatusCode == 200 {
			resp.Body.Close()
			return 0, fmt.Errorf("s3: 服务端忽略 Range(返回 200),offset=%d", startOffset)
		}
		if resp.StatusCode != 200 && resp.StatusCode != 206 {
			resp.Body.Close()
			return 0, fmt.Errorf("s3: GET range %d", resp.StatusCode)
		}
		r.body = resp.Body
	}
	n, err := r.body.Read(p)
	r.offset += int64(n)
	if err == io.EOF {
		r.body.Close()
		r.body = nil
		if r.offset < r.size {
			err = nil // 段读完但对象未尽,下次重开
		}
	}
	return n, err
}

func (r *rangeReadSeekCloser) Seek(off int64, whence int) (int64, error) {
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = off
	case io.SeekCurrent:
		abs = r.offset + off
	case io.SeekEnd:
		abs = r.size + off
	default:
		return 0, fmt.Errorf("s3: 无效 whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("s3: 负偏移")
	}
	if abs != r.offset && r.body != nil {
		r.body.Close()
		r.body = nil
	}
	r.offset = abs
	return abs, nil
}

func (r *rangeReadSeekCloser) Close() error {
	if r.body != nil {
		return r.body.Close()
	}
	return nil
}
