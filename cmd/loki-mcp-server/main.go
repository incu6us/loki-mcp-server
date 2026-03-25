package main

import (
	"fmt"
	"log"
	"os"

	"github.com/incu6us/loki-mcp-server/internal/config"
	"github.com/incu6us/loki-mcp-server/internal/loki"
	"github.com/incu6us/loki-mcp-server/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	logger := log.New(os.Stderr, "loki-mcp: ", log.LstdFlags)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("configuration error: %v", err)
	}

	lokiClient := loki.NewClient(cfg)

	s := server.NewMCPServer(
		"loki-mcp-server",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	for _, register := range []func(loki.Client) (mcp.Tool, server.ToolHandlerFunc){
		tools.NewQueryRangeTool,
		tools.NewQueryTool,
		tools.NewLabelsTool,
		tools.NewLabelValuesTool,
		tools.NewSeriesTool,
	} {
		tool, handler := register(lokiClient)
		s.AddTool(tool, handler)
	}

	if err := server.ServeStdio(s); err != nil {
		logger.Fatalf("server error: %v", err)
	}
}
