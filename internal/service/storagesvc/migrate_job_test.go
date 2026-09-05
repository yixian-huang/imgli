package storagesvc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
)

func TestMigrateJobLocalToLocal(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "job-to", Driver: "local",
		Config: map[string]string{"root": "job-b"}, Enabled: true,
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
	payload := []byte("job-migrate-bytes")
	path := "2026/07/31/job1.png"
	if err := src.Put(context.Background(), path, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	f := model.File{
		Hash: "jobhash001", StoragePolicyID: from.ID, Path: path,
		Size: int64(len(payload)), MIME: "image/png", RefCount: 1,
	}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}

	job, err := r.StartMigrateJob(MigrateJobOpts{
		FromPolicyID: from.ID, ToPolicyID: to.ID, DeleteSource: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	var snap MigrateJobView
	for time.Now().Before(deadline) {
		var ok bool
		snap, ok = r.GetMigrateJob(job.ID)
		if !ok {
			t.Fatal("job missing")
		}
		if snap.Status == MigrateJobDone || snap.Status == MigrateJobFailed {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snap.Status != MigrateJobDone {
		t.Fatalf("status=%s err=%s progress=%+v", snap.Status, snap.Error, snap.Progress)
	}
	if snap.Progress.Copied != 1 {
		t.Fatalf("copied=%d want 1", snap.Progress.Copied)
	}
	var check model.File
	db.First(&check, f.ID)
	if check.StoragePolicyID != to.ID {
		t.Fatalf("policy=%d want %d", check.StoragePolicyID, to.ID)
	}
}

func TestMigrateJobBusySameFrom(t *testing.T) {
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "busy2", Driver: "local", Config: map[string]string{"root": "b2"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: t.TempDir()}, db)
	if err := db.Create(&model.StorageMigrateJob{
		ID: "busyjob000000000000000000000001", FromPolicyID: 1, ToPolicyID: to.ID,
		Status: MigrateJobRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	_, err := r.StartMigrateJob(MigrateJobOpts{FromPolicyID: 1, ToPolicyID: to.ID})
	if !errors.Is(err, ErrMigrateBusy) {
		t.Fatalf("want ErrMigrateBusy, got %v", err)
	}
}

func waitJob(t *testing.T, r *Resolver, id string) MigrateJobView {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var snap MigrateJobView
	for time.Now().Before(deadline) {
		var ok bool
		snap, ok = r.GetMigrateJob(id)
		if !ok {
			t.Fatal("job missing")
		}
		switch snap.Status {
		case MigrateJobDone, MigrateJobFailed, MigrateJobCancelled:
			return snap
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job did not finish: %+v", snap)
	return snap
}

func TestMigrateJobRecoversAfterRestart(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "rec-to", Driver: "local", Config: map[string]string{"root": "rec-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: dir}, db)
	var from model.StoragePolicy
	if err := db.First(&from, 1).Error; err != nil {
		t.Fatal(err)
	}
	src, err := r.Driver(&from)
	if err != nil {
		t.Fatal(err)
	}
	path := "2026/09/05/rec.png"
	if err := src.Put(context.Background(), path, bytes.NewReader([]byte("recover-me"))); err != nil {
		t.Fatal(err)
	}
	f := model.File{
		Hash: "rechash001", StoragePolicyID: from.ID, Path: path,
		Size: 10, MIME: "image/png", RefCount: 1,
	}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	row := model.StorageMigrateJob{
		ID: "recoverjob000000000000000000001", FromPolicyID: from.ID, ToPolicyID: to.ID,
		Status: MigrateJobRunning,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	r2 := New(&config.Config{DataDir: dir}, db)
	if n := r2.RecoverMigrateJobs(); n != 1 {
		t.Fatalf("recovered %d", n)
	}
	snap := waitJob(t, r2, row.ID)
	if snap.Status != MigrateJobDone {
		t.Fatalf("status=%s err=%s", snap.Status, snap.Error)
	}
	var check model.File
	db.First(&check, f.ID)
	if check.StoragePolicyID != to.ID {
		t.Fatalf("policy=%d want %d", check.StoragePolicyID, to.ID)
	}
}

func TestMigrateJobCancelPending(t *testing.T) {
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "can-to", Driver: "local", Config: map[string]string{"root": "can-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: t.TempDir()}, db)
	row := model.StorageMigrateJob{
		ID: "canceljob0000000000000000000001", FromPolicyID: 1, ToPolicyID: to.ID,
		Status: MigrateJobPending,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	view, err := r.CancelMigrateJob(row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != MigrateJobCancelled {
		t.Fatalf("status=%s", view.Status)
	}
	if n := r.RecoverMigrateJobs(); n != 0 {
		t.Fatalf("cancelled job should not recover, n=%d", n)
	}
}

func TestMigrateJobResumeFailed(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "rs-to", Driver: "local", Config: map[string]string{"root": "rs-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: dir}, db)
	var from model.StoragePolicy
	if err := db.First(&from, 1).Error; err != nil {
		t.Fatal(err)
	}
	src, err := r.Driver(&from)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint64
	for i := 0; i < 2; i++ {
		p := fmt.Sprintf("rs/%d.png", i)
		if err := src.Put(context.Background(), p, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatal(err)
		}
		f := model.File{
			Hash: fmt.Sprintf("rshash%02d", i), StoragePolicyID: from.ID, Path: p,
			Size: 1, MIME: "image/png", RefCount: 1,
		}
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
	}
	row := model.StorageMigrateJob{
		ID: "resumejob0000000000000000000001", FromPolicyID: from.ID, ToPolicyID: to.ID,
		Status: MigrateJobFailed, CursorAfterID: ids[0], Scanned: 1, Copied: 1, LastError: "boom",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.File{}).Where("id = ?", ids[0]).Update("storage_policy_id", to.ID).Error; err != nil {
		t.Fatal(err)
	}
	job, err := r.ResumeMigrateJob(row.ID)
	if err != nil {
		t.Fatal(err)
	}
	snap := waitJob(t, r, job.ID)
	if snap.Status != MigrateJobDone {
		t.Fatalf("status=%s err=%s progress=%+v", snap.Status, snap.Error, snap.Progress)
	}
	if snap.Progress.Copied < 2 {
		t.Fatalf("copied=%d want >=2", snap.Progress.Copied)
	}
	var last model.File
	db.First(&last, ids[1])
	if last.StoragePolicyID != to.ID {
		t.Fatalf("file 1 still on %d", last.StoragePolicyID)
	}
}

func TestMigrateJobActiveIndex(t *testing.T) {
	db := model.TestDB(t)
	a := model.StorageMigrateJob{ID: "uniqA000000000000000000000000001", FromPolicyID: 1, ToPolicyID: 1, Status: MigrateJobRunning}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	b := model.StorageMigrateJob{ID: "uniqB000000000000000000000000001", FromPolicyID: 1, ToPolicyID: 1, Status: MigrateJobRunning}
	if err := db.Create(&b).Error; err == nil {
		t.Fatal("expected unique active from")
	}
}

func TestMigrateFilesAfterIDCursor(t *testing.T) {
	dir := t.TempDir()
	db := model.TestDB(t)
	to := model.StoragePolicy{
		Name: "cur-to", Driver: "local", Config: map[string]string{"root": "cur-b"}, Enabled: true,
	}
	if err := db.Create(&to).Error; err != nil {
		t.Fatal(err)
	}
	r := New(&config.Config{DataDir: dir}, db)
	var from model.StoragePolicy
	if err := db.First(&from, 1).Error; err != nil {
		t.Fatal(err)
	}
	src, err := r.Driver(&from)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint64
	for i := 0; i < 3; i++ {
		p := fmt.Sprintf("cur/%d.png", i)
		if err := src.Put(context.Background(), p, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatal(err)
		}
		f := model.File{
			Hash: fmt.Sprintf("chash%02d", i), StoragePolicyID: 1, Path: p,
			Size: 1, MIME: "image/png", RefCount: 1,
		}
		if err := db.Create(&f).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
	}
	res, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 1 || res.LastFileID != ids[0] {
		t.Fatalf("batch1 %+v ids0=%d", res, ids[0])
	}
	res2, err := r.MigrateFiles(context.Background(), db, MigrateOpts{
		FromPolicyID: 1, ToPolicyID: to.ID, AfterID: res.LastFileID, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Scanned != 2 {
		t.Fatalf("batch2 scanned=%d want 2", res2.Scanned)
	}
}
