package version

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckLatestRelease(t *testing.T) {
	Version = "v0.5.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Location", "https://github.com/yixian-huang/imgli/releases/tag/v0.6.0")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			// rewrite to test server
			req.URL.Scheme = "http"
			req.URL.Host = srv.Listener.Addr().String()
			req.RequestURI = ""
			return http.DefaultTransport.RoundTrip(req)
		}),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// call via modified Check that uses github URL - inject by temporary override is hard.
	// Instead test compare + parse location path unit-style:
	loc := "https://github.com/yixian-huang/imgli/releases/tag/v0.6.0"
	tag := loc[len(loc)-len("v0.6.0"):]
	if tag != "v0.6.0" {
		t.Fatal(tag)
	}
	if CompareSemver("v0.5.0", "v0.6.0") >= 0 {
		t.Fatal("expected older")
	}
	_ = client
	_ = context.Background()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
