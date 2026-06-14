.PHONY: build test test-race vet lint lint-all audit vulncheck coverage ci clean install help

BINARY := gochaos
MODULE := github.com/sunny809/gochaos
GO     := go

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the gochaos CLI binary
	$(GO) build -o $(BINARY) ./cmd/gmock

install: ## Install gochaos to $GOPATH/bin
	$(GO) install ./cmd/gmock

test: ## Run all tests
	$(GO) test ./...

test-race: ## Run all tests with race detector
	$(GO) test -race ./...

test-verbose: ## Run all tests with verbose output
	$(GO) test -race -v ./...

vet: ## Run go vet (with shadow detection)
	$(GO) vet -vettool=$(which shadow) ./... 2>/dev/null || $(GO) vet ./...

lint: ## Run golangci-lint (fast, PR gating)
	golangci-lint run --timeout 5m ./...

lint-all: ## Run golangci-lint with all checks including slow linters
	golangci-lint run --timeout 10m ./...

audit: ## Run nilaway nil pointer analysis
	which nilaway >/dev/null 2>&1 || { echo "nilaway not installed. Run: go install go.uber.org/nilaway/cmd/nilaway@latest"; exit 1; }
	nilaway ./...

vulncheck: ## Run govulncheck for dependency vulnerabilities
	which govulncheck >/dev/null 2>&1 || { echo "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1; }
	govulncheck ./...

coverage: ## Run tests with coverage report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem ./...

tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

ci: vet lint test-race ## Full CI pipeline (vet + golangci-lint + race tests)

ci-full: vet lint-all audit test-race ## Extended CI (all checks + nil analysis)

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out coverage.html
