// Package server 装配 HTTP 服务：路由、中间件、依赖注入。
package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/handler"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/mail"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/adminsvc"
	"github.com/yixian-huang/imgli/internal/service/albumsvc"
	"github.com/yixian-huang/imgli/internal/service/apitoken"
	"github.com/yixian-huang/imgli/internal/service/auth"
	"github.com/yixian-huang/imgli/internal/service/discoversvc"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
	"github.com/yixian-huang/imgli/internal/service/moderation"
	"github.com/yixian-huang/imgli/internal/service/servesvc"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/service/upload"
	"github.com/yixian-huang/imgli/internal/service/webhook"
	"github.com/yixian-huang/imgli/internal/task"
	"github.com/yixian-huang/imgli/web"
)

// Options 是 server 的全部外部依赖。Cfg 必填；DB 可为 nil（仅注册 healthz，用于轻量测试）；
// Web 为前端构建产物文件系统，nil 时用 go:embed 的 web/dist。
type Options struct {
	Cfg *config.Config
	DB  *gorm.DB
	Web fs.FS
}

type Server struct {
	opts       Options
	mux        *chi.Mux
	runner     *task.Runner
	imgSvc     *imagesvc.Service
	stats      *stats.Service
	storageRes *storagesvc.Resolver // mountAPI/mountServe 共享，单 driver 缓存
	authRes    authResolver         // API 与 /i /t 共用会话/Token 解析
	imgProc    imaging.Processor
}

func New(opts Options) *Server {
	s := &Server{opts: opts, mux: chi.NewRouter()}
	s.mux.Use(handler.Recoverer)
	s.mux.Use(handler.RealIP(opts.Cfg.TrustProxy))
	// 全局约束：响应信封恒定——未匹配路由/方法也不例外（Task 2 评审发现）
	s.mux.NotFound(func(w http.ResponseWriter, r *http.Request) {
		handler.Fail(w, http.StatusNotFound, handler.CodeNotFound, "资源不存在")
	})
	s.mux.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		handler.Fail(w, http.StatusMethodNotAllowed, handler.CodeInvalidRequest, "方法不允许")
	})
	s.mux.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		handler.OK(w, nil)
	})
	if opts.DB != nil {
		s.stats = stats.New(opts.DB, 60*time.Second)
		s.mountAPI()
		s.mountServe()
	}
	webFS := opts.Web
	if webFS == nil {
		webFS = web.DistFS()
	}
	s.mountWeb(webFS)
	return s
}

// authResolver 组合各服务实现 handler.UserResolver；tokens 由 Task 6 填充。
type authResolver struct {
	auth   *auth.Service
	tokens *apitoken.Service
}

func (a authResolver) UserBySession(t string) (*model.User, error) {
	return a.auth.UserBySession(t)
}

func (a authResolver) UserByAPIToken(t string) (*model.User, string, error) {
	return a.tokens.UserByToken(t)
}

// mountAPI 挂载 /api/v1 业务路由（Task 5-7 逐步充实）。
func (s *Server) mountAPI() {
	st := settings.New(s.opts.DB)
	authSvc := auth.New(s.opts.DB, st)
	ah := &handler.AuthHandlers{Svc: authSvc, Secure: strings.HasPrefix(s.opts.Cfg.BaseURL, "https://")}
	oidcSvc := auth.NewOIDC(s.opts.DB, authSvc, s.opts.Cfg.BaseURL)
	oidcH := &handler.OIDCHandlers{OIDC: oidcSvc, Auth: authSvc, Secure: strings.HasPrefix(s.opts.Cfg.BaseURL, "https://")}
	tokSvc := apitoken.New(s.opts.DB)
	th := &handler.TokenHandlers{Svc: tokSvc}
	res := authResolver{auth: authSvc, tokens: tokSvc}

	// 上传与任务系统（storage/auth 实例挂到 Server，供 mountServe 复用）
	storageRes := storagesvc.New(s.opts.Cfg, s.opts.DB)
	s.storageRes = storageRes
	s.authRes = res
	if s.imgProc == nil {
		s.imgProc = imaging.New()
	}
	runner := task.New(s.opts.DB, runtime.NumCPU())
	s.runner = runner
	upSvc := upload.New(s.opts.DB, storageRes, s.imgProc, runner)
	upSvc.WatermarkDir = filepath.Join(s.opts.Cfg.DataDir, "watermarks")
	runner.Register("delete_file", upSvc.DeleteFileTask)
	modSvc := moderation.New(s.opts.DB, st, storageRes)
	runner.Register("moderate_image", modSvc.ModerateTask)
	// 拒绝通知：读用户邮箱 + 站点名，best-effort 异步发送（mail 未配则静默）。
	mailForReject := mail.New(s.opts.DB)
	modSvc.OnReject = func(img model.Image) {
		if img.UserID == nil {
			return
		}
		var u model.User
		if err := s.opts.DB.First(&u, *img.UserID).Error; err != nil || u.Email == "" {
			return
		}
		siteName := "img.li"
		_ = st.Get(model.SettingSiteName, &siteName)
		lang := "zh"
		if u.Preferences.Lang == "en" {
			lang = "en"
		}
		subj, html := mail.RenderImageRejected(siteName, img.Key, img.Name, lang)
		if err := mailForReject.Send(u.Email, subj, html); err != nil {
			slog.Warn("reject notify mail failed", "key", img.Key, "err", err)
		}
	}
	hooks := webhook.New(s.opts.DB)
	fetchAllow := parseFetchAllow(s.opts.Cfg.FetchAllow)
	uploadH := &handler.UploadHandlers{D: handler.UploadDeps{
		Svc:         upSvc,
		Res:         storageRes,
		MaxBytes:    64 << 20,
		FetchAllow:  fetchAllow,
		FetchClient: handler.NewFetchClient(fetchAllow),
		Hooks:       hooks,
	}}

	// 图片管理（Task 1+）
	imgSvc := imagesvc.New(s.opts.DB, storageRes, runner)
	imgH := &handler.ImageHandlers{D: handler.ImageDeps{Img: imgSvc, Res: storageRes, Stats: s.stats}}
	trashH := &handler.TrashHandlers{D: handler.TrashDeps{Img: imgSvc, Res: storageRes}}
	s.imgSvc = imgSvc // Run 中的回收站清理 goroutine 用
	albH := &handler.AlbumHandlers{D: handler.AlbumDeps{Alb: albumsvc.New(s.opts.DB)}}

	// 管理端（Task 1+）：与广场/鉴权共用 settings，保证开关即时失效缓存
	adm := adminsvc.New(s.opts.DB, st)
	mailSvc := mail.New(s.opts.DB)
	authSvc.Mailer = mailSvc
	authSvc.BaseURL = s.opts.Cfg.BaseURL
	admH := &handler.AdminHandlers{D: handler.AdminDeps{
		Adm: adm, Res: storageRes, Mail: mailSvc, Stats: s.stats, Mod: modSvc, Hooks: hooks,
		OwnHost: baseHost(s.opts.Cfg.BaseURL),
	}}

	uh := &handler.UserHandlers{
		Svc: authSvc, Img: imgSvc, Adm: adm,
		AvatarDir:    filepath.Join(s.opts.Cfg.DataDir, "avatars"),
		WatermarkDir: filepath.Join(s.opts.Cfg.DataDir, "watermarks"),
		Secure:       strings.HasPrefix(s.opts.Cfg.BaseURL, "https://"),
	}

	// 公开配置端点（计划 C-①a Task 2）：品牌/注册模式/游客上传开关+限额/base_url，无需鉴权。
	cfgH := &handler.ConfigHandler{DB: s.opts.DB, BaseURL: s.opts.Cfg.BaseURL}

	limiter := handler.NewLimiterMult(s.opts.Cfg.RateLimitMult)

	s.mux.Route("/api/v1", func(api chi.Router) {
		api.Use(handler.OriginCheck(s.opts.Cfg.BaseURL))
		api.Use(handler.Auth(res))
		api.With(limiter.Middleware("config", 60)).Get("/config", cfgH.Config)
		// 公开发现面（广场 + 用户公开主页 + 分享页）：无需鉴权，按 IP 限速
		dh := &handler.DiscoverHandler{
			DB: s.opts.DB, St: st, Svc: discoversvc.New(s.opts.DB),
		}
		api.With(limiter.IPMiddleware("plaza", 120)).Get("/plaza", dh.Plaza)
		api.With(limiter.IPMiddleware("plaza", 120)).Get("/u/{username}", dh.UserProfile)
		api.With(limiter.IPMiddleware("plaza", 120)).Get("/u/{username}/images", dh.UserImages)
		api.With(limiter.IPMiddleware("share", 120)).Get("/s/{key}", imgH.Share)
		api.With(limiter.IPMiddleware("share_unlock", 30)).Post("/s/{key}/unlock", imgH.UnlockShare)
		api.With(limiter.IPMiddleware("public_album", 120)).Get("/a/{id}", albH.PublicGet)
		api.With(limiter.IPMiddleware("public_album", 120)).Get("/a/{id}/images", albH.PublicImages)
		api.With(limiter.Middleware("auth", 20)).Post("/auth/register", ah.Register)
		api.With(limiter.Middleware("auth", 20)).Post("/auth/login", ah.Login)
		api.With(limiter.Middleware("auth", 10)).Get("/auth/oidc/start", oidcH.Start)
		api.Get("/auth/oidc/callback", oidcH.Callback)
		api.Post("/auth/logout", ah.Logout)
		api.With(limiter.IPMiddleware("forgot", 5)).Post("/auth/forgot-password", ah.ForgotPassword)
		api.With(limiter.Middleware("auth", 20)).Post("/auth/reset-password", ah.ResetPassword)
		api.With(limiter.Middleware("auth", 20)).Post("/auth/verify-email", ah.VerifyEmail)
		api.With(limiter.Middleware("auth", 20)).Post("/auth/confirm-change-email", ah.ConfirmChangeEmail)
		api.Group(func(priv chi.Router) {
			priv.Use(handler.RequireUser, handler.RequireFullScope)
			priv.Get("/auth/session", ah.Session)
			priv.With(limiter.Middleware("resend", 3)).Post("/auth/resend-verification", ah.ResendVerification)
			priv.Get("/user/profile", uh.Profile)
			priv.Patch("/user/profile", uh.UpdateProfile)
			priv.With(limiter.Middleware("auth", 10)).Post("/user/email/change", uh.ChangeEmail)
			priv.Patch("/user/preferences", uh.UpdatePreferences)
			priv.Post("/user/avatar", uh.UploadAvatar)
			priv.Delete("/user/avatar", uh.DeleteAvatar)
			priv.Post("/user/watermark", uh.UploadWatermark)
			priv.Delete("/user/watermark", uh.DeleteWatermark)
			priv.Delete("/user", uh.DeleteAccount)
			priv.Patch("/user/password", uh.ChangePassword)
			priv.Get("/user/quota", uh.Quota)
			priv.Get("/user/policies", uh.Policies)
			priv.Route("/user/tokens", func(tr chi.Router) {
				tr.Get("/", th.List)
				tr.Post("/", th.Create)
				tr.Delete("/{id}", th.Delete)
			})
			priv.Route("/images", func(ir chi.Router) {
				ir.Get("/", imgH.List)
				ir.Post("/batch", imgH.Batch)
				ir.Get("/{key}/stats", imgH.Stats)
				ir.Get("/{key}", imgH.Detail)
				ir.Patch("/{key}", imgH.Update)
				ir.Delete("/{key}", imgH.Delete)
			})
			priv.Route("/trash", func(tr chi.Router) {
				tr.Get("/", trashH.List)
				tr.Post("/{key}/restore", trashH.Restore)
				tr.Delete("/{key}", trashH.Purge)
				tr.Delete("/", trashH.Empty)
			})
			priv.Route("/albums", func(ar chi.Router) {
				ar.Get("/", albH.List)
				ar.Post("/", albH.Create)
				ar.Get("/{id}", albH.Detail)
				ar.Patch("/{id}", albH.Update)
				ar.Delete("/{id}", albH.Delete)
			})
			priv.Route("/admin", func(ar chi.Router) {
				ar.Use(handler.RequireAdmin)
				ar.Get("/stats", admH.Stats)
				ar.Get("/referers/images", admH.RefererImages)
				ar.Get("/users", admH.Users)
				ar.Get("/export/users.csv", admH.ExportUsersCSV)
				ar.Get("/webhooks", admH.GetWebhooks)
				ar.Put("/webhooks", admH.PutWebhooks)
				ar.Get("/oidc", oidcH.GetOIDCAdmin)
				ar.Put("/oidc", oidcH.PutOIDCAdmin)
				ar.Patch("/users/{id}", admH.UpdateUser)
				ar.Post("/users/{id}/reset-password", admH.ResetPassword)
				ar.Get("/images", admH.Images)
				ar.Delete("/images/{key}", admH.DeleteImage)
				ar.Patch("/images/{key}", admH.UpdateImageWhitelist)
				ar.Get("/review", admH.Review)
				ar.Post("/review/batch", admH.ReviewBatch)
				ar.Post("/review/{key}", admH.ReviewDecide)
				ar.Get("/groups", admH.Groups)
				ar.Post("/groups", admH.CreateGroup)
				ar.Patch("/groups/{id}", admH.UpdateGroup)
				ar.Delete("/groups/{id}", admH.DeleteGroup)
				ar.Get("/policies", admH.Policies)
				ar.Post("/policies", admH.CreatePolicy)
				ar.Patch("/policies/{id}", admH.UpdatePolicy)
				ar.Delete("/policies/{id}", admH.DeletePolicy)
				ar.Post("/policies/{id}/test", admH.TestPolicyConn)
				ar.Post("/storage/migrate", admH.StartStorageMigrate)
				ar.Get("/storage/migrate/{id}", admH.GetStorageMigrate)
				ar.Get("/settings", admH.GetSettings)
				ar.Put("/settings", admH.PutSettings)
				ar.Post("/settings/smtp/test", admH.TestSMTP)
				ar.Post("/settings/moderation/test", admH.TestModeration)
				ar.Get("/logs", admH.Logs)
				ar.Get("/invites", admH.Invites)
				ar.Post("/invites", admH.CreateInvites)
				ar.Delete("/invites/{id}", admH.RevokeInvite)
			})
		})
		// 上传：登录（session 或 upload/full scope Bearer）与匿名均可达——
		// 匿名是否真被放行由 Task 3 的 upload.Save 内 guest_upload_enabled 判定。
		// RequireUserOrAnon 只拦截"出示了但解析不出身份"的凭证（过期 session/坏
		// Bearer）为 401，避免被 Auth 悄悄当无凭证放行成游客上传（复审①修复）。
		// 限速改由三档 GroupMiddleware 取代原粗兜底 Middleware("upload",120)，
		// 避免同一请求被两套限速重复计数。
		api.Group(func(up chi.Router) {
			up.Use(handler.RequireUserOrAnon)
			up.With(limiter.GroupMiddleware(s.opts.DB)).Post("/upload", uploadH.Upload)
			up.With(limiter.GroupMiddleware(s.opts.DB)).Post("/upload/url", uploadH.UploadURL)
		})
	})
}

// mountServe 挂载 /i、/t 直链与缩略图路由（不在 /api/v1 下）。直链也过 Auth
// 以便私密图属主可见（匿名放行，由 handler 判定 401/404/410）。
// 复用 mountAPI 已装配的 storageRes / authRes / imgProc，避免双 driver 缓存与双 auth 实例。
func (s *Server) mountServe() {
	own := baseHost(s.opts.Cfg.BaseURL)
	gate := servesvc.New(s.opts.DB, s.stats, own)
	sh := &handler.ServeHandlers{D: handler.ServeDeps{
		DB: s.opts.DB, Res: s.storageRes,
		Stats: s.stats, OwnHost: own,
		Proc: s.imgProc, Gate: gate,
	}}
	s.mux.Group(func(g chi.Router) {
		g.Use(handler.Auth(s.authRes))
		g.Get("/i/{name}", sh.Original)
		g.Get("/t/{name}", sh.Thumbnail)
		g.Get("/avatar/{id}", handler.ServeAvatar(filepath.Join(s.opts.Cfg.DataDir, "avatars")))
	})
}

// baseHost 从 BaseURL 提取 host(去端口,IPv6 去括号——与 refererHost 同用 Hostname
// 规范,防同主机不匹配),解析失败返回空串(hotlink 自站放行退化,不炸)。
func baseHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// parseFetchAllow 把配置里的 host/CIDR 字符串解析为 *net.IPNet 清单，供 URL
// 抓取 SSRF 校验放行。无法解析的条目记日志跳过，不影响其余条目生效。
func parseFetchAllow(entries []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, e := range entries {
		if _, n, err := net.ParseCIDR(e); err == nil {
			nets = append(nets, n)
			continue
		}
		if ip := net.ParseIP(e); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		slog.Warn("fetch_allow 条目无法解析，已忽略", "entry", e)
	}
	return nets
}

func (s *Server) Handler() http.Handler { return s.mux }

// Run 启动监听并阻塞；ctx 取消后优雅停机（等待在途请求最多 30s）。
func (s *Server) Run(ctx context.Context) error {
	if s.runner != nil {
		go s.runner.Start(ctx)
	}
	if s.stats != nil {
		go s.stats.Start(ctx)
	}
	if s.imgSvc != nil {
		go func() {
			s.imgSvc.PurgeExpiredTrash(ctx)
			s.imgSvc.PurgeExpiredImages(ctx)
			t := time.NewTicker(time.Hour)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					s.imgSvc.PurgeExpiredTrash(ctx)
					s.imgSvc.PurgeExpiredImages(ctx)
				}
			}
		}()
	}
	srv := &http.Server{Addr: s.opts.Cfg.Listen, Handler: s.mux}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := srv.Shutdown(shutCtx)
		// 排空完成后权威终刷:在途请求的计数此刻已全部 Record,且 DB 尚未被调用方关闭
		// (Start 内 ctx.Done 的终刷与本刷并发安全,重复刷是幂等空刷;codex 评审 Task2)。
		if s.stats != nil {
			_ = s.stats.Flush()
		}
		return err
	}
}
