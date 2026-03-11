package tools

import (
	"context"

	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewSeriesTool(client loki.Client) (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("series",
		mcp.WithDescription("Find active log stream series matching a selector in Loki"),
		mcp.WithString("match", mcp.Required(), mcp.Description("Stream selector (e.g. {app=\"nginx\"})")),
		mcp.WithString("start", mcp.Description("Start of time range (RFC3339 or Unix nanoseconds). Defaults to 6 hours ago")),
		mcp.WithString("end", mcp.Description("End of time range (RFC3339 or Unix nanoseconds). Defaults to now")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		match := extractString(req, "match")
		if match == "" {
			return mcp.NewToolResultError("match is required"), nil
		}

		series, err := client.Series(ctx, loki.SeriesParams{
			Match: []string{match},
			Start: extractString(req, "start"),
			End:   extractString(req, "end"),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(series)
	}

	return tool, handler
}
