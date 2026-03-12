# loki-mcp-server

[![CI](https://github.com/incu6us/loki-mcp-server/actions/workflows/ci.yml/badge.svg)](https://github.com/incu6us/loki-mcp-server/actions/workflows/ci.yml)
[![Release](https://github.com/incu6us/loki-mcp-server/actions/workflows/release.yml/badge.svg)](https://github.com/incu6us/loki-mcp-server/actions/workflows/release.yml)
[![codecov](https://codecov.io/gh/incu6us/loki-mcp-server/branch/main/graph/badge.svg)](https://codecov.io/gh/incu6us/loki-mcp-server)
[![Go Report Card](https://goreportcard.com/badge/github.com/incu6us/loki-mcp-server)](https://goreportcard.com/report/github.com/incu6us/loki-mcp-server)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A [Model Context Protocol](https://modelcontextprotocol.io/) (MCP) server that exposes Grafana Loki log querying capabilities as MCP tools. Designed to run as a stdio subprocess for MCP clients such as Claude Desktop and Claude Code.

## Motivation

The official [grafana/loki-mcp](https://github.com/grafana/loki-mcp) exposes a single `loki_query` tool, which means the LLM must already know valid label names and values before it can build a query. This project takes a different approach by providing **5 granular tools** — `labels`, `label_values`, and `series` let the LLM discover what's available in Loki first, then construct precise `query_range` or `query` calls. The result is more accurate log retrieval with fewer wasted round-trips.

Additionally, this server enforces **strict input validation** (limit caps, direction validation, label name format checks, mutually exclusive auth) to surface errors early instead of forwarding bad requests to Loki.

## Features

- **query_range** — Execute LogQL range queries to fetch logs over a time window
- **query** — Execute LogQL instant queries for point-in-time evaluation
- **labels** — List all available label names
- **label_values** — List values for a specific label
- **series** — Find active log stream series matching a selector

## Installation

### Homebrew

```bash
brew install incu6us/tap/loki-mcp-server
```

### Go install

```bash
go install github.com/incu6us/loki-mcp-server/cmd/loki-mcp-server@latest
```

Or build from source:

```bash
git clone https://github.com/incu6us/loki-mcp-server.git
cd loki-mcp
go build -o loki-mcp-server ./cmd/loki-mcp-server
```

## Configuration

The server is configured entirely via environment variables, injected by the MCP client.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LOKI_URL` | yes | — | Base URL of the Loki instance |
| `LOKI_USERNAME` | no | — | Basic auth username |
| `LOKI_PASSWORD` | no | — | Basic auth password |
| `LOKI_BEARER_TOKEN` | no | — | Bearer token authentication |
| `LOKI_TLS_SKIP_VERIFY` | no | `false` | Skip TLS certificate verification |
| `LOKI_TENANT_ID` | no | — | `X-Scope-OrgID` header for multi-tenant deployments |
| `LOKI_HTTP_TIMEOUT` | no | `30s` | HTTP request timeout (Go duration, e.g. `10s`, `1m`) |

> **Note:** Basic auth (`LOKI_USERNAME`/`LOKI_PASSWORD`) and bearer token (`LOKI_BEARER_TOKEN`) are mutually exclusive.

## Usage with Claude Code

Add to your Claude Code MCP configuration:

```json
{
  "mcpServers": {
    "loki-mcp-server": {
      "type": "stdio",
      "command": "loki-mcp-server",
      "env": {
        "LOKI_URL": "http://loki:3100",
        "LOKI_TENANT_ID": "my-tenant"
      }
    }
  }
}
```

## Usage with Claude Desktop

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "loki-mcp-server": {
      "type": "stdio",
      "command": "/path/to/loki-mcp-server",
      "args": [],
      "env": {
        "LOKI_URL": "http://loki:3100",
        "LOKI_USERNAME": "admin",
        "LOKI_PASSWORD": "secret"
      }
    }
  }
}
```

## Tools

### query_range

Execute a LogQL range query against Loki to fetch logs over a time window.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | LogQL query expression |
| `start` | string | no | 1 hour ago | Start of time range (RFC3339 or Unix nano) |
| `end` | string | no | now | End of time range |
| `limit` | number | no | 100 | Max entries (max 5000) |
| `direction` | string | no | backward | `forward` or `backward` |

### query

Execute a LogQL instant query for point-in-time evaluation.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `query` | string | yes | — | LogQL query expression |
| `limit` | number | no | 100 | Max entries (max 5000) |
| `time` | string | no | now | Evaluation timestamp |
| `direction` | string | no | backward | `forward` or `backward` |

### labels

List all available label names in Loki.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

### label_values

List values for a specific label.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `label` | string | yes | — | Label name |
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

### series

Find active log stream series matching a selector.

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `match` | string | yes | — | Stream selector (e.g. `{app="nginx"}`) |
| `start` | string | no | 6 hours ago | Start of time range |
| `end` | string | no | now | End of time range |

## Local Development Stack

A Docker Compose setup is included under `deploy/` to spin up a full Loki environment for testing:

- **Loki** — log storage at `http://localhost:3100`
- **Grafana** — UI at `http://localhost:3000` (anonymous admin, Loki pre-configured as datasource)
- **Promtail** — collects container logs and ships them to Loki
- **Log generator** — emits structured JSON logs with randomized apps (`nginx`, `api`, `gateway`, `auth`, `payments`), levels, and messages

```bash
# Start the stack
docker compose -f deploy/docker-compose.yml up -d

# Use loki-mcp-server against local Loki
LOKI_URL=http://localhost:3100 loki-mcp-server

# Stop the stack
docker compose -f deploy/docker-compose.yml down
```

## Development

```bash
# Run tests
go test ./...

# Build
go build -o loki-mcp-server ./cmd/loki-mcp-server

# Vet
go vet ./...
```

## License

MIT
