// Package servesvc 直链门禁与计量：Find/Authorize/Claim/Meter 与 File+Policy 加载，
// 供 handler 层 /i /t 使用；HTTP 占位图仍由 handler 写出。
package servesvc

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/bandwidth"
	"github.com/yixian-huang/imgli/internal/service/stats"
)

// DenyKind 门禁拒绝原因；handler 映射 status/label/code。
type DenyKind string

const (
	DenyNotFound  DenyKind = "not_found"
	DenyRemoved   DenyKind = "removed" // soft-deleted / rejected / pending 非属主
	DenyPrivate   DenyKind = "private"
	DenyExpired   DenyKind = "expired"
	DenyExhausted DenyKind = "exhausted"
	DenyPassword  DenyKind = "password"
	DenyHotlink   DenyKind = "hotlink"
	DenyBandwidth DenyKind = "bandwidth"
	DenyInternal  DenyKind = "internal"
)

// Deny 门禁拒绝。
type Deny struct {
	Kind DenyKind
}

// Access 请求侧已解析的访问上下文（属主/口令由 handler 根据 cookie/session 判定）。
type Access struct {
	IsOwner     bool
	IsAdmin     bool // 管理后台预览：可看他人 private/pending/口令图，不改变匿名门禁
	PasswordOK  bool // 无口令、属主或已解锁
	RefererHost string
}

// Service 门禁与计量。
type Service struct {
	db      *gorm.DB
	stats   *stats.Service // 可选；nil 跳过防盗链
	ownHost string
}

func New(db *gorm.DB, st *stats.Service, ownHost string) *Service {
	return &Service{db: db, stats: st, ownHost: ownHost}
}

// Find 按 key 或 slug 取图。softDeleted=true 表示库中有软删行；not found 时 img=nil 且 softDeleted=false。
func (s *Service) Find(ref string) (img *model.Image, softDeleted bool) {
	var row model.Image
	err := s.db.Where("key = ?", ref).First(&row).Error
	if err != nil {
		err = s.db.Where("slug = ?", ref).First(&row).Error
	}
	if err == nil {
		return &row, false
	}
	var deleted model.Image
	if s.db.Unscoped().Where("key = ? OR slug = ?", ref, ref).First(&deleted).Error == nil {
		return nil, true
	}
	return nil, false
}

// Authorize 对已加载图片做访问控制。nil 表示放行。
func (s *Service) Authorize(img *model.Image, acc Access) *Deny {
	if img == nil {
		return &Deny{Kind: DenyNotFound}
	}
	privileged := acc.IsOwner || acc.IsAdmin
	if img.Status == "rejected" && !acc.IsAdmin {
		return &Deny{Kind: DenyRemoved}
	}
	// pending 仅属主/管理员可看；游客图无属主 → 对非管理员不可直出。
	if img.Status == "pending" && !privileged {
		return &Deny{Kind: DenyRemoved}
	}
	if img.Visibility == "private" && !privileged {
		return &Deny{Kind: DenyPrivate}
	}
	if img.ExpiresAt != nil && img.ExpiresAt.Before(time.Now()) && !acc.IsAdmin {
		return &Deny{Kind: DenyExpired}
	}
	if img.MaxViews > 0 && img.ViewsServed >= img.MaxViews && !privileged {
		return &Deny{Kind: DenyExhausted}
	}
	if hasPassword(img) && !acc.PasswordOK && !privileged {
		return &Deny{Kind: DenyPassword}
	}
	if !privileged && s.stats != nil && !stats.HotlinkAllowed(s.stats.Hotlink(), acc.RefererHost, s.ownHost) {
		return &Deny{Kind: DenyHotlink}
	}
	if img.UserID != nil && !acc.IsAdmin {
		if err := bandwidth.Check(s.db, *img.UserID); errors.Is(err, bandwidth.ErrExceeded) {
			return &Deny{Kind: DenyBandwidth}
		} else if err != nil {
			return &Deny{Kind: DenyInternal}
		}
	}
	return nil
}

// Lookup 便捷：Find + Authorize（PasswordOK/IsOwner 须已正确设置）。
func (s *Service) Lookup(ref string, acc Access) (*model.Image, *Deny) {
	img, soft := s.Find(ref)
	if soft {
		return nil, &Deny{Kind: DenyRemoved}
	}
	if img == nil {
		return nil, &Deny{Kind: DenyNotFound}
	}
	if d := s.Authorize(img, acc); d != nil {
		return nil, d
	}
	return img, nil
}

func hasPassword(img *model.Image) bool {
	return img != nil && img.AccessPasswordHash != ""
}

// ClaimView 原子消耗一次 max_views（仅 max_views>0）。成功 true；用尽 false。
func (s *Service) ClaimView(img *model.Image) bool {
	if img == nil || img.MaxViews <= 0 {
		return true
	}
	res := s.db.Model(&model.Image{}).
		Where("id = ? AND max_views > 0 AND views_served < max_views", img.ID).
		UpdateColumn("views_served", gorm.Expr("views_served + 1"))
	return res.Error == nil && res.RowsAffected == 1
}

// MeterOwner 成功放行后按字节计入属主本月用量。
func (s *Service) MeterOwner(img *model.Image, n int64) {
	if img == nil || img.UserID == nil || n <= 0 {
		return
	}
	_ = bandwidth.Add(s.db, *img.UserID, n)
}

// LoadFilePolicy 按 fileID 取 File+Policy；任一缺失返回 ok=false。
func (s *Service) LoadFilePolicy(fileID uint64) (model.File, model.StoragePolicy, bool) {
	var file model.File
	if err := s.db.First(&file, fileID).Error; err != nil {
		return file, model.StoragePolicy{}, false
	}
	var policy model.StoragePolicy
	if err := s.db.First(&policy, file.StoragePolicyID).Error; err != nil {
		return file, policy, false
	}
	return file, policy, true
}
