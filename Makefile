.PHONY: build test test-race vet lint coverage ci clean install help

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

vet: ## Run go vet
	$(GO) vet ./...

coverage: ## Run tests with coverage report
	$(GO) test -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem ./...

tidy: ## Tidy go.mod and go.sum
	$(GO) mod tidy

ci: vet test-race ## Full CI pipeline (vet + race tests)

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out coverage.html
