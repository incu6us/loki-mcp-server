BINARY     := loki-mcp-server
MODULE     := github.com/incu6us/loki-mcp-server
CMD        := ./cmd/loki-mcp-server
E2E        := ./tests/e2e
DEPLOY     := deploy

.PHONY: build test test-unit test-e2e vet lint clean dev-up dev-down

## Build

build: ## Build the binary
	go build -o $(BINARY) $(CMD)

## Test

test: test-unit test-e2e ## Run all tests

test-unit: ## Run unit tests
	go test ./internal/...

test-e2e: ## Run e2e tests, requires Docker
	go test -v -count=1 -timeout 120s $(E2E)

## Code quality

vet: ## Run go vet
	go vet ./...

lint: vet ## Run linters

## Local dev stack

dev-up: ## Start Loki + Grafana + Promtail + log generator
	docker compose -f $(DEPLOY)/docker-compose.yml up -d

dev-down: ## Stop local dev stack
	docker compose -f $(DEPLOY)/docker-compose.yml down

## Cleanup

clean: ## Remove build artifacts
	rm -f $(BINARY)

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-15s %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
