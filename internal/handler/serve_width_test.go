package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/yixian-huang/imgli/internal/service/storagesvc"
)

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
	rec123 := fx.get("/t/"+fx.name+"?w=123", map[string]string{"Accept": "application/json"})
	if rec123.Code != http.StatusBadRequest {
		t.Fatalf("bad w: %d body=%s", rec123.Code, rec123.Body.String())
	}
	recBad2 := fx.get("/t/"+fx.name+"?w=abc", map[string]string{"Accept": "application/json"})
	if recBad2.Code != http.StatusBadRequest {
		t.Fatalf("non-int w: %d", recBad2.Code)
	}
	recW0 := fx.get("/t/"+fx.name+"?w=0", map[string]string{"Accept": "application/json"})
	if recW0.Code != http.StatusBadRequest {
		t.Fatalf("w=0: %d", recW0.Code)
	}
	rec0 := fx.get("/t/"+fx.name, nil)
	if rec0.Code != http.StatusOK {
		t.Fatalf("default thumb: %d", rec0.Code)
	}
	key := storagesvc.WidthThumbKey("public", "abc", 400)
	if key != "public/.thumbs/w400/g"+storagesvc.ThumbGen+"/abc.jpg" {
		t.Fatalf("WidthThumbKey=%q", key)
	}
}
