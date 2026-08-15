// Package albumsvc 实现相册 CRUD；count/cover 实时查询不反规范化(spec §4)。
package albumsvc

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
)

var (
	ErrNotFound           = errors.New("albumsvc: 相册不存在")
	ErrInvalidName        = errors.New("albumsvc: 名称需 1-128 字节")
	ErrInvalidVisibility  = errors.New("albumsvc: 可见性仅 public|private")
	ErrInvalidDefaultView = errors.New("albumsvc: 默认视图仅 gallery|immersive")
	ErrInvalidDescription = errors.New("albumsvc: 描述过长")
	ErrInvalidCover       = errors.New("albumsvc: 封面图不在此相册")
	ErrInvalidPassword    = errors.New("albumsvc: 口令不合法")
	ErrBadPassword        = errors.New("albumsvc: 口令错误")
	ErrInvalidReorder     = errors.New("albumsvc: 排序 keys 无效")
	ErrAlbumForcesPrivate = errors.New("albumsvc: 私密相册内的图必须为 private")
)

type Service struct {
	db  *gorm.DB
	img *imagesvc.Service
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// WithImages 接入 imagesvc，使批量改可见性走 surface 重挂。测试可省略。
func (s *Service) WithImages(img *imagesvc.Service) *Service {
	s.img = img
	return s
}

// OwnerPublic 公开相册页可选作者信息（不泄露邮箱等）。
type OwnerPublic struct {
	Username      string
	Nickname      string
	PublicProfile bool // true 时前端可链到 /u/{username}
}

// AlbumView 相册 + 实时统计。
type AlbumView struct {
	Album    model.Album
	Count    int64
	CoverKey string
	// Owner 仅 GetPublic 填充；属主列表/详情为 nil。
	Owner *OwnerPublic
}

func normVis(v string) (string, error) {
	if v == "" {
		return "private", nil
	}
	if v != "public" && v != "private" {
		return "", ErrInvalidVisibility
	}
	return v, nil
}

func normDefaultView(v string) (string, error) {
	if v == "" {
		return "gallery", nil
	}
	if v != "gallery" && v != "immersive" {
		return "", ErrInvalidDefaultView
	}
	return v, nil
}

// NormalizeDefaultView 导出给 handler 读库后兜底。
func NormalizeDefaultView(v string) string {
	out, err := normDefaultView(v)
	if err != nil {
		return "gallery"
	}
	return out
}

func (s *Service) resolveCover(alb model.Album, publicOnly bool) (string, error) {
	if k := strings.TrimSpace(alb.CoverKey); k != "" {
		q := s.db.Model(&model.Image{}).
			Where("key = ? AND album_id = ? AND user_id = ? AND deleted_at IS NULL", k, alb.ID, alb.UserID)
		if publicOnly {
			q = q.Where("visibility = ? AND status = ?", "public", "normal").
				Where("(expires_at IS NULL OR expires_at > ?)", timeNow()).
				Where("(access_password_hash = '' OR access_password_hash IS NULL)")
		}
		var n int64
		if err := q.Count(&n).Error; err != nil {
			return "", err
		}
		if n > 0 {
			return k, nil
		}
	}
	q := s.db.Where("album_id = ? AND user_id = ? AND deleted_at IS NULL", alb.ID, alb.UserID)
	if publicOnly {
		q = q.Where("visibility = ? AND status = ?", "public", "normal").
			Where("(expires_at IS NULL OR expires_at > ?)", timeNow()).
			Where("(access_password_hash = '' OR access_password_hash IS NULL)")
	}
	var cover model.Image
	err := q.Order("CASE WHEN album_pos > 0 THEN 0 ELSE 1 END, album_pos ASC, id DESC").First(&cover).Error
	if err == nil {
		return cover.Key, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return "", err
}

func (s *Service) view(alb model.Album) (AlbumView, error) {
	var count int64
	if err := s.db.Model(&model.Image{}).
		Where("album_id = ? AND user_id = ? AND deleted_at IS NULL", alb.ID, alb.UserID).Count(&count).Error; err != nil {
		return AlbumView{}, err
	}
	coverKey, err := s.resolveCover(alb, false)
	if err != nil {
		return AlbumView{}, err
	}
	return AlbumView{Album: alb, Count: count, CoverKey: coverKey}, nil
}

func (s *Service) List(userID uint64) ([]AlbumView, error) {
	var albums []model.Album
	if err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&albums).Error; err != nil {
		return nil, err
	}
	out := make([]AlbumView, 0, len(albums))
	for _, a := range albums {
		v, err := s.view(a)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) Create(userID uint64, name, visibility string) (*model.Album, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return nil, ErrInvalidName
	}
	vis, err := normVis(visibility)
	if err != nil {
		return nil, err
	}
	alb := &model.Album{
		UserID: userID, Name: name, Visibility: vis,
		DefaultView: "gallery", ClickToImmersive: true, ListInPlaza: vis == "public",
	}
	if err := s.db.Create(alb).Error; err != nil {
		return nil, err
	}
	if vis == "private" {
		// GORM 对 bool 零值会跳过写入，default:true 会把 false 吃掉。
		if err := s.db.Model(alb).Update("list_in_plaza", false).Error; err != nil {
			return nil, err
		}
		alb.ListInPlaza = false
	}
	return alb, nil
}

func (s *Service) Get(userID, id uint64) (*AlbumView, error) {
	var alb model.Album
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v, err := s.view(alb)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetPublic 公开相册访客：仅 visibility=public；否则 ErrNotFound（不区分存在性）。
// 附带属主展示信息（active 用户）；public_profile 决定前端是否链到公开主页。
func (s *Service) GetPublic(id uint64) (*AlbumView, error) {
	var alb model.Album
	err := s.db.Where("id = ?", id).First(&alb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if alb.Visibility != "public" {
		return nil, ErrNotFound
	}
	v, err := s.viewPublic(alb)
	if err != nil {
		return nil, err
	}
	var u model.User
	if err := s.db.Select("username", "nickname", "public_profile", "status").
		Where("id = ?", alb.UserID).First(&u).Error; err == nil && u.Status == "active" {
		v.Owner = &OwnerPublic{
			Username:      u.Username,
			Nickname:      u.Nickname,
			PublicProfile: u.PublicProfile,
		}
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &v, nil
}

// viewPublic 统计/封面仅 public+normal+未过期+非口令图（口令图不出现在访客封面网格）。
func (s *Service) viewPublic(alb model.Album) (AlbumView, error) {
	var count int64
	nowQ := s.db.Model(&model.Image{}).
		Where("album_id = ? AND user_id = ? AND deleted_at IS NULL AND visibility = ? AND status = ?",
			alb.ID, alb.UserID, "public", "normal").
		Where("(expires_at IS NULL OR expires_at > ?)", timeNow()).
		Where("(access_password_hash = '' OR access_password_hash IS NULL)")
	if err := nowQ.Count(&count).Error; err != nil {
		return AlbumView{}, err
	}
	coverKey, err := s.resolveCover(alb, true)
	if err != nil {
		return AlbumView{}, err
	}
	return AlbumView{Album: alb, Count: count, CoverKey: coverKey}, nil
}

// PublicImage 访客网格行。
type PublicImage struct {
	Key    string
	Name   string
	Ext    string
	Width  int
	Height int
	Size   int64
}

// ListPublicImages 公开相册内可展示图（cursor=id 降序）。
func (s *Service) ListPublicImages(albumID uint64, cursor string, limit int) ([]PublicImage, string, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	var alb model.Album
	if err := s.db.Where("id = ? AND visibility = ?", albumID, "public").First(&alb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	q := s.db.Model(&model.Image{}).
		Select("images.key, images.name, images.ext, files.width, files.height, files.size, images.id, images.album_pos").
		Joins("JOIN files ON files.id = images.file_id").
		Where("images.album_id = ? AND images.user_id = ? AND images.deleted_at IS NULL", alb.ID, alb.UserID).
		Where("images.visibility = ? AND images.status = ?", "public", "normal").
		Where("(images.expires_at IS NULL OR images.expires_at > ?)", timeNow()).
		Where("(images.access_password_hash = '' OR images.access_password_hash IS NULL)").
		Order("images.album_pos ASC, images.id DESC")
	if cursor != "" {
		// cursor = "pos:id"；兼容旧仅 id
		var pos int
		var cid uint64
		if n, _ := fmt.Sscanf(cursor, "%d:%d", &pos, &cid); n == 2 && cid > 0 {
			q = q.Where(
				"images.album_pos > ? OR (images.album_pos = ? AND images.id < ?)",
				pos, pos, cid,
			)
		} else if _, err := fmt.Sscanf(cursor, "%d", &cid); err == nil && cid > 0 {
			q = q.Where("images.id < ?", cid)
		}
	}
	type row struct {
		Key      string
		Name     string
		Ext      string
		Width    int
		Height   int
		Size     int64
		ID       uint64
		AlbumPos int
	}
	var rows []row
	if err := q.Limit(limit + 1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) > limit {
		last := rows[limit-1]
		next = fmt.Sprintf("%d:%d", last.AlbumPos, last.ID)
		rows = rows[:limit]
	}
	out := make([]PublicImage, 0, len(rows))
	for _, r := range rows {
		out = append(out, PublicImage{Key: r.Key, Name: r.Name, Ext: r.Ext, Width: r.Width, Height: r.Height, Size: r.Size})
	}
	return out, next, nil
}

// timeNow 可测；生产为 time.Now。
var timeNow = func() time.Time { return time.Now() }

// UpdatePatch 属主更新相册可选字段；指针 nil 表示不改。
// AccessPassword：nil=不改；""=清除；非空=写入哈希。
// CoverKey：nil=不改；""=清除手动封面；非空=校验图在相册内（公开相册还须可公开陈列）。
type UpdatePatch struct {
	Name             *string
	Visibility       *string
	DefaultView      *string
	ClickToImmersive *bool
	Description      *string
	CoverKey         *string
	AccessPassword   *string
	ListInPlaza      *bool
}

func (s *Service) Update(userID, id uint64, p UpdatePatch) (*model.Album, error) {
	var alb model.Album
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 128 {
			return nil, ErrInvalidName
		}
		updates["name"] = n
	}
	becamePrivate := false
	if p.Visibility != nil {
		v, err := normVis(*p.Visibility)
		if err != nil {
			return nil, err
		}
		updates["visibility"] = v
		if v == "private" {
			becamePrivate = true
			if p.ListInPlaza == nil {
				updates["list_in_plaza"] = false
			}
		}
	}
	if p.DefaultView != nil {
		dv, err := normDefaultView(*p.DefaultView)
		if err != nil {
			return nil, err
		}
		updates["default_view"] = dv
	}
	if p.ClickToImmersive != nil {
		updates["click_to_immersive"] = *p.ClickToImmersive
	}
	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		if len(d) > 2000 {
			return nil, ErrInvalidDescription
		}
		updates["description"] = d
	}
	if p.CoverKey != nil {
		k := strings.TrimSpace(*p.CoverKey)
		if k == "" {
			updates["cover_key"] = ""
		} else {
			q := s.db.Model(&model.Image{}).
				Where("key = ? AND album_id = ? AND user_id = ? AND deleted_at IS NULL", k, id, userID)
			// 公开相册封面会进广场卡 / OG /a/，必须是可公开陈列的图。
			if alb.Visibility == "public" {
				q = q.Where("visibility = ? AND status = ?", "public", "normal").
					Where("(expires_at IS NULL OR expires_at > ?)", timeNow()).
					Where("(access_password_hash = '' OR access_password_hash IS NULL)")
			}
			var n int64
			if err := q.Count(&n).Error; err != nil {
				return nil, err
			}
			if n == 0 {
				return nil, ErrInvalidCover
			}
			updates["cover_key"] = k
		}
	}
	if p.AccessPassword != nil {
		pw := strings.TrimSpace(*p.AccessPassword)
		if pw == "" {
			updates["access_password_hash"] = ""
		} else {
			if len(pw) > 128 {
				return nil, ErrInvalidPassword
			}
			h, err := hashAlbumPassword(pw)
			if err != nil {
				return nil, err
			}
			updates["access_password_hash"] = h
		}
	}
	if p.ListInPlaza != nil {
		updates["list_in_plaza"] = *p.ListInPlaza
	}
	if len(updates) > 0 {
		if err := s.db.Model(&alb).Updates(updates).Error; err != nil {
			return nil, err
		}
		if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alb).Error; err != nil {
			return nil, err
		}
	}
	if becamePrivate {
		if _, err := s.SetImagesVisibility(userID, id, "private"); err != nil {
			return nil, err
		}
	}
	return &alb, nil
}

// Delete 删除相册。withImages=false：图片移入未分类(album_id=NULL)；true：软删相册内图片。
func (s *Service) Delete(userID, id uint64, withImages bool) error {
	var alb model.Album
	err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&alb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if withImages {
			// 软删相册内所有 live 图（进各自属主回收站，30 天后由清理任务退配额）
			if err := tx.Where("album_id = ? AND user_id = ?", id, userID).Delete(&model.Image{}).Error; err != nil {
				return err
			}
		}
		// 清空该相册所有图片(含已在回收站者)的 album_id，避免悬挂指向已删相册
		if err := tx.Unscoped().Model(&model.Image{}).
			Where("album_id = ? AND user_id = ?", id, userID).
			Update("album_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Album{}, "id = ?", id).Error
	})
}
