package upload

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/storage"
)

// randBase62 返回 n 位随机 base62 字符串。
func randBase62(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = base62[int(buf[i])%62]
	}
	return string(out), nil
}

func (s *Service) uniqueKey(tx *gorm.DB) (string, error) {
	for i := 0; i < 5; i++ {
		key, err := randBase62(12) // model.Image.Key: 12 位 base62，直链用
		if err != nil {
			return "", err
		}
		var n int64
		if err := tx.Unscoped().Model(&model.Image{}).Where("key = ?", key).Count(&n).Error; err != nil {
			return "", err
		}
		if n == 0 {
			return key, nil
		}
	}
	return "", errors.New("upload: 无法生成唯一 key")
}

func readPrefix(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return buf[:got], nil
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func (s *Service) probe(p string) (imaging.Meta, error) {
	f, err := os.Open(p)
	if err != nil {
		return imaging.Meta{}, err
	}
	defer f.Close()
	return s.proc.Probe(f)
}

func (s *Service) thumbnail(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return s.proc.Thumbnail(f, ThumbMaxEdge)
}

func (s *Service) hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) putFile(ctx context.Context, d storage.Driver, key, srcPath string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return d.Put(ctx, key, f)
}

func extAllowed(allowed []string, ext string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, ext) {
			return true
		}
	}
	return len(allowed) == 0 // 空白名单视为不限（保守：应至少有默认后缀）
}

// truncateName 将展示名截断到 <=255 字节且不破坏 UTF-8 字符边界。

// truncateName 将展示名截断到 <=255 字节且不破坏 UTF-8 字符边界。
func truncateName(s string) string {
	const max = 255
	if len(s) <= max {
		return s
	}
	b := []byte(s)[:max]
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return string(b)
}

func normVisibility(v string) string {
	if v == "private" {
		return "private"
	}
	return "public"
}

// visibilityFor 计算落库可见性：游客图（u==nil）恒 public，无视调用方传入的
// visibility——游客无账号，owner 校验永远匹配不到 NULL user_id，一旦允许游客建
// private 记录就会成为谁也看不到的死记录（spec §5 "游客 visibility 恒 public"）。

// visibilityFor 计算落库可见性：游客图（u==nil）恒 public，无视调用方传入的
// visibility——游客无账号，owner 校验永远匹配不到 NULL user_id，一旦允许游客建
// private 记录就会成为谁也看不到的死记录（spec §5 "游客 visibility 恒 public"）。
func visibilityFor(u *model.User, visibility string) string {
	if u == nil {
		return "public"
	}
	return normVisibility(visibility)
}
