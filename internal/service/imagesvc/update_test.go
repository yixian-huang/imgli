package imagesvc

import (
	"errors"
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/model"
)

func ptrStr(s string) *string { return &s }
func ptrI64(v int64) *int64   { return &v }

func TestUpdateRenameAndVisibility(t *testing.T) {
	s, uid := setupSvc(t)
	var img model.Image
	s.db.Where("user_id = ?", uid).First(&img)
	row, err := s.Update(uid, img.Key, UpdatePatch{Name: ptrStr("renamed"), Visibility: ptrStr("private")})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.Name != "renamed" || row.Img.Visibility != "private" {
		t.Fatalf("改名/可见性未生效: %+v", row.Img)
	}
}

func TestUpdateRejectsBadVisibility(t *testing.T) {
	s, uid := setupSvc(t)
	var img model.Image
	s.db.Where("user_id = ?", uid).First(&img)
	if _, err := s.Update(uid, img.Key, UpdatePatch{Visibility: ptrStr("secret")}); !errors.Is(err, ErrInvalidVisibility) {
		t.Errorf("非法可见性应报错, got %v", err)
	}
}

func TestUpdateRejectsPublicWhileInPrivateAlbum(t *testing.T) {
	s, uid := setupSvc(t)
	alb := &model.Album{UserID: uid, Name: "密", Visibility: "private"}
	s.db.Create(alb)
	var img model.Image
	s.db.Where("user_id = ? AND visibility = ?", uid, "public").First(&img)
	if _, err := s.Update(uid, img.Key, UpdatePatch{AlbumID: ptrI64(int64(alb.ID))}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(uid, img.Key, UpdatePatch{Visibility: ptrStr("public")}); !errors.Is(err, ErrAlbumForcesPrivate) {
		t.Fatalf("私密相册内改 public 应 ErrAlbumForcesPrivate, got %v", err)
	}
	var got model.Image
	s.db.Where("key = ?", img.Key).First(&got)
	if got.Visibility != "private" {
		t.Fatalf("拒绝后仍应为 private, got %q", got.Visibility)
	}
}

func TestUpdateMoveToPrivateAlbumForcesPrivate(t *testing.T) {
	s, uid := setupSvc(t)
	alb := &model.Album{UserID: uid, Name: "密", Visibility: "private"}
	s.db.Create(alb)
	var img model.Image
	s.db.Where("user_id = ? AND visibility = ?", uid, "public").First(&img)
	if img.Key == "" {
		t.Fatal("需要一张 public 夹具图")
	}
	row, err := s.Update(uid, img.Key, UpdatePatch{AlbumID: ptrI64(int64(alb.ID))})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.Visibility != "private" {
		t.Fatalf("移入私密相册应为 private, got %q", row.Img.Visibility)
	}
}

func TestUpdateMoveToOwnedAlbumAndClear(t *testing.T) {
	s, uid := setupSvc(t)
	alb := &model.Album{UserID: uid, Name: "工作", Visibility: "private"}
	s.db.Create(alb)
	var img model.Image
	s.db.Where("user_id = ?", uid).First(&img)
	// 移入
	row, err := s.Update(uid, img.Key, UpdatePatch{AlbumID: ptrI64(int64(alb.ID))})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.AlbumID == nil || *row.Img.AlbumID != alb.ID {
		t.Fatalf("移入相册未生效: %+v", row.Img.AlbumID)
	}
	// 移出（album_id=0）
	row, err = s.Update(uid, img.Key, UpdatePatch{AlbumID: ptrI64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.AlbumID != nil {
		t.Fatalf("移出相册未生效: %+v", row.Img.AlbumID)
	}
}

func TestUpdateRejectsForeignAlbum(t *testing.T) {
	s, uid := setupSvc(t)
	other := &model.User{Username: "u9", Email: "u9@img.li", GroupID: 1}
	s.db.Create(other)
	foreign := &model.Album{UserID: other.ID, Name: "别人的", Visibility: "private"}
	s.db.Create(foreign)
	var img model.Image
	s.db.Where("user_id = ?", uid).First(&img)
	if _, err := s.Update(uid, img.Key, UpdatePatch{AlbumID: ptrI64(int64(foreign.ID))}); !errors.Is(err, ErrAlbumNotFound) {
		t.Errorf("移入他人相册应 ErrAlbumNotFound, got %v", err)
	}
}

// TestUpdateExpiresTriState setExpires=false 不改；set+nil 清除；set+T 设。
func TestUpdateExpiresTriState(t *testing.T) {
	s, uid := setupSvc(t)
	var img model.Image
	s.db.Where("user_id = ?", uid).First(&img)
	orig := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	if err := s.db.Model(&img).Update("expires_at", orig).Error; err != nil {
		t.Fatal(err)
	}

	// setExpires=false：不改 expires_at
	row, err := s.Update(uid, img.Key, UpdatePatch{Name: ptrStr("keep-exp")})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.ExpiresAt == nil || !row.Img.ExpiresAt.UTC().Equal(orig) {
		t.Fatalf("setExpires=false 不应改 expires_at: got %v want %v", row.Img.ExpiresAt, orig)
	}
	if row.Img.Name != "keep-exp" {
		t.Fatalf("改名应生效: %q", row.Img.Name)
	}

	// setExpires + expiresAt=T：设
	want := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	row, err = s.Update(uid, img.Key, UpdatePatch{ExpiresAt: &want, SetExpires: true})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.ExpiresAt == nil || !row.Img.ExpiresAt.UTC().Equal(want) {
		t.Fatalf("setExpires+T 应设 expires_at: got %v want %v", row.Img.ExpiresAt, want)
	}

	// setExpires + expiresAt=nil：清除为 NULL
	row, err = s.Update(uid, img.Key, UpdatePatch{SetExpires: true})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.ExpiresAt != nil {
		t.Fatalf("setExpires+nil 应清除 expires_at, got %v", row.Img.ExpiresAt)
	}

	// 再确认 DB 真写 NULL
	var re model.Image
	if err := s.db.Where("key = ?", img.Key).First(&re, nil).Error; err != nil {
		t.Fatal(err)
	}
	if re.ExpiresAt != nil {
		t.Fatalf("DB expires_at 应为 NULL, got %v", re.ExpiresAt)
	}
}
