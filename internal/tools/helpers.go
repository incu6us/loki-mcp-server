package tools

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	DefaultLimit = 100
	MaxLimit     = 5000
)

var validLabelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func extractString(req mcp.CallToolRequest, key string) string {
	v, ok := req.GetArguments()[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func extractLimit(req mcp.CallToolRequest) int {
	v, ok := req.GetArguments()["limit"]
	if !ok {
		return DefaultLimit
	}
	f, ok := v.(float64)
	if !ok {
		return DefaultLimit
	}
	n := int(f)
	if n <= 0 {
		return DefaultLimit
	}
	return n
}

func validateDirection(direction string) error {
	if direction != "" && direction != "forward" && direction != "backward" {
		return fmt.Errorf("direction must be \"forward\" or \"backward\"")
	}
	return nil
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshaling response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
