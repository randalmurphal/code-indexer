# code-indexer Makefile

BINARY := code-indexer
BUILD_DIR := bin
GO := go
GOFLAGS := -v

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: all build test test-unit test-integration lint fmt vet clean install help

all: build

## Build

build: ## Build the binary
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/code-indexer

install: build ## Install binary to GOPATH/bin
	$(GO) install $(LDFLAGS) ./cmd/code-indexer

## Testing

test: ## Run all tests (skips integration tests without env vars)
	$(GO) test ./... -v

test-unit: ## Run unit tests only (no external dependencies)
	$(GO) test ./internal/parser/... ./internal/chunk/... ./internal/indexer/... -v

test-integration: ## Run integration tests (requires QDRANT_URL, VOYAGE_API_KEY)
	$(GO) test ./internal/store/... ./internal/embedding/... ./test/e2e/... -v

test-coverage: ## Run tests with coverage report
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Code Quality

lint: vet fmt ## Run all linters
	@echo "Lint complete"

vet: ## Run go vet
	$(GO) vet ./...

fmt: ## Check formatting (fails if not formatted)
	@test -z "$$(gofmt -l .)" || (echo "Run 'make fmt-fix' to fix formatting" && gofmt -l . && exit 1)

fmt-fix: ## Fix formatting
	gofmt -w .

## Dependencies

deps: ## Download dependencies
	$(GO) mod download

tidy: ## Tidy go.mod
	$(GO) mod tidy

## Cleanup

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f $(BINARY)

## Infrastructure

infra-up: ## Start local infrastructure (Qdrant, Neo4j, Redis)
	cd ~/repos/graphrag && docker compose up -d

infra-down: ## Stop local infrastructure
	cd ~/repos/graphrag && docker compose down

infra-status: ## Check infrastructure status
	@echo "Checking Qdrant..."  && curl -s http://localhost:6333/health || echo "Qdrant not running"
	@echo "Checking Neo4j..."   && curl -s http://localhost:7474 || echo "Neo4j not running"
	@echo "Checking Redis..."   && redis-cli ping 2>/dev/null || echo "Redis not running"

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
