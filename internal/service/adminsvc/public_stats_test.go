package adminsvc

import (
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/model"
)

func TestValidatePublicStatsSince(t *testing.T) {
	t.Parallel()
	if err := ValidatePublicStats(PublicStatsConfig{Since: "2026-07-19"}); err != nil {
		t.Fatalf("valid since: %v", err)
	}
	if err := ValidatePublicStats(PublicStatsConfig{Since: ""}); err != nil {
		t.Fatalf("empty since: %v", err)
	}
	if err := ValidatePublicStats(PublicStatsConfig{Since: "07/19/2026"}); err == nil {
		t.Fatal("want invalid for US date")
	}
	if err := ValidatePublicStats(PublicStatsConfig{Since: "not-a-date"}); err == nil {
		t.Fatal("want invalid")
	}
}

func TestPublicStatsSnapshotDisabled(t *testing.T) {
	InvalidatePublicStatsCache()
	db := model.TestDB(t)
	snap, err := PublicStatsSnapshotFor(db, DefaultPublicStats(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Enabled {
		t.Fatal("default should be disabled")
	}
	if snap.LiveImageCount != nil || snap.UptimeDays != nil {
		t.Fatalf("disabled must not expose counts: %+v", snap)
	}
}

func TestPublicStatsSnapshotLiveImagesAndSince(t *testing.T) {
	InvalidatePublicStatsCache()
	db := model.TestDB(t)
	now := time.Date(2026, 8, 12, 15, 0, 0, 0, time.UTC)
	f := &model.File{Hash: "pubstath1", StoragePolicyID: 1, Path: "public/x.png", Size: 100, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := model.Image{
		Key: "AbCdEfGhIjKl", Name: "x.png", Ext: "png", FileID: f.ID,
		Visibility: "public", Status: "normal", CreatedAt: now.Add(-48 * time.Hour),
	}
	if err := db.Create(&img).Error; err != nil {
		t.Fatal(err)
	}
	img2 := model.Image{
		Key: "XyZ123456789", Name: "y.png", Ext: "png", FileID: f.ID,
		Visibility: "public", Status: "normal", CreatedAt: now.Add(-24 * time.Hour),
	}
	if err := db.Create(&img2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&img2).Error; err != nil {
		t.Fatal(err)
	}

	cfg := PublicStatsConfig{
		Enabled: true, Since: "2026-08-10",
		ShowUptimeDays: true, ShowLiveImages: true, ShowUsers: false, ShowUsedBytes: true,
	}
	snap, err := PublicStatsSnapshotFor(db, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Enabled {
		t.Fatal("enabled")
	}
	if snap.LiveImageCount == nil || *snap.LiveImageCount != 1 {
		t.Fatalf("live_image_count=%v want 1", snap.LiveImageCount)
	}
	if snap.UptimeDays == nil || *snap.UptimeDays != 2 {
		t.Fatalf("uptime_days=%v want 2 (Aug10→Aug12)", snap.UptimeDays)
	}
	if snap.UsedBytes == nil || *snap.UsedBytes != 100 {
		t.Fatalf("used_bytes=%v want 100", snap.UsedBytes)
	}
	if snap.UserCount != nil {
		t.Fatal("users should be omitted")
	}
}

func TestPublicStatsExcludesPrivateAndPassword(t *testing.T) {
	InvalidatePublicStatsCache()
	db := model.TestDB(t)
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	f := &model.File{Hash: "pubstatpriv", StoragePolicyID: 1, Path: "public/p.png", Size: 10, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	mk := func(key, vis, pw string) {
		t.Helper()
		img := model.Image{
			Key: key, Name: key + ".png", Ext: "png", FileID: f.ID,
			Visibility: vis, Status: "normal", AccessPasswordHash: pw,
		}
		if err := db.Create(&img).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk("PubOnly000001", "public", "")
	mk("PrivOnly00001", "private", "")
	mk("PwdOnly000001", "public", "argon2id$dummy")

	cfg := PublicStatsConfig{Enabled: true, ShowLiveImages: true}
	snap, err := PublicStatsSnapshotFor(db, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	if snap.LiveImageCount == nil || *snap.LiveImageCount != 1 {
		t.Fatalf("公开统计应只计无口令 public 图, got %v", snap.LiveImageCount)
	}
}

func TestPublicStatsCache(t *testing.T) {
	InvalidatePublicStatsCache()
	db := model.TestDB(t)
	now := time.Now().UTC()
	cfg := PublicStatsConfig{Enabled: true, ShowLiveImages: true}
	s1, err := PublicStatsSnapshotFor(db, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "pubstath2", StoragePolicyID: 1, Path: "public/z.png", Size: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Image{Key: "KkKkKkKkKk01", Name: "z", Ext: "png", FileID: f.ID, Status: "normal"}).Error; err != nil {
		t.Fatal(err)
	}
	s2, err := PublicStatsSnapshotFor(db, cfg, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if s1.LiveImageCount == nil || s2.LiveImageCount == nil || *s1.LiveImageCount != *s2.LiveImageCount {
		t.Fatalf("cache miss? s1=%v s2=%v", s1.LiveImageCount, s2.LiveImageCount)
	}
	InvalidatePublicStatsCache()
	s3, err := PublicStatsSnapshotFor(db, cfg, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if s3.LiveImageCount == nil || *s3.LiveImageCount < 1 {
		t.Fatalf("after invalidate want >=1 got %v", s3.LiveImageCount)
	}
}
