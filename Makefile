.PHONY: help build test integration-test lint fmt clean web stage-web dev release ci all

# Default goal
.DEFAULT_GOAL := help

# Variables
BINARY      := ccx
GO_PACKAGES := ./...
LDFLAGS     := -s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

stage-web: ## Stage web/out into internal/dashboard for go:embed
	@if [ ! -d web/out ]; then \
		printf "web/out not found: run 'pnpm build' in web/ before staging\n" >&2; \
		exit 1; \
	fi
	rm -rf internal/dashboard/web-out
	mkdir -p internal/dashboard/web-out
	cp -R web/out/. internal/dashboard/web-out/
	touch internal/dashboard/web-out/.gitkeep

build: stage-web ## Build the ccx binary
	@mkdir -p dist
	go build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BINARY) ./cmd/ccx

test: ## Run all Go tests
	go test -race -count=1 $(GO_PACKAGES)

integration-test: stage-web ## Run integration tests
	go test -tags integration -count=1 ./integration_test/...

lint: ## Run linters
	golangci-lint run

fmt: ## Format all Go code
	gofumpt -w .

clean: ## Remove build artifacts
	rm -rf dist web/out web/.next

web: ## Build the Next.js dashboard (Phase 1 A7)
	@echo "web build not yet wired — see Phase 1 plan A7"

dev: ## Run dev mode (CLI + dashboard) — Phase 2
	@echo "dev mode not yet wired — see Phase 2 plan"

release: ## Run goreleaser locally (Phase 1 A8)
	@echo "release not yet wired — see Phase 1 plan A8"

ci: lint test ## Run the full CI gate locally

all: clean fmt lint test build ## Full local pipeline
