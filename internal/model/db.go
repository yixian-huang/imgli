package model

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yixian-huang/imgli/internal/config"
)

// 播种的 settings 键。
const (
	SettingSiteName         = "site_name"
	SettingRegistrationMode = "registration_mode" // "open" | "invite" | "closed"
	SettingGuestUpload      = "guest_upload_enabled"
	SettingPlazaEnabled     = "plaza_enabled" // 公开发现面（广场/用户公开主页）总开关
	SettingModeration       = "moderation"    // 机审配置 JSON，见 internal/service/moderation.Config
	SettingSMTP             = "smtp"          // SMTP 配置 JSON,见 internal/mail.Config
	SettingHotlink          = "hotlink"       // 防盗链配置 JSON,见 stats.HotlinkConfig
	SettingProcessing       = "processing"    // 图片处理规则 JSON,见 upload.Processing
	// 引流/站点插槽（开源通用；内容存 DB，不进代码）
	SettingAnnouncement = "announcement" // 顶栏公告 JSON
	SettingFooter       = "footer"       // 页脚链接组 JSON
	SettingHTMLInject   = "html_inject"  // 自定义 HTML 注入 JSON（head / body_end）
	// 站长可配公开文案/CTA（默认空/关；运营站用 Admin 填，不硬编码作者域名）
	SettingHelpURL        = "help_url"        // 帮助/文档链接（可选）
	SettingUpgradeURL     = "upgrade_url"     // 满额/升级/自托管说明（可选）
	SettingRegisterNotice = "register_notice" // 注册页短说明（可选）
	SettingShareBranding  = "share_branding"  // off | site | links
	// v0.5 轻度站配 / AGPL 合规辅助（单实例 OSS；非白标）
	SettingFaviconURL    = "favicon_url"    // 可选自定义 favicon URL；空=内置 brand
	SettingSourceURL     = "source_url"     // AGPL 对应源码 URL；空不展示
	SettingOSSCredit     = "oss_credit"     // on | off；页脚「基于 imgli」默认可关
	SettingAboutEnabled  = "about_enabled"  // 是否启用 /about
	SettingAboutBody     = "about_body"     // 关于页正文 locale map {zh,en}
	SettingWelcomeEmail  = "welcome_email"  // bool；SMTP 已配时注册欢迎信，默认 true
	SettingMailTemplates = "mail_templates" // JSON，见 internal/mail.Templates；空=内置文案
	// v0.9.5 轻视觉：强调色 + 可选站点背景图
	SettingThemeAccent     = "theme_accent"       // 空 | #RRGGBB；驱动主按钮/强调
	SettingThemeBgColor    = "theme_bg_color"     // 空 | #RGB/#RRGGBB；整站纯色底（可与背景图叠加）
	SettingThemeBgImageURL = "theme_bg_image_url" // 空 | http(s)/站内路径
	SettingThemeBgDim      = "theme_bg_dim"       // 0–1 遮罩强度，默认 0.72
	SettingThemeGlass      = "theme_glass"        // 0–1 面板毛玻璃不透明度，默认 0.78
	// SettingPublicStats 公开实例统计 JSON（默认关闭）；见 adminsvc.PublicStatsConfig
	SettingPublicStats = "public_stats"
)

// settingModerationDefaultJSON 是 moderation.DefaultConfig() 的 JSON 字面量，手写在此
// 是为了避免 model 包 import moderation 包（Task 7 起 moderation.ModerateTask 反向依赖
// model.Image/File/StoragePolicy/AuditLog，若 model 也 import moderation 就会成环）。
// 逐字段必须与 moderation.DefaultConfig() 保持一致，一致性由外部测试包
// moderation_seed_test.go（package model_test，可同时 import 两边而不成环）断言。
const settingModerationDefaultJSON = `{"enabled":false,"provider":"webhook","endpoint":"","api_key":"","threshold":0.8,"action":"pending","access_key_id":"","access_key_secret":"","region":"","ocr_keywords":{"enabled":false,"endpoint":"","api_key":"","keywords":null,"on_hit":""},"login_sample_rate":1,"on_plugin_error":"open","notify_on_reject":false}`

// settingSMTPDefaultJSON 是 mail.DefaultConfig() 的 JSON 字面量,手写原因同上
// (避免 model→mail 依赖;一致性由 mail_seed_test.go 外部断言)。
const settingSMTPDefaultJSON = `{"host":"","port":587,"username":"","password":"","from":"","encryption":"starttls"}`

// settingMailTemplatesDefaultJSON 与 mail.DefaultTemplates() 一致（全空=内置）。
const settingMailTemplatesDefaultJSON = `{}`

// settingHotlinkDefaultJSON 是 stats.DefaultHotlink() 的 JSON 字面量,手写原因同上
// (避免 model→stats 依赖;一致性由 hotlink_seed_test.go 外部断言)。
const settingHotlinkDefaultJSON = `{"enabled":false,"allowed_domains":[],"allow_empty_referer":true}`

// settingProcessingDefaultJSON 是 upload.DefaultProcessing() 的 JSON 字面量,手写原因同上
// (避免 model→upload 依赖;一致性由 processing_seed_test.go 外部断言)。
const settingProcessingDefaultJSON = `{"text_watermark":{"enabled":false,"text":"","position":"br","opacity":0.35,"size_ratio":0.04},"max_edge":0,"strip_exif":true,"jpeg_quality":0,"output_format":"keep","webp_quality":0,"webp_skip_if_larger":true}`

// 插槽默认 JSON：与 adminsvc.DefaultAnnouncement/Footer/HTMLInject 一致。
const settingAnnouncementDefaultJSON = `{"enabled":false,"text":{"zh":"","en":""},"link_url":"","link_label":{"zh":"","en":""},"dismissible":true,"starts_at":"","ends_at":""}`
const settingFooterDefaultJSON = `{"groups":[]}`

// register_notice 播种为 locale map；历史库可能是纯字符串，读侧 LocaleString 兼容。
const settingRegisterNoticeDefaultJSON = `{"zh":"","en":""}`
const settingHTMLInjectDefaultJSON = `{"head":"","body_end":""}`

// Open 按配置打开数据库。sqlite 未配置 DSN 时落到 DataDir/imgli.db 并开 WAL（不可用时回退 DELETE）。
func Open(cfg *config.Config) (*gorm.DB, error) {
	gc := &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			SlowThreshold:             200 * time.Millisecond,
		}),
		TranslateError: true,
		// AutoMigrate 绝不自动建/改外键约束：现存 SQLite 库若有孤儿数据，
		// AutoMigrate 建 FK 会直接报错导致应用起不来。FK 改由 Task 3 显式、
		// 有条件地创建。
		DisableForeignKeyConstraintWhenMigrating: true,
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Database.Driver)) {
	case "sqlite", "":
		return openSQLite(cfg, gc)
	case "postgres":
		db, err := gorm.Open(postgres.Open(cfg.Database.DSN), gc)
		if err != nil {
			return nil, err
		}
		return db, nil
	default:
		return nil, fmt.Errorf("未知数据库驱动 %q（支持 sqlite|postgres）", cfg.Database.Driver)
	}
}

// Migrate 增量迁移全部模型并确保版本行存在。
func Migrate(db *gorm.DB) error {
	fresh := !db.Migrator().HasTable(&SchemaVersion{})
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("迁移失败: %w", err)
	}
	if err := applyForeignKeys(db, fresh); err != nil {
		return fmt.Errorf("外键迁移失败: %w", err)
	}
	if err := ensureIndexes(db); err != nil {
		return fmt.Errorf("索引补建失败: %w", err)
	}
	if err := migrateSurface(db); err != nil {
		return fmt.Errorf("surface 迁移失败: %w", err)
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 1}).Error; err != nil {
		return err
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 2}).Error; err != nil {
		return err
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 3}).Error; err != nil {
		return err
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 4}).Error; err != nil {
		return err
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 5}).Error; err != nil { // images.slug
		return err
	}
	if err := db.FirstOrCreate(&SchemaVersion{Version: 6}).Error; err != nil { // files.surface + (hash,surface) 唯一
		return err
	}
	// v7：用户组月流量硬顶 + 用户月用量字段（AutoMigrate 已加列）；默认组补 5 GiB（0=不限，不能把管理员意图清零）。
	if err := db.FirstOrCreate(&SchemaVersion{Version: 7}).Error; err != nil {
		return err
	}
	if err := migrateBandwidthDefaults(db); err != nil {
		return err
	}
	// v8：存量「私密相册 + public 图」改为 private，堵住匿名 /i 直链。
	if err := db.FirstOrCreate(&SchemaVersion{Version: 8}).Error; err != nil {
		return err
	}
	return migratePrivateAlbumImages(db)
}

// FreeBandwidthQuotaMonth Free/默认组第一期月流量硬顶：5 GiB。
const FreeBandwidthQuotaMonth int64 = 5 << 30

// migrateBandwidthDefaults 仅当默认组 bandwidth_quota_month 仍为 0（新列默认）时写入 Free 5 GiB。
// 已显式设为其它值（含故意 0 不限——极少，管理员可再改）时：只补「从未写过」的场景。
// 用 schema v7 首次出现时对 is_default 组：若列为 0 则设 5GiB（与产品裁决一致）。
func migrateBandwidthDefaults(db *gorm.DB) error {
	return db.Model(&UserGroup{}).
		Where("is_default = ? AND bandwidth_quota_month = 0", true).
		Update("bandwidth_quota_month", FreeBandwidthQuotaMonth).Error
}

// migrateGuestLifecycleDefaults 为存量游客组补生命周期默认（全 0 视为未配置）。
// 管理员若曾显式把某字段改成非 0，不会被覆盖；若全部仍 0，写入与 Seed 一致的 1d/7d/7d。
func migrateGuestLifecycleDefaults(db *gorm.DB) error {
	return db.Model(&UserGroup{}).
		Where("is_guest = ? AND default_expires_in = 0 AND max_expires_in = 0 AND force_max_age_days = 0", true).
		Updates(map[string]any{
			"default_expires_in": 86400,
			"max_expires_in":     7 * 86400,
			"force_max_age_days": 7,
		}).Error
}

// migrateSurface 完成 files.surface 的存量适配：回填空 surface 为 public，并删除旧的
// 单列 hash 唯一索引。AutoMigrate 会据标签建 (hash,surface) 复合唯一索引,但不会自动删
// 旧的 idx_files_hash；不删则 unique(hash) 仍在,阻止同 hash 跨 surface 两行。
// SQLite 与 Postgres 均支持 DROP INDEX IF EXISTS。
// (全新 SQLite 库上该复合索引曾被 FK 重建丢弃,S1 曾在此手工补建;现由 Migrate 的
// ensureIndexes 按模型声明统一补齐,本函数只管「删旧」,不再管「补新」。)
// migratePrivateAlbumImages 把私密相册内仍为 public 的 live 图改为 private。
// 幂等；只改 visibility（门禁 /i），不搬 surface 对象。
func migratePrivateAlbumImages(db *gorm.DB) error {
	return db.Exec(`
		UPDATE images SET visibility = ?
		WHERE deleted_at IS NULL
		  AND visibility = ?
		  AND album_id IS NOT NULL
		  AND EXISTS (
		    SELECT 1 FROM albums WHERE albums.id = images.album_id AND albums.visibility = ?
		  )
	`, "private", "public", "private").Error
}

func migrateSurface(db *gorm.DB) error {
	if err := db.Exec("UPDATE files SET surface = ? WHERE surface IS NULL OR surface = ''", SurfacePublic).Error; err != nil {
		return err
	}
	return db.Exec("DROP INDEX IF EXISTS idx_files_hash").Error
}

// Seed 播种默认数据，幂等：默认组、游客组、本地存储策略、初始 settings。
func Seed(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		defGroup := UserGroup{
			Name: "默认组", IsDefault: true,
			StorageQuota: 10 << 30, MaxFileSize: 20 << 20, // 10 GB / 20 MB（原型 MAX 20 MB）
			// 月流量硬顶 5 GiB（产品裁决 Free）；存储与流量独立。
			BandwidthQuotaMonth: FreeBandwidthQuotaMonth,
			RatePerMinute:       20, RatePerHour: 200, RatePerDay: 1000,
			AllowedExts:      []string{"png", "jpg", "jpeg", "gif", "webp"},
			AllowedPolicyIDs: []uint64{1},
		}
		if err := firstOrCreateBy(tx, &UserGroup{}, "is_default = ?", true, &defGroup); err != nil {
			return err
		}
		guest := UserGroup{
			Name: "游客组", IsGuest: true,
			StorageQuota: 0, MaxFileSize: 5 << 20, // 原型游客态：≤5MB、每日 3 张
			RatePerMinute: 3, RatePerHour: 3, RatePerDay: 3,
			AllowedExts:      []string{"png", "jpg", "jpeg", "gif", "webp"},
			AllowedPolicyIDs: []uint64{1},
			// 游客默认非永久：1 天默认 / 最长 7 天 / 超龄硬清 7 天。
			DefaultExpiresIn: 86400,
			MaxExpiresIn:     7 * 86400,
			ForceMaxAgeDays:  7,
		}
		if err := firstOrCreateBy(tx, &UserGroup{}, "is_guest = ?", true, &guest); err != nil {
			return err
		}
		// 存量库 firstOrCreate 不会更新已有游客组：仅当生命周期字段仍全 0 时补默认。
		if err := migrateGuestLifecycleDefaults(tx); err != nil {
			return err
		}
		local := StoragePolicy{
			Name: "本地存储", Driver: "local",
			Config:       map[string]string{"root": "uploads"}, // 相对 DataDir
			PathTemplate: "{Y}/{m}/{d}/{uniqid}.{ext}",
			Enabled:      true,
		}
		if err := firstOrCreateBy(tx, &StoragePolicy{}, "driver = ?", "local", &local); err != nil {
			return err
		}
		for k, v := range map[string]string{
			SettingSiteName:         `"img.li"`,
			SettingRegistrationMode: `"open"`,
			SettingGuestUpload:      `false`,
			SettingPlazaEnabled:     `false`,
			SettingModeration:       settingModerationDefaultJSON,
			SettingSMTP:             settingSMTPDefaultJSON,
			SettingHotlink:          settingHotlinkDefaultJSON,
			SettingProcessing:       settingProcessingDefaultJSON,
			SettingAnnouncement:     settingAnnouncementDefaultJSON,
			SettingFooter:           settingFooterDefaultJSON,
			SettingHTMLInject:       settingHTMLInjectDefaultJSON,
			SettingHelpURL:          `""`,
			SettingUpgradeURL:       `""`,
			SettingRegisterNotice:   settingRegisterNoticeDefaultJSON,
			// 默认 site：展示站名；开源产品署名在前端始终保留
			SettingShareBranding:   `"site"`,
			SettingFaviconURL:      `""`,
			SettingSourceURL:       `""`,
			SettingOSSCredit:       `"on"`,
			SettingAboutEnabled:    `false`,
			SettingAboutBody:       `{"zh":"","en":""}`,
			SettingWelcomeEmail:    `true`,
			SettingMailTemplates:   settingMailTemplatesDefaultJSON,
			SettingThemeAccent:     `""`,
			SettingThemeBgColor:    `""`,
			SettingThemeBgImageURL: `""`,
			SettingThemeBgDim:      `0.72`,
			SettingThemeGlass:      `0.78`,
			// 默认关闭：自托管零配置不展示；运营站 Admin 打开
			SettingPublicStats: `{"enabled":false,"since":"","show_uptime_days":true,"show_live_images":true,"show_users":false,"show_used_bytes":false}`,
		} {
			if err := tx.Where("key = ?", k).
				FirstOrCreate(&Setting{Key: k, Value: v}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// firstOrCreateBy 按条件查一行，不存在则创建 seed 值（幂等播种辅助）。
// 用 Find+RowsAffected 而非 First，避免 ErrRecordNotFound 在首次播种时刷日志。
func firstOrCreateBy[T any](tx *gorm.DB, probe *T, query string, arg any, seed *T) error {
	res := tx.Where(query, arg).Limit(1).Find(probe)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return tx.Create(seed).Error
	}
	return nil
}
