package storagesvc

import (
	"strings"
	"testing"

	"github.com/yixian-huang/imgli/internal/model"
)

// TestThumbKeyBySurface 缩略图键应按 surface 加前缀——回归 T-A:同 hash 跨 surface
// 两个独立 File 不应共享同一缩略图键(否则删其一,另一个 /t 404)。
func TestThumbKeyBySurface(t *testing.T) {
	pub := ThumbKey(model.SurfacePublic, "h")
	if !strings.HasPrefix(pub, "public/.thumbs/") {
		t.Errorf("public ThumbKey=%q 应以 public/.thumbs/ 为前缀", pub)
	}
	priv := ThumbKey(model.SurfacePrivate, "h")
	if !strings.HasPrefix(priv, "private/.thumbs/") {
		t.Errorf("private ThumbKey=%q 应以 private/.thumbs/ 为前缀", priv)
	}
	if pub == priv {
		t.Error("public/private 缩略图键不应相同")
	}
}

// TestThumbKeyCandidatesPublicIncludesLegacy public 探测顺序需兼容 S1 之前的扁平
// 遗留缩略图路径；private 是 S1 才引入的 surface,不存在遗留数据,且遗留路径是公开
// 可读位置——不应探测,防跨 surface 生命周期耦合(fail-closed)。
func TestThumbKeyCandidatesPublicIncludesLegacy(t *testing.T) {
	pubCandidates := ThumbKeyCandidates(model.SurfacePublic, "h")
	wantLegacy := []string{
		".thumbs/g" + ThumbGen + "/h.webp",
		".thumbs/g" + ThumbGen + "/h.jpg",
		".thumbs/h.webp",
		".thumbs/h.jpg",
	}
	for _, w := range wantLegacy {
		found := false
		for _, c := range pubCandidates {
			if c == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("public 候选缺遗留项 %q, got %v", w, pubCandidates)
		}
	}

	privCandidates := ThumbKeyCandidates(model.SurfacePrivate, "h")
	for _, c := range privCandidates {
		if !strings.HasPrefix(c, "private/.thumbs/") {
			t.Errorf("private 候选不应含非 private 前缀遗留项, got %q (all=%v)", c, privCandidates)
		}
	}
}

// TestThumbKeyEmptySurfaceIsPublic 空 surface ≈ public（migrateSurface / 重挂扫描同口径）。
// 若按 fail-closed 当成 private，公开图的 /t 会去 private/.thumbs 404。
func TestThumbKeyEmptySurfaceIsPublic(t *testing.T) {
	empty := ThumbKey("", "h")
	pub := ThumbKey(model.SurfacePublic, "h")
	if empty != pub {
		t.Errorf("空 surface ThumbKey=%q want %q", empty, pub)
	}
	cEmpty := ThumbKeyCandidates("", "h")
	cPub := ThumbKeyCandidates(model.SurfacePublic, "h")
	if len(cEmpty) != len(cPub) {
		t.Fatalf("空 surface 候选数=%d want %d %v", len(cEmpty), len(cPub), cEmpty)
	}
	for i := range cPub {
		if cEmpty[i] != cPub[i] {
			t.Errorf("候选[%d]=%q want %q", i, cEmpty[i], cPub[i])
		}
	}
}
