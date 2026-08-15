package imagesvc

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/auth"
)

// UpdatePatch 是 Update 的可选字段集合；指针 nil 表示不改。
// AlbumID：nil=不改，0=移出，>0=移入(校验归属)。
// SetExpires=false 不改过期；true 时写入 ExpiresAt（nil 即清除为 NULL）。
// Slug：nil=不改；""=清除；否则校验 [a-z0-9-]{3,32} 并唯一。
// MaxViews：nil=不改；0=不限；1–MaxViewsMax=上限（不重置 views_served）。
// AccessPassword：nil=不改；""=清除；非空=argon2 哈希写入（明文不落库）。
type UpdatePatch struct {
	Name           *string
	Visibility     *string
	AlbumID        *int64
	ExpiresAt      *time.Time
	SetExpires     bool
	Slug           *string
	MaxViews       *int
	AccessPassword *string
}

var (
	// ErrExpiresOverGroup 改期超出用户组有效期限制。
	ErrExpiresOverGroup = errors.New("imagesvc: 有效期超出用户组限制")
	// ErrMaxViewsOverGroup 访问次数超出用户组限制。
	ErrMaxViewsOverGroup = errors.New("imagesvc: 访问次数超出用户组限制")
)

// Update 部分更新单图（见 UpdatePatch）。
func (s *Service) Update(userID uint64, key string, p UpdatePatch) (*Row, error) {
	var img model.Image
	err := s.db.Where("key = ? AND user_id = ?", key, userID).First(&img).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var group model.UserGroup
	if p.SetExpires || p.MaxViews != nil {
		var u model.User
		if err := s.db.First(&u, userID).Error; err != nil {
			return nil, err
		}
		if err := s.db.First(&group, u.GroupID).Error; err != nil {
			return nil, err
		}
	}
	updates := map[string]any{}
	// 移入私密相册：图必须变 private（含直链 /i），公开相册不改可见性。
	if p.AlbumID != nil && *p.AlbumID > 0 {
		var dest model.Album
		err := s.db.Where("id = ? AND user_id = ?", *p.AlbumID, userID).First(&dest).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlbumNotFound
		}
		if err != nil {
			return nil, err
		}
		if dest.Visibility == "private" {
			priv := "private"
			p.Visibility = &priv
		}
	}
	// 已在私密相册内（且未移出）时禁止改回 public。
	if p.Visibility != nil && *p.Visibility == "public" {
		stay := p.AlbumID == nil && img.AlbumID != nil
		if stay {
			var cur model.Album
			if err := s.db.Select("visibility").First(&cur, *img.AlbumID).Error; err == nil && cur.Visibility == "private" {
				return nil, ErrAlbumForcesPrivate
			}
		}
	}
	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 255 {
			return nil, ErrInvalidName
		}
		updates["name"] = n
	}
	if p.Visibility != nil {
		if *p.Visibility != "public" && *p.Visibility != "private" {
			return nil, ErrInvalidVisibility
		}
		updates["visibility"] = *p.Visibility
	}
	if p.AlbumID != nil {
		if *p.AlbumID == 0 {
			updates["album_id"] = nil
		} else {
			var cnt int64
			if err := s.db.Model(&model.Album{}).
				Where("id = ? AND user_id = ?", *p.AlbumID, userID).Count(&cnt).Error; err != nil {
				return nil, err
			}
			if cnt == 0 {
				return nil, ErrAlbumNotFound
			}
			updates["album_id"] = uint64(*p.AlbumID)
		}
	}
	if p.SetExpires {
		if err := checkGroupExpires(&group, p.ExpiresAt, time.Now()); err != nil {
			return nil, err
		}
		// map 中显式写 nil → GORM Updates 写 NULL（与 album_id 清出同模式）
		updates["expires_at"] = p.ExpiresAt
	}
	if p.MaxViews != nil {
		if *p.MaxViews < 0 || *p.MaxViews > MaxViewsMax {
			return nil, ErrInvalidMaxViews
		}
		if err := checkGroupMaxViews(&group, *p.MaxViews); err != nil {
			return nil, err
		}
		updates["max_views"] = *p.MaxViews
	}
	if p.AccessPassword != nil {
		pw := strings.TrimSpace(*p.AccessPassword)
		if pw == "" {
			updates["access_password_hash"] = ""
		} else {
			if len(pw) > 128 {
				return nil, ErrInvalidAccessPassword
			}
			h, err := auth.HashPassword(pw)
			if err != nil {
				return nil, err
			}
			updates["access_password_hash"] = h
		}
	}
	if p.Slug != nil {
		v := strings.ToLower(strings.TrimSpace(*p.Slug))
		if v == "" {
			updates["slug"] = nil
		} else {
			if !slugRe.MatchString(v) {
				return nil, ErrInvalidSlug
			}
			var n int64
			if err := s.db.Model(&model.Image{}).Where("slug = ? AND key <> ?", v, key).Count(&n).Error; err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, ErrSlugTaken
			}
			// 勿与既有 key 冲突
			if err := s.db.Model(&model.Image{}).Where("key = ? AND key <> ?", v, key).Count(&n).Error; err != nil {
				return nil, err
			}
			if n > 0 {
				return nil, ErrSlugTaken
			}
			updates["slug"] = v
		}
	}
	// 可见性写入（含「已是 private 但 File 仍在 public/」的 v8 遗留）→ surface 重挂。
	if p.Visibility != nil {
		var oldFile model.File
		if err := s.db.First(&oldFile, img.FileID).Error; err != nil {
			return nil, err
		}
		if fileSurface(oldFile) != *p.Visibility {
			err := s.rehomeWithUpdates(&img, &oldFile, *p.Visibility, updates)
			if errors.Is(err, errRehomeConflict) {
				return s.Get(userID, key) // 并发已达目标态,返回当前
			}
			if err != nil {
				return nil, err
			}
			return s.Get(userID, key)
		}
	}

	if len(updates) > 0 {
		if err := s.db.Model(&img).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return s.Get(userID, key)
}

// SoftDelete 软删（进回收站，保配额，直链转 410）。非属主→ErrNotFound。
