package tools

import (
	"context"

	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewLabelsTool(client loki.Client) (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("labels",
		mcp.WithDescription("List all available label names in Loki for building LogQL queries"),
		mcp.WithString("start", mcp.Description("Start of time range (RFC3339 or Unix nanoseconds). Defaults to 6 hours ago")),
		mcp.WithString("end", mcp.Description("End of time range (RFC3339 or Unix nanoseconds). Defaults to now")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		labels, err := client.Labels(ctx, loki.LabelsParams{
			Start: extractString(req, "start"),
			End:   extractString(req, "end"),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(labels)
	}

	return tool, handler
}
