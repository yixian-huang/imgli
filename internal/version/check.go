package version

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// UpdateCheck 探测结果（无密钥）。
type UpdateCheck struct {
	Current         string    `json:"current"`
	Latest          string    `json:"latest,omitempty"`
	UpdateAvailable bool      `json:"update_available"`
	ReleaseURL      string    `json:"release_url,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
	Error           string    `json:"error,omitempty"`
}

// CheckLatestRelease 用 GitHub releases/latest 重定向解析最新 tag（与 install.sh 同思路，免 API token）。
func CheckLatestRelease(ctx context.Context, repo string, client *http.Client) UpdateCheck {
	if repo == "" {
		repo = DefaultReleaseRepo
	}
	out := UpdateCheck{
		Current:   Version,
		CheckedAt: time.Now().UTC(),
	}
	if client == nil {
		client = &http.Client{
			Timeout: 12 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	url := "https://github.com/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	req.Header.Set("User-Agent", "imgli-update-check/"+Version)
	res, err := client.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer res.Body.Close()
	loc := res.Header.Get("Location")
	if loc == "" && res.StatusCode >= 300 && res.StatusCode < 400 {
		out.Error = fmt.Sprintf("no Location header (status %d)", res.StatusCode)
		return out
	}
	if loc == "" {
		// some environments return 200 with HTML; try GET Location still empty
		out.Error = fmt.Sprintf("unexpected status %d", res.StatusCode)
		return out
	}
	// .../releases/tag/v0.5.1
	tag := loc
	if i := strings.LastIndex(loc, "/"); i >= 0 {
		tag = loc[i+1:]
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		out.Error = "empty latest tag"
		return out
	}
	out.Latest = tag
	out.ReleaseURL = "https://github.com/" + repo + "/releases/tag/" + tag
	if Version == "dev" || Version == "" {
		out.UpdateAvailable = true
		return out
	}
	out.UpdateAvailable = CompareSemver(Version, tag) < 0
	return out
}
