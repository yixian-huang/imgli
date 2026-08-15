package upload

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
)

func TestSaveVisibilityFallback(t *testing.T) {
	svc, u, _ := setup(t)
	u.Preferences.DefaultVisibility = "private"
	if err := svc.db.Model(u).Select("preferences").Updates(u).Error; err != nil {
		t.Fatal(err)
	}

	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 40, 30), "v1.png", u, Opts{}, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.Visibility != "private" {
		t.Errorf("偏好 private + opts 空 → want private, got %q", res.Image.Visibility)
	}

	res2, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 41, 30), "v2.png", u, Opts{Visibility: "public"}, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Image.Visibility != "public" {
		t.Errorf("opts public 应覆盖偏好, got %q", res2.Image.Visibility)
	}
}

func TestSavePrivateAlbumForcesPrivate(t *testing.T) {
	svc, u, _ := setup(t)
	priv := model.Album{UserID: u.ID, Name: "secret", Visibility: "private"}
	if err := svc.db.Create(&priv).Error; err != nil {
		t.Fatal(err)
	}
	pub := model.Album{UserID: u.ID, Name: "open", Visibility: "public"}
	if err := svc.db.Create(&pub).Error; err != nil {
		t.Fatal(err)
	}

	forced, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 40, 30), "p.png", u,
		Opts{Visibility: "public", AlbumID: &priv.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if forced.Image.Visibility != "private" {
		t.Fatalf("私密相册内的图应为 private, got %q", forced.Image.Visibility)
	}
	if forced.File.Surface != model.SurfacePrivate {
		t.Fatalf("私密相册图 surface 应为 private, got %q", forced.File.Surface)
	}

	kept, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 41, 30), "q.png", u,
		Opts{Visibility: "public", AlbumID: &pub.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if kept.Image.Visibility != "public" {
		t.Fatalf("公开相册应保留显式 public, got %q", kept.Image.Visibility)
	}
}

func TestSaveAlbumTriState(t *testing.T) {
	svc, u, _ := setup(t)
	alb := model.Album{UserID: u.ID, Name: "mine", Visibility: "private"}
	if err := svc.db.Create(&alb).Error; err != nil {
		t.Fatal(err)
	}
	u.Preferences.DefaultAlbumID = &alb.ID
	if err := svc.db.Model(u).Select("preferences").Updates(u).Error; err != nil {
		t.Fatal(err)
	}

	// opts nil → 回退偏好 X
	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 20, 20), "a1.png", u, Opts{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.AlbumID == nil || *res.Image.AlbumID != alb.ID {
		t.Errorf("opts nil 应归档到偏好相册, got %v", res.Image.AlbumID)
	}

	// &0 → 明确不归档
	zero := uint64(0)
	res2, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 21, 20), "a2.png", u, Opts{AlbumID: &zero}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Image.AlbumID != nil {
		t.Errorf("&0 应不归档, got %v", *res2.Image.AlbumID)
	}

	// 不存在相册 → ErrAlbumNotFound
	bad := uint64(99999)
	if _, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 22, 20), "a3.png", u, Opts{AlbumID: &bad}, ""); !errors.Is(err, ErrAlbumNotFound) {
		t.Errorf("他人/不存在相册 want ErrAlbumNotFound, got %v", err)
	}

	// 删 X 后 opts nil → 悬空静默
	if err := svc.db.Delete(&alb).Error; err != nil {
		t.Fatal(err)
	}
	// 刷新内存偏好仍指向已删 ID
	res3, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 23, 20), "a4.png", u, Opts{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res3.Image.AlbumID != nil {
		t.Errorf("偏好悬空应静默忽略, got %v", *res3.Image.AlbumID)
	}
}

func TestSavePolicyChain(t *testing.T) {
	svc, u, _ := setup(t)
	// 额外策略 P2，并把组 allowed 扩成 [1, P2]
	root := t.TempDir()
	p2 := &model.StoragePolicy{
		Name: "p2", Driver: "local", Enabled: true,
		Config: map[string]string{"root": root},
	}
	if err := svc.db.Create(p2).Error; err != nil {
		t.Fatal(err)
	}
	var g model.UserGroup
	if err := svc.db.First(&g, u.GroupID).Error; err != nil {
		t.Fatal(err)
	}
	g.AllowedPolicyIDs = []uint64{1, p2.ID}
	if err := svc.db.Model(&g).Select("allowed_policy_ids").Updates(&g).Error; err != nil {
		t.Fatal(err)
	}

	// 显式 P2
	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 30, 30), "p1.png", u, Opts{PolicyID: p2.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Policy.ID != p2.ID || res.File.StoragePolicyID != p2.ID {
		t.Errorf("显式策略应 P2, policy=%d file=%d", res.Policy.ID, res.File.StoragePolicyID)
	}

	// 非法策略
	if _, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 31, 30), "p2.png", u, Opts{PolicyID: 999}, ""); !errors.Is(err, ErrPolicyNotAllowed) {
		t.Errorf("want ErrPolicyNotAllowed, got %v", err)
	}

	// 偏好悬空 999 + opts 0 → 组默认
	bad := uint64(999)
	u.Preferences.DefaultPolicyID = &bad
	if err := svc.db.Model(u).Select("preferences").Updates(u).Error; err != nil {
		t.Fatal(err)
	}
	res3, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 32, 30), "p3.png", u, Opts{}, "")
	if err != nil {
		t.Fatalf("偏好悬空应降级组默认: %v", err)
	}
	if res3.Policy.ID != 1 {
		t.Errorf("组默认应为 id=1, got %d", res3.Policy.ID)
	}
}

func TestSaveGuestIgnoresOpts(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	aid := uint64(5)
	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 50, 50), "g.png", nil,
		Opts{Visibility: "private", AlbumID: &aid, PolicyID: 999}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.Visibility != "public" {
		t.Errorf("游客 visibility 应 public, got %q", res.Image.Visibility)
	}
	if res.Image.AlbumID != nil {
		t.Errorf("游客 AlbumID 应 nil, got %v", *res.Image.AlbumID)
	}
	if res.Policy.ID != 1 {
		t.Errorf("游客应组默认策略 id=1, got %d", res.Policy.ID)
	}
}

func TestSaveInstantAlbum(t *testing.T) {
	svc, u, _ := setup(t)
	alb := model.Album{UserID: u.ID, Name: "inst", Visibility: "public"}
	if err := svc.db.Create(&alb).Error; err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if _, err := svc.Save(context.Background(), pngFile(t, dir, 60, 40), "i1.png", u, Opts{}, ""); err != nil {
		t.Fatal(err)
	}
	res2, err := svc.Save(context.Background(), pngFile(t, dir, 60, 40), "i2.png", u, Opts{AlbumID: &alb.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Fatal("应秒传")
	}
	if res2.Image.AlbumID == nil || *res2.Image.AlbumID != alb.ID {
		t.Errorf("秒传 image AlbumID 应为 %d, got %v", alb.ID, res2.Image.AlbumID)
	}
}

func TestSaveUserGoneRace(t *testing.T) {
	svc, u, _ := setup(t)
	uid := u.ID
	if err := svc.db.Delete(&model.User{}, uid).Error; err != nil {
		t.Fatal(err)
	}
	_, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 25, 25), "gone.png", u, Opts{}, "")
	if err == nil {
		t.Fatal("用户已删 Save 应返回错误")
	}
	var n int64
	svc.db.Unscoped().Model(&model.Image{}).Where("user_id = ?", uid).Count(&n)
	if n != 0 {
		t.Errorf("不应留下孤儿 image, got %d", n)
	}
	// 允许 ErrRecordNotFound 或 FK 等错误
	_ = errors.Is(err, gorm.ErrRecordNotFound)
}
