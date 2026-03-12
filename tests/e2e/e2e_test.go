package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/incu6us/loki-mcp-server/internal/config"
	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/incu6us/loki-mcp-server/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	lokiClient loki.Client
	lokiURL    string
)

func TestMain(m *testing.M) {
	ctx := context.Background()

	lokiContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "grafana/loki:2.9.0",
			ExposedPorts: []string{"3100/tcp"},
			Cmd:          []string{"-config.file=/etc/loki/local-config.yaml"},
			WaitingFor: wait.ForHTTP("/ready").
				WithPort("3100/tcp").
				WithStatusCodeMatcher(func(status int) bool { return status == http.StatusOK }).
				WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start loki container: %v\n", err)
		os.Exit(1)
	}

	host, _ := lokiContainer.Host(ctx)
	port, _ := lokiContainer.MappedPort(ctx, "3100/tcp")
	lokiURL = fmt.Sprintf("http://%s:%s", host, port.Port())

	lokiClient = loki.NewClient(&config.Config{
		LokiURL:     lokiURL,
		HTTPTimeout: 30 * time.Second,
	})

	if err := waitForLokiReady(ctx, lokiURL, 30*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "loki not ready: %v\n", err)
		os.Exit(1)
	}

	if err := pushTestLogs(ctx, lokiURL); err != nil {
		fmt.Fprintf(os.Stderr, "failed to push test logs: %v\n", err)
		os.Exit(1)
	}

	// Allow Loki to index the pushed logs
	time.Sleep(2 * time.Second)

	code := m.Run()

	_ = lokiContainer.Terminate(ctx)
	os.Exit(code)
}

func waitForLokiReady(ctx context.Context, baseURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/ready")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("loki not ready after %v", timeout)
}

func pushTestLogs(ctx context.Context, baseURL string) error {
	now := time.Now()

	streams := []map[string]any{
		{
			"stream": map[string]string{"app": "nginx", "env": "production", "level": "info"},
			"values": [][]string{
				{fmt.Sprintf("%d", now.Add(-30*time.Minute).UnixNano()), `{"msg":"GET /api/health 200","duration":"1ms"}`},
				{fmt.Sprintf("%d", now.Add(-20*time.Minute).UnixNano()), `{"msg":"GET /api/users 200","duration":"15ms"}`},
				{fmt.Sprintf("%d", now.Add(-10*time.Minute).UnixNano()), `{"msg":"POST /api/login 200","duration":"45ms"}`},
			},
		},
		{
			"stream": map[string]string{"app": "nginx", "env": "production", "level": "error"},
			"values": [][]string{
				{fmt.Sprintf("%d", now.Add(-15*time.Minute).UnixNano()), `{"msg":"GET /api/orders 500","duration":"2s","error":"connection refused"}`},
			},
		},
		{
			"stream": map[string]string{"app": "api", "env": "staging", "level": "info"},
			"values": [][]string{
				{fmt.Sprintf("%d", now.Add(-25*time.Minute).UnixNano()), `{"msg":"processing request","request_id":"abc-123"}`},
				{fmt.Sprintf("%d", now.Add(-5*time.Minute).UnixNano()), `{"msg":"request completed","request_id":"abc-123"}`},
			},
		},
	}

	payload, _ := json.Marshal(map[string]any{"streams": streams})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/loki/api/v1/push", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("push failed with status %d", resp.StatusCode)
	}
	return nil
}

// Helper to build MCP request for tool handlers.
func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func getResultText(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

// --- Tests ---

func TestQueryRange(t *testing.T) {
	_, handler := tools.NewQueryRangeTool(lokiClient)

	t.Run("fetch nginx logs", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"query": `{app="nginx"}`,
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
			"limit": float64(100),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var resp loki.QueryResponse
		if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp.Status != "success" {
			t.Fatalf("status = %q", resp.Status)
		}
		if resp.Data.ResultType != "streams" {
			t.Fatalf("resultType = %q, want streams", resp.Data.ResultType)
		}

		var streams []loki.StreamResult
		if err := json.Unmarshal(resp.Data.Result, &streams); err != nil {
			t.Fatalf("unmarshal streams: %v", err)
		}
		if len(streams) == 0 {
			t.Fatal("expected at least one stream for nginx")
		}

		totalEntries := 0
		for _, s := range streams {
			totalEntries += len(s.Values)
		}
		if totalEntries != 4 {
			t.Errorf("expected 4 nginx log entries, got %d", totalEntries)
		}
	})

	t.Run("filter by level", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"query": `{app="nginx", level="error"}`,
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var resp loki.QueryResponse
		_ = json.Unmarshal([]byte(getResultText(result)), &resp)

		var streams []loki.StreamResult
		_ = json.Unmarshal(resp.Data.Result, &streams)

		totalEntries := 0
		for _, s := range streams {
			totalEntries += len(s.Values)
		}
		if totalEntries != 1 {
			t.Errorf("expected 1 error entry, got %d", totalEntries)
		}
	})

	t.Run("forward direction", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"query":     `{app="nginx", level="info"}`,
			"start":     time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":       time.Now().Format(time.RFC3339Nano),
			"direction": "forward",
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var resp loki.QueryResponse
		_ = json.Unmarshal([]byte(getResultText(result)), &resp)

		var streams []loki.StreamResult
		_ = json.Unmarshal(resp.Data.Result, &streams)

		if len(streams) == 0 || len(streams[0].Values) < 2 {
			t.Fatal("expected multiple entries")
		}
		// Forward: first timestamp should be earlier than last
		first := streams[0].Values[0][0]
		last := streams[0].Values[len(streams[0].Values)-1][0]
		if first > last {
			t.Errorf("forward direction: first ts %s should be <= last ts %s", first, last)
		}
	})

	t.Run("invalid query returns error", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"query": `{invalid query`,
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid LogQL")
		}
	})
}

func TestQuery(t *testing.T) {
	_, handler := tools.NewQueryTool(lokiClient)

	result, err := handler(context.Background(), makeRequest(map[string]any{
		"query": `{app="nginx"}`,
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", getResultText(result))
	}

	var resp loki.QueryResponse
	if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "success" {
		t.Fatalf("status = %q", resp.Status)
	}
}

func TestLabels(t *testing.T) {
	_, handler := tools.NewLabelsTool(lokiClient)

	result, err := handler(context.Background(), makeRequest(map[string]any{
		"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
		"end":   time.Now().Format(time.RFC3339Nano),
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", getResultText(result))
	}

	var labels []string
	if err := json.Unmarshal([]byte(getResultText(result)), &labels); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expected := map[string]bool{"app": false, "env": false, "level": false}
	for _, l := range labels {
		if _, ok := expected[l]; ok {
			expected[l] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected label %q not found in %v", name, labels)
		}
	}
}

func TestLabelValues(t *testing.T) {
	_, handler := tools.NewLabelValuesTool(lokiClient)

	t.Run("app label values", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"label": "app",
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var values []string
		_ = json.Unmarshal([]byte(getResultText(result)), &values)

		found := map[string]bool{"nginx": false, "api": false}
		for _, v := range values {
			if _, ok := found[v]; ok {
				found[v] = true
			}
		}
		for name, ok := range found {
			if !ok {
				t.Errorf("expected value %q not found in %v", name, values)
			}
		}
	})

	t.Run("env label values", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"label": "env",
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var values []string
		_ = json.Unmarshal([]byte(getResultText(result)), &values)

		found := map[string]bool{"production": false, "staging": false}
		for _, v := range values {
			if _, ok := found[v]; ok {
				found[v] = true
			}
		}
		for name, ok := range found {
			if !ok {
				t.Errorf("expected value %q not found in %v", name, values)
			}
		}
	})
}

func TestSeries(t *testing.T) {
	_, handler := tools.NewSeriesTool(lokiClient)

	t.Run("match nginx", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"match": `{app="nginx"}`,
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var series []map[string]string
		if err := json.Unmarshal([]byte(getResultText(result)), &series); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(series) == 0 {
			t.Fatal("expected at least one series for nginx")
		}
		// Should have 2 series: nginx/info and nginx/error
		if len(series) != 2 {
			t.Errorf("expected 2 nginx series (info + error), got %d", len(series))
		}
		for _, s := range series {
			if s["app"] != "nginx" {
				t.Errorf("expected app=nginx, got app=%s", s["app"])
			}
		}
	})

	t.Run("match all apps", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"match": `{env=~".+"}`,
			"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			"end":   time.Now().Format(time.RFC3339Nano),
		}))
		if err != nil {
			t.Fatalf("handler error: %v", err)
		}
		if result.IsError {
			t.Fatalf("tool error: %s", getResultText(result))
		}

		var series []map[string]string
		_ = json.Unmarshal([]byte(getResultText(result)), &series)

		// Should have 3 series total: nginx/info, nginx/error, api/info
		if len(series) != 3 {
			t.Errorf("expected 3 total series, got %d", len(series))
		}
	})
}

func TestWorkflow_DiscoverThenQuery(t *testing.T) {
	// Simulates an LLM workflow: discover labels -> get values -> query
	ctx := context.Background()
	timeRange := map[string]any{
		"start": time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
		"end":   time.Now().Format(time.RFC3339Nano),
	}

	// Step 1: Discover labels
	_, labelsHandler := tools.NewLabelsTool(lokiClient)
	result, err := labelsHandler(ctx, makeRequest(timeRange))
	if err != nil || result.IsError {
		t.Fatalf("labels failed: err=%v, toolErr=%s", err, getResultText(result))
	}

	var labels []string
	_ = json.Unmarshal([]byte(getResultText(result)), &labels)
	t.Logf("discovered labels: %v", labels)

	// Step 2: Get values for "app" label
	_, valuesHandler := tools.NewLabelValuesTool(lokiClient)
	args := map[string]any{"label": "app"}
	for k, v := range timeRange {
		args[k] = v
	}
	result, err = valuesHandler(ctx, makeRequest(args))
	if err != nil || result.IsError {
		t.Fatalf("label_values failed: err=%v, toolErr=%s", err, getResultText(result))
	}

	var appValues []string
	_ = json.Unmarshal([]byte(getResultText(result)), &appValues)
	t.Logf("app values: %v", appValues)

	// Step 3: Query logs for the first discovered app
	if len(appValues) == 0 {
		t.Fatal("no app values discovered")
	}

	_, queryHandler := tools.NewQueryRangeTool(lokiClient)
	queryArgs := map[string]any{
		"query": fmt.Sprintf(`{app="%s"}`, appValues[0]),
		"limit": float64(50),
	}
	for k, v := range timeRange {
		queryArgs[k] = v
	}
	result, err = queryHandler(ctx, makeRequest(queryArgs))
	if err != nil || result.IsError {
		t.Fatalf("query_range failed: err=%v, toolErr=%s", err, getResultText(result))
	}

	var resp loki.QueryResponse
	_ = json.Unmarshal([]byte(getResultText(result)), &resp)
	t.Logf("queried app=%s: resultType=%s", appValues[0], resp.Data.ResultType)

	var streams []loki.StreamResult
	_ = json.Unmarshal(resp.Data.Result, &streams)

	totalEntries := 0
	for _, s := range streams {
		totalEntries += len(s.Values)
	}
	if totalEntries == 0 {
		t.Errorf("expected log entries for app=%s, got 0", appValues[0])
	}
	t.Logf("got %d entries for app=%s", totalEntries, appValues[0])
}
