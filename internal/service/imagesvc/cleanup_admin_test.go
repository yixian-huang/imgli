package imagesvc

import (
	"context"
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
	"github.com/yixian-huang/imgli/internal/task"
)

func TestPreviewAndRunExpiredCleanup(t *testing.T) {
	db := model.TestDB(t)
	dir := t.TempDir()
	res := storagesvc.New(&config.Config{DataDir: dir}, db)
	runner := task.New(db, 1)
	s := New(db, res, runner)

	past := time.Now().Add(-time.Hour)
	f := model.File{Hash: "clehash1", StoragePolicyID: 1, Path: "c/1.png", Size: 1, MIME: "image/png", RefCount: 1}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	img := model.Image{
		Key: "cleankey00001", FileID: f.ID, Name: "x.png", Ext: "png",
		Status: "normal", ExpiresAt: &past,
	}
	if err := db.Create(&img).Error; err != nil {
		t.Fatal(err)
	}

	pre, err := s.PreviewCleanup([]string{CleanupExpired}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pre) != 1 || pre[0].Count < 1 {
		t.Fatalf("preview %+v", pre)
	}

	if _, err := s.RunCleanup(context.Background(), []string{CleanupExpired}, 10, false); err == nil {
		t.Fatal("want confirm required")
	}
	resu, err := s.RunCleanup(context.Background(), []string{CleanupExpired}, 10, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(resu) != 1 || resu[0].Deleted < 1 {
		t.Fatalf("run %+v", resu)
	}
}
