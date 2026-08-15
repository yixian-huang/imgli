package upload

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
)

// applyAlbumVisibility 私密相册覆盖调用方可见性为 private；公开相册/无相册不改。
func applyAlbumVisibility(db *gorm.DB, albumID *uint64, vis string) string {
	if albumID == nil {
		return vis
	}
	var alb model.Album
	if err := db.Select("visibility").First(&alb, *albumID).Error; err != nil {
		return vis
	}
	if alb.Visibility == "private" {
		return "private"
	}
	return vis
}

// resolveAlbum 解析相册三态。游客(u==nil)恒 nil。
func (s *Service) resolveAlbum(u *model.User, explicit *uint64) (*uint64, error) {
	if u == nil {
		return nil, nil
	}
	if explicit != nil {
		if *explicit == 0 {
			return nil, nil // 明确不归档,不回退偏好
		}
		var n int64
		if err := s.db.Model(&model.Album{}).
			Where("id = ? AND user_id = ?", *explicit, u.ID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrAlbumNotFound
		}
		return explicit, nil
	}
	if p := u.Preferences.DefaultAlbumID; p != nil {
		var n int64
		if err := s.db.Model(&model.Album{}).
			Where("id = ? AND user_id = ?", *p, u.ID).Count(&n).Error; err != nil {
			return nil, err
		}
		if n > 0 {
			return p, nil
		}
		// 偏好悬空(相册已删):静默忽略(spec 风险 1)
	}
	return nil, nil
}

// resolvePolicy 解析策略:显式(须在组 allowed 且 enabled,否则 ErrPolicyNotAllowed)>
// 偏好(悬空静默降级)> 组默认 pickPolicy。游客忽略显式与偏好。

// resolvePolicy 解析策略:显式(须在组 allowed 且 enabled,否则 ErrPolicyNotAllowed)>
// 偏好(悬空静默降级)> 组默认 pickPolicy。游客忽略显式与偏好。
func (s *Service) resolvePolicy(g *model.UserGroup, u *model.User, explicit uint64) (*model.StoragePolicy, error) {
	inAllowed := func(id uint64) bool {
		for _, a := range g.AllowedPolicyIDs {
			if a == id {
				return true
			}
		}
		return false
	}
	if u != nil && explicit != 0 {
		if !inAllowed(explicit) {
			return nil, ErrPolicyNotAllowed
		}
		var p model.StoragePolicy
		if err := s.db.First(&p, "id = ? AND enabled = ?", explicit, true).Error; err != nil {
			return nil, ErrPolicyNotAllowed
		}
		return &p, nil
	}
	if u != nil && u.Preferences.DefaultPolicyID != nil && inAllowed(*u.Preferences.DefaultPolicyID) {
		var p model.StoragePolicy
		if err := s.db.First(&p, "id = ? AND enabled = ?", *u.Preferences.DefaultPolicyID, true).Error; err == nil {
			return &p, nil
		}
	}
	return s.pickPolicy(g)
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// randBase62 返回 n 位随机 base62 字符串。

func (s *Service) pickPolicy(g *model.UserGroup) (*model.StoragePolicy, error) {
	var p model.StoragePolicy
	if len(g.AllowedPolicyIDs) > 0 {
		if err := s.db.First(&p, "id = ? AND enabled = ?", g.AllowedPolicyIDs[0], true).Error; err == nil {
			return &p, nil
		}
	}
	if err := s.db.First(&p, "enabled = ?", true).Error; err != nil {
		return nil, fmt.Errorf("upload: 无可用存储策略: %w", err)
	}
	return &p, nil
}
