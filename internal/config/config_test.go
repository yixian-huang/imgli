package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":8686" {
		t.Errorf("Listen = %q, want :8686", cfg.Listen)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("Database.Driver = %q, want sqlite", cfg.Database.Driver)
	}
	if cfg.BaseURL != "http://localhost:8686" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
}

func TestLoadYAMLThenEnvOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "imgli.yaml")
	os.WriteFile(p, []byte("listen: \":9000\"\ndata_dir: /srv/imgli\ndatabase:\n  driver: postgres\n  dsn: postgres://a\n"), 0o644)

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9000" || cfg.Database.Driver != "postgres" {
		t.Errorf("yaml 未生效: %+v", cfg)
	}

	t.Setenv("IMGLI_LISTEN", ":9001")
	t.Setenv("IMGLI_DATABASE_DSN", "postgres://b")
	cfg, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != ":9001" {
		t.Errorf("env 未覆盖 yaml: Listen = %q", cfg.Listen)
	}
	if cfg.Database.DSN != "postgres://b" {
		t.Errorf("env 未覆盖 yaml: DSN = %q", cfg.Database.DSN)
	}
}

func TestSQLiteDefaultDSN(t *testing.T) {
	cfg, _ := Load("")
	want := filepath.Join("./data", "imgli.db")
	if got := cfg.SQLiteDefaultDSN(); got != want {
		t.Errorf("SQLiteDefaultDSN = %q, want %q", got, want)
	}
}

func TestTrustProxyEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "imgli.yaml")
	os.WriteFile(p, []byte("trust_proxy: true\n"), 0o644)

	t.Setenv("IMGLI_TRUST_PROXY", "false")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TrustProxy {
		t.Error("env false 应能覆盖 yaml true")
	}

	t.Setenv("IMGLI_TRUST_PROXY", "TRUE")
	cfg, _ = Load(p)
	if !cfg.TrustProxy {
		t.Error("ParseBool 应接受 TRUE")
	}
}

func TestServeCacheEnv(t *testing.T) {
	t.Setenv("IMGLI_SERVE_CACHE_DISABLED", "true")
	t.Setenv("IMGLI_SERVE_CACHE_MAX_BYTES", "1048576")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ServeCacheDisabled {
		t.Error("IMGLI_SERVE_CACHE_DISABLED=true 应关闭缓存")
	}
	if cfg.ServeCacheMaxBytes != 1048576 {
		t.Errorf("ServeCacheMaxBytes=%d", cfg.ServeCacheMaxBytes)
	}
}

func TestFetchAllowYAMLEntriesTrimmed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "imgli.yaml")
	os.WriteFile(p, []byte("fetch_allow:\n  - \"10.0.0.0/8 \"\n  - \" 192.168.1.5\"\n  - \"\"\n  - \"  \"\n"), 0o644)

	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"10.0.0.0/8", "192.168.1.5"}
	if len(cfg.FetchAllow) != len(want) {
		t.Fatalf("FetchAllow = %#v, want %#v", cfg.FetchAllow, want)
	}
	for i, w := range want {
		if cfg.FetchAllow[i] != w {
			t.Errorf("FetchAllow[%d] = %q, want %q", i, cfg.FetchAllow[i], w)
		}
	}
}
