package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
)

func TestHeicGroupsMissing(t *testing.T) {
	ok := model.UserGroup{Name: "默认组", AllowedExts: []string{"png", "heic", "heif"}}
	bad := model.UserGroup{Name: "锁 png", AllowedExts: []string{"png"}}
	heicOnly := model.UserGroup{ID: 9, AllowedExts: []string{"heic"}}
	got := heicGroupsMissing([]model.UserGroup{ok, bad, heicOnly})
	if len(got) != 2 || got[0] != "锁 png" || got[1] != "#9" {
		t.Fatalf("got %v", got)
	}
	if groupAllowsHeicHeif([]string{"HEIC", "Heif"}) != true {
		t.Fatal("case fold")
	}
}

func TestCheckBaseURL(t *testing.T) {
	lv, _ := CheckBaseURL("")
	if lv != Fail {
		t.Fatal("empty fail")
	}
	lv, msg := CheckBaseURL("https://img.li")
	if lv != OK || !strings.Contains(msg, "img.li") {
		t.Fatalf("%v %q", lv, msg)
	}
	lv, _ = CheckBaseURL("http://localhost:8686")
	if lv != Warn {
		t.Fatalf("localhost want warn got %v", lv)
	}
	lv, _ = CheckBaseURL("ftp://x")
	if lv != Fail {
		t.Fatal("ftp fail")
	}
}

func TestRunHappyLocal(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Listen:  "127.0.0.1:0",
		BaseURL: "https://img.li",
		DataDir: dir,
		Database: config.Database{
			Driver: "sqlite",
			DSN:    filepath.Join(dir, "t.db"),
		},
		TrustProxy: false,
	}
	// seed via open+migrate+seed
	db, err := model.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := model.Seed(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	rep := Run(cfg)
	out := Format(rep)
	t.Log(out)
	if rep.HardFail {
		t.Fatalf("unexpected hard fail: %s", out)
	}
	// data dir should be ok
	found := false
	for _, c := range rep.Checks {
		if c.Name == "data_dir" && c.Level == OK {
			found = true
		}
	}
	if !found {
		t.Fatal("missing data_dir ok")
	}
	// cleanup probe files
	_ = os.Remove(filepath.Join(dir, ".imgli-doctor-write"))
}

func TestPathStyleVendorWarn(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Listen:  "127.0.0.1:0",
		BaseURL: "https://img.li",
		DataDir: dir,
		Database: config.Database{
			Driver: "sqlite",
			DSN:    filepath.Join(dir, "ps.db"),
		},
	}
	db, err := model.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	p := model.StoragePolicy{
		Name: "oss-path", Driver: "s3", Enabled: true,
		Config: map[string]string{
			"endpoint": "oss-cn-hangzhou.aliyuncs.com", "region": "cn-hangzhou",
			"bucket": "b", "access_key_id": "a", "secret_access_key": "s",
			"path_style": "true",
		},
	}
	if err := db.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	rep := Run(cfg)
	var found *Check
	for i := range rep.Checks {
		if rep.Checks[i].Name == "path_style_vendor" {
			found = &rep.Checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("missing path_style_vendor check")
	}
	if found.Level != Warn {
		t.Fatalf("level=%s want warn msg=%q", found.Level, found.Message)
	}
	if !strings.Contains(found.Message, "虚拟主机") && !strings.Contains(found.Message, "path_style") {
		t.Fatalf("msg=%q", found.Message)
	}
}

func TestCDNMeteringWarn(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Listen:  "127.0.0.1:0",
		BaseURL: "https://img.li",
		DataDir: dir,
		Database: config.Database{
			Driver: "sqlite",
			DSN:    filepath.Join(dir, "cdn.db"),
		},
	}
	db, err := model.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := model.Seed(db); err != nil {
		t.Fatal(err)
	}
	// Set CDN on default policy if present; else create one.
	var p model.StoragePolicy
	if err := db.Where("enabled = ?", true).First(&p).Error; err != nil {
		p = model.StoragePolicy{Name: "cdn-pol", Driver: "local", Enabled: true, CDNDomain: "https://i.example", Config: map[string]string{"root": "uploads"}}
		if err := db.Create(&p).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.Model(&p).Update("cdn_domain", "https://i.example").Error; err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()

	rep := Run(cfg)
	var found *Check
	for i := range rep.Checks {
		if rep.Checks[i].Name == "cdn_metering" {
			found = &rep.Checks[i]
			break
		}
	}
	if found == nil {
		t.Fatal("missing cdn_metering check")
	}
	if found.Level != Warn {
		t.Fatalf("cdn_metering level=%s want warn msg=%q", found.Level, found.Message)
	}
	if !strings.Contains(found.Message, "边缘") && !strings.Contains(strings.ToLower(found.Message), "cdn") {
		t.Fatalf("msg should mention CDN caveat: %q", found.Message)
	}
}

func TestRunDataDirNotWritable(t *testing.T) {
	// skip if root
	if os.Getuid() == 0 {
		t.Skip("root")
	}
	// use a file as data_dir path parent that is not a dir
	f := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Listen:   ":8686",
		BaseURL:  "http://localhost:8686",
		DataDir:  f,
		Database: config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "x.db")},
	}
	rep := Run(cfg)
	if !rep.HardFail {
		t.Fatal("want hard fail for non-dir data_dir")
	}
}
