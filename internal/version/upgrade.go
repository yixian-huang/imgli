package version

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrUpgradeDocker     = errors.New("version: docker/container install — use image pull / redeploy, not binary replace")
	ErrUpgradeNoConfirm  = errors.New("version: confirm=true required")
	ErrUpgradeChecksum   = errors.New("version: checksum mismatch")
	ErrUpgradeNotFound   = errors.New("version: release asset not found")
	ErrUpgradeNotAllowed = errors.New("version: upgrade not allowed in this environment")
)

// UpgradeResult 一键升级结果（无密钥）。
type UpgradeResult struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Executable string `json:"executable"`
	Restart    string `json:"restart"` // always "required" for binary path
	Mode       string `json:"mode"`    // binary | docker_blocked
	Message    string `json:"message,omitempty"`
}

// IsDockerish 粗判容器环境（有 /.dockerenv 或 cgroup 含 docker/containerd/kubepods）。
func IsDockerish() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	b, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		return false
	}
	s := string(b)
	return strings.Contains(s, "docker") || strings.Contains(s, "containerd") || strings.Contains(s, "kubepods")
}

// UpgradeBinary 下载指定 tag（空则 latest）的 release 资产、校验 checksum、替换当前可执行文件。
// confirm 必须为 true。Docker 环境直接 ErrUpgradeDocker。
func UpgradeBinary(ctx context.Context, repo, tag string, confirm bool, client *http.Client) (UpgradeResult, error) {
	out := UpgradeResult{From: Version, Mode: "binary", Restart: "required"}
	if !confirm {
		return out, ErrUpgradeNoConfirm
	}
	if IsDockerish() {
		out.Mode = "docker_blocked"
		out.Message = "Running in a container: pull a new image / redeploy; binary replace is disabled."
		return out, ErrUpgradeDocker
	}
	if repo == "" {
		repo = DefaultReleaseRepo
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	if tag == "" {
		chk := CheckLatestRelease(ctx, repo, nil)
		if chk.Error != "" || chk.Latest == "" {
			return out, fmt.Errorf("resolve latest: %s", chk.Error)
		}
		tag = chk.Latest
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	out.To = tag
	if Version != "dev" && CompareSemver(Version, tag) >= 0 {
		out.Message = "already at or newer than target"
		return out, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return out, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return out, err
	}
	out.Executable = exe

	goos, goarch, asset, err := releaseAssetName(tag)
	if err != nil {
		return out, err
	}
	_ = goos
	_ = goarch
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, tag)
	assetURL := base + "/" + asset
	sumURL := base + "/checksums.txt"

	tmpdir, err := os.MkdirTemp("", "imgli-upgrade-*")
	if err != nil {
		return out, err
	}
	defer os.RemoveAll(tmpdir)

	assetPath := filepath.Join(tmpdir, asset)
	if err := downloadFile(ctx, client, assetURL, assetPath); err != nil {
		return out, fmt.Errorf("%w: %v", ErrUpgradeNotFound, err)
	}
	sumPath := filepath.Join(tmpdir, "checksums.txt")
	if err := downloadFile(ctx, client, sumURL, sumPath); err != nil {
		return out, fmt.Errorf("checksums.txt: %w", err)
	}
	if err := verifySHA256(assetPath, sumPath, asset); err != nil {
		return out, err
	}
	binName := "imgli"
	if runtime.GOOS == "windows" {
		binName = "imgli.exe"
	}
	extracted, err := extractBinary(assetPath, tmpdir, binName)
	if err != nil {
		return out, err
	}
	if err := replaceExecutable(extracted, exe); err != nil {
		return out, err
	}
	out.Message = "binary replaced; restart the imgli process to load the new version"
	return out, nil
}

func releaseAssetName(tag string) (goos, goarch, asset string, err error) {
	goos = runtime.GOOS
	goarch = runtime.GOARCH
	var goreleaserOS, goreleaserArch string
	switch goos {
	case "linux":
		goreleaserOS = "Linux"
	case "darwin":
		goreleaserOS = "Darwin"
	case "windows":
		goreleaserOS = "Windows"
	default:
		return "", "", "", fmt.Errorf("unsupported GOOS %s", goos)
	}
	switch goarch {
	case "amd64":
		goreleaserArch = "x86_64"
	case "arm64":
		goreleaserArch = "arm64"
	default:
		return "", "", "", fmt.Errorf("unsupported GOARCH %s", goarch)
	}
	ver := strings.TrimPrefix(tag, "v")
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}
	asset = fmt.Sprintf("imgli_%s_%s_%s.%s", ver, goreleaserOS, goreleaserArch, ext)
	return goos, goarch, asset, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "imgli-upgrade/"+Version)
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", res.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func verifySHA256(assetPath, sumPath, assetName string) error {
	b, err := os.ReadFile(sumPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("%w: asset not in checksums.txt", ErrUpgradeChecksum)
	}
	f, err := os.Open(assetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("%w: want %s got %s", ErrUpgradeChecksum, want, got)
	}
	return nil
}

func extractBinary(archivePath, destDir, binName string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return "", errors.New("windows zip upgrade not supported in this build; use install script")
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		base := filepath.Base(hdr.Name)
		if base != binName {
			continue
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		outPath := filepath.Join(destDir, binName)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		return outPath, nil
	}
	return "", fmt.Errorf("binary %s not in archive", binName)
}

func replaceExecutable(src, dest string) error {
	dir := filepath.Dir(dest)
	tmp := filepath.Join(dir, ".imgli-upgrade-new")
	bak := dest + ".bak"
	// copy to same dir first (cross-device rename safe)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Remove(bak)
	if err := os.Rename(dest, bak); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		// try restore
		_ = os.Rename(bak, dest)
		os.Remove(tmp)
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}
