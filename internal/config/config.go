package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	LokiURL       string
	Username      string
	Password      string
	BearerToken   string
	TLSSkipVerify bool
	TenantID      string
	HTTPTimeout   time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPTimeout: 30 * time.Second,
	}

	cfg.LokiURL = strings.TrimRight(os.Getenv("LOKI_URL"), "/")
	if cfg.LokiURL == "" {
		return nil, fmt.Errorf("LOKI_URL is required")
	}

	u, err := url.Parse(cfg.LokiURL)
	if err != nil {
		return nil, fmt.Errorf("LOKI_URL is not a valid URL: %w", err)
	}
	if !u.IsAbs() || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("LOKI_URL must be an absolute HTTP or HTTPS URL")
	}

	cfg.Username = os.Getenv("LOKI_USERNAME")
	cfg.Password = os.Getenv("LOKI_PASSWORD")
	cfg.BearerToken = os.Getenv("LOKI_BEARER_TOKEN")

	if (cfg.Username != "" || cfg.Password != "") && cfg.BearerToken != "" {
		return nil, fmt.Errorf("LOKI_USERNAME/LOKI_PASSWORD and LOKI_BEARER_TOKEN are mutually exclusive")
	}

	if v := os.Getenv("LOKI_TLS_SKIP_VERIFY"); v == "true" || v == "1" {
		cfg.TLSSkipVerify = true
	}

	cfg.TenantID = os.Getenv("LOKI_TENANT_ID")

	if v := os.Getenv("LOKI_HTTP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("LOKI_HTTP_TIMEOUT is not a valid duration: %w", err)
		}
		cfg.HTTPTimeout = d
	}

	return cfg, nil
}
