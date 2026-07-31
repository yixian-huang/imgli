package version

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReleaseAssetName(t *testing.T) {
	_, _, asset, err := releaseAssetName("v0.5.1")
	if err != nil {
		t.Fatal(err)
	}
	if asset == "" {
		t.Fatal("empty asset")
	}
	// contains version without v and platform
	if !contains(asset, "0.5.1") {
		t.Fatalf("asset %s missing version", asset)
	}
	_ = runtime.GOOS
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("hello-imgli")
	asset := "imgli_0.0.0_Linux_x86_64.tar.gz"
	ap := filepath.Join(dir, asset)
	if err := os.WriteFile(ap, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	sumPath := filepath.Join(dir, "checksums.txt")
	line := hex.EncodeToString(sum[:]) + "  " + asset + "\n"
	if err := os.WriteFile(sumPath, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(ap, sumPath, asset); err != nil {
		t.Fatal(err)
	}
	// mismatch
	if err := os.WriteFile(ap, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(ap, sumPath, asset); err == nil {
		t.Fatal("want checksum error")
	}
}

func TestUpgradeRequiresConfirm(t *testing.T) {
	_, err := UpgradeBinary(context.Background(), DefaultReleaseRepo, "v0.0.0", false, nil)
	if err != ErrUpgradeNoConfirm {
		t.Fatalf("got %v", err)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
