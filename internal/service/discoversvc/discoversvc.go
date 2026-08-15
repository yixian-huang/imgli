// Package discoversvc 提供只读公开发现面（广场 + 用户公开主页）查询。
// 所有列表均经 eligible 硬过滤，只吐可公开陈列的图；与属主作用域的 imagesvc 分家。
package discoversvc

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
)

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

// Author 公开陈列时的作者信息。
type Author struct {
	UserID        uint64 `json:"user_id"`
	Username      string `json:"username"`
	Nickname      string `json:"nickname"`
	AvatarVersion int64  `json:"avatar_version"` // = user.UpdatedAt.Unix()，前端破缓存用
}

// Row 广场/用户公开列表中的一行。
type Row struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	Ext       string    `json:"ext"`
	CreatedAt time.Time `json:"created_at"`
	Views     int64     `json:"views"`
	Author    Author    `json:"author"`
}

// Profile 用户公开主页元信息。
type Profile struct {
	Username         string    `json:"username"`
	Nickname         string    `json:"nickname"`
	AvatarVersion    int64     `json:"avatar_version"`
	JoinedAt         time.Time `json:"joined_at"`
	PublicImageCount int64     `json:"public_image_count"`
}

// ErrNotFound: 用户不存在 / public_profile=false / banned —— 统一此错误（handler 映射 404 防枚举）
var ErrNotFound = errors.New("discoversvc: 主页不存在或未公开")

// ErrBadCursor 游标格式错误或 sort 与 cursor 不匹配（handler 映射 400）
var ErrBadCursor = errors.New("discoversvc: 游标格式错误")

// eligible 恒定资格硬过滤：只吐可公开陈列的图。所有列表/计数必须经此单点。
// 有相册时须同时满足：相册公开、且 list_in_plaza=true。
// 私密相册即使 list_in_plaza 仍为默认 true，其内图片也不得进广场/公开主页。
// 设了访问口令的图（/t 会 401）不得进广场。无相册（album_id NULL）仍可进广场。
func (s *Service) eligible() *gorm.DB {
	return s.db.Model(&model.Image{}).
		Joins("JOIN users ON users.id = images.user_id").
		Where("images.visibility = ?", "public").
		Where("images.status = ?", "normal").
		Where("images.deleted_at IS NULL").
		Where("images.access_password_hash = ?", "").
		Where("images.expires_at IS NULL OR images.expires_at > ?", time.Now()).
		Where("images.user_id IS NOT NULL").
		Where("users.public_profile = ?", true).
		Where("users.status = ?", "active").
		Where(`images.album_id IS NULL OR EXISTS (
			SELECT 1 FROM albums WHERE albums.id = images.album_id
			AND albums.visibility = ? AND albums.list_in_plaza = ?
		)`, "public", true)
}

// scanRow 一次 join 投影接收结构。
type scanRow struct {
	Key             string
	Name            string
	Ext             string
	CreatedAt       time.Time
	Views           int64
	ID              uint64 // images.id，供 cursor
	AuthorID        uint64
	Username        string
	Nickname        string
	AuthorUpdatedAt time.Time
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 24
	}
	if limit > 60 {
		return 60
	}
	return limit
}

func normalizeSort(sort string) string {
	if sort != "hot" {
		return "new"
	}
	return "hot"
}

// feed 公共 keyset 分页查询：base 已含 eligible（及可选 user 范围）。
// 两种 sort 都 LEFT JOIN access_stats 汇总以填充 Views。
func (s *Service) feed(base *gorm.DB, sort, cursor string, limit int) ([]Row, string, error) {
	limit = clampLimit(limit)
	sort = normalizeSort(sort)

	q := base.Joins("LEFT JOIN (SELECT image_id, SUM(views) AS v FROM access_stats GROUP BY image_id) st ON st.image_id = images.id")

	if cursor != "" {
		cur, err := decodeCursor(cursor)
		if err != nil || cur.Sort != sort {
			return nil, "", ErrBadCursor
		}
		if sort == "hot" {
			q = q.Where("(COALESCE(st.v,0) < ?) OR (COALESCE(st.v,0) = ? AND images.id < ?)",
				cur.Val, cur.Val, cur.ID)
		} else {
			// Val 用 Unix 秒（非纳秒）：SQLite/GORM 落盘与比较多为秒级；
			// 纳秒游标会导致 created_at = ? 永不相等，同秒/同刻行无法用 id 排除。
			t := time.Unix(cur.Val, 0).UTC()
			q = q.Where("(images.created_at < ?) OR (images.created_at = ? AND images.id < ?)",
				t, t, cur.ID)
		}
	}

	if sort == "hot" {
		q = q.Order("COALESCE(st.v,0) DESC, images.id DESC")
	} else {
		q = q.Order("images.created_at DESC, images.id DESC")
	}

	q = q.Select("images.key, images.name, images.ext, images.created_at, images.id, " +
		"users.id AS author_id, users.username, users.nickname, users.updated_at AS author_updated_at, " +
		"COALESCE(st.v,0) AS views").
		Limit(limit + 1)

	var scans []scanRow
	if err := q.Scan(&scans).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(scans) > limit {
		last := scans[limit-1]
		scans = scans[:limit]
		if sort == "hot" {
			nextCursor = encodeCursor(feedCursor{Sort: sort, Val: last.Views, ID: last.ID})
		} else {
			nextCursor = encodeCursor(feedCursor{Sort: sort, Val: last.CreatedAt.UTC().Unix(), ID: last.ID})
		}
	}

	rows := make([]Row, 0, len(scans))
	for _, sc := range scans {
		rows = append(rows, Row{
			Key:       sc.Key,
			Name:      sc.Name,
			Ext:       sc.Ext,
			CreatedAt: sc.CreatedAt,
			Views:     sc.Views,
			Author: Author{
				UserID:        sc.AuthorID,
				Username:      sc.Username,
				Nickname:      sc.Nickname,
				AvatarVersion: sc.AuthorUpdatedAt.Unix(),
			},
		})
	}
	return rows, nextCursor, nil
}

// PlazaFeed 全站广场。sort ∈ {"new","hot"}；其它值按 "new" 宽容回落。
// cursor 空串 = 首页；返回的 nextCursor 空串 = 无更多页。
func (s *Service) PlazaFeed(sort, cursor string, limit int) ([]Row, string, error) {
	return s.feed(s.eligible(), sort, cursor, limit)
}

// UserPublic 用户公开主页元信息。不存在/未公开/banned 统一 ErrNotFound。
func (s *Service) UserPublic(username string) (Profile, error) {
	var u model.User
	err := s.db.Where("username = ? AND public_profile = ? AND status = ?", username, true, "active").
		First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	var cnt int64
	if err := s.eligible().Where("images.user_id = ?", u.ID).Count(&cnt).Error; err != nil {
		return Profile{}, err
	}
	return Profile{
		Username:         u.Username,
		Nickname:         u.Nickname,
		AvatarVersion:    u.UpdatedAt.Unix(),
		JoinedAt:         u.CreatedAt,
		PublicImageCount: cnt,
	}, nil
}

// UserImages 某用户公开图列表（仍经 eligible，不会吐私图）。
func (s *Service) UserImages(userID uint64, sort, cursor string, limit int) ([]Row, string, error) {
	return s.feed(s.eligible().Where("images.user_id = ?", userID), sort, cursor, limit)
}
