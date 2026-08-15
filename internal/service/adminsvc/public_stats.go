package adminsvc

import (
	"errors"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/apperr"
	"github.com/yixian-huang/imgli/internal/model"
)

// PublicStatsConfig 公开实例统计（settings "public_stats" JSON）。
// 默认关闭：自托管零配置与现在一致；运营站可在 Admin 打开。
type PublicStatsConfig struct {
	Enabled        bool   `json:"enabled"`
	Since          string `json:"since"` // YYYY-MM-DD；空=自动取最早 live 图或用户
	ShowUptimeDays bool   `json:"show_uptime_days"`
	ShowLiveImages bool   `json:"show_live_images"`
	ShowUsers      bool   `json:"show_users"`
	ShowUsedBytes  bool   `json:"show_used_bytes"`
}

// DefaultPublicStats 出厂默认：关闭；打开后默认展示运行天数 + 图片数。
func DefaultPublicStats() PublicStatsConfig {
	return PublicStatsConfig{
		Enabled:        false,
		Since:          "",
		ShowUptimeDays: true,
		ShowLiveImages: true,
		ShowUsers:      false,
		ShowUsedBytes:  false,
	}
}

// ErrPublicStatsInvalid public_stats 校验失败。
var ErrPublicStatsInvalid = apperr.New("public_stats 配置无效")

// NormalizePublicStats 规整 since 空白；布尔保持原值。
func NormalizePublicStats(c PublicStatsConfig) PublicStatsConfig {
	c.Since = strings.TrimSpace(c.Since)
	return c
}

// ValidatePublicStats 校验 since 形态（空或 YYYY-MM-DD，且可解析为 UTC 日期）。
func ValidatePublicStats(c PublicStatsConfig) error {
	c = NormalizePublicStats(c)
	if c.Since == "" {
		return nil
	}
	if _, err := time.ParseInLocation("2006-01-02", c.Since, time.UTC); err != nil {
		return ErrPublicStatsInvalid
	}
	return nil
}

// PublicStatsSnapshot 公开 API 输出（无隐私细节；字段按配置裁剪）。
type PublicStatsSnapshot struct {
	Enabled        bool   `json:"enabled"`
	UptimeDays     *int64 `json:"uptime_days,omitempty"`
	LiveImageCount *int64 `json:"live_image_count,omitempty"`
	UserCount      *int64 `json:"user_count,omitempty"`
	UsedBytes      *int64 `json:"used_bytes,omitempty"`
	AsOf           string `json:"as_of,omitempty"` // RFC3339 UTC
}

var (
	publicStatsMu    sync.Mutex
	publicStatsCache struct {
		at   time.Time
		snap PublicStatsSnapshot
	}
	publicStatsTTL = 60 * time.Second
)

// InvalidatePublicStatsCache 供设置写入后立即失效（可选；TTL 内也会过期）。
func InvalidatePublicStatsCache() {
	publicStatsMu.Lock()
	publicStatsCache.at = time.Time{}
	publicStatsMu.Unlock()
}

// PublicStatsSnapshotFor 根据配置计算公开快照；enabled=false 时仅返回 {enabled:false}。
// 有 60s 进程内缓存，减轻首页轮询对 DB 的压力。
func PublicStatsSnapshotFor(db *gorm.DB, cfg PublicStatsConfig, now time.Time) (PublicStatsSnapshot, error) {
	cfg = NormalizePublicStats(cfg)
	if !cfg.Enabled {
		return PublicStatsSnapshot{Enabled: false}, nil
	}

	publicStatsMu.Lock()
	if !publicStatsCache.at.IsZero() && now.Sub(publicStatsCache.at) < publicStatsTTL && publicStatsCache.snap.Enabled {
		snap := publicStatsCache.snap
		publicStatsMu.Unlock()
		return snap, nil
	}
	publicStatsMu.Unlock()

	snap, err := computePublicStats(db, cfg, now)
	if err != nil {
		return PublicStatsSnapshot{}, err
	}

	publicStatsMu.Lock()
	publicStatsCache.at = now
	publicStatsCache.snap = snap
	publicStatsMu.Unlock()
	return snap, nil
}

func computePublicStats(db *gorm.DB, cfg PublicStatsConfig, now time.Time) (PublicStatsSnapshot, error) {
	if db == nil {
		return PublicStatsSnapshot{}, errors.New("public stats: nil db")
	}
	out := PublicStatsSnapshot{
		Enabled: true,
		AsOf:    now.UTC().Format(time.RFC3339),
	}

	if cfg.ShowLiveImages {
		var n int64
		// 只计可公开陈列的 live 图：public + normal + 未过期 + 无口令（软删自动排除回收站）。
		if err := db.Model(&model.Image{}).
			Where("status = ? AND visibility = ?", "normal", "public").
			Where("access_password_hash = ?", "").
			Where("expires_at IS NULL OR expires_at > ?", now).
			Count(&n).Error; err != nil {
			return PublicStatsSnapshot{}, err
		}
		out.LiveImageCount = &n
	}

	if cfg.ShowUsers {
		var n int64
		if err := db.Model(&model.User{}).Where("status = ?", "active").Count(&n).Error; err != nil {
			return PublicStatsSnapshot{}, err
		}
		out.UserCount = &n
	}

	if cfg.ShowUsedBytes {
		var n int64
		if err := db.Model(&model.File{}).Where("ref_count > 0").
			Select("COALESCE(SUM(size), 0)").Scan(&n).Error; err != nil {
			return PublicStatsSnapshot{}, err
		}
		out.UsedBytes = &n
	}

	if cfg.ShowUptimeDays {
		days, err := uptimeDays(db, cfg.Since, now)
		if err != nil {
			return PublicStatsSnapshot{}, err
		}
		out.UptimeDays = &days
	}

	return out, nil
}

func uptimeDays(db *gorm.DB, since string, now time.Time) (int64, error) {
	var start time.Time
	if since != "" {
		t, err := time.ParseInLocation("2006-01-02", since, time.UTC)
		if err != nil {
			return 0, ErrPublicStatsInvalid
		}
		start = t
	} else {
		// 最早 live 图；无则最早用户；再无则 now（0 天）
		var img model.Image
		err := db.Model(&model.Image{}).Where("status = ?", "normal").
			Order("created_at ASC").Limit(1).Take(&img).Error
		if err == nil {
			start = img.CreatedAt.UTC()
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			var u model.User
			err2 := db.Model(&model.User{}).Order("created_at ASC").Limit(1).Take(&u).Error
			if err2 == nil {
				start = u.CreatedAt.UTC()
			} else if errors.Is(err2, gorm.ErrRecordNotFound) {
				start = now.UTC()
			} else {
				return 0, err2
			}
		} else {
			return 0, err
		}
	}

	// 按 UTC 日历日差；不足 1 天记 0
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	nowDay := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	d := int64(nowDay.Sub(startDay).Hours() / 24)
	if d < 0 {
		d = 0
	}
	return d, nil
}
