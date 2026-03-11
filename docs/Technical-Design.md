# Loki MCP Server — Technical Design

## Overview

Loki MCP Server is a Model Context Protocol (MCP) server that exposes Grafana Loki log querying capabilities as MCP tools. It operates exclusively in **stdio mode**, designed to be launched as a subprocess by MCP clients (Claude Desktop, Claude Code, etc.).

**Stdio constraint:** The MCP JSON-RPC protocol uses stdout for communication. All application logging must go to **stderr only**. Writing anything other than MCP JSON-RPC messages to stdout will corrupt the protocol stream and break the client connection.

## Configuration

The server is configured entirely via environment variables, injected by the MCP client:

```json
"loki-mcp-server": {
  "type": "stdio",
  "command": "/usr/local/bin/loki-mcp-server",
  "args": [],
  "env": {
    "LOKI_URL": "http://loki.example.com:3100",
    "LOKI_TLS_SKIP_VERIFY": "true",
    "LOKI_USERNAME": "admin",
    "LOKI_PASSWORD": "secret"
  }
}
```

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LOKI_URL` | yes | — | Base URL of the Loki instance |
| `LOKI_USERNAME` | no | — | Basic auth username |
| `LOKI_PASSWORD` | no | — | Basic auth password |
| `LOKI_BEARER_TOKEN` | no | — | Bearer token for `Authorization: Bearer <token>` auth |
| `LOKI_TLS_SKIP_VERIFY` | no | `false` | Skip TLS certificate verification |
| `LOKI_TENANT_ID` | no | — | `X-Scope-OrgID` header for multi-tenant deployments |
| `LOKI_HTTP_TIMEOUT` | no | `30s` | HTTP request timeout (Go duration, e.g. `10s`, `1m`) |

Authentication is mutually exclusive: configure either basic auth (`LOKI_USERNAME`/`LOKI_PASSWORD`) or bearer token (`LOKI_BEARER_TOKEN`), not both. The server returns a configuration error if both are set.

## Project Structure

```
loki-mcp/
├── cmd/
│   └── loki-mcp-server/
│       └── main.go                 # Entry point
├── internal/
│   ├── config/
│   │   └── config.go              # Configuration loading
│   ├── loki/
│   │   ├── client.go              # Loki HTTP client
│   │   └── types.go               # API request/response types
│   └── tools/
│       ├── query_range.go         # query_range tool
│       ├── query.go               # instant query tool
│       ├── labels.go              # labels tool
│       ├── label_values.go        # label_values tool
│       └── series.go              # series tool
├── go.mod
└── go.sum
```

## Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/mark3labs/mcp-go` | MCP SDK — server creation, tool registration, stdio transport |
| Go standard library | HTTP client, TLS, JSON, environment variables |

The `mark3labs/mcp-go` SDK is chosen for its mature builder-pattern API, simple handler signatures, and broad adoption in the Go MCP ecosystem.

## Component Design

### Config (`internal/config`)

```go
type Config struct {
    LokiURL       string
    Username      string
    Password      string
    BearerToken   string
    TLSSkipVerify bool
    TenantID      string
    HTTPTimeout   time.Duration
}

func Load() (*Config, error)
```

`Load()` reads environment variables and performs validation:
- `LOKI_URL` is required and must be a valid absolute HTTP or HTTPS URL
- Trailing slash on `LOKI_URL` is stripped during load (normalized to `http://host:3100` not `http://host:3100/`)
- Basic auth and bearer token are mutually exclusive
- `LOKI_HTTP_TIMEOUT` is parsed as `time.Duration`, defaults to `30s`

Returns an error on invalid configuration.

### Loki Client (`internal/loki`)

#### Interface

```go
type Client interface {
    QueryRange(ctx context.Context, params QueryRangeParams) (*QueryResponse, error)
    Query(ctx context.Context, params QueryParams) (*QueryResponse, error)
    Labels(ctx context.Context, params LabelsParams) ([]string, error)
    LabelValues(ctx context.Context, label string, params LabelsParams) ([]string, error)
    Series(ctx context.Context, params SeriesParams) ([]map[string]string, error)
}
```

The interface enables dependency injection and testability — tool handlers depend on the interface, not the concrete HTTP client.

#### HTTP Client Implementation

- Constructs `http.Client` with configurable `Timeout` (from `cfg.HTTPTimeout`, default `30s`)
- Custom `http.Transport` with `tls.Config{InsecureSkipVerify: cfg.TLSSkipVerify}` and tuned connection pooling (`MaxIdleConns: 10`, `MaxIdleConnsPerHost: 10`, `IdleConnTimeout: 90s`)
- Injects `Authorization: Basic base64(username:password)` header when basic auth credentials are configured
- Injects `Authorization: Bearer <token>` header when bearer token is configured
- Injects `X-Scope-OrgID` header when tenant ID is configured
- Centralizes all HTTP request logic in a private `do()` method (see below)
- Wraps all errors with `fmt.Errorf` for traceability

#### Centralized HTTP Request Helper

All Loki API calls go through a single private method that eliminates duplication:

```go
func (c *httpClient) do(ctx context.Context, endpoint string, queryParams url.Values, result any) error {
    reqURL := fmt.Sprintf("%s%s?%s", c.baseURL, endpoint, queryParams.Encode())

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
    if err != nil {
        return fmt.Errorf("creating request: %w", err)
    }

    c.setAuthHeaders(req)

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("executing request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return fmt.Errorf("reading response body: %w", err)
    }

    if resp.StatusCode != http.StatusOK {
        return c.parseLokiError(resp.StatusCode, body)
    }

    if err := json.Unmarshal(body, result); err != nil {
        return fmt.Errorf("decoding response: %w", err)
    }

    return c.validateStatus(result)
}
```

Each public method (`QueryRange`, `Query`, `Labels`, etc.) builds `url.Values` from its params and delegates to `do()`.

#### Loki Error Envelope Parsing

Non-200 responses are parsed as structured Loki errors instead of returning raw status codes:

```go
type lokiErrorResponse struct {
    Status    string `json:"status"`
    ErrorType string `json:"errorType"`
    Error     string `json:"error"`
}

func (c *httpClient) parseLokiError(statusCode int, body []byte) error {
    var lokiErr lokiErrorResponse
    if err := json.Unmarshal(body, &lokiErr); err == nil && lokiErr.Error != "" {
        return fmt.Errorf("loki %s error (HTTP %d): %s", lokiErr.ErrorType, statusCode, lokiErr.Error)
    }
    return fmt.Errorf("loki request failed (HTTP %d): %s", statusCode, string(body))
}
```

This produces actionable error messages like `loki bad_data error (HTTP 400): parse error at line 1: unexpected end of input` instead of opaque status codes.

#### Success Envelope Validation

Even on HTTP 200, the response `status` field is checked. Loki may return `{"status": "error", ...}` with a 200 status code in certain edge cases:

```go
type statusCarrier interface {
    GetStatus() string
}

func (c *httpClient) validateStatus(result any) error {
    if carrier, ok := result.(statusCarrier); ok {
        if carrier.GetStatus() != "success" {
            return fmt.Errorf("loki returned non-success status: %s", carrier.GetStatus())
        }
    }
    return nil
}
```

Response types (`QueryResponse`, `LabelsResponse`) implement `statusCarrier` via a `GetStatus()` method.

#### Request/Response Types

```go
type QueryRangeParams struct {
    Query     string
    Start     string
    End       string
    Limit     int
    Direction string
}

type QueryParams struct {
    Query     string
    Limit     int
    Time      string
    Direction string
}

type LabelsParams struct {
    Start string
    End   string
}

type SeriesParams struct {
    Match []string
    Start string
    End   string
}

type QueryResponse struct {
    Status string         `json:"status"`
    Data   QueryData      `json:"data"`
}

type QueryData struct {
    ResultType string          `json:"resultType"`
    Result     json.RawMessage `json:"result"`
    Stats      json.RawMessage `json:"stats"`
}

type StreamResult struct {
    Stream map[string]string `json:"stream"`
    Values [][]string        `json:"values"`
}

type MatrixResult struct {
    Metric map[string]string `json:"metric"`
    Values [][]interface{}   `json:"values"`
}
```

`Result` is kept as `json.RawMessage` to preserve the raw JSON for the LLM. The `StreamResult` and `MatrixResult` types are available for cases where structured parsing is needed.

### Loki API Endpoints

| Method | Endpoint | Used By |
|--------|----------|---------|
| GET | `/loki/api/v1/query_range` | `QueryRange()` |
| GET | `/loki/api/v1/query` | `Query()` |
| GET | `/loki/api/v1/labels` | `Labels()` |
| GET | `/loki/api/v1/label/{name}/values` | `LabelValues()` |
| GET | `/loki/api/v1/series` | `Series()` |

All endpoints return JSON with the envelope `{"status": "success", "data": ...}`. On error, Loki returns `{"status": "error", "errorType": "...", "error": "..."}`.

### MCP Tools (`internal/tools`)

Each tool is registered with the MCP server via `mcp.NewTool()` and a handler function.

#### 1. `query_range` — Range Log Query

The primary tool for fetching logs over a time window.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | LogQL query expression |
| `start` | string | no | 1 hour ago | Start of time range (RFC3339 or Unix nano) |
| `end` | string | no | now | End of time range |
| `limit` | number | no | 100 | Maximum number of entries |
| `direction` | string | no | `backward` | Sort order: `forward` or `backward` |

#### 2. `query` — Instant Query

Point-in-time query evaluation.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | LogQL query expression |
| `limit` | number | no | 100 | Maximum number of entries |
| `time` | string | no | now | Evaluation timestamp |
| `direction` | string | no | `backward` | Sort order: `forward` or `backward` |

#### 3. `labels` — List Label Names

Discover available label names for building queries.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

#### 4. `label_values` — List Label Values

Retrieve values for a specific label.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `label` | string | yes | — | Label name |
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

#### 5. `series` — Find Series

Discover active stream combinations matching selectors.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `match` | string | yes | — | Stream selector (e.g. `{app="nginx"}`) |
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

The MCP tool accepts `match` as a single string for simplicity. The tool handler converts it to the `match[]` query parameter format required by the Loki `/series` endpoint.

#### Tool Parameter Validation

All tool handlers validate inputs before calling the Loki client:

- **Required parameters** — return `mcp.NewToolResultError()` if missing
- **`direction`** — must be `"forward"` or `"backward"` when provided
- **`limit`** — values above `MaxLimit = 5000` return a validation error; values `<= 0` default to `DefaultLimit = 100`
- **`label`** — must be non-empty and contain only valid label characters (`[a-zA-Z_][a-zA-Z0-9_]*`)
- **`match`** — must be non-empty for series queries

Constants:

```go
const (
    DefaultLimit = 100
    MaxLimit     = 5000
)
```

#### Tool Handler Pattern

All tool handlers follow the same pattern:

```go
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
        // 1. Extract and validate parameters
        // 2. Apply defaults and cap limit
        // 3. Call loki client
        // 4. Marshal response to JSON
        // 5. Return mcp.NewToolResultText(jsonString) or mcp.NewToolResultError(errMsg)
    }

    return tool, handler
}
```

Each `New*Tool` function returns both the tool definition and its handler, keeping registration clean in `main.go`.

### Entry Point (`cmd/loki-mcp-server/main.go`)

```go
func main() {
    // All logging must go to stderr — stdout is reserved for MCP JSON-RPC
    logger := log.New(os.Stderr, "loki-mcp: ", log.LstdFlags)

    // 1. Load configuration from environment
    cfg, err := config.Load()
    if err != nil {
        logger.Fatalf("configuration error: %v", err)
    }

    // 2. Create Loki HTTP client
    lokiClient := loki.NewClient(cfg)

    // 3. Create MCP server
    s := server.NewMCPServer(
        "loki-mcp-server",
        "0.1.0",
        server.WithToolCapabilities(false),
    )

    // 4. Register tools
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

    // 5. Start stdio transport (blocks until stdin closes)
    if err := server.ServeStdio(s); err != nil {
        logger.Fatalf("server error: %v", err)
    }
}
```

## Data Flow

```
┌──────────────┐    stdio (JSON-RPC)    ┌─────────────────┐    HTTP/HTTPS    ┌──────────┐
│  MCP Client  │ ◄────────────────────► │ loki-mcp-server │ ──────────────► │   Loki   │
│ (Claude Code)│                        │                 │                  │  Server  │
└──────────────┘                        └─────────────────┘                  └──────────┘
```

1. MCP client spawns `loki-mcp-server` as a subprocess with env vars
2. Client sends JSON-RPC tool calls over stdin
3. Server parses the request, calls Loki HTTP API
4. Loki returns JSON response
5. Server wraps response as MCP tool result, writes to stdout
6. Client receives the result and presents it to the LLM

## Error Handling

- **Configuration errors** — server exits with a descriptive error message to stderr
- **Loki API errors** — parsed from Loki's `{"status": "error", "errorType": "...", "error": "..."}` envelope; returned as `mcp.NewToolResultError()` with a structured message (e.g. `loki bad_data error (HTTP 400): parse error at line 1`)
- **HTTP timeout** — `context.DeadlineExceeded` errors surfaced as tool errors with the configured timeout value
- **Network/TLS errors** — wrapped with context and returned as tool errors
- **Invalid tool parameters** — validated before any HTTP call; returned immediately as tool errors (e.g. `direction must be "forward" or "backward"`, `limit must not exceed 5000`)
- **Success envelope** — HTTP 200 responses with `"status": "error"` are detected and returned as tool errors

The server never crashes on a tool invocation error. Only fatal configuration issues cause an exit.

## Testing Strategy

- **Loki client**: table-driven tests with `httptest.Server` to mock Loki API responses
- **Tool handlers**: table-driven tests with a mock `loki.Client` interface implementation
- **Config**: table-driven tests with `t.Setenv()` for environment variable scenarios
- **Integration**: manual testing against a real Loki instance

## Build

```bash
go build -o loki-mcp-server ./cmd/loki-mcp-server

# Install to system path
go build -o /usr/local/bin/loki-mcp-server ./cmd/loki-mcp-server
```
