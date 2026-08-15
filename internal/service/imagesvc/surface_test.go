package imagesvc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/storage"
)

// setupSurface 构造带本地存储的 imagesvc + 一个公开 File 及其磁盘对象。
func setupSurface(t *testing.T) (*Service, *storagesvc.Resolver, *model.StoragePolicy, *model.File) {
	t.Helper()
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir(), BaseURL: "https://img.li"}
	res := storagesvc.New(cfg, db)
	svc := New(db, res, nil)

	var policy model.StoragePolicy
	if err := db.First(&policy, "driver = ?", "local").Error; err != nil {
		t.Fatalf("取本地策略失败(Seed 应建): %v", err)
	}
	d, err := res.Driver(&policy)
	if err != nil {
		t.Fatal(err)
	}
	// 公开 File + 其对象(含缩略图)
	f := &model.File{Hash: "hh1", Surface: model.SurfacePublic, StoragePolicyID: policy.ID,
		Path: "public/2026/07/a.png", Size: 5, MIME: "image/png", Width: 1, Height: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Put(context.Background(), f.Path, bytes.NewReader([]byte("bytes"))); err != nil {
		t.Fatal(err)
	}
	// 公开缩略图(jpg 现行世代)
	if err := d.Put(context.Background(), storagesvc.ThumbKey(model.SurfacePublic, f.Hash), bytes.NewReader([]byte("thumb"))); err != nil {
		t.Fatal(err)
	}
	return svc, res, &policy, f
}

// TestResolveFileForSurfaceHit 命中同 surface File 直接返回(不复制)。
func TestResolveFileForSurfaceHit(t *testing.T) {
	svc, _, policy, f := setupSurface(t)
	got, err := svc.resolveFileForSurface(policy, f, model.SurfacePublic)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != f.ID {
		t.Errorf("命中应返回既有 File id=%d, got %d", f.ID, got.ID)
	}
}

// TestResolveFileForSurfaceCopiesOnMiss 未命中 → 复制对象+缩略图到目标 surface,建 ref-0 File。
func TestResolveFileForSurfaceCopiesOnMiss(t *testing.T) {
	svc, res, policy, f := setupSurface(t)
	got, err := svc.resolveFileForSurface(policy, f, model.SurfacePrivate)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == f.ID {
		t.Fatal("跨 surface 应新建独立 File,非复用")
	}
	if got.Surface != model.SurfacePrivate {
		t.Errorf("新 File surface = %q, want private", got.Surface)
	}
	if got.RefCount != 0 {
		t.Errorf("新 File ref_count = %d, want 0(调用方负责 ++)", got.RefCount)
	}
	if got.Hash != f.Hash {
		t.Errorf("新 File hash 应同源 %q, got %q", f.Hash, got.Hash)
	}
	if got.Path[:8] != "private/" {
		t.Errorf("新 File path 应带 private/ 前缀, got %q", got.Path)
	}
	// 对象已复制到新键
	d, _ := res.Driver(policy)
	if ok, _ := d.Exists(context.Background(), got.Path); !ok {
		t.Error("原图对象应已复制到新键")
	}
	// 缩略图已复制到 private surface 键
	if ok, _ := d.Exists(context.Background(), storagesvc.ThumbKey(model.SurfacePrivate, f.Hash)); !ok {
		t.Error("缩略图应已复制到 private surface 键")
	}
}

// lenAssertDriver 包装真实驱动,断言 Put 收到的 body 带 Len()(即已知 Content-Length,
// 非 chunked)——守护 copyObjectKey 对 s3 的 MissingContentLength 回归。
type lenAssertDriver struct {
	storage.Driver
	t         *testing.T
	putHadLen bool
}

func (w *lenAssertDriver) Put(ctx context.Context, key string, r io.Reader) error {
	if _, ok := r.(interface{ Len() int }); ok {
		w.putHadLen = true
	} else {
		w.t.Errorf("Put body 应带 Len()(Content-Length),否则 s3 会 chunked 被拒; key=%s", key)
	}
	return w.Driver.Put(ctx, key, r)
}

func TestCopyObjectKeyPutsWithLength(t *testing.T) {
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir()}
	res := storagesvc.New(cfg, db)
	var pol model.StoragePolicy
	db.First(&pol, 1)
	base, err := res.Driver(&pol)
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Put(context.Background(), "public/src.png", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatal(err)
	}
	w := &lenAssertDriver{Driver: base, t: t}
	if err := copyObjectKey(context.Background(), w, "public/src.png", "private/dst.png"); err != nil {
		t.Fatal(err)
	}
	if !w.putHadLen {
		t.Error("copyObjectKey 未以带 Len 的 body 调 Put")
	}
	// 目标对象内容正确
	rc, err := base.Open(context.Background(), "private/dst.png")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if string(got) != "hello" {
		t.Errorf("复制内容 = %q, want hello", got)
	}
}

// TestUpdateVisibilityRehomesSoleRef 独占引用切换 public→private:img 重挂私密 File,
// 对象已复制,旧公开 File 归零被删,旧对象投递异步删除(run==nil 时不投,但 File 行已删)。
func TestUpdateVisibilityRehomesSoleRef(t *testing.T) {
	svc, res, policy, f := setupSurface(t)
	// 建属主 + 指向该公开 File 的公开图
	u := &model.User{Username: "bob", Email: "b@img.li", GroupID: 1, Status: "active"}
	if err := svc.db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "k1surface001", UserID: &u.ID, FileID: f.ID, Name: "a", Ext: "png",
		Visibility: model.SurfacePublic, Status: "normal"}
	if err := svc.db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	priv := model.SurfacePrivate
	row, err := svc.Update(u.ID, img.Key, UpdatePatch{Visibility: &priv})
	if err != nil {
		t.Fatal(err)
	}
	if row.Img.Visibility != model.SurfacePrivate {
		t.Errorf("可见性应为 private, got %q", row.Img.Visibility)
	}
	// img 已重挂到新私密 File
	var got model.Image
	svc.db.First(&got, img.ID)
	if got.FileID == f.ID {
		t.Error("img 应重挂到新私密 File,FileID 不应仍指旧公开 File")
	}
	var newFile model.File
	if err := svc.db.First(&newFile, got.FileID).Error; err != nil {
		t.Fatal(err)
	}
	if newFile.Surface != model.SurfacePrivate || newFile.RefCount != 1 {
		t.Errorf("新 File surface=%q ref=%d, want private/1", newFile.Surface, newFile.RefCount)
	}
	// 新私密对象存在
	d, _ := res.Driver(policy)
	if ok, _ := d.Exists(context.Background(), newFile.Path); !ok {
		t.Error("新私密对象应存在")
	}
	// 旧公开 File 独占引用 → 归零被删
	var cnt int64
	svc.db.Model(&model.File{}).Where("id = ?", f.ID).Count(&cnt)
	if cnt != 0 {
		t.Error("旧公开 File 独占引用切换后应被删(ref 归零)")
	}
}

// TestUpdateVisibilitySharedOldFileKept 共享旧 File(ref>1)切换:旧 File ref-- 不归零,保留。
func TestUpdateVisibilitySharedOldFileKept(t *testing.T) {
	svc, _, _, f := setupSurface(t)
	// 把旧 File 置为被两图共享
	svc.db.Model(&model.File{}).Where("id = ?", f.ID).Update("ref_count", 2)
	u := &model.User{Username: "carol", Email: "c@img.li", GroupID: 1, Status: "active"}
	svc.db.Create(u)
	img := &model.Image{Key: "k1surface002", UserID: &u.ID, FileID: f.ID, Name: "a", Ext: "png",
		Visibility: model.SurfacePublic, Status: "normal"}
	svc.db.Create(img)

	priv := model.SurfacePrivate
	if _, err := svc.Update(u.ID, img.Key, UpdatePatch{Visibility: &priv}); err != nil {
		t.Fatal(err)
	}
	// 旧公开 File 仍在,ref 2→1
	var old model.File
	if err := svc.db.First(&old, f.ID).Error; err != nil {
		t.Fatal("共享旧 File 不应被删")
	}
	if old.RefCount != 1 {
		t.Errorf("共享旧 File ref 应 2→1, got %d", old.RefCount)
	}
}

// TestUpdateRehomesWhenAlreadyPrivate 可见性已是 private、但 File 仍在 public/（v8 只改列）
// 时，再 PATCH private 必须重挂，不能当 no-op。
func TestUpdateRehomesWhenAlreadyPrivate(t *testing.T) {
	svc, res, policy, f := setupSurface(t)
	u := &model.User{Username: "erin", Email: "e@img.li", GroupID: 1, Status: "active"}
	if err := svc.db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "k1surface010", UserID: &u.ID, FileID: f.ID, Name: "a", Ext: "png",
		Visibility: model.SurfacePrivate, Status: "normal"}
	if err := svc.db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	priv := model.SurfacePrivate
	row, err := svc.Update(u.ID, img.Key, UpdatePatch{Visibility: &priv})
	if err != nil {
		t.Fatal(err)
	}
	if row.File.Surface != model.SurfacePrivate {
		t.Fatalf("已 private 但 surface 不匹配时应重挂, file.surface=%q", row.File.Surface)
	}
	if row.File.ID == f.ID {
		t.Fatal("应重挂到新私密 File, 不应仍指公开 File")
	}
	d, _ := res.Driver(policy)
	if ok, _ := d.Exists(context.Background(), row.File.Path); !ok {
		t.Error("新私密对象应存在")
	}
}

// TestRehomeMismatchedSurfaces 存量扫描：private 图挂在 public File 上 → 重挂。
func TestRehomeMismatchedSurfaces(t *testing.T) {
	svc, res, policy, f := setupSurface(t)
	u := &model.User{Username: "frank", Email: "f@img.li", GroupID: 1, Status: "active"}
	if err := svc.db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "k1surface011", UserID: &u.ID, FileID: f.ID, Name: "a", Ext: "png",
		Visibility: model.SurfacePrivate, Status: "normal"}
	if err := svc.db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	n, err := svc.RehomeMismatchedSurfaces()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("应重挂 1 张, got %d", n)
	}
	var got model.Image
	if err := svc.db.First(&got, img.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.FileID == f.ID {
		t.Fatal("扫描重挂后 FileID 不应仍指公开 File")
	}
	var nf model.File
	if err := svc.db.First(&nf, got.FileID).Error; err != nil {
		t.Fatal(err)
	}
	if nf.Surface != model.SurfacePrivate {
		t.Fatalf("新 File surface=%q, want private", nf.Surface)
	}
	d, _ := res.Driver(policy)
	if ok, _ := d.Exists(context.Background(), nf.Path); !ok {
		t.Error("新私密对象应存在")
	}
}

// TestUpdateVisibilityConcurrentRehomeNoDoubleRef 模拟并发已重挂:img 已被改指到私密 File,
// 本次 Update(→private)应识别冲突、不重复调 ref、返回当前态。
func TestUpdateVisibilityConcurrentRehomeNoDoubleRef(t *testing.T) {
	svc, res, policy, f := setupSurface(t)
	u := &model.User{Username: "dan", Email: "d@img.li", GroupID: 1, Status: "active"}
	svc.db.Create(u)
	img := &model.Image{Key: "k1surface003", UserID: &u.ID, FileID: f.ID, Name: "a", Ext: "png",
		Visibility: model.SurfacePublic, Status: "normal"}
	svc.db.Create(img)
	// 模拟"另一并发请求已重挂":先建目标私密 File 并把 img 改指它(ref=1)
	d, _ := res.Driver(policy)
	_ = d.Put(context.Background(), "private/pre.png", bytes.NewReader([]byte("x")))
	pre := &model.File{Hash: f.Hash, Surface: model.SurfacePrivate, StoragePolicyID: policy.ID,
		Path: "private/pre.png", Size: 5, MIME: "image/png", RefCount: 1}
	svc.db.Create(pre)
	svc.db.Model(&model.Image{}).Where("id = ?", img.ID).Update("file_id", pre.ID)
	// 旧公开 File 此时应已无引用(但测试里 ref 仍 1,构造态);记录 pre.ref 以断言不被再 ++
	priv := model.SurfacePrivate
	if _, err := svc.Update(u.ID, img.Key, UpdatePatch{Visibility: &priv}); err != nil {
		t.Fatal(err)
	}
	// pre File 的 ref 不应被本次再 ++(仍为 1)——CAS 命中冲突回滚了 ref 变更
	var got model.File
	svc.db.First(&got, pre.ID)
	if got.RefCount != 1 {
		t.Errorf("并发冲突下 pre File ref 不应被重复 ++,got %d want 1", got.RefCount)
	}
}

// putFailDriver 包装真实驱动,Put 恒失败——验证 copyThumbAcross 源存在时复制失败会上抛。
type putFailDriver struct {
	storage.Driver
}

func (w *putFailDriver) Put(ctx context.Context, key string, r io.Reader) error {
	return errors.New("put failed")
}

func TestCopyThumbAcrossAbortsOnPutError(t *testing.T) {
	db := model.TestDB(t)
	cfg := &config.Config{DataDir: t.TempDir()}
	res := storagesvc.New(cfg, db)
	var pol model.StoragePolicy
	db.First(&pol, 1)
	base, _ := res.Driver(&pol)
	svc := New(db, res, nil)
	// 源缩略图存在
	_ = base.Put(context.Background(), storagesvc.ThumbKey(model.SurfacePublic, "hx"), bytes.NewReader([]byte("t")))
	w := &putFailDriver{Driver: base}
	if err := svc.copyThumbAcross(context.Background(), w, model.SurfacePublic, model.SurfacePrivate, "hx"); err == nil {
		t.Error("源存在但 Put 失败应返回错误(中止重挂),而非静默容忍")
	}
	// 源都不存在时容忍(返回 nil)
	if err := svc.copyThumbAcross(context.Background(), base, model.SurfacePublic, model.SurfacePrivate, "no-such-hash"); err != nil {
		t.Errorf("源都不存在应容忍返回 nil, got %v", err)
	}
}

// TestResolveFileForSurfaceCleansOrphanOnThumbFail 缩略图复制失败时,已复制到 newPath 的
// 原图对象应被补偿删除,不留孤儿(codex 复审 major:该孤儿对象无对应 File 行,ref-0 清理
// 只扫 File 行,永远发现不了,泄漏是永久性的)。
//
// 用本地驱动的真实 MkdirAll 语义制造缩略图失败,不借助包装/mock 驱动:先在
// "private/.thumbs" 键 Put 一个普通文件,占住该路径段;随后缩略图要写入
// "private/.thumbs/g1/<hash>.jpg" 时,os.MkdirAll 会因该路径段已是文件(非目录)而报错,
// 从而在真实调用 resolveFileForSurface 时触发缩略图失败分支。
func TestResolveFileForSurfaceCleansOrphanOnThumbFail(t *testing.T) {
	db := model.TestDB(t)
	dataDir := t.TempDir()
	cfg := &config.Config{DataDir: dataDir}
	res := storagesvc.New(cfg, db)
	svc := New(db, res, nil)

	var policy model.StoragePolicy
	if err := db.First(&policy, "driver = ?", "local").Error; err != nil {
		t.Fatalf("取本地策略失败(Seed 应建): %v", err)
	}
	d, err := res.Driver(&policy)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 源:公开原图 + 公开缩略图(现行世代 jpg)
	f := &model.File{Hash: "orphhash1", Surface: model.SurfacePublic, StoragePolicyID: policy.ID,
		Path: "public/2026/07/24/src.png", Size: 5, MIME: "image/png", RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Put(ctx, f.Path, bytes.NewReader([]byte("bytes"))); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(ctx, storagesvc.ThumbKey(model.SurfacePublic, f.Hash), bytes.NewReader([]byte("thumb"))); err != nil {
		t.Fatal(err)
	}

	// 挡路:private/.thumbs 键先放一个普通文件,后续缩略图 Put 到 private/.thumbs/g1/... 时
	// os.MkdirAll(private/.thumbs/g1) 因该段已是文件而失败。
	if err := d.Put(ctx, "private/.thumbs", bytes.NewReader([]byte("blocker"))); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.resolveFileForSurface(&policy, f, model.SurfacePrivate); err == nil {
		t.Fatal("缩略图 Put 失败应导致 resolveFileForSurface 返回错误")
	}

	// 原图对象不应留孤儿:private/ 下当天日期目录是本次复制唯一可能写入的位置,
	// 补偿删除后应为空(或目录本不存在)。
	dateDir := filepath.Join(dataDir, "uploads", "private",
		time.Now().Format("2006"), time.Now().Format("01"), time.Now().Format("02"))
	entries, statErr := os.ReadDir(dateDir)
	if statErr != nil && !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}
	if len(entries) != 0 {
		t.Errorf("缩略图失败后原图对象应被补偿删除,不留孤儿;但 %s 下仍有残留: %v", dateDir, entries)
	}
}
