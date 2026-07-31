package storagesvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/storage"
)

func TestMigrateFilesLocalToLocal(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	// 播种已有 id=1 local；再建目标 local 策略
	to := model.StoragePolicy{
		Name: "local-b", Driver: "local",
		Config: map[string]string{"root": "uploads-b"}, Enabled: true,
		PathTemplate: "{Y}/{m}/{d}/{uniqid}.{ext}",
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	var from model.StoragePolicy
	if err := db.First(&from, 1).Error; err != nil {
		t.Fatal(err)
	}

	r := New(&config.Config{DataDir: dir}, db)
	src, err := r.Driver(&from)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("migrate-payload-png-bytes")
	path := "2026/07/20/abc123xyz.png"
	if err := src.Put(context.Background(), path, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	// 假缩略图
	hash := "deadbeefcafebabe"
	if err := src.Put(context.Background(), ThumbKey(model.SurfacePublic, hash), bytes.NewReader([]byte("jpgthumb"))); err != nil {
		t.Fatal(err)
	}
	f := model.File{
		Hash: hash, StoragePolicyID: from.ID, Path: path,
		Size: int64(len(payload)), MIME: "image/png", RefCount: 1,
	}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}

	// dry-run
	dry, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: from.ID, ToPolicyID: to.ID, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Scanned != 1 || dry.Copied != 1 {
		t.Fatalf("dry-run = %+v", dry)
	}
	// 库仍指向 from
	var check model.File
	db.First(&check, f.ID)
	if check.StoragePolicyID != from.ID {
		t.Fatal("dry-run 不应改库")
	}

	res, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: from.ID, ToPolicyID: to.ID, DeleteSource: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Copied != 1 || res.Failed != 0 {
		t.Fatalf("migrate = %+v", res)
	}
	db.First(&check, f.ID)
	if check.StoragePolicyID != to.ID {
		t.Fatalf("policy_id=%d want %d", check.StoragePolicyID, to.ID)
	}
	dst, _ := r.Driver(&to)
	rc, err := dst.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch %q", got)
	}
	// 源已删
	if _, err := src.Open(context.Background(), path); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("源应已删, err=%v", err)
	}
	// 缩略图在目标
	if ok, _ := dst.Exists(context.Background(), ThumbKey(model.SurfacePublic, hash)); !ok {
		t.Error("目标应有 jpg 缩略图")
	}
}

func TestMigrateFilesRejectsSamePolicy(t *testing.T) {
	db := model.TestDB(t)
	r := New(&config.Config{DataDir: t.TempDir()}, db)
	_, err := r.MigrateFiles(context.Background(), db, MigrateOpts{FromPolicyID: 1, ToPolicyID: 1})
	if err == nil {
		t.Fatal("same policy should fail")
	}
}

func TestMigrateFilesRejectsDisabledTarget(t *testing.T) {
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "disabled-to", Driver: "local",
		Config: map[string]string{"root": "off"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	// GORM default:true 对 bool 零值会当默认 true；显式关掉。
	if err := db.Model(&to).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: t.TempDir()}, db)
	_, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID,
	})
	if !errors.Is(err, ErrMigrateTargetDisabled) {
		t.Fatalf("want ErrMigrateTargetDisabled, got %v", err)
	}
}

func TestMigrateFilesBusySameFrom(t *testing.T) {
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "busy-to", Driver: "local",
		Config: map[string]string{"root": "busy-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: t.TempDir()}, db)
	if err := r.tryBeginMigrate(1); err != nil {
		t.Fatal(err)
	}
	defer r.endMigrate(1)
	if !r.MigrateActive(1) {
		t.Fatal("expected active after tryBeginMigrate")
	}
	_, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID, DryRun: true,
	})
	if !errors.Is(err, ErrMigrateBusy) {
		t.Fatalf("want ErrMigrateBusy, got %v", err)
	}
}

func TestRedactStoragePath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"a.png", "a.png"},
		{"07/a.png", "07/a.png"},               // ≤2 段原样
		{"2026/07/a.png", "…/07/a.png"},         // 3 段脱敏
		{"uploads/private/2026/07/a.png", "…/07/a.png"},
		{`C:\data\uploads\x\y\z.png`, "…/y/z.png"},
	}
	for _, c := range cases {
		if got := RedactStoragePath(c.in); got != c.want {
			t.Errorf("RedactStoragePath(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestMigrateResultProgressNoSecretsShape(t *testing.T) {
	res := MigrateResult{
		Scanned: 2, Copied: 1, Skipped: 1, Failed: 0,
		Errors:      []string{"file#1 put …/a.png: boom"},
		SamplePaths: []string{"…/07/a.png"},
	}
	p := res.Progress()
	if p.Scanned != 2 || p.Copied != 1 || p.Skipped != 1 {
		t.Fatalf("progress counts: %+v", p)
	}
	// 调用方改切片不应影响原结果
	p.SamplePaths[0] = "mutated"
	if res.SamplePaths[0] != "…/07/a.png" {
		t.Fatal("Progress should copy SamplePaths")
	}
}

func TestMigrateFilesSizeMismatchFailsWithoutPolicyFlip(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "size-to", Driver: "local", Config: map[string]string{"root": "size-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	var from model.StoragePolicy
	db.First(&from, 1)
	r := New(&config.Config{DataDir: dir}, db)
	src, _ := r.Driver(&from)
	path := "size/bad.png"
	// object is short
	if err := src.Put(context.Background(), path, bytes.NewReader([]byte("short"))); err != nil {
		t.Fatal(err)
	}
	f := model.File{
		Hash: "sizemismatch", StoragePolicyID: from.ID, Path: path,
		Size: 9999, MIME: "image/png", RefCount: 1, // claim larger size
	}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	res, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: from.ID, ToPolicyID: to.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Copied != 0 {
		t.Fatalf("want failed=1 copied=0 got %+v", res)
	}
	var check model.File
	db.First(&check, f.ID)
	if check.StoragePolicyID != from.ID {
		t.Fatalf("policy should stay on from, got %d", check.StoragePolicyID)
	}
}

func TestMigrateFilesUserFilter(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "uf-to", Driver: "local", Config: map[string]string{"root": "uf-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	u1 := model.User{Username: "mu1", Email: "mu1@t.li", PasswordHash: "x", GroupID: 1}
	u2 := model.User{Username: "mu2", Email: "mu2@t.li", PasswordHash: "x", GroupID: 1}
	if err := db.Create(&u1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&u2).Error; err != nil {
		t.Fatal(err)
	}
	var from model.StoragePolicy
	if err := db.First(&from, 1).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: dir}, db)
	src, err := r.Driver(&from)
	if err != nil {
		t.Fatal(err)
	}
	for i, uid := range []uint64{u1.ID, u2.ID} {
		p := fmt.Sprintf("uf/%d.png", i)
		if err := src.Put(context.Background(), p, bytes.NewReader([]byte("xx"))); err != nil {
			t.Fatal(err)
		}
		f := model.File{Hash: fmt.Sprintf("ufhash%02d", i), StoragePolicyID: 1, Path: p, Size: 2, MIME: "image/png", RefCount: 1}
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		id := uid
		img := model.Image{
			Key: fmt.Sprintf("ukey%08dxx", i), UserID: &id, FileID: f.ID,
			Name: "n.png", Ext: "png", Status: "normal",
		}
		if err := db.Create(&img).Error; err != nil {
			t.Fatal(err)
		}
	}
	uid := u1.ID
	res, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID, UserID: &uid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 1 || res.Copied != 1 {
		t.Fatalf("user filter: %+v", res)
	}
}

func TestMigrateFilesMissingSourceSkipped(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "b", Driver: "local", Config: map[string]string{"root": "b"}, Enabled: true,
	}
	db.Create(&to)
	f := model.File{
		Hash: "h1", StoragePolicyID: 1, Path: "no/such/file.png",
		Size: 1, MIME: "image/png", RefCount: 1,
	}
	db.Create(&f)
	r := New(&config.Config{DataDir: dir}, db)
	res, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skipped != 1 || res.Copied != 0 {
		t.Fatalf("want skipped=1, got %+v", res)
	}
}
