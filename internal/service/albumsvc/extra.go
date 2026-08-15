package albumsvc

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/auth"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
)

func hashAlbumPassword(pw string) (string, error) {
	return auth.HashPassword(pw)
}

// HasAccessPassword 相册是否设置访客口令。
func HasAccessPassword(alb *model.Album) bool {
	return alb != nil && strings.TrimSpace(alb.AccessPasswordHash) != ""
}

// VerifyAccessPassword 校验相册口令。
func VerifyAccessPassword(alb *model.Album, pw string) error {
	if alb == nil {
		return ErrNotFound
	}
	if !HasAccessPassword(alb) {
		return nil
	}
	pw = strings.TrimSpace(pw)
	if pw == "" || !auth.VerifyPassword(alb.AccessPasswordHash, pw) {
		return ErrBadPassword
	}
	return nil
}

// RecordView 公开访客页 PV +1（日聚合 upsert）。
func (s *Service) RecordView(albumID uint64) error {
	date := timeNow().Format("2006-01-02")
	row := model.AlbumAccessStat{AlbumID: albumID, Date: date, Views: 1}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "album_id"}, {Name: "date"}},
		DoUpdates: clause.Assignments(map[string]any{
			"views": gorm.Expr("album_access_stats.views + 1"),
		}),
	}).Create(&row).Error
}

// DayViews 属主统计单日。
type DayViews struct {
	Date  string `json:"date"`
	Views int64  `json:"views"`
}

// Stats 属主相册访问统计：total + 近 30 日。
func (s *Service) Stats(userID, id uint64) (int64, []DayViews, error) {
	var alb model.Album
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil, ErrNotFound
	}
	if err != nil {
		return 0, nil, err
	}
	var rows []model.AlbumAccessStat
	if err := s.db.Where("album_id = ?", alb.ID).Find(&rows).Error; err != nil {
		return 0, nil, err
	}
	var total int64
	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		total += r.Views
		byDate[r.Date] = r.Views
	}
	now := timeNow()
	start := now.AddDate(0, 0, -29)
	daily := make([]DayViews, 0, 30)
	for i := 0; i < 30; i++ {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		daily = append(daily, DayViews{Date: d, Views: byDate[d]})
	}
	return total, daily, nil
}

// Reorder 按 keys 顺序写入 album_pos=1..n（须均属该相册）。
func (s *Service) Reorder(userID, albumID uint64, keys []string) error {
	if len(keys) == 0 || len(keys) > 500 {
		return ErrInvalidReorder
	}
	var alb model.Album
	if err := s.db.Where("id = ? AND user_id = ?", albumID, userID).First(&alb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		seen := make(map[string]struct{}, len(keys))
		for i, k := range keys {
			k = strings.TrimSpace(k)
			if k == "" {
				return ErrInvalidReorder
			}
			if _, dup := seen[k]; dup {
				return ErrInvalidReorder
			}
			seen[k] = struct{}{}
			res := tx.Model(&model.Image{}).
				Where("key = ? AND user_id = ? AND album_id = ? AND deleted_at IS NULL", k, userID, albumID).
				Update("album_pos", i+1)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return ErrInvalidReorder
			}
		}
		return nil
	})
}

// SetImagesVisibility 相册内全部 live 图改可见性。
func (s *Service) SetImagesVisibility(userID, albumID uint64, visibility string) (int64, error) {
	v, err := normVis(visibility)
	if err != nil {
		return 0, err
	}
	var alb model.Album
	if err := s.db.Where("id = ? AND user_id = ?", albumID, userID).First(&alb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if alb.Visibility == "private" && v == "public" {
		return 0, ErrAlbumForcesPrivate
	}
	if s.img != nil {
		var imgs []model.Image
		if err := s.db.Where("album_id = ? AND user_id = ? AND deleted_at IS NULL", albumID, userID).
			Find(&imgs).Error; err != nil {
			return 0, err
		}
		var n int64
		for i := range imgs {
			vis := v
			if _, err := s.img.Update(userID, imgs[i].Key, imagesvc.UpdatePatch{Visibility: &vis}); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	}
	res := s.db.Model(&model.Image{}).
		Where("album_id = ? AND user_id = ? AND deleted_at IS NULL", albumID, userID).
		Update("visibility", v)
	return res.RowsAffected, res.Error
}

// PublicAlbumCard 发现/主页上的公开相册卡片。
type PublicAlbumCard struct {
	ID          uint64
	Name        string
	Description string
	ImageCount  int64
	CoverKey    string
	CoverExt    string
	Views       int64
	Username    string
	Nickname    string
	CreatedAt   time.Time
}

type publicAlbumScan struct {
	ID          uint64
	Name        string
	Description string
	CoverKey    string
	CreatedAt   time.Time
	UserID      uint64
	Username    string
	Nickname    string
	ImageCount  int64
	Views       int64
}

func (s *Service) fillCover(card *PublicAlbumCard, coverKey string, userID uint64, albumID uint64) {
	key, err := s.resolveCover(model.Album{ID: albumID, UserID: userID, CoverKey: coverKey}, true)
	if err != nil || key == "" {
		return
	}
	card.CoverKey = key
	card.CoverExt = "jpg"
	var img model.Image
	if err := s.db.Where("key = ?", key).First(&img).Error; err == nil && img.Ext != "" {
		card.CoverExt = img.Ext
	}
}

// ListPublicAlbums 广场公开相册流（cursor=id 降序）。
func (s *Service) ListPublicAlbums(cursor string, limit int) ([]PublicAlbumCard, string, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 60 {
		limit = 60
	}
	now := timeNow()
	q := s.db.Table("albums").
		Select(`albums.id, albums.name, albums.description, albums.cover_key, albums.created_at, albums.user_id,
			users.username, users.nickname,
			(SELECT COUNT(*) FROM images WHERE images.album_id = albums.id AND images.deleted_at IS NULL
				AND images.visibility = 'public' AND images.status = 'normal'
				AND (images.expires_at IS NULL OR images.expires_at > ?)
				AND (images.access_password_hash = '' OR images.access_password_hash IS NULL)) AS image_count,
			COALESCE((SELECT SUM(views) FROM album_access_stats WHERE album_access_stats.album_id = albums.id), 0) AS views`,
			now).
		Joins("JOIN users ON users.id = albums.user_id").
		Where("albums.visibility = ?", "public").
		Where("users.public_profile = ? AND users.status = ?", true, "active").
		// 至少一张可展示 public 图
		Where(`EXISTS (
			SELECT 1 FROM images WHERE images.album_id = albums.id AND images.deleted_at IS NULL
			AND images.visibility = 'public' AND images.status = 'normal'
			AND (images.expires_at IS NULL OR images.expires_at > ?)
			AND (images.access_password_hash = '' OR images.access_password_hash IS NULL)
		)`, now).
		Order("albums.id DESC")
	if n, err := strconv.ParseUint(strings.TrimSpace(cursor), 10, 64); err == nil && n > 0 {
		q = q.Where("albums.id < ?", n)
	}
	var rows []publicAlbumScan
	if err := q.Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		next = strconv.FormatUint(rows[limit-1].ID, 10)
		rows = rows[:limit]
	}
	out := make([]PublicAlbumCard, 0, len(rows))
	for _, r := range rows {
		card := PublicAlbumCard{
			ID: r.ID, Name: r.Name, Description: r.Description,
			ImageCount: r.ImageCount, Views: r.Views,
			Username: r.Username, Nickname: r.Nickname, CreatedAt: r.CreatedAt,
		}
		s.fillCover(&card, r.CoverKey, r.UserID, r.ID)
		out = append(out, card)
	}
	return out, next, nil
}

// ListUserPublicAlbums 用户公开主页相册。
func (s *Service) ListUserPublicAlbums(username, cursor string, limit int) ([]PublicAlbumCard, string, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 60 {
		limit = 60
	}
	var u model.User
	err := s.db.Where("username = ? AND public_profile = ? AND status = ?", username, true, "active").First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	now := timeNow()
	q := s.db.Table("albums").
		Select(`albums.id, albums.name, albums.description, albums.cover_key, albums.created_at, albums.user_id,
			(SELECT COUNT(*) FROM images WHERE images.album_id = albums.id AND images.deleted_at IS NULL
				AND images.visibility = 'public' AND images.status = 'normal'
				AND (images.expires_at IS NULL OR images.expires_at > ?)
				AND (images.access_password_hash = '' OR images.access_password_hash IS NULL)) AS image_count,
			COALESCE((SELECT SUM(views) FROM album_access_stats WHERE album_access_stats.album_id = albums.id), 0) AS views`,
			now).
		Where("albums.user_id = ? AND albums.visibility = ?", u.ID, "public").
		Where(`EXISTS (
			SELECT 1 FROM images WHERE images.album_id = albums.id AND images.deleted_at IS NULL
			AND images.visibility = 'public' AND images.status = 'normal'
			AND (images.expires_at IS NULL OR images.expires_at > ?)
			AND (images.access_password_hash = '' OR images.access_password_hash IS NULL)
		)`, now).
		Order("albums.id DESC")
	if n, err := strconv.ParseUint(strings.TrimSpace(cursor), 10, 64); err == nil && n > 0 {
		q = q.Where("albums.id < ?", n)
	}
	var rows []publicAlbumScan
	if err := q.Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		next = strconv.FormatUint(rows[limit-1].ID, 10)
		rows = rows[:limit]
	}
	out := make([]PublicAlbumCard, 0, len(rows))
	for _, r := range rows {
		card := PublicAlbumCard{
			ID: r.ID, Name: r.Name, Description: r.Description,
			ImageCount: r.ImageCount, Views: r.Views,
			Username: u.Username, Nickname: u.Nickname, CreatedAt: r.CreatedAt,
		}
		s.fillCover(&card, r.CoverKey, r.UserID, r.ID)
		out = append(out, card)
	}
	return out, next, nil
}
