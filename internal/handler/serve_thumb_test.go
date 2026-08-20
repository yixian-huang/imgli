package handler

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yixian-huang/imgli/internal/config"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/servecache"
	"github.com/yixian-huang/imgli/internal/service/stats"
	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

// thumbFixture 最小 /t 双探测环境:公开图 + 可写本地存储根 + 挂载路由。
type thumbFixture struct {
	mux     *chi.Mux
	sh      *ServeHandlers
	dataDir string
	hash    string
	name    string
}

func newThumbFixture(t *testing.T) *thumbFixture {
	t.Helper()
	db := model.TestDB(t)
	dataDir := t.TempDir()
	cfg := &config.Config{DataDir: dataDir, BaseURL: "http://img.li:8686"}

	u := &model.User{Username: "tu", Email: "tu@img.li", GroupID: 1}
	if err := db.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	path := "2024/02/02/thumbprobe01.png"
	hash := "thumbprobehash001"
	f := &model.File{
		Hash: hash, StoragePolicyID: 1, Path: path,
		Size: 12, MIME: "image/png", Width: 1, Height: 1, RefCount: 1,
	}
	if err := db.Create(f).Error; err != nil {
		t.Fatal(err)
	}
	img := &model.Image{
		Key: "thumbprobekey01", UserID: &u.ID, FileID: f.ID,
		Name: "tp", Ext: "png", Visibility: "public", Status: "normal",
	}
	if err := db.Create(img).Error; err != nil {
		t.Fatal(err)
	}

	abs := filepath.Join(dataDir, "uploads", filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("fakepngbytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := stats.New(db, time.Hour)
	sh := &ServeHandlers{D: ServeDeps{
		DB: db, Res: storagesvc.New(cfg, db),
		Stats: st, OwnHost: "img.li",
	}}
	mux := chi.NewRouter()
	mux.Get("/t/{name}", sh.Thumbnail)
	return &thumbFixture{mux: mux, sh: sh, dataDir: dataDir, hash: hash, name: img.Key + ".png"}
}

func (f *thumbFixture) thumbPath(key string) string {
	return filepath.Join(f.dataDir, "uploads", filepath.FromSlash(key))
}

func (f *thumbFixture) writeThumb(t *testing.T, key string, body []byte) {
	t.Helper()
	p := f.thumbPath(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *thumbFixture) getT() *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/t/"+f.name, nil)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func TestThumbDualProbeWebPThenJPEGThen404(t *testing.T) {
	fx := newThumbFixture(t)
	webpKey := storagesvc.ThumbKeyWebP(model.SurfacePublic, fx.hash)
	jpgKey := storagesvc.ThumbKey(model.SurfacePublic, fx.hash)

	// 仅 .webp → image/webp
	fx.writeThumb(t, webpKey, []byte("RIFF....WEBPfake"))
	rec := fx.getT()
	if rec.Code != 200 {
		t.Fatalf("webp: status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/webp" {
		t.Errorf("webp: Content-Type=%q want image/webp", ct)
	}
	if !strings.Contains(rec.Body.String(), "WEBP") {
		t.Errorf("webp: body 未返回假 webp 字节: %q", rec.Body.String())
	}

	// 删 webp 只留 jpg → image/jpeg
	if err := os.Remove(fx.thumbPath(webpKey)); err != nil {
		t.Fatal(err)
	}
	fx.writeThumb(t, jpgKey, []byte("fakejpgbytes"))
	rec = fx.getT()
	if rec.Code != 200 {
		t.Fatalf("jpg: status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("jpg: Content-Type=%q want image/jpeg", ct)
	}
	if rec.Body.String() != "fakejpgbytes" {
		t.Errorf("jpg: body=%q want fakejpgbytes", rec.Body.String())
	}

	// 两者皆无 → 再测遗留无版本路径仍可读
	if err := os.Remove(fx.thumbPath(jpgKey)); err != nil {
		t.Fatal(err)
	}
	legacy := ".thumbs/" + fx.hash + ".jpg"
	fx.writeThumb(t, legacy, []byte("legacyjpg"))
	rec = fx.getT()
	if rec.Code != 200 || rec.Body.String() != "legacyjpg" {
		t.Fatalf("legacy key: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if err := os.Remove(fx.thumbPath(legacy)); err != nil {
		t.Fatal(err)
	}
	// 全无 → 404
	rec = fx.getT()
	if rec.Code != 404 {
		t.Fatalf("none: status=%d want 404 body=%s", rec.Code, rec.Body.String())
	}
}

func TestThumbEmptySurfaceReadsPublicKeys(t *testing.T) {
	fx := newThumbFixture(t)
	if err := fx.sh.D.DB.Exec("UPDATE files SET surface = '' WHERE hash = ?", fx.hash).Error; err != nil {
		t.Fatal(err)
	}
	// 原图不在盘上：逼 /t 只靠缩略图键，不能走 ensureDefaultThumb。
	orig := filepath.Join(fx.dataDir, "uploads", "2024", "02", "02", "thumbprobe01.png")
	_ = os.Remove(orig)

	pubThumb := storagesvc.ThumbKey(model.SurfacePublic, fx.hash)
	fx.writeThumb(t, pubThumb, []byte("emptysurfacejpg"))
	rec := fx.getT()
	if rec.Code != 200 {
		t.Fatalf("空 surface 应命中 public/.thumbs, status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "emptysurfacejpg" {
		t.Errorf("body=%q", rec.Body.String())
	}
}

func tinyPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestServeThumbFallsBackToCDNOrigin(t *testing.T) {
	fx := newThumbFixture(t)
	origRel := "2024/02/02/thumbprobe01.png"
	_ = os.Remove(filepath.Join(fx.dataDir, "uploads", filepath.FromSlash(origRel)))
	for _, tk := range storagesvc.ThumbKeyCandidates(model.SurfacePublic, fx.hash) {
		_ = os.Remove(fx.thumbPath(tk))
	}

	pngBytes := tinyPNG()
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+origRel {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	t.Cleanup(cdn.Close)
	if err := fx.sh.D.DB.Model(&model.StoragePolicy{}).Where("id = 1").
		Update("cdn_domain", cdn.URL).Error; err != nil {
		t.Fatal(err)
	}

	rec := fx.getT()
	if rec.Code != 200 {
		t.Fatalf("源站原图缺失时应从 CDN 生成 /t, status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "image/jpeg" && ct != "image/webp" {
		t.Errorf("Content-Type=%q", ct)
	}
	if rec.Body.Len() < 32 {
		t.Errorf("生成缩略图过短: %d", rec.Body.Len())
	}
	if strings.Contains(rec.Body.String(), "NOT FOUND") {
		t.Fatal("不应再返回 NOT FOUND 占位")
	}
}

func TestServeThumbHitsDiskCache(t *testing.T) {
	fx := newThumbFixture(t)
	cache, err := servecache.New(filepath.Join(t.TempDir(), "c"), 1<<20, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	fx.sh.D.Cache = cache

	webpKey := storagesvc.ThumbKeyWebP(model.SurfacePublic, fx.hash)
	body := []byte("RIFF....WEBPfake")
	fx.writeThumb(t, webpKey, body)
	rec := fx.getT()
	if rec.Code != 200 {
		t.Fatalf("first: status=%d", rec.Code)
	}
	if err := os.Remove(fx.thumbPath(webpKey)); err != nil {
		t.Fatal(err)
	}
	rec = fx.getT()
	if rec.Code != 200 {
		t.Fatalf("origin 已删仍应命中缓存, got %d", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Errorf("cached body=%q", rec.Body.String())
	}
}
