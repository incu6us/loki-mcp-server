package loki

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/incu6us/loki-mcp-server/internal/config"
)

func newTestServer(handler http.HandlerFunc) (*httptest.Server, Client) {
	ts := httptest.NewServer(handler)
	cfg := &config.Config{
		LokiURL:     ts.URL,
		HTTPTimeout: 5 * time.Second,
	}
	return ts, NewClient(cfg)
}

func TestQueryRange(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query_range" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("query") != `{app="test"}` {
			t.Errorf("unexpected query param: %s", r.URL.Query().Get("query"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "streams",
				"result":     []any{},
			},
		})
	})
	defer ts.Close()

	resp, err := client.QueryRange(context.Background(), QueryRangeParams{
		Query:     `{app="test"}`,
		Start:     "2024-01-01T00:00:00Z",
		End:       "2024-01-01T01:00:00Z",
		Limit:     100,
		Direction: "backward",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("status = %q", resp.Status)
	}
}

func TestQuery(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "streams",
				"result":     []any{},
			},
		})
	})
	defer ts.Close()

	resp, err := client.Query(context.Background(), QueryParams{Query: `{app="test"}`})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "success" {
		t.Errorf("status = %q", resp.Status)
	}
}

func TestLabels(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/labels" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   []string{"app", "env"},
		})
	})
	defer ts.Close()

	labels, err := client.Labels(context.Background(), LabelsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("got %d labels, want 2", len(labels))
	}
}

func TestLabelValues(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/label/app/values" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   []string{"nginx", "api"},
		})
	})
	defer ts.Close()

	values, err := client.LabelValues(context.Background(), "app", LabelsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(values) != 2 {
		t.Errorf("got %d values, want 2", len(values))
	}
}

func TestSeries(t *testing.T) {
	ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/series" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("match[]") != `{app="nginx"}` {
			t.Errorf("unexpected match param: %s", r.URL.Query().Get("match[]"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"data":   []map[string]string{{"app": "nginx"}},
		})
	})
	defer ts.Close()

	series, err := client.Series(context.Background(), SeriesParams{
		Match: []string{`{app="nginx"}`},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 1 {
		t.Errorf("got %d series, want 1", len(series))
	}
}

func TestAuthHeaders(t *testing.T) {
	t.Run("basic auth", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if !ok || user != "admin" || pass != "secret" {
				t.Errorf("basic auth not set correctly: ok=%v user=%q pass=%q", ok, user, pass)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []string{}})
		}))
		defer ts.Close()

		client := NewClient(&config.Config{
			LokiURL:     ts.URL,
			Username:    "admin",
			Password:    "secret",
			HTTPTimeout: 5 * time.Second,
		})
		_, _ = client.Labels(context.Background(), LabelsParams{})
	})

	t.Run("bearer token", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer mytoken" {
				t.Errorf("bearer token not set: %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []string{}})
		}))
		defer ts.Close()

		client := NewClient(&config.Config{
			LokiURL:     ts.URL,
			BearerToken: "mytoken",
			HTTPTimeout: 5 * time.Second,
		})
		_, _ = client.Labels(context.Background(), LabelsParams{})
	})

	t.Run("tenant ID", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Scope-OrgID") != "tenant1" {
				t.Errorf("tenant ID not set: %q", r.Header.Get("X-Scope-OrgID"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "data": []string{}})
		}))
		defer ts.Close()

		client := NewClient(&config.Config{
			LokiURL:     ts.URL,
			TenantID:    "tenant1",
			HTTPTimeout: 5 * time.Second,
		})
		_, _ = client.Labels(context.Background(), LabelsParams{})
	})
}

func TestLokiErrorParsing(t *testing.T) {
	t.Run("structured loki error", func(t *testing.T) {
		ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":    "error",
				"errorType": "bad_data",
				"error":     "parse error at line 1",
			})
		})
		defer ts.Close()

		_, err := client.Labels(context.Background(), LabelsParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !contains(err.Error(), "bad_data") || !contains(err.Error(), "parse error") {
			t.Errorf("unexpected error format: %v", err)
		}
	})

	t.Run("raw error", func(t *testing.T) {
		ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		})
		defer ts.Close()

		_, err := client.Labels(context.Background(), LabelsParams{})
		if err == nil {
			t.Fatal("expected error")
		}
		if !contains(err.Error(), "500") {
			t.Errorf("expected HTTP 500 in error: %v", err)
		}
	})

	t.Run("success envelope with error status", func(t *testing.T) {
		ts, client := newTestServer(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "error",
				"data":   []string{},
			})
		})
		defer ts.Close()

		_, err := client.Labels(context.Background(), LabelsParams{})
		if err == nil {
			t.Fatal("expected error for non-success status")
		}
		if !contains(err.Error(), "non-success") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
