// Package doctor implements `imgli doctor` self-host diagnostics.
package doctor

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/storage"
	appver "github.com/yixian-huang/imgli/internal/version"
)

// Level is the severity of a check result.
type Level string

const (
	OK   Level = "ok"
	Warn Level = "warn"
	Fail Level = "fail"
)

// Check is one diagnostic line.
type Check struct {
	Name    string
	Level   Level
	Message string
}

// Report is the full doctor run.
type Report struct {
	Checks   []Check
	HardFail bool // true if any Fail
}

func (r *Report) add(name string, level Level, msg string) {
	r.Checks = append(r.Checks, Check{Name: name, Level: level, Message: msg})
	if level == Fail {
		r.HardFail = true
	}
}

// Run loads-independent checks against cfg; opens DB and probes local storage when possible.
func Run(cfg *config.Config) Report {
	var r Report
	if cfg == nil {
		r.add("config", Fail, "配置为空")
		return r
	}

	checkDataDir(cfg, &r)
	checkBaseURL(cfg, &r)
	checkTrustProxy(cfg, &r)
	checkListen(cfg, &r)
	checkBinaryUpgradePath(&r)

	db, err := model.Open(cfg)
	if err != nil {
		r.add("database", Fail, fmt.Sprintf("打开失败: %v", err))
		return r
	}
	defer func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}()
	checkDatabase(db, cfg, &r)
	checkLocalPolicies(cfg, db, &r)
	checkCDNMetering(db, &r)
	checkStorageCaps(db, &r)
	checkHeicGroups(db, &r)
	return r
}

// checkStorageCaps reports driver tier/caps and warns on CDN vs capability, insecure
// remote transport, and site-wide compat-only storage.
func checkStorageCaps(db *gorm.DB, r *Report) {
	var policies []model.StoragePolicy
	if err := db.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		r.add("storage_caps", Warn, fmt.Sprintf("列举存储策略失败: %v", err))
		return
	}
	if len(policies) == 0 {
		r.add("storage_caps", OK, "无启用存储策略")
		return
	}
	compatOnly := true
	var lines []string
	for _, p := range policies {
		caps, err := storage.CapsForDriver(p.Driver)
		if err != nil {
			r.add("storage_caps", Warn, fmt.Sprintf("策略 %q 未知驱动 %s", p.Name, p.Driver))
			compatOnly = false
			continue
		}
		eff, _ := storage.EffectiveFor(p.Driver, p.Config, p.CDNDomain)
		lines = append(lines, fmt.Sprintf("%s driver=%s tier=%s presign_capable=%v cdn_recommended=%v",
			p.Name, p.Driver, caps.Tier, caps.PrivatePresignCapable, caps.PublicCDNOffloadRecommended))
		if strings.TrimSpace(p.CDNDomain) != "" && !caps.PublicCDNOffloadRecommended {
			r.add("cdn_vs_caps", Warn, fmt.Sprintf("策略 %q 配置了 CDN 但驱动不推荐作为对象 CDN 回源（原图 /i 仍可能 302）", p.Name))
		}
		if p.Driver != "local" && !eff.TransportIsTLS {
			r.add("insecure_transport", Warn, fmt.Sprintf("策略 %q 远程传输可能未使用 TLS", p.Name))
		}
		if caps.PrivatePresignCapable && !eff.PrivatePresignReady {
			r.add("presign_unconfigured", OK, fmt.Sprintf("策略 %q 未配置 presign_domain：私密图经服务端流式", p.Name))
		}
		if p.Driver == "s3" &&
			strings.EqualFold(strings.TrimSpace(p.Config["path_style"]), "true") &&
			storage.PathStyleLikelyUnsupported(p.Config["endpoint"]) {
			r.add("path_style_vendor", Warn, fmt.Sprintf(
				"策略 %q 对公有云类 Endpoint（%s）使用了路径风格（path_style=true）；OSS/COS/R2 等通常应改用虚拟主机，否则上传/探针易失败",
				p.Name, strings.TrimSpace(p.Config["endpoint"])))
		}
		if caps.Tier != storage.TierCompat {
			compatOnly = false
		}
	}
	r.add("storage_caps", OK, strings.Join(lines, "; "))
	if compatOnly {
		r.add("compat_only", Warn, "全部启用策略均为兼容层（compat）；高流量建议增加本地或 S3")
	}
}

func groupAllowsHeicHeif(exts []string) bool {
	var heic, heif bool
	for _, raw := range exts {
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "heic":
			heic = true
		case "heif":
			heif = true
		}
	}
	return heic && heif
}

func heicGroupsMissing(groups []model.UserGroup) []string {
	var names []string
	for _, g := range groups {
		if !groupAllowsHeicHeif(g.AllowedExts) {
			name := strings.TrimSpace(g.Name)
			if name == "" {
				name = fmt.Sprintf("#%d", g.ID)
			}
			names = append(names, name)
		}
	}
	return names
}

// checkHeicGroups WARNs when this build can decode HEIC but a user group omits heic/heif.
func checkHeicGroups(db *gorm.DB, r *Report) {
	if !imaging.HeicDecodeAvailable() {
		return
	}
	var groups []model.UserGroup
	if err := db.Find(&groups).Error; err != nil {
		r.add("heic_groups", Warn, fmt.Sprintf("列举用户组失败: %v", err))
		return
	}
	missing := heicGroupsMissing(groups)
	if len(missing) == 0 {
		r.add("heic_groups", OK, "用户组均允许 heic/heif")
		return
	}
	r.add("heic_groups", Warn, fmt.Sprintf(
		"构建可解码 HEIC，但用户组未允许 heic/heif：%s（iPhone 上传会 415 ext_not_allowed）",
		strings.Join(missing, ", ")))
}

// checkCDNMetering warns when any enabled policy has cdn_domain set: admin
// traffic/referer stats are origin-only and under-count edge cache hits.
func checkCDNMetering(db *gorm.DB, r *Report) {
	var policies []model.StoragePolicy
	if err := db.Where("enabled = ?", true).Find(&policies).Error; err != nil {
		r.add("cdn_metering", Warn, fmt.Sprintf("列举存储策略失败: %v（可先 imgli migrate）", err))
		return
	}
	var named []string
	for _, p := range policies {
		cdn := strings.TrimSpace(p.CDNDomain)
		if cdn == "" {
			continue
		}
		named = append(named, fmt.Sprintf("%q→%s", p.Name, cdn))
	}
	if len(named) == 0 {
		r.add("cdn_metering", OK, "无启用策略配置 cdn_domain；仪表盘流量≈源站可见访问")
		return
	}
	r.add("cdn_metering", Warn,
		"已配置 CDN 回源前缀 ("+strings.Join(named, "; ")+
			")：管理端流量/Referer 仅统计源站 /i 门禁命中，边缘缓存未计入；成本请看 CDN/桶账单。见 deploy/ops/admin-stats-metering.md")
}

func checkDataDir(cfg *config.Config, r *Report) {
	dir := strings.TrimSpace(cfg.DataDir)
	if dir == "" {
		r.add("data_dir", Fail, "data_dir 为空")
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		r.add("data_dir", Fail, fmt.Sprintf("解析路径失败: %v", err))
		return
	}
	if st, err := os.Stat(abs); err != nil {
		if os.IsNotExist(err) {
			if mkErr := os.MkdirAll(abs, 0o755); mkErr != nil {
				r.add("data_dir", Fail, fmt.Sprintf("不存在且无法创建 %s: %v", abs, mkErr))
				return
			}
			r.add("data_dir", Warn, fmt.Sprintf("已创建目录 %s", abs))
		} else {
			r.add("data_dir", Fail, fmt.Sprintf("无法访问 %s: %v", abs, err))
			return
		}
	} else if !st.IsDir() {
		r.add("data_dir", Fail, fmt.Sprintf("%s 不是目录", abs))
		return
	}
	// write probe
	probe := filepath.Join(abs, ".imgli-doctor-write")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		hint := ""
		if os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission") {
			hint = "；Docker 官方镜像以 uid 1000 运行，绑定挂载请 chown 1000:1000 该目录，或使用命名卷 / 镜像 entrypoint 修正属主"
		}
		r.add("data_dir", Fail, fmt.Sprintf("%s 不可写: %v%s", abs, err, hint))
		return
	}
	_ = os.Remove(probe)
	r.add("data_dir", OK, fmt.Sprintf("可写: %s", abs))
}

// CheckBaseURL validates public base URL shape (exported for unit tests).
func CheckBaseURL(raw string) (Level, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Fail, "base_url 为空（设置 IMGLI_BASE_URL 或配置 base_url）"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Fail, fmt.Sprintf("base_url 无效: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Fail, "base_url 需为 http 或 https"
	}
	if u.Host == "" {
		return Fail, "base_url 缺少 host"
	}
	host := u.Hostname()
	msg := fmt.Sprintf("%s (生成外链用)", strings.TrimRight(raw, "/"))
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return Warn, msg + " — 生产请改为公网域名，否则复制出的链接仅本机可访问"
	}
	if u.Scheme == "http" && host != "localhost" && host != "127.0.0.1" {
		return Warn, msg + " — 非本机建议 https"
	}
	return OK, msg
}

func checkBaseURL(cfg *config.Config, r *Report) {
	lv, msg := CheckBaseURL(cfg.BaseURL)
	r.add("base_url", lv, msg)
}

func checkTrustProxy(cfg *config.Config, r *Report) {
	if cfg.TrustProxy {
		r.add("trust_proxy", Warn,
			"trust_proxy=true：仅在可信反代后开启；否则客户端可伪造 X-Forwarded-For 影响限速与审计 IP")
		return
	}
	// listen on all interfaces without trust_proxy is fine for direct access
	r.add("trust_proxy", OK, "false（直连或未声明反代；若前有 Nginx/Caddy 且需真实 IP，请设 true）")
}

// checkBinaryUpgradePath 探测可执行文件旁是否可写（systemd ProtectSystem=strict 未放行 bin 时一键升级会失败）。
func checkBinaryUpgradePath(r *Report) {
	if appver.IsDockerish() {
		r.add("binary_upgrade", OK, "容器环境：请用镜像升级，跳过二进制旁写探针")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		r.add("binary_upgrade", Warn, fmt.Sprintf("无法解析可执行路径: %v", err))
		return
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		r.add("binary_upgrade", Warn, fmt.Sprintf("解析可执行路径: %v", err))
		return
	}
	dir := filepath.Dir(exe)
	probe := filepath.Join(dir, ".imgli-doctor-upgrade-write")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		r.add("binary_upgrade", Warn,
			fmt.Sprintf("%s 不可写: %v — 管理端一键升级会失败。systemd ProtectSystem=strict 时请将二进制目录加入 ReadWritePaths（见 deploy/imgli.service.example）", dir, err))
		return
	}
	_ = os.Remove(probe)
	r.add("binary_upgrade", OK, fmt.Sprintf("%s 可写（一键升级可用）", dir))
}

func checkListen(cfg *config.Config, r *Report) {
	listen := strings.TrimSpace(cfg.Listen)
	if listen == "" {
		r.add("listen", Fail, "listen 为空")
		return
	}
	// Try brief bind probe on ephemeral? Don't bind production port if in use.
	// Just validate format.
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		// ":8686" is valid for SplitHostPort
		r.add("listen", Fail, fmt.Sprintf("listen 格式无效 %q: %v", listen, err))
		return
	}
	msg := fmt.Sprintf("%s (host=%q port=%s)", listen, host, port)
	if host == "" || host == "0.0.0.0" {
		msg += " — 监听全部网卡"
	}
	if host == "127.0.0.1" || host == "::1" {
		msg += " — 仅本机；公网需反代到该端口"
	}
	r.add("listen", OK, msg)
}

func checkDatabase(db *gorm.DB, cfg *config.Config, r *Report) {
	sqlDB, err := db.DB()
	if err != nil {
		r.add("database", Fail, fmt.Sprintf("底层连接: %v", err))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		r.add("database", Fail, fmt.Sprintf("ping 失败: %v", err))
		return
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	dsn := cfg.Database.DSN
	if driver == "sqlite" && dsn == "" {
		dsn = cfg.SQLiteDefaultDSN()
	}
	// soft schema probe
	if err := db.Exec("SELECT 1").Error; err != nil {
		r.add("database", Fail, fmt.Sprintf("查询失败: %v", err))
		return
	}
	r.add("database", OK, fmt.Sprintf("driver=%s 连通", driver))
	if driver == "sqlite" {
		r.add("database_dsn", OK, fmt.Sprintf("sqlite file ≈ %s", dsn))
		var mmap int64
		if err := db.Raw("PRAGMA mmap_size").Scan(&mmap).Error; err == nil {
			if mmap != 0 {
				r.add("sqlite_mmap", Warn, fmt.Sprintf("mmap_size=%d（非 0 时低内存/绑定挂载更易 OOM；默认连接会强制 0）", mmap))
			} else {
				r.add("sqlite_mmap", OK, "mmap_size=0")
			}
		}
		var journal string
		if err := db.Raw("PRAGMA journal_mode").Scan(&journal).Error; err == nil {
			r.add("sqlite_journal", OK, fmt.Sprintf("journal_mode=%s", journal))
		}
	}
}

func checkLocalPolicies(cfg *config.Config, db *gorm.DB, r *Report) {
	var policies []model.StoragePolicy
	if err := db.Where("enabled = ? AND driver = ?", true, "local").Find(&policies).Error; err != nil {
		r.add("storage_local", Warn, fmt.Sprintf("列举策略失败: %v（可先 imgli migrate）", err))
		return
	}
	if len(policies) == 0 {
		r.add("storage_local", OK, "无启用中的 local 策略（可能全用 S3/WebDAV）")
		return
	}
	res := storagesvc.New(cfg, db)
	for i := range policies {
		p := &policies[i]
		abs := storage.LocalRoot(cfg.DataDir, p.Config["root"])
		d, err := res.Driver(p)
		if err != nil {
			r.add(fmt.Sprintf("storage_local#%d", p.ID), Fail, fmt.Sprintf("策略 %q 驱动: %v", p.Name, err))
			continue
		}
		// write/read/delete tiny object
		key := fmt.Sprintf(".imgli-doctor/%d-%d", p.ID, time.Now().UnixNano())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = d.Put(ctx, key, strings.NewReader("doctor"))
		if err != nil {
			cancel()
			r.add(fmt.Sprintf("storage_local#%d", p.ID), Fail, fmt.Sprintf("策略 %q Put 失败 (%s): %v", p.Name, abs, err))
			continue
		}
		rc, err := d.Open(ctx, key)
		if err != nil {
			cancel()
			r.add(fmt.Sprintf("storage_local#%d", p.ID), Fail, fmt.Sprintf("策略 %q Open 失败: %v", p.Name, err))
			_ = d.Delete(ctx, key)
			continue
		}
		_ = rc.Close()
		_ = d.Delete(ctx, key)
		cancel()
		r.add(fmt.Sprintf("storage_local#%d", p.ID), OK, fmt.Sprintf("策略 %q 读写删 OK (root=%s)", p.Name, abs))
	}
}

// Format prints a human-readable report to a string.
func Format(rep Report) string {
	var b strings.Builder
	b.WriteString("imgli doctor\n")
	for _, c := range rep.Checks {
		mark := "·"
		switch c.Level {
		case OK:
			mark = "ok  "
		case Warn:
			mark = "WARN"
		case Fail:
			mark = "FAIL"
		}
		fmt.Fprintf(&b, "  [%s] %s: %s\n", mark, c.Name, c.Message)
	}
	if rep.HardFail {
		b.WriteString("结果: 存在失败项（exit 1）\n")
	} else {
		b.WriteString("结果: 通过（可有 WARN）\n")
	}
	return b.String()
}
