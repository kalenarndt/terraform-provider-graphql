.PHONY: help build test lint clean release-snapshot setup-gpg

# Default target
help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Build the provider locally
build: ## Build the provider locally
	go build -o terraform-provider-graphql

# Run tests
test: ## Run all tests
	go test -v -coverprofile=coverage.out ./...

# Run tests with coverage report
test-coverage: test ## Run tests and show coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint: ## Run golangci-lint
	golangci-lint run

# Clean build artifacts
clean: ## Clean build artifacts
	rm -rf dist/
	rm -f terraform-provider-graphql
	rm -f coverage.out coverage.html

# Generate documentation
docs: ## Generate documentation
	go generate ./...

# Run GoReleaser snapshot
release-snapshot: ## Create a snapshot release locally
	goreleaser release --snapshot --clean --skip-publish

# Setup GPG key
setup-gpg: ## Setup GPG key for signing releases
	@echo "Setting up GPG key for signing releases..."
	@chmod +x scripts/setup-gpg.sh
	./scripts/setup-gpg.sh

# Install dependencies
deps: ## Install dependencies
	go mod download
	go mod tidy

# Format code
fmt: ## Format Go code
	go fmt ./...

# Vet code
vet: ## Vet Go code
	go vet ./...

# Check for security issues
security: ## Check for security issues
	gosec ./...

# Full development workflow
dev: deps fmt vet lint test ## Run full development workflow

# Create a new release
release: ## Create a new release (requires tag)
	@echo "Creating release..."
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG is required. Usage: make release TAG=v1.0.0"; \
		exit 1; \
	fi
	git tag $(TAG)
	git push origin $(TAG)

# Show current version
version: ## Show current version
	@echo "Current version: $(shell git describe --tags --abbrev=0 2>/dev/null || echo "No tags found")"

# Install provider locally
install: build ## Build and install provider locally
	@echo "Installing provider locally..."
	@mkdir -p ~/.terraform.d/plugins/registry.terraform.io/kalenarndt/graphql/0.0.0/$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m)
	@cp terraform-provider-graphql ~/.terraform.d/plugins/registry.terraform.io/kalenarndt/graphql/0.0.0/$(shell uname -s | tr '[:upper:]' '[:lower:]')_$(shell uname -m)/

# Run integration tests
test-integration: ## Run integration tests
	@echo "Running integration tests..."
	cd e2e && go test -v

# Show help by default
.DEFAULT_GOAL := help