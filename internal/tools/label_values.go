package tools

import (
	"context"

	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewLabelValuesTool(client loki.Client) (mcp.Tool, server.ToolHandlerFunc) {
	tool := mcp.NewTool("label_values",
		mcp.WithDescription("List values for a specific label name in Loki"),
		mcp.WithString("label", mcp.Required(), mcp.Description("Label name to retrieve values for")),
		mcp.WithString("start", mcp.Description("Start of time range (RFC3339 or Unix nanoseconds). Defaults to 6 hours ago")),
		mcp.WithString("end", mcp.Description("End of time range (RFC3339 or Unix nanoseconds). Defaults to now")),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		label := extractString(req, "label")
		if label == "" {
			return mcp.NewToolResultError("label is required"), nil
		}
		if !validLabelName.MatchString(label) {
			return mcp.NewToolResultError("label must match [a-zA-Z_][a-zA-Z0-9_]*"), nil
		}

		values, err := client.LabelValues(ctx, label, loki.LabelsParams{
			Start: extractString(req, "start"),
			End:   extractString(req, "end"),
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		return jsonResult(values)
	}

	return tool, handler
}
