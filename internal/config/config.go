// Package config 加载运维参数：默认值 → yaml 文件 → IMGLI_* 环境变量。
// 业务参数（站点名、注册模式等）不在此处，在 settings 表。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Database struct {
	Driver string `yaml:"driver"` // sqlite | postgres
	DSN    string `yaml:"dsn"`
}

type Config struct {
	Listen     string   `yaml:"listen"`
	BaseURL    string   `yaml:"base_url"` // 生成外链的基础地址，如 https://img.li
	DataDir    string   `yaml:"data_dir"`
	Database   Database `yaml:"database"`
	TrustProxy bool     `yaml:"trust_proxy"` // 为 true 时信任 X-Forwarded-For 最右值（可信反代追加的那跳）
	FetchAllow []string `yaml:"fetch_allow"` // URL 抓取额外放行的 host/CIDR（默认空=严格）
	// RateLimitMult 固定命名桶(auth/config/resend/forgot)每分钟限额的倍率，默认 1。
	// 供 e2e 等高并发自动化场景整体放宽(IMGLI_RATE_LIMIT_MULT),生产保持 1 不受影响。
	RateLimitMult float64 `yaml:"rate_limit_mult"`
	// ServeCacheDisabled 关闭 /t、未 302 的 /i 本地代理缓存（默认开启）。
	ServeCacheDisabled bool `yaml:"serve_cache_disabled"`
	// ServeCacheMaxBytes 缓存目录上限；0=512MiB。
	ServeCacheMaxBytes int64 `yaml:"serve_cache_max_bytes"`
}

// Load 解析配置。path 为空则只用默认值+环境变量；文件不存在视为错误。
func Load(path string) (*Config, error) {
	cfg := &Config{
		Listen:  ":8686",
		BaseURL: "http://localhost:8686",
		DataDir: "./data",
		Database: Database{
			Driver: "sqlite",
		},
		RateLimitMult: 1,
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件: %w", err)
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return nil, fmt.Errorf("解析配置文件: %w", err)
		}
	}
	overrideEnv(&cfg.Listen, "IMGLI_LISTEN")
	overrideEnv(&cfg.BaseURL, "IMGLI_BASE_URL")
	overrideEnv(&cfg.DataDir, "IMGLI_DATA_DIR")
	overrideEnv(&cfg.Database.Driver, "IMGLI_DATABASE_DRIVER")
	overrideEnv(&cfg.Database.DSN, "IMGLI_DATABASE_DSN")
	if v := os.Getenv("IMGLI_TRUST_PROXY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.TrustProxy = b
		}
	}
	if v := os.Getenv("IMGLI_RATE_LIMIT_MULT"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			cfg.RateLimitMult = f
		}
	}
	if v := os.Getenv("IMGLI_FETCH_ALLOW"); v != "" {
		parts := strings.Split(v, ",")
		cfg.FetchAllow = cfg.FetchAllow[:0]
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				cfg.FetchAllow = append(cfg.FetchAllow, s)
			}
		}
	}
	// yaml 来源的 fetch_allow 条目也需 trim+去空——否则 "10.0.0.0/8 " 这类
	// 带首尾空白的条目会在 parseFetchAllow 里悄悄解析失败（与 env 来源同口径）。
	trimmed := make([]string, 0, len(cfg.FetchAllow))
	for _, e := range cfg.FetchAllow {
		if s := strings.TrimSpace(e); s != "" {
			trimmed = append(trimmed, s)
		}
	}
	cfg.FetchAllow = trimmed
	if cfg.RateLimitMult <= 0 { // yaml 显式 0 或缺省未覆盖时兜底为 1
		cfg.RateLimitMult = 1
	}
	if v := os.Getenv("IMGLI_SERVE_CACHE_DISABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.ServeCacheDisabled = b
		}
	}
	if v := os.Getenv("IMGLI_SERVE_CACHE_MAX_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			cfg.ServeCacheMaxBytes = n
		}
	}
	return cfg, nil
}

func overrideEnv(dst *string, key string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

// SQLiteDefaultDSN 返回 sqlite 驱动在未显式配置 DSN 时的数据库文件路径。
func (c *Config) SQLiteDefaultDSN() string {
	return filepath.Join(c.DataDir, "imgli.db")
}
