package tools

import (
	"context"
	"fmt"

	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewQueryRangeTool(client loki.Client) (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("query_range",
		mcp.WithDescription("Execute a LogQL range query against Loki to fetch logs over a time window"),
		mcp.WithString("query", mcp.Required(), mcp.Description("LogQL query expression")),
		mcp.WithString("start", mcp.Description("Start of time range (RFC3339 or Unix nanoseconds). Defaults to 1 hour ago")),
		mcp.WithString("end", mcp.Description("End of time range (RFC3339 or Unix nanoseconds). Defaults to now")),
		mcp.WithNumber("limit", mcp.Description("Maximum number of entries to return. Defaults to 100, max 5000")),
		mcp.WithString("direction", mcp.Description("Sort order: forward or backward. Defaults to backward")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query := extractString(req, "query")
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		direction := extractString(req, "direction")
		if err := validateDirection(direction); err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		limit := extractLimit(req)
		if limit > MaxLimit {
			return mcp.NewToolResultError(fmt.Sprintf("limit must not exceed %d", MaxLimit)), nil
		}

		resp, err := client.QueryRange(ctx, loki.QueryRangeParams{
			Query:     query,
			Start:     extractString(req, "start"),
			End:       extractString(req, "end"),
			Limit:     limit,
			Direction: direction,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(resp)
	}

	return tool, handler
}
