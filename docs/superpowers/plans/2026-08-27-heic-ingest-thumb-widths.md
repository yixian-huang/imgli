# HEIC ingest + `/t?w=` whitelist Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship v0.9.17 so iPhone HEIC/HEIF uploads become browser-embeddable JPEG/WebP on vips builds, and `/t?w=` accepts 120, 200, 240, 400, 480, 800, 960, 1600.

**Architecture:** Sniff ISO-BMFF `ftyp` brands in `internal/imaging`, transcode HEIF→JPEG with libvips (`autorot` + `jpegsave`) **before** content-hash in `upload.Service.Save`, then reuse today’s `burn` pipeline. Thumb widths stay origin-generated disk keys; `/i` CDN 302 is unchanged. Pure-Go GitHub binaries reject HEIC with `heic_unsupported`.

**Tech Stack:** Go 1.26, libvips (cgo `-tags vips`), chi handlers, SQLite/Postgres via GORM, React SPA i18n.

**Spec:** `docs/superpowers/specs/2026-08-27-heic-ingest-thumb-widths-design.md`

## Global Constraints

- Version: **v0.9.17** patch. Do not tag in this plan.
- Do **not** store or serve HEIC bytes. `images.ext` / `links.url` are `jpg` or `webp`.
- Do **not** add `/i/{key}.webp` derived originals or arbitrary `w`.
- Do **not** rewrite existing `user_groups.allowed_exts` (only new Seed rows).
- `heic` and `heif` are **not** aliases: filename `.heif` requires `heif` on the group list.
- `HeicDecodeAvailable()` is **runtime** (vips + HEIF loader), not merely `-tags vips`.
- PicGo `data.links.url` shape unchanged (only the stored extension may change).
- No vendor S3 SDKs / OpenList work in this plan.
- Tests: `go test ./... -count=1` (pure Go CI). Vips path: `make test-vips` plus tagged tests under `internal/service/upload`.
- Copy: zh backend `message` + machine `code`; en UI maps `errors.<code>`.

## File map

| File | Responsibility |
|------|----------------|
| `internal/imaging/heif.go` | `SniffHEIF`, `HEIFAllowExt`, `ErrHeicUnavailable`, pure-Go `HeicDecodeAvailable`/`DecodeHEIFToJPEG` stubs |
| `internal/imaging/heif_vips.go` | `-tags vips`: runtime loader probe + decode/autorot/JPEG |
| `internal/imaging/heif_test.go` | sniff + allow-ext + pure-Go Probe/Decode errors |
| `internal/imaging/heif_vips_test.go` | `-tags vips`: skip unless loader present |
| `internal/imaging/imaging.go` | `goProcessor.Probe` sniffs HEIF → `ErrHeicUnavailable` |
| `internal/imaging/vips.go` | `vipsProcessor.Probe` routes HEIF to vips header |
| `internal/service/upload/save.go` | transcode before `burn` / hash |
| `internal/service/upload/upload.go` | `ErrHeicUnavailable` |
| `internal/handler/upload.go` | HTTP 415 `heic_unsupported` |
| `internal/handler/respond.go` | `CodeHeicUnsupported` |
| `internal/handler/serve.go` | width whitelist + generated error text |
| `internal/handler/thumbwidth.go` | whitelist helpers (new, keep `serve.go` smaller) |
| `internal/handler/admin_health.go` | `heic_decode` on runtime |
| `internal/service/adminsvc/settings.go` | `processing_capabilities.heic_decode` |
| `internal/model/db.go` | default + guest seed exts |
| `web/src/i18n/locales/{zh,en}/errors.ts` | `heic_unsupported` |
| `web/src/i18n/errorText.ts` | prefer i18n for that code |
| `web/src/i18n/locales/{zh,en}/adminA.ts` | capability label |
| `web/src/pages/admin/system/SystemPage.tsx` | show `heic_decode` |
| `web/src/api/types.ts` / `adminHooks.ts` | types |
| `web/src/pages/admin/groups/GroupsPage.tsx` | new-group default exts |
| `cmd/imgli/import_dir.go` | `.heic` / `.heif` |
| `Dockerfile` | HEIF loader runtime package |
| README / integrations / CHANGELOG | whitelist + HEIC note |

---

### Task 1: HEIF sniff + pure-Go Probe error

**Files:**
- Create: `internal/imaging/heif.go`
- Create: `internal/imaging/heif_test.go`
- Modify: `internal/imaging/imaging.go` (`goProcessor.Probe`)

**Interfaces:**
- Consumes: none
- Produces:
  - `var ErrHeicUnavailable error` (`imaging: HEIC requires libvips+libheif`)
  - `func SniffHEIF(b []byte) bool`
  - `func HEIFAllowExt(filename string) string` → `"heic"` or `"heif"`
  - `func HeicDecodeAvailable() bool` (pure-Go file; vips file in Task 2)
  - `func DecodeHEIFToJPEG(data []byte, jpegQuality int) ([]byte, Meta, error)` (pure-Go stub)
  - `goProcessor.Probe`: HEIF magic → `ErrHeicUnavailable` (do not `ErrUnsupported`)

- [ ] **Step 1: Write the failing tests**

Create `internal/imaging/heif_test.go`:

```go
package imaging

import (
	"bytes"
	"errors"
	"testing"
)

func ftypBox(brand string) []byte {
	// size(4) + "ftyp" + brand(4) + minor(4)
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], brand)
	return b
}

func TestSniffHEIFBrands(t *testing.T) {
	for _, brand := range []string{"heic", "heix", "heif", "mif1", "msf1", "heis"} {
		if !SniffHEIF(ftypBox(brand)) {
			t.Errorf("brand %s: want true", brand)
		}
	}
	if SniffHEIF(ftypBox("isom")) {
		t.Error("isom must not sniff as HEIF")
	}
	if SniffHEIF([]byte("MZ not iso bmff")) {
		t.Error("random bytes")
	}
	if SniffHEIF(nil) || SniffHEIF([]byte("ftyp")) {
		t.Error("short buffer")
	}
}

func TestHEIFAllowExt(t *testing.T) {
	if g := HEIFAllowExt("IMG_1.HEIC"); g != "heic" {
		t.Errorf("HEIC → %q", g)
	}
	if g := HEIFAllowExt("a.heif"); g != "heif" {
		t.Errorf("heif → %q", g)
	}
	if g := HEIFAllowExt("noext"); g != "heic" {
		t.Errorf("none → %q", g)
	}
}

func TestProbeHEIFMagicIsUnavailableOnPureGo(t *testing.T) {
	if HeicDecodeAvailable() {
		t.Skip("vips+heif build")
	}
	_, err := NewGo().Probe(bytes.NewReader(ftypBox("heic")))
	if !errors.Is(err, ErrHeicUnavailable) {
		t.Errorf("err=%v, want ErrHeicUnavailable", err)
	}
}

func TestDecodeHEIFToJPEGUnavailableOnPureGo(t *testing.T) {
	if HeicDecodeAvailable() {
		t.Skip("vips+heif build")
	}
	_, _, err := DecodeHEIFToJPEG(ftypBox("heic"), 90)
	if !errors.Is(err, ErrHeicUnavailable) {
		t.Errorf("err=%v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/imaging/ -count=1 -run 'TestSniffHEIFBrands|TestHEIFAllowExt|TestProbeHEIFMagicIsUnavailableOnPureGo|TestDecodeHEIFToJPEGUnavailableOnPureGo'
```

Expected: FAIL (`SniffHEIF` undefined).

- [ ] **Step 3: Implement sniff + stubs + Probe**

Create `internal/imaging/heif.go` (**no** `vips` build tag — always compiled):

```go
package imaging

import (
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
)

// ErrHeicUnavailable 魔数已认出 HEIF，但本进程没有可用的 HEIF 解码器。
var ErrHeicUnavailable = errors.New("imaging: HEIC requires libvips+libheif")

var heifBrands = map[string]struct{}{
	"heic": {}, "heix": {}, "hevc": {}, "hevx": {},
	"heim": {}, "heis": {}, "hevm": {}, "hevs": {},
	"mif1": {}, "msf1": {}, "heif": {},
}

// SniffHEIF 识别 ISO-BMFF ftyp 品牌。文件名不可靠（iCloud 导出、无后缀）。
func SniffHEIF(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	if string(b[4:8]) != "ftyp" {
		return false
	}
	brand := string(b[8:12])
	if _, ok := heifBrands[brand]; ok {
		return true
	}
	// compatible brands start at offset 16, 4 bytes each
	for i := 16; i+4 <= len(b); i += 4 {
		if _, ok := heifBrands[string(b[i:i+4])]; ok {
			return true
		}
	}
	return false
}

// HEIFAllowExt 组白名单用的原始后缀：.heif → heif，其余（含无后缀）→ heic。
func HEIFAllowExt(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	if ext == "heif" {
		return "heif"
	}
	return "heic"
}

func readProbePrefix(r io.Reader) ([]byte, io.Reader, error) {
	buf := make([]byte, 64)
	n, err := io.ReadFull(r, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		buf = buf[:n]
		err = nil
	}
	if err != nil {
		return nil, nil, err
	}
	return buf, io.MultiReader(bytes.NewReader(buf), r), nil
}
```

In the **same file**, pure-Go stubs. Split stubs into `heif_stub.go` with `//go:build !vips` so vips build can provide real funcs:

Create `internal/imaging/heif_stub.go`:

```go
//go:build !vips

package imaging

// HeicDecodeAvailable 纯 Go 构建无 HEIF 解码器。
func HeicDecodeAvailable() bool { return false }

// DecodeHEIFToJPEG 纯 Go 构建不可用。
func DecodeHEIFToJPEG(_ []byte, _ int) ([]byte, Meta, error) {
	return nil, Meta{}, ErrHeicUnavailable
}
```

Keep `readProbePrefix` / `SniffHEIF` in `heif.go` (all builds).

Modify `goProcessor.Probe` in `internal/imaging/imaging.go` — replace the function body with:

```go
func (goProcessor) Probe(r io.Reader) (Meta, error) {
	prefix, rest, err := readProbePrefix(r)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	if SniffHEIF(prefix) {
		return Meta{}, ErrHeicUnavailable
	}
	cfg, format, err := image.DecodeConfig(rest)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	fm, ok := formatMeta[format]
	if !ok {
		return Meta{}, ErrUnsupported
	}
	return Meta{Width: cfg.Width, Height: cfg.Height, MIME: fm.mime, Ext: fm.ext}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/imaging/ -count=1 -run 'TestSniffHEIFBrands|TestHEIFAllowExt|TestProbeHEIFMagicIsUnavailableOnPureGo|TestDecodeHEIFToJPEGUnavailableOnPureGo|TestProbePNG|TestProbeRejectsNonImage'
```

Expected: PASS. Existing PNG probe still works (prefix + MultiReader).

- [ ] **Step 5: Commit**

```bash
git add internal/imaging/heif.go internal/imaging/heif_stub.go internal/imaging/heif_test.go internal/imaging/imaging.go
git commit -m "$(cat <<'EOF'
feat(imaging): sniff HEIF and fail closed on pure-Go

Recognize ISO-BMFF HEIC/HEIF brands and return ErrHeicUnavailable
when libheif is not available. Filename suffix maps to heic vs heif
for group allowlists.
EOF
)"
```

---

### Task 2: vips runtime probe + HEIF→JPEG

**Files:**
- Create: `internal/imaging/heif_vips.go` (`//go:build vips`)
- Create: `internal/imaging/heif_vips_test.go` (`//go:build vips`)
- Modify: `internal/imaging/vips.go` (`vipsProcessor.Probe`)

**Interfaces:**
- Consumes: `SniffHEIF`, `ErrHeicUnavailable`, `readProbePrefix`, `Meta`, `clampJPEGQuality`
- Produces:
  - `func HeicDecodeAvailable() bool` — runtime; false if vips init fails or no HEIF loader
  - `func DecodeHEIFToJPEG(data []byte, jpegQuality int) ([]byte, Meta, error)` — autorot, JPEG, `Meta.Ext=="jpg"`, `Meta.MIME=="image/jpeg"`
  - `vipsProcessor.Probe` on HEIF magic: unavailable → `ErrHeicUnavailable`; else dimensions from vips; `Ext` stays `"heic"` / caller applies `HEIFAllowExt`

Do **not** call `vips_heifload_buffer` by name (may be missing at compile time). Use `vips_image_new_from_buffer` + `vips_foreign_find_load_buffer`.

- [ ] **Step 1: Write the vips tests**

Create `internal/imaging/heif_vips_test.go`:

```go
//go:build vips

package imaging

import (
	"bytes"
	"errors"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHeicDecodeAvailableIsRuntime(t *testing.T) {
	_ = HeicDecodeAvailable() // must not panic
}

func TestDecodeHEIFToJPEGRoundtrip(t *testing.T) {
	if !HeicDecodeAvailable() {
		t.Skip("no HEIF loader in this libvips")
	}
	src := makeTinyHEIF(t)
	out, meta, err := DecodeHEIFToJPEG(src, 90)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Ext != "jpg" || meta.MIME != "image/jpeg" {
		t.Errorf("meta=%+v", meta)
	}
	if meta.Width < 1 || meta.Height < 1 {
		t.Errorf("dims %+v", meta)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatal("not jpeg:", err)
	}
	if SniffHEIF(out) {
		t.Fatal("output still sniffs as HEIF")
	}
}

func TestVipsProbeHEIF(t *testing.T) {
	if !HeicDecodeAvailable() {
		_, err := NewVips().Probe(bytes.NewReader(ftypBox("heic")))
		if !errors.Is(err, ErrHeicUnavailable) {
			t.Fatalf("err=%v, want ErrHeicUnavailable", err)
		}
		return
	}
	src := makeTinyHEIF(t)
	m, err := NewVips().Probe(bytes.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.Width < 1 || m.Height < 1 {
		t.Errorf("meta=%+v", m)
	}
	if m.Ext != "heic" {
		t.Errorf("probe ext=%q, want heic (allowlist applied in upload)", m.Ext)
	}
}

func makeTinyHEIF(t *testing.T) []byte {
	t.Helper()
	png := alphaPNG(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	out := filepath.Join(dir, "out.heic")
	if err := os.WriteFile(in, png, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("vips", "copy", in, out)
	if err := cmd.Run(); err != nil {
		t.Skip("cannot encode HEIF via vips copy:", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !SniffHEIF(b) {
		t.Skip("vips copy did not produce HEIF")
	}
	return b
}
```

- [ ] **Step 2: Run on pure-Go to ensure tagged file is ignored**

```bash
go test ./internal/imaging/ -count=1
```

Expected: PASS (file skipped by build tag).

- [ ] **Step 3: Implement `heif_vips.go`**

```go
//go:build vips

package imaging

/*
#cgo pkg-config: vips
#include <vips/vips.h>
#include <stdlib.h>

static int heif_loader_present(void *in, size_t in_len) {
	const char *loader = vips_foreign_find_load_buffer(in, in_len);
	return loader != NULL;
}

static int heif_probe(void *in, size_t in_len, int *w, int *h) {
	VipsImage *im = vips_image_new_from_buffer(in, in_len, "", NULL);
	if (!im) {
		return -1;
	}
	*w = vips_image_get_width(im);
	*h = vips_image_get_height(im);
	g_object_unref(im);
	return 0;
}

static int heif_to_jpeg(void *in, size_t in_len, int q, void **out, size_t *out_len, int *w, int *h) {
	VipsImage *im = NULL;
	VipsImage *rot = NULL;
	int ret = -1;
	if (q < 1) q = 1;
	if (q > 100) q = 100;
	im = vips_image_new_from_buffer(in, in_len, "", NULL);
	if (!im) {
		return -1;
	}
	if (vips_autorot(im, &rot, NULL)) {
		goto done;
	}
	if (vips_jpegsave_buffer(rot, out, out_len, "Q", q, "strip", TRUE, NULL)) {
		goto done;
	}
	*w = vips_image_get_width(rot);
	*h = vips_image_get_height(rot);
	ret = 0;
done:
	if (rot) g_object_unref(rot);
	if (im) g_object_unref(im);
	return ret;
}
*/
import "C"

import (
	"bytes"
	"io"
	"sync"
	"unsafe"
)

var (
	heifAvailOnce sync.Once
	heifAvail     bool
)

// HeicDecodeAvailable vips 构建：运行时探测 HEIF loader（无 libheif 则为 false）。
func HeicDecodeAvailable() bool {
	heifAvailOnce.Do(func() {
		if err := ensureVips(); err != nil {
			return
		}
		probe := []byte{
			0, 0, 0, 16,
			'f', 't', 'y', 'p',
			'h', 'e', 'i', 'c',
			0, 0, 0, 0,
		}
		in := C.CBytes(probe)
		defer C.free(in)
		heifAvail = C.heif_loader_present(in, C.size_t(len(probe))) != 0
		if !heifAvail {
			C.vips_error_clear()
		}
	})
	return heifAvail
}

func DecodeHEIFToJPEG(data []byte, jpegQuality int) ([]byte, Meta, error) {
	if len(data) == 0 || !SniffHEIF(data) {
		return nil, Meta{}, ErrUnsupported
	}
	if !HeicDecodeAvailable() {
		return nil, Meta{}, ErrHeicUnavailable
	}
	if err := ensureVips(); err != nil {
		return nil, Meta{}, ErrHeicUnavailable
	}
	q := clampJPEGQuality(jpegQuality)
	in := C.CBytes(data)
	defer C.free(in)
	var out unsafe.Pointer
	var outLen C.size_t
	var w, h C.int
	if C.heif_to_jpeg(in, C.size_t(len(data)), C.int(q), &out, &outLen, &w, &h) != 0 {
		C.vips_error_clear()
		return nil, Meta{}, ErrUnsupported
	}
	if out == nil || outLen == 0 {
		return nil, Meta{}, ErrUnsupported
	}
	buf := C.GoBytes(out, C.int(outLen))
	C.g_free(C.gpointer(out))
	return buf, Meta{Width: int(w), Height: int(h), MIME: "image/jpeg", Ext: "jpg"}, nil
}

func probeHEIF(data []byte) (Meta, error) {
	if !HeicDecodeAvailable() {
		return Meta{}, ErrHeicUnavailable
	}
	if err := ensureVips(); err != nil {
		return Meta{}, ErrHeicUnavailable
	}
	in := C.CBytes(data)
	defer C.free(in)
	var w, h C.int
	if C.heif_probe(in, C.size_t(len(data)), &w, &h) != 0 {
		C.vips_error_clear()
		return Meta{}, ErrUnsupported
	}
	if int(w) < 1 || int(h) < 1 {
		return Meta{}, ErrUnsupported
	}
	return Meta{Width: int(w), Height: int(h), MIME: "image/heic", Ext: "heic"}, nil
}

func (vipsProcessor) Probe(r io.Reader) (Meta, error) {
	prefix, rest, err := readProbePrefix(r)
	if err != nil {
		return Meta{}, ErrUnsupported
	}
	if SniffHEIF(prefix) {
		data, rerr := io.ReadAll(io.MultiReader(bytes.NewReader(prefix), rest))
		if rerr != nil {
			return Meta{}, ErrUnsupported
		}
		return probeHEIF(data)
	}
	return goProcessor{}.Probe(io.MultiReader(bytes.NewReader(prefix), rest))
}
```

**If `vipsProcessor.Probe` already exists** in `vips.go`, **delete** that method from `vips.go` so the new one is the only definition. Keep `vipsProcessor.Thumbnail` in `vips.go`.

Note: `goProcessor.Probe` already sniffs and returns `ErrHeicUnavailable`. Calling it from vipsProcessor for non-HEIF is fine. For HEIF, vipsProcessor must **not** call `goProcessor.Probe`.

- [ ] **Step 4: Run vips tests**

```bash
CGO_ENABLED=1 go test -tags vips ./internal/imaging/ -count=1 -run 'TestHeicDecodeAvailableIsRuntime|TestDecodeHEIFToJPEGRoundtrip|TestVipsProbeHEIF|TestProbePNG'
```

Expected: PASS, or Skip when loader/encoder missing (not FAIL). If compile error on `vips_foreign_find_load_buffer` / `vips_image_new_from_buffer` / `vips_autorot` / `vips_jpegsave_buffer`, fix signatures against the machine’s `vips.h` (still generic, no `heifload` symbol).

- [ ] **Step 5: Commit**

```bash
git add internal/imaging/heif_vips.go internal/imaging/heif_vips_test.go internal/imaging/vips.go
git commit -m "$(cat <<'EOF'
feat(imaging): HEIF decode via libvips when the loader exists

Runtime-probe the HEIF loader; convert to JPEG with autorot.
Builds without libheif still compile and report unavailable.
EOF
)"
```

---

### Task 3: Upload Save transcode-before-hash

**Files:**
- Modify: `internal/service/upload/upload.go` (add `ErrHeicUnavailable`)
- Modify: `internal/service/upload/save.go`
- Modify: `internal/service/upload/upload_test.go`

**Interfaces:**
- Consumes: `imaging.SniffHEIF`, `imaging.HEIFAllowExt`, `imaging.HeicDecodeAvailable`, `imaging.DecodeHEIFToJPEG`, `imaging.ErrHeicUnavailable`, `Processing.EffectiveJPEGQuality`
- Produces: `var ErrHeicUnavailable = errors.New("upload: 当前构建无法解码 HEIC")` wrapping or distinct sentinel mapped in Task 4

In `save.go`, **after** `probe` and **before** `extAllowed` / `burn`:

1. If `errors.Is(err, imaging.ErrHeicUnavailable)` → `return nil, ErrHeicUnavailable`
2. If probe ok and `SniffHEIF` of temp file (or `meta.MIME == "image/heic"`): `allow = HEIFAllowExt(filename)` else `allow = meta.Ext`
3. `extAllowed(group, allow)` as today
4. If HEIF family: `DecodeHEIFToJPEG` using `EffectiveJPEGQuality` from processing settings (same Get as `burn`; if settings missing, `DefaultProcessing()`). Write temp. Re-probe. Refresh `size`. If decode returns `ErrHeicUnavailable`, map to `ErrHeicUnavailable`. If `ErrUnsupported`, map to `ErrInvalidImage`. Then fall through to existing `burn`.
5. Apply `MaxDimension` on post-decode meta.

Do not convert before allowlist.

- [ ] **Step 1: Write failing upload tests**

Add to `internal/service/upload/upload_test.go` (reuse `setup`, `pngFile` helpers already in that file):

```go
func TestSaveHEIFUnavailableOnPureGo(t *testing.T) {
	if imaging.HeicDecodeAvailable() {
		t.Skip("vips+heif")
	}
	svc, u, _ := setup(t)
	p := filepath.Join(t.TempDir(), "x.heic")
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], "heic")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Save(context.Background(), p, "IMG.HEIC", u, Opts{Visibility: "public"}, "")
	if !errors.Is(err, ErrHeicUnavailable) {
		t.Errorf("err=%v, want ErrHeicUnavailable", err)
	}
}

func TestSaveHEIFExtNotAllowed(t *testing.T) {
	svc, u, _ := setup(t)
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("allowed_exts", `["png","jpg","jpeg","gif","webp"]`)
	p := filepath.Join(t.TempDir(), "x.heic")
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], "heic")
	os.WriteFile(p, b, 0o644)
	_, err := svc.Save(context.Background(), p, "IMG.HEIC", u, Opts{Visibility: "public"}, "")
	if imaging.HeicDecodeAvailable() {
		if !errors.Is(err, ErrExtNotAllowed) {
			t.Errorf("err=%v, want ExtNotAllowed when group lacks heic", err)
		}
		return
	}
	if !errors.Is(err, ErrHeicUnavailable) && !errors.Is(err, ErrExtNotAllowed) {
		t.Errorf("err=%v, want HeicUnavailable or ExtNotAllowed", err)
	}
}
```

**Order in Save must check allowlist before unavailable if the group lacks heic** — spec H3: existing groups get `ext_not_allowed`. So even on pure-Go, **allowlist first**:

Correct order:

```
prefix sniff on temp file OR probe
allowExt := meta.Ext
if SniffHEIF(fileBytes prefix) { allowExt = HEIFAllowExt(filename) }
if !extAllowed(group, allowExt) { return ErrExtNotAllowed }
if HEIF {
  if !HeicDecodeAvailable() { return ErrHeicUnavailable }
  DecodeHEIFToJPEG ...
}
else {
  if probe failed { return ErrInvalidImage }
}
```

On pure-Go, Probe currently errors before we have Meta. **Do not use Probe as the only sniff.** Read 64 bytes from temp (or `imaging.SniffHEIF` after `os.ReadFile` of a prefix).

Implementation sketch for `save.go` (insert after size/MaxFileSize check, replacing the current probe block):

```go
	prefix, err := readPrefix(tmpPath, 64)
	if err != nil {
		return nil, err
	}
	heif := imaging.SniffHEIF(prefix)
	var meta imaging.Meta
	if heif {
		allow := imaging.HEIFAllowExt(filename)
		if !extAllowed(group.AllowedExts, allow) {
			return nil, ErrExtNotAllowed
		}
		if !imaging.HeicDecodeAvailable() {
			return nil, ErrHeicUnavailable
		}
		raw, rerr := os.ReadFile(tmpPath)
		if rerr != nil {
			return nil, rerr
		}
		var proc Processing
		if gerr := s.st.Get(model.SettingProcessing, &proc); gerr != nil {
			if !errors.Is(gerr, settings.ErrNotFound) {
				return nil, gerr
			}
			proc = DefaultProcessing()
		}
		jpeg, jmeta, jerr := imaging.DecodeHEIFToJPEG(raw, proc.EffectiveJPEGQuality())
		if errors.Is(jerr, imaging.ErrHeicUnavailable) {
			return nil, ErrHeicUnavailable
		}
		if jerr != nil {
			return nil, ErrInvalidImage
		}
		if jmeta.Width > MaxDimension || jmeta.Height > MaxDimension {
			return nil, ErrDimensionOver
		}
		if err := os.WriteFile(tmpPath, jpeg, 0o600); err != nil {
			return nil, err
		}
		meta = jmeta
		size = int64(len(jpeg))
	} else {
		meta, err = s.probe(tmpPath)
		if err != nil {
			return nil, ErrInvalidImage
		}
		if meta.Width > MaxDimension || meta.Height > MaxDimension {
			return nil, ErrDimensionOver
		}
		if !extAllowed(group.AllowedExts, meta.Ext) {
			return nil, ErrExtNotAllowed
		}
	}
	// existing burn(tmpPath, meta.Ext, u) ...
```

Add helper `readPrefix` in `fileops.go`:

```go
func readPrefix(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return buf[:got], nil
	}
	if err != nil {
		return nil, err
	}
	return buf, nil
}
```

Then **fix TestSaveHEIFExtNotAllowed** expected value: **always** `ErrExtNotAllowed` when group lacks heic, even on pure-Go.

```go
func TestSaveHEIFExtNotAllowed(t *testing.T) {
	svc, u, _ := setup(t)
	svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).Update("allowed_exts", `["png","jpg","jpeg","gif","webp"]`)
	p := filepath.Join(t.TempDir(), "x.heic")
	b := make([]byte, 16)
	b[3] = 16
	copy(b[4:8], "ftyp")
	copy(b[8:12], "heic")
	os.WriteFile(p, b, 0o644)
	_, err := svc.Save(context.Background(), p, "IMG.HEIC", u, Opts{Visibility: "public"}, "")
	if !errors.Is(err, ErrExtNotAllowed) {
		t.Errorf("err=%v, want ErrExtNotAllowed", err)
	}
}
```

Add `ErrHeicUnavailable` to `upload.go` var block:

```go
ErrHeicUnavailable = errors.New("upload: 当前构建无法解码 HEIC")
```

If `setup(t)` already seeds default group with `heic` after Task 5, Task 3 tests that mutate `allowed_exts` still isolate H3.

Optional vips test file `upload_heif_vips_test.go` with `//go:build vips`: allow `heic` on the group, Save real tiny HEIF (same `vips copy` trick), assert `res.Image.Ext == "jpg" || res.Image.Ext == "webp"` and `!strings.HasSuffix(res.Image.Ext, "heic")`. Skip if `!HeicDecodeAvailable()`.

- [ ] **Step 2: Run tests — expect fail**

```bash
go test ./internal/service/upload/ -count=1 -run 'TestSaveHEIFUnavailableOnPureGo|TestSaveHEIFExtNotAllowed'
```

Expected: FAIL until `ErrHeicUnavailable` / Save order exist.

- [ ] **Step 3: Implement Save + helper + sentinel**

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/service/upload/ -count=1
```

Expected: PASS. Then if vips available:

```bash
CGO_ENABLED=1 go test -tags vips ./internal/service/upload/ ./internal/imaging/ -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/service/upload/upload.go internal/service/upload/save.go internal/service/upload/fileops.go internal/service/upload/upload_test.go internal/service/upload/upload_heif_vips_test.go
git commit -m "$(cat <<'EOF'
feat(upload): transcode HEIC to JPEG before hash

Allowlist uses original heic/heif; stored bytes are JPEG/WebP so
instant reuse and /i never see HEIF. Pure-Go returns a distinct error.
EOF
)"
```

---

### Task 4: HTTP, i18n, seed, capabilities

**Files:**
- Modify: `internal/handler/respond.go` — add `CodeHeicUnsupported = "heic_unsupported"`
- Modify: `internal/handler/upload.go` — map `upload.ErrHeicUnavailable` → 415 + that code + spec zh message
- Modify: `internal/handler/admin_health.go` — `"heic_decode": imaging.HeicDecodeAvailable()`
- Modify: `internal/service/adminsvc/settings.go` — capabilities map add `heic_decode`
- Modify: `internal/model/db.go` — default + guest `AllowedExts` append `"heic","heif"`
- Modify: `web/src/pages/admin/groups/GroupsPage.tsx` — `NEW_FORM.exts` same list
- Modify: `web/src/i18n/locales/zh/errors.ts` and `en/errors.ts`
- Modify: `web/src/i18n/errorText.ts` — treat `heic_unsupported` like `expires_over_group` (always i18n)
- Modify: `web/src/api/types.ts` `ProcessingCapabilities` add `heic_decode: boolean`
- Modify: `web/src/api/adminHooks.ts` runtime type
- Modify: `web/src/i18n/locales/{zh,en}/adminA.ts` + `SystemPage.tsx`
- Modify: tests: `internal/server/api_admin_health_test.go`, any seed/group tests that hardcode five exts
- Modify: `web/src/pages/admin/settings/settingsForm.ts` if it should surface the flag (read-only hint next to WebP is enough on System page)

**Produces:** H2/H3 API; new installs allow HEIC; admin sees capability.

Zh message (exact): `当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）`

En i18n: `This build cannot decode HEIC. Use the official Docker image or make build-vips (libheif required).`

- [ ] **Step 1: Write/extend failing tests**

In a handler upload test (follow existing `internal/server/api_upload_test.go` style): upload the 16-byte ftyp fixture as `file` with name `a.heic` **after** adding heic to the group (or on a fresh seed from later in this task). Expect 415 and `code=heic_unsupported` on pure-Go.

If this task also changes seed, split: first map the error (group manually includes heic), then change seed and assert `TestDB` default group contains `heic` and `heif`.

Add health test field `HeicDecode bool` and: pure-go → false; vips without loader may still be false.

- [ ] **Step 2: Run — expect fail**

```bash
go test ./internal/handler/ ./internal/server/ ./internal/model/ -count=1
```

- [ ] **Step 3: Implement mappings, seed, UI strings**

`failUpload` case:

```go
case errors.Is(err, upload.ErrHeicUnavailable):
    Fail(w, http.StatusUnsupportedMediaType, CodeHeicUnsupported, "当前构建无法解码 HEIC，请使用官方 Docker 镜像或 make build-vips（需 libheif）")
```

`errorText.ts` special-case:

```ts
if (code === 'expires_over_group' || code === 'max_views_over_group' || code === 'heic_unsupported') {
```

System page: clone the `webp_encode` row for `heic_decode` with `t('adminA.heicDecode')` = `HEIC 解码` / `HEIC decode`.

Update `imagingHintVips` / `imagingHintPureGo` one clause: vips still needs libheif for HEIC; pure-Go cannot decode HEIC.

- [ ] **Step 4: Tests + frontend unit**

```bash
go test ./internal/handler/ ./internal/server/ ./internal/model/ ./internal/service/adminsvc/ -count=1
cd web && npx vitest run src/i18n src/pages/admin/system src/pages/admin/groups src/pages/admin/settings src/upload
```

Expected: PASS. Fix any snapshot/ext-list assertions (`GroupsPage` tests, `api_config_test` guest `allowed_exts`).

- [ ] **Step 5: Commit**

```bash
git add internal/handler/respond.go internal/handler/upload.go internal/handler/admin_health.go internal/service/adminsvc/settings.go internal/model/db.go internal/server/api_admin_health_test.go web/src
git commit -m "$(cat <<'EOF'
feat(upload): heic_unsupported API and default-group HEIC

Map the vips-missing path to 415, show heic_decode in admin health,
and seed new default/guest groups with heic/heif. Existing groups
are unchanged.
EOF
)"
```

---

### Task 5: `/t?w=` whitelist

**Files:**
- Create: `internal/handler/thumbwidth.go`
- Modify: `internal/handler/serve.go` (use helpers; delete local `allowedThumbWidths` map)
- Modify: `internal/handler/serve_width_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  - `var AllowedThumbWidths = []int{120, 200, 240, 400, 480, 800, 960, 1600}`
  - `func ThumbWidthAllowed(n int) bool`
  - `func ThumbWidthHint() string` → `120、200、240、400、480、800、960 或 1600`

JSON error: `fmt.Sprintf("w 须为 %s", ThumbWidthHint())` for both invalid int and not-in-set branches (today two identical strings).

Keep 200/400/800. Do not change `WidthThumbKey` or JPEG content-type.

- [ ] **Step 1: Failing test**

Extend `TestThumbWidthWhitelist`:

```go
func TestThumbWidthWhitelist(t *testing.T) {
	fx := newServeFixture(t)
	for _, w := range []int{120, 200, 240, 400, 480, 800, 960, 1600} {
		rec := fx.get(fmt.Sprintf("/t/%s?w=%d", fx.name, w), nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("w=%d: %d body=%s", w, rec.Code, rec.Body.String())
		}
	}
	recBad := fx.get("/t/"+fx.name+"?w=300", map[string]string{"Accept": "application/json"})
	if recBad.Code != http.StatusBadRequest {
		t.Fatalf("w=300: %d", recBad.Code)
	}
	if !bytes.Contains(recBad.Body.Bytes(), []byte("1600")) {
		t.Fatalf("hint missing 1600: %s", recBad.Body.String())
	}
	// existing w=123 / abc / default thumb / WidthThumbKey assertions remain
}
```

Add imports `bytes` and `fmt`.

- [ ] **Step 2: Run — fail on 120/1600 (currently 400)**

```bash
go test ./internal/handler/ -count=1 -run TestThumbWidthWhitelist
```

- [ ] **Step 3: Implement `thumbwidth.go` and switch `serve.go`**

```go
package handler

import "strconv"

var AllowedThumbWidths = []int{120, 200, 240, 400, 480, 800, 960, 1600}

func ThumbWidthAllowed(n int) bool {
	for _, w := range AllowedThumbWidths {
		if w == n {
			return true
		}
	}
	return false
}

func ThumbWidthHint() string {
	if len(AllowedThumbWidths) == 0 {
		return ""
	}
	s := ""
	for i, w := range AllowedThumbWidths {
		switch {
		case i == 0:
			s = strconv.Itoa(w)
		case i == len(AllowedThumbWidths)-1:
			s += " 或 " + strconv.Itoa(w)
		default:
			s += "、" + strconv.Itoa(w)
		}
	}
	return s
}
```

In `Thumbnail`, replace map lookup and hard-coded strings with `ThumbWidthAllowed` / `"w 须为 "+ThumbWidthHint()`.

- [ ] **Step 4: Run**

```bash
go test ./internal/handler/ -count=1 -run 'TestThumbWidth|TestServe'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/thumbwidth.go internal/handler/serve.go internal/handler/serve_width_test.go
git commit -m "$(cat <<'EOF'
feat(serve): expand /t?w= whitelist

Allow 120, 240, 480, 960, 1600 in addition to 200, 400, 800.
Invalid w still 400 with a generated hint. Disk keys unchanged.
EOF
)"
```

---

### Task 6: CLI, Docker loader, docs

**Files:**
- Modify: `cmd/imgli/import_dir.go` — `importExts` add `".heic": {}, ".heif": {}`
- Test: if there is `import_dir` test, update; else add a tiny test that the map contains those keys (table in `cmd/imgli` is in `package main` — a `import_dir_test.go` in the same package can assert `_, ok := importExts[".heic"]`)
- Modify: `Dockerfile` runtime `apk add`: keep `vips su-exec ca-certificates tzdata`, add `libheif`. If the Alpine 3.20 repo provides `vips-heif`, add it too (plugin split). Build stage already has `vips-dev`; add `libheif-dev` so cgo/vips is built with heif headers if needed (runtime loader is still the `.so`).
- Verify command (must run before claiming done):

```bash
make docker-build
docker run --rm --entrypoint imgli imgli:local version
# Probe loader: start container, POST a HEIF if testdata exists; or
docker run --rm --entrypoint sh imgli:local -c 'ls /usr/lib/libheif* 2>/dev/null; vips -l 2>/dev/null | grep -i heif || true'
```

If `HeicDecodeAvailable` is still false in the image, the Dockerfile is wrong — fix packages until a vips-tagged binary inside the image returns true **or** `vips copy in.png /tmp/x.heic` works in the image.

Do **not** skip this and ship.

- Modify: `README.md` + `README.zh-CN.md` transforms line: `/t/{key}?w=120|200|240|400|480|800|960|1600`. Features bullet: HEIC/HEIF upload on official Docker (converted to JPEG/WebP); GitHub pure-Go archives reject with `heic_unsupported`.
- Modify: `docs/integrations/README.md` table cell for `/t`.
- Modify: `CHANGELOG.md` `[Unreleased]`:

```markdown
## [Unreleased]

Theme: **HEIC ingest · more `/t` widths**.

### Added

- **HEIC / HEIF upload (vips + libheif):** decoded and stored as JPEG (then the existing processing pipeline, including optional original WebP). New default/guest groups allow `heic` and `heif`. Pure-Go binaries return `415 heic_unsupported`. Existing groups are unchanged.

### Changed

- **`/t?w=` whitelist:** `120`, `200`, `240`, `400`, `480`, `800`, `960`, `1600`. 200/400/800 keep working.
```

No S3 matrix edits.

- [ ] **Step 1: import-dir test then map**

```go
// cmd/imgli/import_dir_test.go
package main

import "testing"

func TestImportExtsIncludeHEIF(t *testing.T) {
	if _, ok := importExts[".heic"]; !ok {
		t.Fatal("missing .heic")
	}
	if _, ok := importExts[".heif"]; !ok {
		t.Fatal("missing .heif")
	}
}
```

- [ ] **Step 2: fail then add map entries; `go test ./cmd/imgli/ -count=1`**

- [ ] **Step 3: Dockerfile packages; docker-build loader check**

Alpine 3.20 runtime line becomes:

```
RUN apk add --no-cache ca-certificates tzdata vips libheif su-exec \
```

If `vips-heif` exists (`apk search vips-heif` in the build container), add it.

- [ ] **Step 4: docs + CHANGELOG**

- [ ] **Step 5: Full verification**

```bash
go test ./... -count=1
# if libvips+heif on the machine:
CGO_ENABLED=1 go test -tags vips ./internal/imaging/ ./internal/service/upload/ -count=1
cd web && npx vitest run
make docker-build
```

- [ ] **Step 6: Commit**

```bash
git add cmd/imgli/import_dir.go cmd/imgli/import_dir_test.go Dockerfile README.md README.zh-CN.md docs/integrations/README.md CHANGELOG.md
git commit -m "$(cat <<'EOF'
feat(ops): HEIC in import-dir and Docker libheif

CLI sends .heic/.heif to the server. Official image installs the
HEIF loader. Docs and changelog list the new /t widths.
EOF
)"
```

---

## Spec coverage

| Spec | Task |
|------|------|
| Sniff ftyp brands; filename allowlist | 1 |
| Runtime `HeicDecodeAvailable`; autorot JPEG | 2 |
| Save before hash; burn; no stored HEIC | 3 |
| 415 `heic_unsupported`; i18n; new seed; health | 4 |
| Width union; generated hint; CDN `/i` untouched | 5 |
| import-dir; Docker loader gate; README/CHANGELOG | 6 |
| H1 Docker HEIC | 6 docker-build + 3 vips test |
| H2 pure-Go 415 | 3+4 |
| H3 existing group | 3 allowlist-first |
| H4 webp processing | existing `burn` after JPEG |
| H5 widths | 5 |
| H6 CDN 302 | no `/i` change |
| H7 import-dir | 6 |
| Non-goals (AVIF, `/i.webp`, S3, migrate groups) | Global Constraints |

## Placeholder / type check

- Sentinels: `imaging.ErrHeicUnavailable` vs `upload.ErrHeicUnavailable` — handler maps **upload** error only.
- `DecodeHEIFToJPEG(data []byte, jpegQuality int) ([]byte, Meta, error)` used in Tasks 2–3.
- `HEIFAllowExt` / `SniffHEIF` names stable.
- `CodeHeicUnsupported = "heic_unsupported"`.
- Widths exactly `120, 200, 240, 400, 480, 800, 960, 1600`.
