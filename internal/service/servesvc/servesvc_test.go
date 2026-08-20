package servesvc

import (
	"testing"
	"time"

	"github.com/yixian-huang/imgli/internal/model"
)

func TestFindAndAuthorizePublic(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "img.li")
	f := &model.File{Hash: "h1", StoragePolicyID: 1, Path: "p", Size: 10, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "abc123xyz012", FileID: f.ID, Name: "a.png", Ext: "png", Visibility: "public", Status: "normal"}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	got, soft := svc.Find("abc123xyz012")
	if soft || got == nil {
		t.Fatal("find")
	}
	if d := svc.Authorize(got, Access{PasswordOK: true}); d != nil {
		t.Fatalf("auth: %v", d)
	}
}

func TestAuthorizePrivate(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	u := &model.User{Username: "owner1", Email: "o1@x.li", GroupID: 1, Status: "active"}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "h2", StoragePolicyID: 1, Path: "p2", Size: 10, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "privkey000001", FileID: f.ID, Name: "p.png", Ext: "png", Visibility: "private", Status: "normal", UserID: &u.ID}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	got, soft := svc.Find("privkey000001")
	if soft || got == nil {
		t.Fatalf("find failed soft=%v", soft)
	}
	if d := svc.Authorize(got, Access{IsOwner: false, PasswordOK: true}); d == nil || d.Kind != DenyPrivate {
		t.Fatalf("want private, got %v", d)
	}
	if d := svc.Authorize(got, Access{IsOwner: true, PasswordOK: true}); d != nil {
		t.Fatalf("owner: %v", d)
	}
	if d := svc.Authorize(got, Access{IsAdmin: true, PasswordOK: true}); d != nil {
		t.Fatalf("admin: %v", d)
	}
}

func TestAuthorizeAdminBypassesPending(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	u := &model.User{Username: "owner2", Email: "o2@x.li", GroupID: 1, Status: "active"}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	f := &model.File{Hash: "h2b", StoragePolicyID: 1, Path: "p2b", Size: 10, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{
		Key: "pendkey000001", FileID: f.ID, Name: "p.png", Ext: "png",
		Visibility: "public", Status: "pending", UserID: &u.ID,
	}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Find("pendkey000001")
	if d := svc.Authorize(got, Access{}); d == nil || d.Kind != DenyRemoved {
		t.Fatalf("anon pending want removed, got %v", d)
	}
	if d := svc.Authorize(got, Access{IsOwner: true}); d != nil {
		t.Fatalf("owner pending: %v", d)
	}
	if d := svc.Authorize(got, Access{IsAdmin: true}); d != nil {
		t.Fatalf("admin pending: %v", d)
	}
}

func TestAuthorizeExpired(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	f := &model.File{Hash: "h3", StoragePolicyID: 1, Path: "p3", Size: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	img := &model.Image{Key: "expkey0000001", FileID: f.ID, Name: "e.png", Ext: "png", Visibility: "public", Status: "normal", ExpiresAt: &past}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Find("expkey0000001")
	if got == nil {
		t.Fatal("find")
	}
	if d := svc.Authorize(got, Access{PasswordOK: true}); d == nil || d.Kind != DenyExpired {
		t.Fatalf("want expired, got %v", d)
	}
}

func TestClaimView(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	f := &model.File{Hash: "h4", StoragePolicyID: 1, Path: "p4", Size: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "maxvkey000001", FileID: f.ID, Name: "m.png", Ext: "png", Visibility: "public", Status: "normal", MaxViews: 1}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	if !svc.ClaimView(img) {
		t.Fatal("first claim")
	}
	var got model.Image
	db.First(&got, img.ID)
	if got.ViewsServed != 1 {
		t.Fatalf("views=%d", got.ViewsServed)
	}
	if svc.ClaimView(&got) {
		t.Fatal("second claim should fail")
	}
}

func TestLoadFilePolicy(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	f := &model.File{Hash: "h5", StoragePolicyID: 1, Path: "p5", Size: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	file, pol, ok := svc.LoadFilePolicy(f.ID)
	if !ok || file.ID != f.ID || pol.ID != 1 {
		t.Fatalf("ok=%v file=%d pol=%d", ok, file.ID, pol.ID)
	}
	_, _, ok = svc.LoadFilePolicy(99999)
	if ok {
		t.Fatal("missing should fail")
	}
}

func TestFindSoftDeleted(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db, nil, "")
	f := &model.File{Hash: "h6", StoragePolicyID: 1, Path: "p6", Size: 1, RefCount: 1}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{Key: "delkey0000001", FileID: f.ID, Name: "d.png", Ext: "png", Visibility: "public", Status: "normal"}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}
	db.Delete(img)
	got, soft := svc.Find("delkey0000001")
	if got != nil || !soft {
		t.Fatalf("want soft deleted, got=%v soft=%v", got, soft)
	}
}
