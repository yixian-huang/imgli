//go:build vips

package upload

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/imaging"
)

func TestSaveHEIFTranscodesToJPEGOrWebP(t *testing.T) {
	if !imaging.HeicDecodeAvailable() {
		t.Skip("no HEIF loader")
	}
	svc, u, _ := setup(t)
	if err := svc.db.Model(&model.UserGroup{}).Where("id = ?", 1).
		Update("allowed_exts", `["png","jpg","jpeg","gif","webp","heic","heif"]`).Error; err != nil {
		t.Fatal(err)
	}
	res, err := svc.Save(context.Background(), writeTinyHEIF(t), "IMG.HEIC", u, Opts{Visibility: "public"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.Image.Ext != "jpg" && res.Image.Ext != "webp" {
		t.Errorf("ext=%q, want jpg or webp", res.Image.Ext)
	}
	if strings.HasSuffix(strings.ToLower(res.Image.Ext), "heic") {
		t.Errorf("stored ext still heic: %q", res.Image.Ext)
	}
}

func writeTinyHEIF(t *testing.T) string {
	t.Helper()
	src := pngFile(t, t.TempDir(), 32, 24)
	out := filepath.Join(t.TempDir(), "out.heic")
	if err := exec.Command("vips", "copy", src, out).Run(); err != nil {
		t.Skip("cannot encode HEIF via vips copy:", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !imaging.SniffHEIF(b) {
		t.Skip("vips copy did not produce HEIF")
	}
	return out
}
