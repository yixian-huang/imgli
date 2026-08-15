package discoversvc

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/yixian-huang/imgli/internal/model"
)

func mkUser(t *testing.T, db *gorm.DB, name string, pub bool, status string) model.User {
	t.Helper()
	u := model.User{
		Username:      name,
		Email:         name + "@test.img.li",
		GroupID:       1,
		Status:        status,
		PublicProfile: pub,
		Nickname:      name + "_nick",
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func mkImage(t *testing.T, db *gorm.DB, key string, uid *uint64, vis, status string, exp *time.Time) model.Image {
	t.Helper()
	f := model.File{
		Hash:            key + "hash",
		StoragePolicyID: 1,
		Path:            "p/" + key,
		Size:            100,
		MIME:            "image/png",
		Width:           10,
		Height:          10,
		RefCount:        1,
	}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	img := model.Image{
		Key:        key,
		UserID:     uid,
		FileID:     f.ID,
		Name:       key,
		Ext:        "png",
		Visibility: vis,
		Status:     status,
		ExpiresAt:  exp,
	}
	if err := db.Create(&img).Error; err != nil {
		t.Fatal(err)
	}
	return img
}

func addViews(t *testing.T, db *gorm.DB, imageID uint64, date string, n int64) {
	t.Helper()
	if err := db.Create(&model.AccessStat{ImageID: imageID, Date: date, Views: n}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPlazaFeed_EligibilityMatrix(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	owner := mkUser(t, db, "owner", true, "active")
	uid := owner.ID
	expPast := time.Now().Add(-1 * time.Hour)

	// A: 唯一应纳入
	mkImage(t, db, "imgA", &uid, "public", "normal", nil)
	// B–F: 同用户下各种不合格
	mkImage(t, db, "imgB", &uid, "private", "normal", nil)
	mkImage(t, db, "imgC", &uid, "public", "pending", nil)
	mkImage(t, db, "imgD", &uid, "public", "rejected", nil)
	mkImage(t, db, "imgE", &uid, "public", "normal", &expPast)
	fImg := mkImage(t, db, "imgF", &uid, "public", "normal", nil)
	if err := db.Delete(&fImg).Error; err != nil {
		t.Fatal(err)
	}

	// G: 未公开主页用户
	gUser := mkUser(t, db, "hidden", false, "active")
	gUID := gUser.ID
	mkImage(t, db, "imgG", &gUID, "public", "normal", nil)

	// H: banned 用户
	hUser := mkUser(t, db, "bannedu", true, "banned")
	hUID := hUser.ID
	mkImage(t, db, "imgH", &hUID, "public", "normal", nil)

	// I: 游客图
	mkImage(t, db, "imgI", nil, "public", "normal", nil)

	rows, next, err := svc.PlazaFeed("new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Errorf("单页应无 next, got %q", next)
	}
	if len(rows) != 1 {
		t.Fatalf("资格矩阵应只含 A, got %d rows: %+v", len(rows), rows)
	}
	if rows[0].Key != "imgA" {
		t.Fatalf("应只含 imgA, got %q", rows[0].Key)
	}
	if rows[0].Author.Username != "owner" {
		t.Errorf("author.username: got %q", rows[0].Author.Username)
	}
}

func TestPlazaFeed_SortNewAndCursor(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	u := mkUser(t, db, "newuser", true, "active")
	uid := u.ID

	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 3, 10, 0, 0, 0, time.UTC)

	i1 := mkImage(t, db, "new1", &uid, "public", "normal", nil)
	i2 := mkImage(t, db, "new2", &uid, "public", "normal", nil)
	i3 := mkImage(t, db, "new3", &uid, "public", "normal", nil)
	if err := db.Model(&i1).Update("created_at", t1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&i2).Update("created_at", t2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&i3).Update("created_at", t3).Error; err != nil {
		t.Fatal(err)
	}

	page1, next, err := svc.PlazaFeed("new", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len want 2, got %d", len(page1))
	}
	if page1[0].Key != "new3" || page1[1].Key != "new2" {
		t.Fatalf("page1 order want [new3,new2], got [%s,%s]", page1[0].Key, page1[1].Key)
	}
	if next == "" {
		t.Fatal("应有 nextCursor")
	}

	page2, next2, err := svc.PlazaFeed("new", next, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 || page2[0].Key != "new1" {
		t.Fatalf("page2 want [new1], got %+v", page2)
	}
	if next2 != "" {
		t.Errorf("末页 next 应空, got %q", next2)
	}

	// 无重无漏
	seen := map[string]bool{page1[0].Key: true, page1[1].Key: true, page2[0].Key: true}
	if len(seen) != 3 {
		t.Fatalf("应覆盖 3 张无重, got %v", seen)
	}
}

func TestPlazaFeed_SortHot(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	u := mkUser(t, db, "hotuser", true, "active")
	uid := u.ID

	// views totals: 5 / 20 / 0
	a := mkImage(t, db, "hot5", &uid, "public", "normal", nil)
	b := mkImage(t, db, "hot20", &uid, "public", "normal", nil)
	_ = mkImage(t, db, "hot0", &uid, "public", "normal", nil)

	addViews(t, db, a.ID, "2024-01-01", 2)
	addViews(t, db, a.ID, "2024-01-02", 3) // SUM = 5
	addViews(t, db, b.ID, "2024-01-01", 20)

	rows, next, err := svc.PlazaFeed("hot", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Errorf("单页 next 应空, got %q", next)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3, got %d", len(rows))
	}
	want := []struct {
		key   string
		views int64
	}{
		{"hot20", 20},
		{"hot5", 5},
		{"hot0", 0},
	}
	for i, w := range want {
		if rows[i].Key != w.key || rows[i].Views != w.views {
			t.Errorf("pos %d: want {%s,%d}, got {%s,%d}", i, w.key, w.views, rows[i].Key, rows[i].Views)
		}
	}
}

func TestUserPublic_NotFoundBranches(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	_ = mkUser(t, db, "privu", false, "active")
	_ = mkUser(t, db, "banu", true, "banned")
	ok := mkUser(t, db, "okuser", true, "active")
	okUID := ok.ID
	mkImage(t, db, "okpub", &okUID, "public", "normal", nil)
	mkImage(t, db, "okpriv", &okUID, "private", "normal", nil)

	for _, name := range []string{"nosuch", "privu", "banu"} {
		_, err := svc.UserPublic(name)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: want ErrNotFound, got %v", name, err)
		}
	}

	p, err := svc.UserPublic("okuser")
	if err != nil {
		t.Fatal(err)
	}
	if p.Username != "okuser" || p.Nickname != "okuser_nick" {
		t.Errorf("profile fields: %+v", p)
	}
	if p.PublicImageCount != 1 {
		t.Errorf("PublicImageCount want 1 (only public+eligible), got %d", p.PublicImageCount)
	}
	if p.AvatarVersion != ok.UpdatedAt.Unix() {
		t.Errorf("AvatarVersion want %d, got %d", ok.UpdatedAt.Unix(), p.AvatarVersion)
	}
}

func TestUserImages_ScopedAndEligible(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	u1 := mkUser(t, db, "u1pub", true, "active")
	u2 := mkUser(t, db, "u2pub", true, "active")
	u1ID, u2ID := u1.ID, u2.ID

	mkImage(t, db, "u1ok", &u1ID, "public", "normal", nil)
	mkImage(t, db, "u1priv", &u1ID, "private", "normal", nil)
	mkImage(t, db, "u2ok", &u2ID, "public", "normal", nil)

	rows, next, err := svc.UserImages(u1ID, "new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if next != "" {
		t.Errorf("next should be empty, got %q", next)
	}
	if len(rows) != 1 || rows[0].Key != "u1ok" {
		t.Fatalf("want only u1ok, got %+v", rows)
	}
	if rows[0].Author.UserID != u1ID {
		t.Errorf("author uid want %d, got %d", u1ID, rows[0].Author.UserID)
	}
}

func TestPlazaFeed_BadCursor(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)
	_, _, err := svc.PlazaFeed("new", "@@@bad", 50)
	if !errors.Is(err, ErrBadCursor) {
		t.Fatalf("want ErrBadCursor, got %v", err)
	}
}

func keysOf(rows []Row) map[string]bool {
	got := make(map[string]bool, len(rows))
	for _, r := range rows {
		got[r.Key] = true
	}
	return got
}

func mkAlbum(t *testing.T, db *gorm.DB, uid uint64, name, vis string, listInPlaza bool) model.Album {
	t.Helper()
	alb := model.Album{UserID: uid, Name: name, Visibility: vis, ListInPlaza: listInPlaza}
	if err := db.Create(&alb).Error; err != nil {
		t.Fatal(err)
	}
	// GORM 对 bool 零值会跳过写入，default:true 会把 false 吃掉；显式落库。
	if err := db.Model(&alb).Update("list_in_plaza", listInPlaza).Error; err != nil {
		t.Fatal(err)
	}
	return alb
}

func putInAlbum(t *testing.T, db *gorm.DB, img model.Image, albumID uint64) {
	t.Helper()
	if err := db.Model(&img).Update("album_id", albumID).Error; err != nil {
		t.Fatal(err)
	}
}

// TestPlazaFeed_PrivateAlbumExcluded 私密相册内的 public 图不得进广场/公开主页。
// 回归：eligible 曾只看 list_in_plaza，私密相册默认 list_in_plaza=true，上传默认 public，
// 导致「相册设为私密」后图片仍出现在广场。
func TestPlazaFeed_PrivateAlbumExcluded(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)
	owner := mkUser(t, db, "owner", true, "active")
	uid := owner.ID

	priv := mkAlbum(t, db, uid, "secret", "private", true) // 默认路径：私密 + 仍参与广场
	pub := mkAlbum(t, db, uid, "open", "public", true)
	optOut := mkAlbum(t, db, uid, "quiet", "public", false)

	putInAlbum(t, db, mkImage(t, db, "inpriv", &uid, "public", "normal", nil), priv.ID)
	putInAlbum(t, db, mkImage(t, db, "inpub", &uid, "public", "normal", nil), pub.ID)
	putInAlbum(t, db, mkImage(t, db, "optout", &uid, "public", "normal", nil), optOut.ID)
	mkImage(t, db, "loose", &uid, "public", "normal", nil)

	assertPlazaKeys := func(t *testing.T, rows []Row) {
		t.Helper()
		got := keysOf(rows)
		if got["inpriv"] {
			t.Error("私密相册内的 public 图泄漏到了公开流")
		}
		if !got["inpub"] {
			t.Error("公开相册 + list_in_plaza 的图应出现")
		}
		if got["optout"] {
			t.Error("list_in_plaza=false 的图不应出现")
		}
		if !got["loose"] {
			t.Error("未分类 public 图应出现")
		}
	}

	rows, _, err := svc.PlazaFeed("new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertPlazaKeys(t, rows)

	urows, _, err := svc.UserImages(uid, "new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertPlazaKeys(t, urows)
}

// TestPlazaFeed_ExpiryOrParenthesization 锁死资格过滤中 (expires_at IS NULL OR expires_at > now)
// 的括号：一张 private 但 expires_at 在未来、属主为公开 active 用户的图——若 OR 未加括号，
// SQL 优先级会让「expires_at > now AND user_id NOT NULL AND public_profile AND active」独立成
// 一支而泄漏私图。此用例守护 GORM 自动加括号的行为不被未来重构破坏。
func TestPlazaFeed_ExpiryOrParenthesization(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)
	owner := mkUser(t, db, "owner", true, "active")
	uid := owner.ID
	expFuture := time.Now().Add(1 * time.Hour)
	mkImage(t, db, "priv", &uid, "private", "normal", &expFuture) // private + 未来过期 → 必须排除
	mkImage(t, db, "pub", &uid, "public", "normal", &expFuture)   // public + 未来过期 → 纳入

	rows, _, err := svc.PlazaFeed("new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != "pub" {
		t.Fatalf("private+未来过期图泄漏！应只含 pub, got %+v", rows)
	}
}

// TestPlazaFeed_PasswordExcluded 公开但设了访问口令的图不得进广场/公开主页
//（/t 会 401，卡片会烂；口令图不是可公开陈列）。
func TestPlazaFeed_PasswordExcluded(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)
	owner := mkUser(t, db, "owner", true, "active")
	uid := owner.ID

	plain := mkImage(t, db, "plain", &uid, "public", "normal", nil)
	locked := mkImage(t, db, "locked", &uid, "public", "normal", nil)
	if err := db.Model(&locked).Update("access_password_hash", "argon2id$dummy").Error; err != nil {
		t.Fatal(err)
	}

	assertKeys := func(t *testing.T, rows []Row) {
		t.Helper()
		got := keysOf(rows)
		if got["locked"] {
			t.Error("口令图泄漏到了公开流")
		}
		if !got["plain"] {
			t.Error("无口令 public 图应出现")
		}
		if got["plain"] && locked.Key == plain.Key {
			t.Fatal("夹具 key 冲突")
		}
	}

	rows, _, err := svc.PlazaFeed("new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, rows)

	urows, _, err := svc.UserImages(uid, "new", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	assertKeys(t, urows)
}
