package tools

import (
	"context"
	"fmt"
	"testing"

	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/mark3labs/mcp-go/mcp"
)

// mockClient implements loki.Client for testing.
type mockClient struct {
	queryRangeResp *loki.QueryResponse
	queryResp      *loki.QueryResponse
	labelsResp     []string
	labelValResp   []string
	seriesResp     []map[string]string
	err            error
}

func (m *mockClient) QueryRange(_ context.Context, _ loki.QueryRangeParams) (*loki.QueryResponse, error) {
	return m.queryRangeResp, m.err
}
func (m *mockClient) Query(_ context.Context, _ loki.QueryParams) (*loki.QueryResponse, error) {
	return m.queryResp, m.err
}
func (m *mockClient) Labels(_ context.Context, _ loki.LabelsParams) ([]string, error) {
	return m.labelsResp, m.err
}
func (m *mockClient) LabelValues(_ context.Context, _ string, _ loki.LabelsParams) ([]string, error) {
	return m.labelValResp, m.err
}
func (m *mockClient) Series(_ context.Context, _ loki.SeriesParams) ([]map[string]string, error) {
	return m.seriesResp, m.err
}

func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestQueryRangeTool(t *testing.T) {
	client := &mockClient{
		queryRangeResp: &loki.QueryResponse{Status: "success"},
	}
	_, handler := NewQueryRangeTool(client)

	t.Run("missing query", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{}))
		assertIsError(t, result, "query is required")
	})

	t.Run("invalid direction", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{
			"query": `{app="test"}`, "direction": "invalid",
		}))
		assertIsError(t, result, "direction must be")
	})

	t.Run("limit exceeds max", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{
			"query": `{app="test"}`, "limit": float64(6000),
		}))
		assertIsError(t, result, "limit must not exceed")
	})

	t.Run("success", func(t *testing.T) {
		result, err := handler(context.Background(), makeRequest(map[string]any{
			"query": `{app="test"}`,
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertIsSuccess(t, result)
	})

	t.Run("client error", func(t *testing.T) {
		errClient := &mockClient{err: fmt.Errorf("connection refused")}
		_, h := NewQueryRangeTool(errClient)
		result, _ := h(context.Background(), makeRequest(map[string]any{"query": `{app="test"}`}))
		assertIsError(t, result, "connection refused")
	})
}

func TestQueryTool(t *testing.T) {
	client := &mockClient{
		queryResp: &loki.QueryResponse{Status: "success"},
	}
	_, handler := NewQueryTool(client)

	t.Run("missing query", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{}))
		assertIsError(t, result, "query is required")
	})

	t.Run("success", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"query": `{app="test"}`}))
		assertIsSuccess(t, result)
	})
}

func TestLabelsTool(t *testing.T) {
	client := &mockClient{labelsResp: []string{"app", "env"}}
	_, handler := NewLabelsTool(client)

	result, err := handler(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsSuccess(t, result)
}

func TestLabelValuesTool(t *testing.T) {
	client := &mockClient{labelValResp: []string{"nginx", "api"}}
	_, handler := NewLabelValuesTool(client)

	t.Run("missing label", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{}))
		assertIsError(t, result, "label is required")
	})

	t.Run("invalid label name", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"label": "123bad"}))
		assertIsError(t, result, "label must match")
	})

	t.Run("invalid label with special chars", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"label": "foo-bar"}))
		assertIsError(t, result, "label must match")
	})

	t.Run("valid label", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"label": "app"}))
		assertIsSuccess(t, result)
	})

	t.Run("valid label with underscore", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"label": "_my_label"}))
		assertIsSuccess(t, result)
	})
}

func TestSeriesTool(t *testing.T) {
	client := &mockClient{seriesResp: []map[string]string{{"app": "nginx"}}}
	_, handler := NewSeriesTool(client)

	t.Run("missing match", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{}))
		assertIsError(t, result, "match is required")
	})

	t.Run("success", func(t *testing.T) {
		result, _ := handler(context.Background(), makeRequest(map[string]any{"match": `{app="nginx"}`}))
		assertIsSuccess(t, result)
	})
}

func assertIsError(t *testing.T, result *mcp.CallToolResult, substring string) {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	text := getResultText(result)
	if !containsStr(text, substring) {
		t.Errorf("error %q does not contain %q", text, substring)
	}
}

func assertIsSuccess(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
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

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
