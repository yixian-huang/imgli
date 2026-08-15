package upload

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/task"
)

func setup(t *testing.T) (*Service, *model.User, *config.Config) {
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir(), BaseURL: "https://img.li"}
	res := storagesvc.New(cfg, db)
	runner := task.New(db, 1)
	svc := New(db, res, imaging.NewGo(), runner)
	runner.Register("delete_file", svc.DeleteFileTask)
	u := &model.User{Username: "alice", Email: "a@img.li", GroupID: 1, Status: "active"}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	return svc, u, cfg
}

func pngFile(t *testing.T, dir string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	}
	p := filepath.Join(dir, "in.png")
	f, _ := os.Create(p)
	png.Encode(f, img)
	f.Close()
	return p
}

func TestSaveSuccessCreatesFileImageAndThumb(t *testing.T) {
	svc, u, cfg := setup(t)
	tmp := pngFile(t, t.TempDir(), 300, 200)

	res, err := svc.Save(context.Background(), tmp, "shot.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.Instant {
		t.Error("首次上传不应是秒传")
	}
	if res.Image.Key == "" || res.Image.Ext != "png" || res.File.RefCount != 1 {
		t.Errorf("记录不符: %+v / file %+v", res.Image, res.File)
	}
	if res.File.Width != 300 || res.File.Height != 200 {
		t.Errorf("尺寸 %dx%d", res.File.Width, res.File.Height)
	}
	// used_storage 已累加
	var got model.User
	svc.db.First(&got, u.ID)
	if got.UsedStorage != res.File.Size {
		t.Errorf("used_storage=%d, want %d", got.UsedStorage, res.File.Size)
	}
	// 原图与缩略图都已落盘
	d, _ := storagesvc.New(cfg, svc.db).Driver(res.Policy)
	if ok, _ := d.Exists(context.Background(), res.File.Path); !ok {
		t.Error("原图未落盘")
	}
	if ok, _ := d.Exists(context.Background(), storagesvc.ThumbKey(res.File.Surface, res.File.Hash)); !ok {
		t.Error("缩略图未落盘")
	}
}

func TestSaveInstantDedup(t *testing.T) {
	svc, u, _ := setup(t)
	dir := t.TempDir()
	tmp1 := pngFile(t, dir, 300, 200)
	res1, err := svc.Save(context.Background(), tmp1, "a.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	// 同用户同选项再传 → 复用 live image（图库不重复、不二次扣配额）
	tmp2 := pngFile(t, dir, 300, 200)
	res2, err := svc.Save(context.Background(), tmp2, "b.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant || !res2.Reused {
		t.Errorf("相同内容同用户应秒传复用: instant=%v reused=%v", res2.Instant, res2.Reused)
	}
	if res2.File.ID != res1.File.ID {
		t.Error("秒传应复用同一 file")
	}
	if res2.Image.Key != res1.Image.Key {
		t.Errorf("应返回同一 key: got %q want %q", res2.Image.Key, res1.Image.Key)
	}
	if res2.File.RefCount != 1 {
		t.Errorf("复用不应增加 ref_count, got %d", res2.File.RefCount)
	}
	if res2.Image.Name != "b.png" {
		t.Errorf("复用可刷新文件名, got %q", res2.Image.Name)
	}
	var cnt int64
	svc.db.Model(&model.Image{}).Count(&cnt)
	if cnt != 1 {
		t.Errorf("image 记录数 %d, want 1", cnt)
	}
	var got model.User
	svc.db.First(&got, u.ID)
	if got.UsedStorage != res1.File.Size {
		t.Errorf("复用不应二次扣配额: used=%d want %d", got.UsedStorage, res1.File.Size)
	}
}

func TestSaveInstantDifferentOptsCreatesNewImage(t *testing.T) {
	svc, u, _ := setup(t)
	alb := model.Album{UserID: u.ID, Name: "alb", Visibility: "public"}
	if err := svc.db.Create(&alb).Error; err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	res1, err := svc.Save(context.Background(), pngFile(t, dir, 80, 60), "a.png", u, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	res2, err := svc.Save(context.Background(), pngFile(t, dir, 80, 60), "b.png", u, Opts{Visibility: "public", AlbumID: &alb.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant || res2.Reused {
		t.Fatalf("选项不同应秒传新建: instant=%v reused=%v", res2.Instant, res2.Reused)
	}
	if res2.Image.Key == res1.Image.Key {
		t.Fatal("应新 key")
	}
	if res2.File.RefCount != 2 {
		t.Fatalf("ref_count=%d want 2", res2.File.RefCount)
	}
	var got model.User
	svc.db.First(&got, u.ID)
	if got.UsedStorage != res1.File.Size*2 {
		t.Fatalf("新建应扣配额: used=%d", got.UsedStorage)
	}
}

func TestSaveInstantCrossUserCreatesOwnImage(t *testing.T) {
	svc, a, _ := setup(t)
	b := &model.User{Username: "bob", Email: "b@img.li", GroupID: 1, Status: "active"}
	if err := svc.db.Create(b).Error; err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	resA, err := svc.Save(context.Background(), pngFile(t, dir, 90, 70), "a.png", a, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	resB, err := svc.Save(context.Background(), pngFile(t, dir, 90, 70), "b.png", b, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !resB.Instant || resB.Reused {
		t.Fatalf("跨用户应秒传不复用: instant=%v reused=%v", resB.Instant, resB.Reused)
	}
	if resB.Image.Key == resA.Image.Key {
		t.Fatal("不得返回他人 key")
	}
	if resB.File.ID != resA.File.ID || resB.File.RefCount != 2 {
		t.Fatalf("应共享 file: %+v", resB.File)
	}
	var ub model.User
	svc.db.First(&ub, b.ID)
	if ub.UsedStorage != resB.File.Size {
		t.Fatalf("B 应扣自己配额: %d", ub.UsedStorage)
	}
}

func TestSaveInstantAfterSoftDeleteCreatesNew(t *testing.T) {
	svc, u, _ := setup(t)
	dir := t.TempDir()
	res1, err := svc.Save(context.Background(), pngFile(t, dir, 50, 40), "a.png", u, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Delete(&model.Image{}, res1.Image.ID).Error; err != nil {
		t.Fatal(err)
	}
	res2, err := svc.Save(context.Background(), pngFile(t, dir, 50, 40), "a.png", u, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant || res2.Reused {
		t.Fatalf("软删后应新建非 reuse: instant=%v reused=%v", res2.Instant, res2.Reused)
	}
	if res2.Image.Key == res1.Image.Key {
		t.Fatal("不应复用已软删 key")
	}
}

func TestSaveInstantInheritsRejectedStatus(t *testing.T) {
	// 内容安全 P1：同 file 已 rejected 时秒传新 image 不得复活为 normal。
	svc, u, _ := setup(t)
	dir := t.TempDir()
	tmp1 := pngFile(t, dir, 300, 200)
	res1, err := svc.Save(context.Background(), tmp1, "a.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	score := 0.95
	if err := svc.db.Model(&model.Image{}).Where("id = ?", res1.Image.ID).
		Updates(map[string]any{"status": "rejected", "nsfw_score": score}).Error; err != nil {
		t.Fatal(err)
	}
	// 清掉首次上传入队的 moderate 任务，便于断言秒传不再入队
	svc.db.Where("type = ?", "moderate_image").Delete(&model.Task{})

	tmp2 := pngFile(t, dir, 300, 200)
	res2, err := svc.Save(context.Background(), tmp2, "b.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Fatal("应秒传")
	}
	if res2.Image.Status != "rejected" {
		t.Errorf("秒传 status = %q, want rejected", res2.Image.Status)
	}
	if res2.Image.NSFWScore == nil || *res2.Image.NSFWScore != score {
		t.Errorf("秒传 nsfw_score = %v, want %v", res2.Image.NSFWScore, score)
	}
	var pending int64
	svc.db.Model(&model.Task{}).Where("type = ? AND status = ?", "moderate_image", "pending").Count(&pending)
	if pending != 0 {
		t.Errorf("已继承 rejected 不应再入队 moderate_image, pending=%d", pending)
	}
}

func TestSaveInstantInheritsPendingAndScore(t *testing.T) {
	svc, u, _ := setup(t)
	dir := t.TempDir()
	tmp1 := pngFile(t, dir, 280, 180)
	res1, err := svc.Save(context.Background(), tmp1, "a.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	score := 0.88
	if err := svc.db.Model(&model.Image{}).Where("id = ?", res1.Image.ID).
		Updates(map[string]any{"status": "pending", "nsfw_score": score}).Error; err != nil {
		t.Fatal(err)
	}
	svc.db.Where("type = ?", "moderate_image").Delete(&model.Task{})

	tmp2 := pngFile(t, dir, 280, 180)
	res2, err := svc.Save(context.Background(), tmp2, "b.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Fatal("应秒传")
	}
	if res2.Image.Status != "pending" {
		t.Errorf("秒传 status = %q, want pending", res2.Image.Status)
	}
	if res2.Image.NSFWScore == nil || *res2.Image.NSFWScore != score {
		t.Errorf("秒传 nsfw_score = %v, want %v", res2.Image.NSFWScore, score)
	}
	var pending int64
	svc.db.Model(&model.Task{}).Where("type = ? AND status = ?", "moderate_image", "pending").Count(&pending)
	if pending != 0 {
		t.Errorf("已继承 pending 不应再入队, pending=%d", pending)
	}
}

func TestInheritModerationFromSeverity(t *testing.T) {
	// rejected 优先于 pending；分数取 max。
	svc, _, _ := setup(t)
	f := &model.File{Hash: "h1", StoragePolicyID: 1, Path: "p", Size: 1, MIME: "image/png", RefCount: 1}
	if err := svc.db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	s1, s2 := 0.5, 0.9
	svc.db.Create(&model.Image{Key: "k1aaaaaaaaaa", FileID: f.ID, Status: "pending", NSFWScore: &s1, Name: "a", Ext: "png", Visibility: "public"})
	svc.db.Create(&model.Image{Key: "k2aaaaaaaaaa", FileID: f.ID, Status: "rejected", NSFWScore: &s2, Name: "b", Ext: "png", Visibility: "public"})
	st, sc := inheritModerationFrom(svc.db, f.ID)
	if st != "rejected" {
		t.Errorf("status = %q, want rejected", st)
	}
	if sc == nil || *sc != 0.9 {
		t.Errorf("score = %v, want 0.9", sc)
	}
}

func TestSaveRejectsExtNotAllowed(t *testing.T) {
	svc, u, _ := setup(t)
	// 把用户组允许后缀改为只有 gif
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("allowed_exts", `["gif"]`)
	tmp := pngFile(t, t.TempDir(), 10, 10)
	if _, err := svc.Save(context.Background(), tmp, "x.png", u, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrExtNotAllowed) {
		t.Errorf("err=%v, want ErrExtNotAllowed", err)
	}
}

func TestSaveRejectsOverFileSize(t *testing.T) {
	svc, u, _ := setup(t)
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("max_file_size", 10) // 10 字节
	tmp := pngFile(t, t.TempDir(), 300, 200)
	if _, err := svc.Save(context.Background(), tmp, "x.png", u, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("err=%v, want ErrFileTooLarge", err)
	}
}

func TestSaveRejectsQuotaExceeded(t *testing.T) {
	svc, u, _ := setup(t)
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("storage_quota", 10)
	tmp := pngFile(t, t.TempDir(), 300, 200)
	if _, err := svc.Save(context.Background(), tmp, "x.png", u, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("err=%v, want ErrQuotaExceeded", err)
	}
}

func TestSaveRejectsNonImage(t *testing.T) {
	svc, u, _ := setup(t)
	p := filepath.Join(t.TempDir(), "x.png")
	os.WriteFile(p, []byte("not an image"), 0o644)
	if _, err := svc.Save(context.Background(), p, "x.png", u, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrInvalidImage) {
		t.Errorf("err=%v, want ErrInvalidImage", err)
	}
}

func TestSaveQuotaRefetchedNotStale(t *testing.T) {
	svc, u, _ := setup(t)
	// 配额只够一张图（用第一张的实际大小设限）
	dir := t.TempDir()
	res1, err := svc.Save(context.Background(), pngFile(t, dir, 300, 200), "a.png", u, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	// 把配额收紧到刚好等于已用量：复用同一个 u（其 UsedStorage 仍是旧值 0）
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("storage_quota", res1.File.Size)
	// 第二张不同内容（非秒传），应因配额被拒——即使传入的 u.UsedStorage 是陈旧的 0
	if _, err := svc.Save(context.Background(), pngFile(t, dir, 301, 200), "b.png", u, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("应基于最新已用量拒绝, got %v", err)
	}
}

// TestAddUsedStorageAtomicQuota 事务内条件更新：已满配额时 0 行 → ErrQuotaExceeded，used 不超标。
func TestAddUsedStorageAtomicQuota(t *testing.T) {
	svc, u, _ := setup(t)
	const quota int64 = 1000
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("storage_quota", quota)
	svc.db.Model(&model.User{}).Where("id = ?", u.ID).Update("used_storage", 900)
	err := svc.db.Transaction(func(tx *gorm.DB) error {
		return addUsedStorage(tx, u.ID, 200, quota) // 900+200 > 1000
	})
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("err=%v want ErrQuotaExceeded", err)
	}
	var got model.User
	svc.db.First(&got, u.ID)
	if got.UsedStorage != 900 {
		t.Fatalf("失败路径不应改 used_storage, got %d", got.UsedStorage)
	}
	// 刚好顶满：900+100=1000 放行
	if err := svc.db.Transaction(func(tx *gorm.DB) error {
		return addUsedStorage(tx, u.ID, 100, quota)
	}); err != nil {
		t.Fatal(err)
	}
	svc.db.First(&got, u.ID)
	if got.UsedStorage != 1000 {
		t.Fatalf("used=%d want 1000", got.UsedStorage)
	}
}

// TestCommitInstantFailsWhenFilePurged 秒传时 file 行已被 purge 删掉 → 回滚，不建孤儿 image。
func TestCommitInstantFailsWhenFilePurged(t *testing.T) {
	svc, u, _ := setup(t)
	f := &model.File{Hash: "deadbeef", StoragePolicyID: 1, Path: "x", Size: 10, MIME: "image/png", RefCount: 1}
	if err := svc.db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	// 模拟 purge 已删 file 行
	svc.db.Delete(f)
	_, err := svc.commitInstant(u, f, "a.png", "png", "public", "", 10, nil, nil, 0, "", 0)
	if err == nil {
		t.Fatal("file 已删应失败")
	}
	var n int64
	svc.db.Model(&model.Image{}).Where("file_id = ?", f.ID).Count(&n)
	if n != 0 {
		t.Fatalf("不应留下孤儿 image, n=%d", n)
	}
}

func TestSaveNilUserGuest(t *testing.T) {
	svc, _, _ := setup(t)
	if _, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 10, 10), "x.png", nil, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrGuestNotSupported) {
		t.Error("nil user 应返回 ErrGuestNotSupported")
	}
}

func TestSaveGuestDisabledRejectsNilUser(t *testing.T) {
	svc, _, _ := setup(t)
	// guest_upload_enabled 播种默认 false，未显式开启
	if _, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 10, 10), "x.png", nil, Opts{Visibility: "public"}, "9.9.9.9"); !errors.Is(err, ErrGuestNotSupported) {
		t.Errorf("err=%v, want ErrGuestNotSupported", err)
	}
}

func TestSaveGuestEnabledSucceeds(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	tmp := pngFile(t, t.TempDir(), 300, 200)
	res, err := svc.Save(context.Background(), tmp, "guest.png", nil, Opts{Visibility: "public"}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.UserID != nil {
		t.Errorf("游客图 UserID 应为 nil, got %v", *res.Image.UserID)
	}
	if res.Image.Status != "normal" {
		t.Errorf("status = %q, want normal", res.Image.Status)
	}
	if res.Instant {
		t.Error("首次上传不应是秒传")
	}
}

func TestSaveGuestRejectsOverGuestGroupFileSize(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	// 游客组播种 max_file_size = 5MB；改小到 10 字节以便测试
	if err := svc.db.Model(&model.UserGroup{}).Where("is_guest = ?", true).
		Update("max_file_size", 10).Error; err != nil {
		t.Fatal(err)
	}
	tmp := pngFile(t, t.TempDir(), 300, 200)
	if _, err := svc.Save(context.Background(), tmp, "guest.png", nil, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("err=%v, want ErrFileTooLarge", err)
	}
}

func TestSaveGuestRejectsExtNotAllowed(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&model.UserGroup{}).Where("is_guest = ?", true).
		Update("allowed_exts", `["gif"]`).Error; err != nil {
		t.Fatal(err)
	}
	tmp := pngFile(t, t.TempDir(), 10, 10)
	if _, err := svc.Save(context.Background(), tmp, "guest.png", nil, Opts{Visibility: "public"}, ""); !errors.Is(err, ErrExtNotAllowed) {
		t.Errorf("err=%v, want ErrExtNotAllowed", err)
	}
}

func TestSaveGuestInstantDedup(t *testing.T) {
	svc, u, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	// 先由登录用户上传一份内容，建立既有 file
	tmp1 := pngFile(t, dir, 300, 200)
	res1, err := svc.Save(context.Background(), tmp1, "a.png", u, Opts{Visibility: "public"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	// 游客上传相同内容 → 应秒传命中，UserID 仍为 nil，且不累加 used_storage
	tmp2 := pngFile(t, dir, 300, 200)
	res2, err := svc.Save(context.Background(), tmp2, "b.png", nil, Opts{Visibility: "public"}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Error("相同内容游客上传应秒传")
	}
	if res2.Image.UserID != nil {
		t.Errorf("游客秒传 UserID 应为 nil, got %v", *res2.Image.UserID)
	}
	if res2.File.ID != res1.File.ID || res2.File.RefCount != 2 {
		t.Errorf("秒传应复用同一 file 且 ref_count=2: %+v", res2.File)
	}
	var got model.User
	svc.db.First(&got, u.ID)
	if got.UsedStorage != res1.File.Size {
		t.Errorf("游客秒传不应累加登录用户 used_storage: used=%d want %d", got.UsedStorage, res1.File.Size)
	}
}

// --- 复审修复③：游客图必须恒 public，无视调用方传入的 visibility ---
// （direct API 调用方可传 visibility=private；owner 校验永远匹配不到 NULL
// user_id，一旦落库为 private 就是谁也看不到的死记录）。

// TestSaveGuestForcesPublicVisibilityOnNewFile 新文件路径：游客传 visibility=private
// 应被强制改写为 public。
func TestSaveGuestForcesPublicVisibilityOnNewFile(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	tmp := pngFile(t, t.TempDir(), 300, 200)
	res, err := svc.Save(context.Background(), tmp, "guest.png", nil, Opts{Visibility: "private"}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.Visibility != "public" {
		t.Errorf("游客图 visibility = %q, want public（不应尊重 private 请求）", res.Image.Visibility)
	}
}

// TestSaveGuestForcesPublicVisibilityOnInstantDedup 秒传路径（commitInstant）同样要强制。
func TestSaveGuestForcesPublicVisibilityOnInstantDedup(t *testing.T) {
	svc, u, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tmp1 := pngFile(t, dir, 300, 200)
	if _, err := svc.Save(context.Background(), tmp1, "a.png", u, Opts{Visibility: "public"}, "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	tmp2 := pngFile(t, dir, 300, 200)
	res2, err := svc.Save(context.Background(), tmp2, "b.png", nil, Opts{Visibility: "private"}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Fatal("相同内容游客上传应秒传")
	}
	if res2.Image.Visibility != "public" {
		t.Errorf("游客秒传 visibility = %q, want public（不应尊重 private 请求）", res2.Image.Visibility)
	}
}

// --- 复审修复④：零值 ID 的 *model.User 应被规整为游客（u==nil），判定口径统一 ---

// TestSaveZeroIDUserTreatedAsGuest 一个非 nil 但 ID==0 的 *model.User（如调用方
// 误传未落库的零值结构体）应被当作游客处理：走游客组配额/限制、UserID 落 NULL、
// visibility 强制 public，而不是尝试用 id=0 去查用户组/累加 used_storage。
func TestSaveZeroIDUserTreatedAsGuest(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	zeroUser := &model.User{} // ID == 0，未落库
	tmp := pngFile(t, t.TempDir(), 300, 200)
	res, err := svc.Save(context.Background(), tmp, "zero.png", zeroUser, Opts{Visibility: "private"}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.UserID != nil {
		t.Errorf("零 ID 用户应按游客处理，UserID 应为 nil, got %v", *res.Image.UserID)
	}
	if res.Image.Visibility != "public" {
		t.Errorf("零 ID 用户应按游客处理，visibility = %q, want public", res.Image.Visibility)
	}
}

// TestSaveExpiresAtPersists 上传 Opts.ExpiresAt 非 nil 时落库；nil 时保持永久。
func TestSaveExpiresAtPersists(t *testing.T) {
	svc, u, _ := setup(t)
	exp := time.Now().Add(24 * time.Hour)
	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 40, 30), "e1.png", u,
		Opts{Visibility: "public", ExpiresAt: &exp}, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.ExpiresAt == nil {
		t.Fatal("ExpiresAt 应写入")
	}
	delta := res.Image.ExpiresAt.Sub(exp)
	if delta < -2*time.Second || delta > 2*time.Second {
		t.Errorf("ExpiresAt=%v want ~%v", res.Image.ExpiresAt, exp)
	}
	var got model.Image
	if err := svc.db.First(&got, "key = ?", res.Image.Key).Error; err != nil {
		t.Fatal(err)
	}
	if got.ExpiresAt == nil {
		t.Fatal("库中 ExpiresAt 应非 nil")
	}

	res2, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 41, 30), "e2.png", u,
		Opts{Visibility: "public"}, "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if res2.Image.ExpiresAt != nil {
		t.Errorf("缺省 ExpiresAt 应为 nil, got %v", res2.Image.ExpiresAt)
	}
}

// TestSaveGuestExpiresAt 游客上传也可设过期。
func TestSaveGuestExpiresAt(t *testing.T) {
	svc, _, _ := setup(t)
	if err := settings.New(svc.db).Set(model.SettingGuestUpload, true); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour)
	res, err := svc.Save(context.Background(), pngFile(t, t.TempDir(), 20, 20), "gexp.png", nil,
		Opts{Visibility: "public", ExpiresAt: &exp}, "9.9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.ExpiresAt == nil {
		t.Fatal("游客图应带 ExpiresAt")
	}
}

// TestSaveInstantExpiresAt 秒传路径同样写入 ExpiresAt。
func TestSaveInstantExpiresAt(t *testing.T) {
	svc, u, _ := setup(t)
	dir := t.TempDir()
	if _, err := svc.Save(context.Background(), pngFile(t, dir, 50, 40), "a.png", u, Opts{Visibility: "public"}, ""); err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(2 * time.Hour)
	res2, err := svc.Save(context.Background(), pngFile(t, dir, 50, 40), "b.png", u,
		Opts{Visibility: "public", ExpiresAt: &exp}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !res2.Instant {
		t.Fatal("应秒传")
	}
	if res2.Image.ExpiresAt == nil {
		t.Fatal("秒传 ExpiresAt 应写入")
	}
}
