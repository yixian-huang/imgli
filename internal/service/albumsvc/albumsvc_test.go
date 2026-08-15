package albumsvc

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/imagesvc"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

func setup(t *testing.T) (*Service, uint64) {
	db := model.TestDB(t)
	u := &model.User{Username: "a", Email: "a@img.li", GroupID: 1}
	db.Create(u)
	return New(db), u.ID
}

func TestCreateAndListWithCountCover(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "工作", "private")
	if err != nil {
		t.Fatal(err)
	}
	if alb.ListInPlaza {
		t.Error("新建私密相册 list_in_plaza 应为 false")
	}
	// 放两张图进相册（第二张更晚→cover）
	f := &model.File{Hash: "h", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}
	s.db.Create(f)
	s.db.Create(&model.Image{Key: "img000000001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "one", Ext: "png", Visibility: "public", Status: "normal"})
	s.db.Create(&model.Image{Key: "img000000002", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "two", Ext: "png", Visibility: "public", Status: "normal"})
	views, err := s.List(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Count != 2 {
		t.Fatalf("count 应为 2: %+v", views)
	}
	if views[0].CoverKey != "img000000002" {
		t.Errorf("cover 应为最新图, got %q", views[0].CoverKey)
	}
}

func TestDeleteAlbumOnlyDetachesImages(t *testing.T) {
	s, uid := setup(t)
	alb, _ := s.Create(uid, "x", "private")
	f := &model.File{Hash: "h", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}
	s.db.Create(f)
	s.db.Create(&model.Image{Key: "detach000001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "n", Ext: "png", Visibility: "public", Status: "normal"})
	if err := s.Delete(uid, alb.ID, false); err != nil {
		t.Fatal(err)
	}
	var img model.Image
	s.db.Where("key = ?", "detach000001").First(&img)
	if img.AlbumID != nil {
		t.Error("with_images=false 应把图片移入未分类(album_id=NULL)")
	}
	if img.DeletedAt.Valid {
		t.Error("with_images=false 不应删图")
	}
}

func TestDeleteAlbumWithImagesSoftDeletes(t *testing.T) {
	s, uid := setup(t)
	alb, _ := s.Create(uid, "x", "private")
	f := &model.File{Hash: "h", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}
	s.db.Create(f)
	s.db.Create(&model.Image{Key: "withimg00001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "n", Ext: "png", Visibility: "public", Status: "normal"})
	if err := s.Delete(uid, alb.ID, true); err != nil {
		t.Fatal(err)
	}
	var cnt int64
	s.db.Model(&model.Image{}).Where("key = ?", "withimg00001").Count(&cnt) // 软删后默认查不到
	if cnt != 0 {
		t.Error("with_images=true 应软删相册内图片")
	}
}

func TestDeleteAlbumClearsAlbumIDOnTrashedImages(t *testing.T) {
	s, uid := setup(t)
	alb, _ := s.Create(uid, "工作", "private")
	f := &model.File{Hash: "h", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}
	s.db.Create(f)
	// 一张 live 图 + 一张已在回收站的图，都属于该相册
	s.db.Create(&model.Image{Key: "livekey00001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "live", Ext: "png", Visibility: "public", Status: "normal"})
	trashed := &model.Image{Key: "trashkey0001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "trashed", Ext: "png", Visibility: "public", Status: "normal"}
	s.db.Create(trashed)
	s.db.Delete(trashed) // 软删：进回收站，但仍带 album_id

	// with_images=false 删除相册
	if err := s.Delete(uid, alb.ID, false); err != nil {
		t.Fatal(err)
	}
	// live 图 album_id 应清空
	var live model.Image
	s.db.Where("key = ?", "livekey00001").First(&live)
	if live.AlbumID != nil {
		t.Errorf("live 图 album_id 应清空, got %v", live.AlbumID)
	}
	// 已在回收站的图 album_id 也应清空(不能悬挂指向已删相册)——用 Unscoped 才查得到
	var tr model.Image
	s.db.Unscoped().Where("key = ?", "trashkey0001").First(&tr)
	if tr.AlbumID != nil {
		t.Errorf("回收站中的图 album_id 也应清空, got %v", tr.AlbumID)
	}
}

func TestAlbumForeignReturnsNotFound(t *testing.T) {
	s, uid := setup(t)
	alb, _ := s.Create(uid, "x", "private")
	other := &model.User{Username: "b", Email: "b@img.li", GroupID: 1}
	s.db.Create(other)
	if _, err := s.Get(other.ID, alb.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("他人相册应 ErrNotFound, got %v", err)
	}
}

func TestUpdateDefaultView(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "视图", "public")
	if err != nil {
		t.Fatal(err)
	}
	if !alb.ClickToImmersive {
		t.Error("新建相册 ClickToImmersive 默认应为 true")
	}
	bad := "carousel"
	if _, err := s.Update(uid, alb.ID, UpdatePatch{DefaultView: &bad}); !errors.Is(err, ErrInvalidDefaultView) {
		t.Fatalf("非法 default_view 应 ErrInvalidDefaultView, got %v", err)
	}
	imm := "immersive"
	got, err := s.Update(uid, alb.ID, UpdatePatch{DefaultView: &imm})
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultView != "immersive" {
		t.Errorf("DefaultView=%q want immersive", got.DefaultView)
	}
	v, err := s.GetPublic(alb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if NormalizeDefaultView(v.Album.DefaultView) != "immersive" {
		t.Errorf("public DefaultView=%q", v.Album.DefaultView)
	}
}

func TestUpdateClickToImmersive(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "点击", "public")
	if err != nil {
		t.Fatal(err)
	}
	off := false
	got, err := s.Update(uid, alb.ID, UpdatePatch{ClickToImmersive: &off})
	if err != nil {
		t.Fatal(err)
	}
	if got.ClickToImmersive {
		t.Error("ClickToImmersive 应为 false")
	}
	v, err := s.GetPublic(alb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Album.ClickToImmersive {
		t.Error("public ClickToImmersive 应为 false")
	}
	on := true
	got, err = s.Update(uid, alb.ID, UpdatePatch{ClickToImmersive: &on})
	if err != nil {
		t.Fatal(err)
	}
	if !got.ClickToImmersive {
		t.Error("ClickToImmersive 应为 true")
	}
}

func TestUpdatePrivateCascadesImages(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "将私密", "public")
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "hc", StoragePolicyID: 1, Path: "pc", Size: 1, RefCount: 1}
	s.db.Create(f)
	s.db.Create(&model.Image{Key: "cascpub000001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "a", Ext: "png", Visibility: "public", Status: "normal"})
	s.db.Create(&model.Image{Key: "cascpub000002", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "b", Ext: "png", Visibility: "public", Status: "normal"})

	priv := "private"
	got, err := s.Update(uid, alb.ID, UpdatePatch{Visibility: &priv})
	if err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private" {
		t.Fatalf("相册应为 private, got %q", got.Visibility)
	}
	if got.ListInPlaza {
		t.Error("设为私密后 list_in_plaza 应为 false")
	}
	var pubLeft int64
	s.db.Model(&model.Image{}).Where("album_id = ? AND visibility = ?", alb.ID, "public").Count(&pubLeft)
	if pubLeft != 0 {
		t.Fatalf("私密相册内仍有 %d 张 public 图", pubLeft)
	}
}

func TestSetImagesVisibilityRejectsPublicOnPrivateAlbum(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "密", "private")
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "hr", StoragePolicyID: 1, Path: "pr", Size: 1, RefCount: 1}
	s.db.Create(f)
	s.db.Create(&model.Image{Key: "rejpub0000001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID, Name: "a", Ext: "png", Visibility: "private", Status: "normal"})
	if _, err := s.SetImagesVisibility(uid, alb.ID, "public"); !errors.Is(err, ErrAlbumForcesPrivate) {
		t.Fatalf("私密相册批量公开应 ErrAlbumForcesPrivate, got %v", err)
	}
}

func TestGetPublicOwnerAndVisibility(t *testing.T) {
	s, uid := setup(t)
	// setup 创建的用户无 nickname/public_profile；补全
	s.db.Model(&model.User{}).Where("id = ?", uid).Updates(map[string]any{
		"nickname": "阿狸", "public_profile": true, "status": "active",
	})
	priv, _ := s.Create(uid, "私密", "private")
	if _, err := s.GetPublic(priv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("私密相册 GetPublic 应 ErrNotFound, got %v", err)
	}
	pub, _ := s.Create(uid, "公开游记", "public")
	f := &model.File{Hash: "hp", StoragePolicyID: 1, Path: "pp", Size: 1, RefCount: 1}
	s.db.Create(f)
	// 仅 public+normal 计入访客
	s.db.Create(&model.Image{Key: "pubimg000001", UserID: &uid, FileID: f.ID, AlbumID: &pub.ID, Name: "ok", Ext: "png", Visibility: "public", Status: "normal"})
	s.db.Create(&model.Image{Key: "privimg00001", UserID: &uid, FileID: f.ID, AlbumID: &pub.ID, Name: "hid", Ext: "png", Visibility: "private", Status: "normal"})

	v, err := s.GetPublic(pub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Count != 1 {
		t.Errorf("访客 count 应为 1（仅公开图）, got %d", v.Count)
	}
	if v.Owner == nil || v.Owner.Username != "a" || v.Owner.Nickname != "阿狸" || !v.Owner.PublicProfile {
		t.Errorf("Owner 异常: %+v", v.Owner)
	}
	items, _, err := s.ListPublicImages(pub.ID, "", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Key != "pubimg000001" {
		t.Errorf("ListPublicImages 应只吐公开图: %+v", items)
	}
}

func TestUpdateCoverRejectsPrivateOnPublicAlbum(t *testing.T) {
	s, uid := setup(t)
	alb, err := s.Create(uid, "公开册", "public")
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "hcover1", StoragePolicyID: 1, Path: "p", Size: 1, RefCount: 1}
	s.db.Create(f)
	priv := &model.Image{Key: "coverpriv00001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID,
		Name: "hid", Ext: "png", Visibility: "private", Status: "normal"}
	s.db.Create(priv)
	ck := priv.Key
	if _, err := s.Update(uid, alb.ID, UpdatePatch{CoverKey: &ck}); !errors.Is(err, ErrInvalidCover) {
		t.Fatalf("公开相册不得把私密图设为封面, got %v", err)
	}
}

func TestListPublicAlbumsSkipsPrivateManualCover(t *testing.T) {
	s, uid := setup(t)
	if err := s.db.Model(&model.User{}).Where("id = ?", uid).Updates(map[string]any{
		"public_profile": true, "status": "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	alb, err := s.Create(uid, "游记", "public")
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "hcover2", StoragePolicyID: 1, Path: "p2", Size: 1, RefCount: 1}
	s.db.Create(f)
	priv := &model.Image{Key: "plazapriv00001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID,
		Name: "hid", Ext: "png", Visibility: "private", Status: "normal"}
	pub := &model.Image{Key: "plazapub000001", UserID: &uid, FileID: f.ID, AlbumID: &alb.ID,
		Name: "ok", Ext: "png", Visibility: "public", Status: "normal"}
	s.db.Create(priv)
	s.db.Create(pub)
	if err := s.db.Model(alb).Update("cover_key", priv.Key).Error; err != nil {
		t.Fatal(err)
	}

	cards, _, err := s.ListPublicAlbums("", 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("应有 1 张公开相册卡, got %d", len(cards))
	}
	if cards[0].CoverKey == priv.Key {
		t.Fatal("广场相册卡泄漏了私密封面 key")
	}
	if cards[0].CoverKey != pub.Key {
		t.Fatalf("应回落到公开图封面, got %q", cards[0].CoverKey)
	}

	v, err := s.GetPublic(alb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.CoverKey == priv.Key {
		t.Fatal("访客相册页泄漏了私密封面 key")
	}
	if v.CoverKey != pub.Key {
		t.Fatalf("访客封面应回落公开图, got %q", v.CoverKey)
	}
}

// TestSetImagesVisibilityRehomesSurface 批量改私密须走 imagesvc 重挂，不能只改 visibility 列。
func TestSetImagesVisibilityRehomesSurface(t *testing.T) {
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir(), BaseURL: "https://img.li"}
	res := storagesvc.New(cfg, db)
	s := New(db).WithImages(imagesvc.New(db, res, nil))

	u := &model.User{Username: "a", Email: "a@img.li", GroupID: 1}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	alb, err := s.Create(u.ID, "将私密", "public")
	if err != nil {
		t.Fatal(err)
	}
	var policy model.StoragePolicy
	if err := db.First(&policy, "driver = ?", "local").Error; err != nil {
		t.Fatal(err)
	}
	d, err := res.Driver(&policy)
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "bulkhash1", Surface: model.SurfacePublic, StoragePolicyID: policy.ID,
		Path: "public/2026/07/bulk.png", Size: 5, MIME: "image/png", Width: 1, Height: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Put(context.Background(), f.Path, bytes.NewReader([]byte("bytes"))); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(context.Background(), storagesvc.ThumbKey(model.SurfacePublic, f.Hash), bytes.NewReader([]byte("thumb"))); err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "bulkrehome0001", UserID: &u.ID, FileID: f.ID, AlbumID: &alb.ID,
		Name: "a", Ext: "png", Visibility: "public", Status: "normal"}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}

	n, err := s.SetImagesVisibility(u.ID, alb.ID, "private")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应更新 1 张, got %d", n)
	}
	var got model.Image
	if err := db.First(&got, img.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private" {
		t.Fatalf("可见性应为 private, got %q", got.Visibility)
	}
	var nf model.File
	if err := db.First(&nf, got.FileID).Error; err != nil {
		t.Fatal(err)
	}
	if nf.Surface != model.SurfacePrivate {
		t.Fatalf("批量改私密应重挂 surface, got %q", nf.Surface)
	}
}
