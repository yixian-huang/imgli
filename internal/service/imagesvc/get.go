package imagesvc

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
)

// Get 取属主单图（连 file/policy）。非属主或不存在返回 ErrNotFound。
func (s *Service) Get(userID uint64, key string) (*Row, error) {
	var img model.Image
	err := s.db.Where("key = ? AND user_id = ?", key, userID).First(&img).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var file model.File
	if err := s.db.First(&file, img.FileID).Error; err != nil {
		return nil, err
	}
	var pol model.StoragePolicy
	if err := s.db.First(&pol, file.StoragePolicyID).Error; err != nil {
		return nil, err
	}
	return &Row{Img: img, File: file, Policy: pol}, nil
}

// GetPublicShare 公开分享页：visibility=public、status=normal、未过期，且父相册（若有）为 public。
// 支持 key 或 slug；其余一律 ErrNotFound（不区分存在性）。
func (s *Service) GetPublicShare(ref string) (*Row, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrNotFound
	}
	var img model.Image
	err := s.db.Where("key = ?", ref).First(&img).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = s.db.Where("slug = ?", ref).First(&img).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if img.Visibility != "public" || img.Status != "normal" {
		return nil, ErrNotFound
	}
	if img.ExpiresAt != nil && !img.ExpiresAt.After(time.Now()) {
		return nil, ErrNotFound
	}
	if img.MaxViews > 0 && img.ViewsServed >= img.MaxViews {
		return nil, ErrNotFound
	}
	// 父相册私密：即使图仍标 public（遗留/竞态），分享页与 OG 也不得公开。
	if img.AlbumID != nil {
		var alb model.Album
		err := s.db.Select("visibility").First(&alb, *img.AlbumID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if alb.Visibility != "public" {
			return nil, ErrNotFound
		}
	}
	var file model.File
	if err := s.db.First(&file, img.FileID).Error; err != nil {
		return nil, err
	}
	var pol model.StoragePolicy
	if err := s.db.First(&pol, file.StoragePolicyID).Error; err != nil {
		return nil, err
	}
	return &Row{Img: img, File: file, Policy: pol}, nil
}
