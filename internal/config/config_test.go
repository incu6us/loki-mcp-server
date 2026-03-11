package config

import (
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, cfg *Config)
	}{
		{
			name:    "missing LOKI_URL",
			env:     map[string]string{},
			wantErr: "LOKI_URL is required",
		},
		{
			name:    "invalid URL scheme",
			env:     map[string]string{"LOKI_URL": "ftp://loki:3100"},
			wantErr: "LOKI_URL must be an absolute HTTP or HTTPS URL",
		},
		{
			name:    "relative URL",
			env:     map[string]string{"LOKI_URL": "/loki"},
			wantErr: "LOKI_URL must be an absolute HTTP or HTTPS URL",
		},
		{
			name: "basic auth and bearer token mutually exclusive",
			env: map[string]string{
				"LOKI_URL":          "http://loki:3100",
				"LOKI_USERNAME":     "admin",
				"LOKI_PASSWORD":     "secret",
				"LOKI_BEARER_TOKEN": "tok",
			},
			wantErr: "mutually exclusive",
		},
		{
			name:    "invalid timeout",
			env:     map[string]string{"LOKI_URL": "http://loki:3100", "LOKI_HTTP_TIMEOUT": "notaduration"},
			wantErr: "LOKI_HTTP_TIMEOUT is not a valid duration",
		},
		{
			name: "minimal valid config",
			env:  map[string]string{"LOKI_URL": "http://loki:3100"},
			check: func(t *testing.T, cfg *Config) {
				if cfg.LokiURL != "http://loki:3100" {
					t.Errorf("LokiURL = %q, want %q", cfg.LokiURL, "http://loki:3100")
				}
				if cfg.HTTPTimeout != 30*time.Second {
					t.Errorf("HTTPTimeout = %v, want 30s", cfg.HTTPTimeout)
				}
			},
		},
		{
			name: "trailing slash stripped",
			env:  map[string]string{"LOKI_URL": "http://loki:3100/"},
			check: func(t *testing.T, cfg *Config) {
				if cfg.LokiURL != "http://loki:3100" {
					t.Errorf("LokiURL = %q, want no trailing slash", cfg.LokiURL)
				}
			},
		},
		{
			name: "all options",
			env: map[string]string{
				"LOKI_URL":             "https://loki.example.com",
				"LOKI_USERNAME":        "admin",
				"LOKI_PASSWORD":        "secret",
				"LOKI_TLS_SKIP_VERIFY": "true",
				"LOKI_TENANT_ID":       "tenant1",
				"LOKI_HTTP_TIMEOUT":    "10s",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.Username != "admin" {
					t.Errorf("Username = %q", cfg.Username)
				}
				if cfg.Password != "secret" {
					t.Errorf("Password = %q", cfg.Password)
				}
				if !cfg.TLSSkipVerify {
					t.Error("TLSSkipVerify should be true")
				}
				if cfg.TenantID != "tenant1" {
					t.Errorf("TenantID = %q", cfg.TenantID)
				}
				if cfg.HTTPTimeout != 10*time.Second {
					t.Errorf("HTTPTimeout = %v", cfg.HTTPTimeout)
				}
			},
		},
		{
			name: "bearer token auth",
			env: map[string]string{
				"LOKI_URL":          "http://loki:3100",
				"LOKI_BEARER_TOKEN": "mytoken",
			},
			check: func(t *testing.T, cfg *Config) {
				if cfg.BearerToken != "mytoken" {
					t.Errorf("BearerToken = %q", cfg.BearerToken)
				}
			},
		},
		{
			name: "TLS skip verify with 1",
			env:  map[string]string{"LOKI_URL": "http://loki:3100", "LOKI_TLS_SKIP_VERIFY": "1"},
			check: func(t *testing.T, cfg *Config) {
				if !cfg.TLSSkipVerify {
					t.Error("TLSSkipVerify should be true for value '1'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all relevant env vars
			for _, key := range []string{
				"LOKI_URL", "LOKI_USERNAME", "LOKI_PASSWORD",
				"LOKI_BEARER_TOKEN", "LOKI_TLS_SKIP_VERIFY",
				"LOKI_TENANT_ID", "LOKI_HTTP_TIMEOUT",
			} {
				t.Setenv(key, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
