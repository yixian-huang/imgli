package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/servecache"
	"github.com/yixian-huang/imgli/internal/service/servesvc"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/storage"
)

// 受控边长白名单（/t?w=）；其它值 400。
var allowedThumbWidths = map[int]struct{}{200: {}, 400: {}, 800: {}}

// errThumbGen 标记单次 ?w= 生成失败，调用方回退默认 thumb（非存储/IO 故障）。
var errThumbGen = errors.New("serve: width thumb generate failed")

// ServeDeps 直链处理器依赖。
type ServeDeps struct {
	DB      *gorm.DB
	Res     *storagesvc.Resolver
	Stats   *stats.Service    // D-①:hotlink 快照+访问计数;nil=跳过(轻量测试兼容)
	OwnHost string            // BaseURL 的 host(去端口),hotlink 恒放行自站
	Proc    imaging.Processor // 可选；nil 时按需 imaging.New()（测试可省略）
	Gate    *servesvc.Service // 可选；nil 时按 DB/Stats/OwnHost 惰性构造（测试兼容）
	Cache   *servecache.Cache // 可选；公开流式 200 的本地代理缓存
}

type ServeHandlers struct {
	D       ServeDeps
	gate    *servesvc.Service
	thumbSF singleflight.Group // ?w= 缓存生成防击穿
}

func (h *ServeHandlers) proc() imaging.Processor {
	if h.D.Proc != nil {
		return h.D.Proc
	}
	return imaging.New()
}

func (h *ServeHandlers) gateSvc() *servesvc.Service {
	if h.D.Gate != nil {
		return h.D.Gate
	}
	if h.gate == nil {
		h.gate = servesvc.New(h.D.DB, h.D.Stats, h.D.OwnHost)
	}
	return h.gate
}

func (h *ServeHandlers) loadFilePolicy(fileID uint64) (model.File, model.StoragePolicy, bool) {
	return h.gateSvc().LoadFilePolicy(fileID)
}

// splitKeyExt 从 "aB3xK9mQ2wZp.png" 拆出 key 与 ext。
func splitKeyExt(name string) (key, ext string) {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return name, ""
	}
	return name[:i], name[i+1:]
}

// refererHost 从 Referer 头取来源域(去端口小写,IPv6 去括号——Hostname 统一规范);
// 无/坏 Referer 返回空串。
func refererHost(r *http.Request) string {
	ref := r.Header.Get("Referer")
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// statusCapture 捕获最终写出的状态码与写错误:ServeContent 不返回错误(416/500 自行
// 写出,客户端中断只体现为拷贝失败),计数方须据此只记「2xx 且响应体完整写出」。
// 实现 io.ReaderFrom 透传,保底层 sendfile 优化。
type statusCapture struct {
	http.ResponseWriter
	status int
	werr   error
}

func (s *statusCapture) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusCapture) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	if err != nil && s.werr == nil {
		s.werr = err
	}
	return n, err
}

func (s *statusCapture) ReadFrom(rd io.Reader) (int64, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	var n int64
	var err error
	if rf, ok := s.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(rd)
	} else {
		n, err = io.Copy(struct{ io.Writer }{s.ResponseWriter}, rd)
	}
	if err != nil && s.werr == nil {
		s.werr = err
	}
	return n, err
}

func (s *statusCapture) ok() bool { return s.status >= 200 && s.status < 300 && s.werr == nil }

// lookupServable 按 key 查图并执行访问控制；命中返回 (img,true)，
// 否则已写好占位响应(404/410/401)并返回 (nil,false)。是 /i 与 /t 唯一的门禁。
func (h *ServeHandlers) lookupServable(w http.ResponseWriter, r *http.Request) (*model.Image, bool) {
	key, _ := splitKeyExt(chi.URLParam(r, "name"))
	g := h.gateSvc()
	img, soft := g.Find(key)
	if soft {
		h.placeholder(w, r, http.StatusGone, "IMAGE REMOVED")
		return nil, false
	}
	if img == nil {
		h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
		return nil, false
	}
	acc := servesvc.Access{
		IsOwner:     h.isOwner(r, img),
		PasswordOK:  imgPasswordOK(r, img),
		RefererHost: refererHost(r),
	}
	if d := g.Authorize(img, acc); d != nil {
		h.writeDeny(w, r, d)
		return nil, false
	}
	return img, true
}

// writeDeny 把 servesvc.Deny 映射为占位图/JSON 信封。
func (h *ServeHandlers) writeDeny(w http.ResponseWriter, r *http.Request, d *servesvc.Deny) {
	switch d.Kind {
	case servesvc.DenyRemoved:
		h.placeholder(w, r, http.StatusGone, "IMAGE REMOVED")
	case servesvc.DenyPrivate:
		h.placeholder(w, r, http.StatusUnauthorized, "PRIVATE IMAGE")
	case servesvc.DenyExpired:
		h.placeholder(w, r, http.StatusGone, "IMAGE EXPIRED")
	case servesvc.DenyExhausted:
		h.placeholder(w, r, http.StatusGone, "IMAGE EXHAUSTED")
	case servesvc.DenyPassword:
		h.placeholder(w, r, http.StatusUnauthorized, "PASSWORD REQUIRED")
	case servesvc.DenyHotlink:
		h.placeholder(w, r, http.StatusForbidden, "HOTLINK DENIED")
	case servesvc.DenyBandwidth:
		h.placeholder(w, r, http.StatusTooManyRequests, "BANDWIDTH EXCEEDED")
	case servesvc.DenyInternal:
		h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "服务器内部错误")
	default:
		h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
	}
}

// meterOwner 成功放行后按字节计入属主本月用量（裁决 1/3）。
func (h *ServeHandlers) meterOwner(img *model.Image, n int64) {
	h.gateSvc().MeterOwner(img, n)
}

// claimView 原子消耗一次 max_views 配额（仅 max_views>0）。多实例靠 DB 原子 UPDATE。
// 成功返回 true；已用尽或错误返回 false（调用方应 410）。
func (h *ServeHandlers) claimView(img *model.Image) bool {
	return h.gateSvc().ClaimView(img)
}

// Original GET /i/{name} —— 原图直链，访问控制的唯一汇聚点。
func (h *ServeHandlers) Original(w http.ResponseWriter, r *http.Request) {
	img, ok := h.lookupServable(w, r)
	if !ok {
		return
	}
	// 次数上限：非属主在送字节前 claim；属主不消耗配额。
	// 有 max_views 的公开图禁止 CDN 302，否则边缘缓存绕过计数。
	limited := img.MaxViews > 0 || imgHasPassword(img)
	owner := h.isOwner(r, img)
	if limited && !owner {
		if !h.claimView(img) {
			h.placeholder(w, r, http.StatusGone, "IMAGE EXHAUSTED")
			return
		}
	}
	// 门禁已在 lookupServable 过完,以下只决定「怎么送字节」,不再做访问控制。
	// File+Policy 一次加载，供 CDN/预签名/流式共用（避免 stream 路径再 First 两次）。
	file, policy, havePolicy := h.loadFilePolicy(img.FileID)

	// 公开图 + 策略配 CDNDomain → 302 卸带宽(裁决 3)。
	// S4 纵深：visibility、file.surface、对象键前缀三道门；任一非公开则不拼 CDN URL
	//（ObjectURL 对 private/ 键也会返回空，见 storagesvc.CDNEligibleObjectKey）。
	// 次数受限图永不走公开 CDN。
	if !limited && havePolicy && img.Visibility == "public" &&
		(file.Surface == "" || file.Surface == model.SurfacePublic) {
		if u := h.D.Res.ObjectURL(&policy, file.Path); u != "" {
			if h.D.Stats != nil {
				h.D.Stats.Record(img.ID, refererHost(r)) // 302 也计一次公开访问
			}
			h.meterOwner(img, file.Size) // 302：按原图 size 计量（不追 CDN 命中）
			w.Header().Set("Cache-Control", "public, max-age=300")
			http.Redirect(w, r, u, http.StatusFound)
			return
		}
	}

	if havePolicy && img.Visibility != "public" {
		// 私密图 + 驱动支持预签名 + 策略配了 presign_domain → 302 到 60s 时效签名
		// URL(裁决 8)。签名失败(配置/时钟问题)一律回落流式,不让用户看不到自己的图。
		// 次数受限：claim 后仍可短签；Cache-Control 强制 no-store。
		if d, err := h.D.Res.Driver(&policy); err == nil {
			if p, ok := d.(storage.Presigner); ok {
				if u, err := p.PresignGet(r.Context(), file.Path, storage.PresignTTL); err == nil && u != "" {
					if h.D.Stats != nil {
						h.D.Stats.Record(img.ID, refererHost(r))
					}
					h.meterOwner(img, file.Size)
					// 签名 60s 后失效:绝不能沿用公开图那条 public,max-age=300——
					// 缓存里会躺死链接,更糟的是可能被中间层跨用户复用。
					w.Header().Set("Cache-Control", "private, no-store")
					http.Redirect(w, r, u, http.StatusFound)
					return
				}
			}
		}
	}

	// 次数受限图：禁止中间层长缓存
	if limited {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	if !havePolicy {
		h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
		return
	}
	if n, ok := h.streamFile(w, r, &file, &policy, false, img.Visibility == "public" && !limited); ok {
		if h.D.Stats != nil {
			h.D.Stats.Record(img.ID, refererHost(r))
		}
		h.meterOwner(img, n)
	}
}

// Thumbnail GET /t/{name} —— 缩略图（.webp 优先,回退 .jpg）。
// 可选 ?w=200|400|800：白名单边长变体，磁盘缓存键与默认 thumb 隔离。
func (h *ServeHandlers) Thumbnail(w http.ResponseWriter, r *http.Request) {
	img, ok := h.lookupServable(w, r)
	if !ok {
		return
	}
	file, policy, havePolicy := h.loadFilePolicy(img.FileID)
	if !havePolicy {
		h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
		return
	}
	public := img.Visibility == "public" && !imgHasPassword(img) && img.MaxViews == 0
	if ws := strings.TrimSpace(r.URL.Query().Get("w")); ws != "" {
		n, err := strconv.Atoi(ws)
		if err != nil {
			if strings.Contains(r.Header.Get("Accept"), "application/json") {
				Fail(w, http.StatusBadRequest, CodeInvalidRequest, "w 须为 200、400 或 800")
			} else {
				h.placeholder(w, r, http.StatusBadRequest, "BAD WIDTH")
			}
			return
		}
		if _, ok := allowedThumbWidths[n]; !ok {
			if strings.Contains(r.Header.Get("Accept"), "application/json") {
				Fail(w, http.StatusBadRequest, CodeInvalidRequest, "w 须为 200、400 或 800")
			} else {
				h.placeholder(w, r, http.StatusBadRequest, "BAD WIDTH")
			}
			return
		}
		if m, ok := h.streamWidthThumb(w, r, &file, &policy, n, public); ok {
			h.meterOwner(img, m)
		}
		return
	}
	if n, ok := h.streamFile(w, r, &file, &policy, true, public); ok {
		h.meterOwner(img, n)
	}
}

// streamWidthThumb 提供白名单边长 JPEG；缓存未命中时从原图生成并 Put。
// file/policy 由调用方一次加载，避免与 streamFile 重复查库。
func (h *ServeHandlers) streamWidthThumb(w http.ResponseWriter, r *http.Request, file *model.File, policy *model.StoragePolicy, width int, public bool) (int64, bool) {
	d, err := h.D.Res.Driver(policy)
	if err != nil {
		h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "存储不可用")
		return 0, false
	}
	etag := `"` + file.Hash + "-w" + strconv.Itoa(width) + "-t" + storagesvc.ThumbGen + `"`
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return 0, true
	}
	ckey := serveCacheKey(file, true, width)
	if public {
		if cached, ctype, ok := h.openServeCache(ckey, "image/jpeg"); ok {
			defer cached.Close()
			meter := int64(0)
			if n, err := cached.Seek(0, io.SeekEnd); err == nil && n > 0 {
				meter = n
				_, _ = cached.Seek(0, io.SeekStart)
			}
			w.Header().Set("Content-Type", ctype)
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			sc := &statusCapture{ResponseWriter: w}
			http.ServeContent(sc, r, "", file.CreatedAt, cached)
			if !sc.ok() {
				return 0, false
			}
			return meter, true
		}
	}
	key := storagesvc.WidthThumbKey(file.Surface, file.Hash, width)
	rc, err := d.Open(r.Context(), key)
	if errors.Is(err, storage.ErrNotFound) {
		// singleflight：同对象同边长并发 miss 只生成一次，防缓存击穿。
		sfKey := file.Hash + "|w" + strconv.Itoa(width) + "|" + file.Surface
		_, genErr, _ := h.thumbSF.Do(sfKey, func() (any, error) {
			// 已知像素过大则跳过现场解码（pure-Go 全图 RGBA 可达数百 MB）。
			if file.Width > 0 && file.Height > 0 {
				if int64(file.Width)*int64(file.Height) > int64(imaging.MaxDecodePixels) {
					return nil, errThumbGen
				}
			}
			if file.Size > int64(imaging.MaxThumbSourceBytes) {
				return nil, errThumbGen
			}
			src, oerr := d.Open(r.Context(), file.Path)
			if oerr != nil {
				return nil, oerr
			}
			// Thumbnail 内再 LimitReader；此处仍限流，避免先把巨型对象完整拉入堆。
			raw, rerr := io.ReadAll(io.LimitReader(src, int64(imaging.MaxThumbSourceBytes)+1))
			_ = src.Close()
			if rerr != nil {
				return nil, rerr
			}
			if len(raw) > imaging.MaxThumbSourceBytes {
				return nil, errThumbGen
			}
			out, terr := h.proc().Thumbnail(bytes.NewReader(raw), width)
			if terr != nil {
				return nil, errThumbGen
			}
			_ = d.Put(r.Context(), key, bytes.NewReader(out))
			return nil, nil
		})
		if genErr != nil {
			if errors.Is(genErr, errThumbGen) {
				return h.streamFile(w, r, file, policy, true, public)
			}
			if errors.Is(genErr, storage.ErrNotFound) {
				h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
				return 0, false
			}
			h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "读取失败")
			return 0, false
		}
		rc, err = d.Open(r.Context(), key)
	}
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
			return 0, false
		}
		h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "读取失败")
		return 0, false
	}
	if public && h.D.Cache != nil {
		var n int64
		var ferr error
		rc, _, n, ferr = h.fillServeCache(ckey, rc, "image/jpeg")
		if ferr != nil {
			h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "读取失败")
			return 0, false
		}
		defer rc.Close()
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		sc := &statusCapture{ResponseWriter: w}
		http.ServeContent(sc, r, "", file.CreatedAt, rc)
		if !sc.ok() {
			return 0, false
		}
		return n, true
	}
	defer rc.Close()
	meter := int64(0)
	if n, err := rc.Seek(0, io.SeekEnd); err == nil && n > 0 {
		meter = n
		_, _ = rc.Seek(0, io.SeekStart)
	} else {
		_, _ = rc.Seek(0, io.SeekStart)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", etag)
	if public {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	sc := &statusCapture{ResponseWriter: w}
	http.ServeContent(sc, r, "", file.CreatedAt, rc)
	if !sc.ok() {
		return 0, false
	}
	return meter, true
}

func (h *ServeHandlers) isOwner(r *http.Request, img *model.Image) bool {
	p := PrincipalFrom(r)
	return p != nil && img.UserID != nil && *img.UserID == p.User.ID
}

// openThumb 按 ThumbKeyCandidates 探测:surface 前缀现行世代 webp/jpg → (仅 public)
// 遗留无前缀路径。返回 reader 与 Content-Type;全未命中返回 ErrNotFound,中间非
// NotFound 错误原样上抛。
func openThumb(ctx context.Context, d storage.Driver, surface, hash string) (io.ReadSeekCloser, string, error) {
	var lastMiss, lastHard error
	for _, key := range storagesvc.ThumbKeyCandidates(surface, hash) {
		rc, err := d.Open(ctx, key)
		if err == nil {
			ctype := "image/jpeg"
			if strings.HasSuffix(key, ".webp") {
				ctype = "image/webp"
			}
			return rc, ctype, nil
		}
		if errors.Is(err, storage.ErrNotFound) {
			lastMiss = err
			continue
		}
		lastHard = err
	}
	if lastMiss != nil {
		return nil, "", lastMiss
	}
	if lastHard != nil {
		return nil, "", lastHard
	}
	return nil, "", storage.ErrNotFound
}

type byteRSC struct{ *bytes.Reader }

func (byteRSC) Close() error { return nil }

func sniffImageCType(b []byte, fallback string) string {
	if len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff {
		return "image/jpeg"
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	if len(b) >= 6 && (string(b[:6]) == "GIF87a" || string(b[:6]) == "GIF89a") {
		return "image/gif"
	}
	return fallback
}

func serveCacheKey(file *model.File, thumb bool, width int) string {
	if width > 0 {
		return file.Hash + "-w" + strconv.Itoa(width) + "-t" + storagesvc.ThumbGen
	}
	if thumb {
		return file.Hash + "-t" + storagesvc.ThumbGen
	}
	return file.Hash
}

func (h *ServeHandlers) openServeCache(key, fallback string) (io.ReadSeekCloser, string, bool) {
	if h.D.Cache == nil {
		return nil, "", false
	}
	f, ok := h.D.Cache.Get(key)
	if !ok {
		return nil, "", false
	}
	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return byteRSC{bytes.NewReader(data)}, sniffImageCType(data, fallback), true
}

func (h *ServeHandlers) fillServeCache(key string, rc io.ReadSeekCloser, ctype string) (io.ReadSeekCloser, string, int64, error) {
	data, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		return nil, ctype, 0, err
	}
	if h.D.Cache != nil && int64(len(data)) <= h.D.Cache.MaxFileBytes() {
		_ = h.D.Cache.Put(key, data)
	}
	if ctype == "" || strings.HasPrefix(ctype, "image/") {
		ctype = sniffImageCType(data, ctype)
	}
	return byteRSC{bytes.NewReader(data)}, ctype, int64(len(data)), nil
}

// streamFile 打开 file（thumb 为 true 则打开该文件哈希对应的缩略图键）并经
// ServeContent 回源。public 为 false（私密图）时禁止共享缓存，避免经 CDN
// 泄露给非所有者。成功返回 (meterBytes, true)；失败 (0, false)。
// meterBytes：原图用 file.Size；缩略图用可读长度（Seek）；304 计 0。
// file/policy 须已由调用方加载（Original/Thumbnail 各一次）。
func (h *ServeHandlers) streamFile(w http.ResponseWriter, r *http.Request, file *model.File, policy *model.StoragePolicy, thumb bool, public bool) (int64, bool) {
	etag := `"` + file.Hash + `"`
	if thumb {
		etag = `"` + file.Hash + "-t" + storagesvc.ThumbGen + `"`
	}
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return 0, true // 304 无传输体，不计流量
	}

	ckey := serveCacheKey(file, thumb, 0)
	fallback := file.MIME
	if thumb {
		fallback = "image/jpeg"
	}
	var rc io.ReadSeekCloser
	var ctype string
	if public {
		if cached, ct, ok := h.openServeCache(ckey, fallback); ok {
			rc, ctype = cached, ct
		}
	}
	if rc == nil {
		d, err := h.D.Res.Driver(policy)
		if err != nil {
			h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "存储不可用")
			return 0, false
		}
		if thumb {
			// vips 构建产 .webp,纯 Go 构建产 .jpg;混合存量双探测,构建切换免迁移(D-②)
			rc, ctype, err = openThumb(r.Context(), d, file.Surface, file.Hash)
		} else {
			ctype = file.MIME
			rc, err = d.Open(r.Context(), file.Path)
		}
		if err != nil {
			if thumb && errors.Is(err, storage.ErrNotFound) {
				if grc, gct, gerr := h.ensureDefaultThumb(r.Context(), d, file); gerr == nil {
					rc, ctype = grc, gct
				} else {
					h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
					return 0, false
				}
			} else if errors.Is(err, storage.ErrNotFound) {
				h.placeholder(w, r, http.StatusNotFound, "NOT FOUND")
				return 0, false
			} else {
				h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "读取失败")
				return 0, false
			}
		}
		if rc != nil && public && h.D.Cache != nil && (thumb || file.Size <= h.D.Cache.MaxFileBytes()) {
			var n int64
			rc, ctype, n, err = h.fillServeCache(ckey, rc, ctype)
			if err != nil {
				h.imageFail(w, r, http.StatusInternalServerError, "READ ERROR", CodeInternal, "读取失败")
				return 0, false
			}
			if thumb && n == 0 {
				// keep going; meter 下面按 Seek 再算
			}
		}
	}
	defer rc.Close()
	meter := file.Size
	if thumb {
		if n, err := rc.Seek(0, io.SeekEnd); err == nil && n > 0 {
			meter = n
			_, _ = rc.Seek(0, io.SeekStart)
		} else {
			meter = 0 // 无可靠 size 不计（裁决：有 size 才计）
			_, _ = rc.Seek(0, io.SeekStart)
		}
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("ETag", etag)
	if public {
		// 长缓存 + immutable 仍安全:URL key 唯一且 ETag 绑 hash/世代;键变更靠新 ThumbGen。
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	// ServeContent 自行写 416/500 不返回错误——捕获状态码,仅 2xx(含 206)算成功,
	// 防非法 Range 抬高访问统计(codex 评审 Task2)。
	sc := &statusCapture{ResponseWriter: w}
	http.ServeContent(sc, r, "", file.CreatedAt, rc)
	if !sc.ok() {
		return 0, false
	}
	return meter, true
}

// imageFail 图片 URL 的错误：默认 SVG + no-store，避免 <img> 吃到 JSON 500
// 显示裂图，也避免反代把 5xx 当 jpg 缓存。显式 Accept JSON 仍走信封。
func (h *ServeHandlers) imageFail(w http.ResponseWriter, r *http.Request, status int, label, code, msg string) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		Fail(w, status, code, msg)
		return
	}
	h.placeholder(w, r, status, label)
}

// ensureDefaultThumb 默认 /t 未命中时从原图现场生成（与上传 ThumbMaxEdge=400 对齐）。
// Put 失败仍返回内存中的图，避免 MinIO 刚写入不可见或 thumb Put 失败时第一次 /t 裂图。
func (h *ServeHandlers) ensureDefaultThumb(ctx context.Context, d storage.Driver, file *model.File) (io.ReadSeekCloser, string, error) {
	sfKey := file.Hash + "|t|" + file.Surface
	v, genErr, _ := h.thumbSF.Do(sfKey, func() (any, error) {
		if file.Width > 0 && file.Height > 0 {
			if int64(file.Width)*int64(file.Height) > int64(imaging.MaxDecodePixels) {
				return nil, errThumbGen
			}
		}
		if file.Size > int64(imaging.MaxThumbSourceBytes) {
			return nil, errThumbGen
		}
		src, oerr := d.Open(ctx, file.Path)
		if oerr != nil {
			return nil, oerr
		}
		raw, rerr := io.ReadAll(io.LimitReader(src, int64(imaging.MaxThumbSourceBytes)+1))
		_ = src.Close()
		if rerr != nil {
			return nil, rerr
		}
		if len(raw) > imaging.MaxThumbSourceBytes {
			return nil, errThumbGen
		}
		out, terr := h.proc().Thumbnail(bytes.NewReader(raw), 400)
		if terr != nil {
			return nil, errThumbGen
		}
		thumbKey := storagesvc.ThumbKey(file.Surface, file.Hash)
		if h.proc().ThumbExt() == "webp" {
			thumbKey = storagesvc.ThumbKeyWebP(file.Surface, file.Hash)
		}
		_ = d.Put(ctx, thumbKey, bytes.NewReader(out))
		return out, nil
	})
	if genErr != nil {
		return nil, "", genErr
	}
	out, _ := v.([]byte)
	if len(out) == 0 {
		return nil, "", errThumbGen
	}
	ctype := "image/jpeg"
	if h.proc().ThumbExt() == "webp" {
		ctype = "image/webp"
	}
	return byteRSC{bytes.NewReader(out)}, ctype, nil
}

// placeholder 返回占位。默认返回 SVG 占位图，仅当调用方显式
// Accept: application/json 时返回信封 JSON。始终带正确状态码。
func (h *ServeHandlers) placeholder(w http.ResponseWriter, r *http.Request, status int, label string) {
	// /i、/t 是图片 URL：默认返回 SVG 占位图（浏览器 <img>、热链均得图形占位）；
	// 仅当调用方显式 Accept: application/json 时返回恒定错误信封。
	if !strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		w.Write([]byte(placeholderSVG(label)))
		return
	}
	code := CodeNotFound
	msg := "资源不存在"
	switch status {
	case http.StatusGone:
		code, msg = CodeGone, "图片已删除"
	case http.StatusUnauthorized:
		code, msg = CodeUnauthorized, "私密图片，请登录后查看"
	case http.StatusForbidden:
		code, msg = CodeForbidden, "防盗链拦截"
	case http.StatusTooManyRequests:
		code, msg = CodeBandwidthExceeded, "本月流量已用尽"
	}
	Fail(w, status, code, msg)
}

func placeholderSVG(label string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="280" height="210" viewBox="0 0 280 210">` +
		`<rect width="280" height="210" fill="#f1f1ef"/>` +
		`<text x="140" y="110" font-family="ui-monospace,monospace" font-size="14" ` +
		`fill="#77777f" text-anchor="middle">` + label + `</text></svg>`
}
