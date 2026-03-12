BINARY     := loki-mcp-server
MODULE     := github.com/incu6us/loki-mcp-server
CMD        := ./cmd/loki-mcp-server
E2E        := ./tests/e2e
DEPLOY     := deploy

## Build
.PHONY: build
build: ## Build the binary
	go build -o $(BINARY) $(CMD)

## Test
.PHONY: test
test: test-unit test-e2e ## Run all tests

.PHONY: test-unit
test-unit: ## Run unit tests
	go test ./internal/...

.PHONY: test-e2e
test-e2e: ## Run e2e tests, requires Docker
	go test -v -count=1 -timeout 120s $(E2E)

## Code quality
.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: vet ## Run linters

## Local dev stack
.PHONY: dev-up
dev-up: ## Start Loki + Grafana + Promtail + log generator
	docker compose -f $(DEPLOY)/docker-compose.yml up -d

.PHONY: dev-down
dev-down: ## Stop local dev stack
	docker compose -f $(DEPLOY)/docker-compose.yml down

## Cleanup
.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BINARY)

## Help
.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-15s %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
