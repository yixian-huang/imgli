package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/service/upload"
)

// UploadDeps 上传处理器依赖（由 server 装配注入）。
type UploadDeps struct {
	Svc         *upload.Service
	Res         *storagesvc.Resolver
	MaxBytes    int64        // 硬上限（防超大 body），实际按用户组上限精确拦截
	FetchAllow  []*net.IPNet // URL 抓取运维内网允许清单（默认空=严格）
	FetchClient *http.Client // 抓取远程图片专用客户端（拨号期 SSRF 校验，按 FetchAllow 构造）
	Hooks       interface {
		Emit(eventType string, data map[string]any)
	}
}

type UploadHandlers struct{ D UploadDeps }

func uploadResultDTO(res *upload.Result, res2 *storagesvc.Resolver) map[string]any {
	base := res2.LinkBase(res.Policy)
	links := imageLinksFrom(base, res.Image)
	var expires any
	if res.Image.ExpiresAt != nil {
		expires = res.Image.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"key":        res.Image.Key,
		"name":       res.Image.Name,
		"size":       res.File.Size,
		"instant":    res.Instant,
		"reused":     res.Reused,
		"links":      links,
		"expires_at": expires,
	}
}

func failUpload(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, upload.ErrFileTooLarge):
		Fail(w, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "文件超过大小上限")
	case errors.Is(err, upload.ErrQuotaExceeded):
		Fail(w, http.StatusRequestEntityTooLarge, CodeQuotaExceeded, "存储配额不足")
	case errors.Is(err, upload.ErrBandwidthExceeded):
		Fail(w, http.StatusTooManyRequests, CodeBandwidthExceeded, "本月流量已用尽")
	case errors.Is(err, upload.ErrExtNotAllowed):
		Fail(w, http.StatusUnsupportedMediaType, CodeExtNotAllowed, "文件类型不被允许")
	case errors.Is(err, upload.ErrHeicUnavailable):
		Fail(w, http.StatusUnsupportedMediaType, CodeHeicUnsupported, "当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）")
	case errors.Is(err, upload.ErrDimensionOver):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "图片尺寸超过像素上限")
	case errors.Is(err, upload.ErrInvalidImage):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "不是有效的图片文件")
	case errors.Is(err, upload.ErrPolicyNotAllowed):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "存储策略不可用")
	case errors.Is(err, upload.ErrAlbumNotFound):
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "相册不存在")
	case errors.Is(err, upload.ErrGuestNotSupported):
		Fail(w, http.StatusForbidden, CodeForbidden, "游客上传暂未开放")
	case errors.Is(err, upload.ErrExpiresOverGroup):
		Fail(w, http.StatusBadRequest, CodeExpiresOverGroup, "有效期超出用户组限制")
	case errors.Is(err, upload.ErrMaxViewsOverGroup):
		Fail(w, http.StatusBadRequest, CodeMaxViewsOverGroup, "访问次数超出用户组限制")
	default:
		Fail(w, http.StatusInternalServerError, CodeInternal, "服务器内部错误")
	}
}

// spoolToTemp 把 reader 落到临时文件（便于 Probe/hash/thumbnail 多次独立读取）。
func spoolToTemp(r io.Reader, limit int64) (string, error) {
	f, err := os.CreateTemp("", "imgli-up-*")
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.Copy(f, io.LimitReader(r, limit))
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// Upload POST /api/v1/upload（multipart file）。
func (h *UploadHandlers) Upload(w http.ResponseWriter, r *http.Request) {
	var u *model.User
	if p := PrincipalFrom(r); p != nil {
		u = p.User
	}
	// 匿名请求提前查一次 guest_upload_enabled：开关关闭时在读 body/落盘之前就 403，
	// 避免浪费一次落盘（Save 内的门禁仍是权威判定，此处只是省资源的前置检查）。
	if u == nil && !h.D.Svc.GuestEnabled() {
		Fail(w, http.StatusForbidden, CodeForbidden, "游客上传暂未开放")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.D.MaxBytes)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			Fail(w, http.StatusRequestEntityTooLarge, CodeFileTooLarge, "文件超过大小上限")
			return
		}
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "缺少 file 字段")
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	// 选项解析先于落盘:解析失败提前返回时尚无临时文件可泄漏
	// (Save 的 defer 只兜它接手之后的清理,codex 评审 Task4)。
	opts := upload.Opts{Visibility: r.FormValue("visibility")}
	if r.MultipartForm != nil {
		if vals := r.MultipartForm.Value["album_id"]; len(vals) > 0 && vals[0] != "" {
			n, perr := strconv.ParseUint(vals[0], 10, 64)
			if perr != nil {
				Fail(w, http.StatusBadRequest, CodeInvalidRequest, "album_id 不合法")
				return
			}
			opts.AlbumID = &n
		}
	}
	if pv := r.FormValue("policy_id"); pv != "" {
		n, perr := strconv.ParseUint(pv, 10, 64)
		if perr != nil {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "policy_id 不合法")
			return
		}
		opts.PolicyID = n
	}
	if ev := r.FormValue("expires_in"); ev != "" {
		sec, err := strconv.Atoi(ev)
		if err != nil || sec < 0 || sec > MaxExpiresInSec {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "expires_in 不合法")
			return
		}
		if sec > 0 {
			t := time.Now().Add(time.Duration(sec) * time.Second)
			opts.ExpiresAt = &t
		}
	}
	if mv := r.FormValue("max_views"); mv != "" {
		n, err := strconv.Atoi(mv)
		if err != nil || n < 0 || n > imagesvc.MaxViewsMax {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "max_views 不合法")
			return
		}
		opts.MaxViews = n
	}
	if ap := strings.TrimSpace(r.FormValue("access_password")); ap != "" {
		if len(ap) > imgPassMaxLen {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "access_password 过长")
			return
		}
		hsh, herr := HashAccessPassword(ap)
		if herr != nil {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "access_password 不合法")
			return
		}
		opts.AccessPasswordHash = hsh
	}
	tmp, err := spoolToTemp(file, h.D.MaxBytes)
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "接收文件失败")
		return
	}
	res, err := h.D.Svc.Save(r.Context(), tmp, hdr.Filename, u, opts, ClientIP(r))
	if err != nil {
		failUpload(w, err)
		return
	}
	if h.D.Hooks != nil && res != nil && res.Image != nil {
		h.D.Hooks.Emit("image.uploaded", map[string]any{
			"key": res.Image.Key, "name": res.Image.Name, "status": res.Image.Status,
		})
	}
	OK(w, uploadResultDTO(res, h.D.Res))
}

// guardedDial 返回带 SSRF 拨号期校验的 DialContext：对即将连接的字面量 IP 做
// isPublicIP 判定，命中允许清单则放行（与 ValidateFetchURL 同口径）。
//
// 这是 SSRF 防护的权威防线（而非 ValidateFetchURL 那个前置校验）：
//   - 覆盖重定向——每次跳转都会重新拨号，Control 都会重新校验新目标；
//   - 覆盖 DNS rebinding——校验的是"即将真正连接的那个 IP"，不是校验时刻另
//     行查询到的 IP，二者之间不存在 TOCTOU 窗口。
func guardedDial(allow []*net.IPNet) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := &net.Dialer{Timeout: 10 * time.Second}
		d.Control = func(_, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("非法拨号地址: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("非法拨号地址: %s", host)
			}
			if !isPublicIP(ip) && !ipAllowed(ip, allow) {
				return fmt.Errorf("拒绝内网/保留地址: %s", ip)
			}
			return nil
		}
		return d.DialContext(ctx, network, address)
	}
}

// NewFetchClient 构造抓取远程图片专用客户端：拒重定向 + 拨号期 SSRF 校验（含允许清单）。
// 导出供 server 包装配 UploadDeps.FetchClient 时调用。
func NewFetchClient(allow []*net.IPNet) *http.Client {
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: &http.Transport{DialContext: guardedDial(allow)},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("重定向不被允许")
		},
	}
}

// UploadURL POST /api/v1/upload/url（远程抓取，SSRF 防护）。
func (h *UploadHandlers) UploadURL(w http.ResponseWriter, r *http.Request) {
	var u *model.User
	if p := PrincipalFrom(r); p != nil {
		u = p.User
	}
	// 匿名请求提前查一次 guest_upload_enabled：开关关闭时在发起抓取之前就 403，
	// 避免对远程 URL 做一次白跑的 SSRF 校验+抓取（Save 内的门禁仍是权威判定）。
	if u == nil && !h.D.Svc.GuestEnabled() {
		Fail(w, http.StatusForbidden, CodeForbidden, "游客上传暂未开放")
		return
	}
	var req struct {
		URL            string  `json:"url"`
		Visibility     string  `json:"visibility"`
		AlbumID        *uint64 `json:"album_id"`
		PolicyID       uint64  `json:"policy_id"`
		ExpiresIn      int     `json:"expires_in"`
		MaxViews       int     `json:"max_views"`
		AccessPassword string  `json:"access_password"`
	}
	if err := DecodeJSON(r, &req); err != nil || req.URL == "" {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "需要 url")
		return
	}
	if req.ExpiresIn < 0 || req.ExpiresIn > MaxExpiresInSec {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "expires_in 不合法")
		return
	}
	if req.MaxViews < 0 || req.MaxViews > imagesvc.MaxViewsMax {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "max_views 不合法")
		return
	}
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	var accessHash string
	if ap := strings.TrimSpace(req.AccessPassword); ap != "" {
		if len(ap) > imgPassMaxLen {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "access_password 过长")
			return
		}
		hsh, herr := HashAccessPassword(ap)
		if herr != nil {
			Fail(w, http.StatusBadRequest, CodeInvalidRequest, "access_password 不合法")
			return
		}
		accessHash = hsh
	}
	if err := ValidateFetchURL(req.URL, h.D.FetchAllow); err != nil {
		slog.Warn("URL 抓取被拒", "url", req.URL, "err", err)
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "URL 不被允许")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	httpReq, _ := http.NewRequestWithContext(ctx, "GET", req.URL, nil)
	resp, err := h.D.FetchClient.Do(httpReq)
	if err != nil {
		slog.Warn("URL 抓取失败", "url", req.URL, "err", err)
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "抓取失败")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		Fail(w, http.StatusBadRequest, CodeInvalidRequest, "远程返回非 200")
		return
	}
	tmp, err := spoolToTemp(resp.Body, h.D.MaxBytes)
	if err != nil {
		Fail(w, http.StatusInternalServerError, CodeInternal, "接收文件失败")
		return
	}
	name := filenameFromURL(req.URL)
	res, err := h.D.Svc.Save(ctx, tmp, name, u, upload.Opts{
		Visibility: req.Visibility, AlbumID: req.AlbumID, PolicyID: req.PolicyID, ExpiresAt: expiresAt, MaxViews: req.MaxViews,
		AccessPasswordHash: accessHash,
	}, ClientIP(r))
	if err != nil {
		failUpload(w, err)
		return
	}
	if h.D.Hooks != nil && res != nil && res.Image != nil {
		h.D.Hooks.Emit("image.uploaded", map[string]any{
			"key": res.Image.Key, "name": res.Image.Name, "status": res.Image.Status,
		})
	}
	OK(w, uploadResultDTO(res, h.D.Res))
}

func filenameFromURL(raw string) string {
	name := "remote"
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == '/' {
			cand := raw[i+1:]
			if cand != "" {
				name = cand
			}
			break
		}
	}
	// 去掉查询串
	for i := 0; i < len(name); i++ {
		if name[i] == '?' || name[i] == '#' {
			name = name[:i]
			break
		}
	}
	if name == "" {
		name = "remote"
	}
	return name
}
