#!/bin/bash

set -euo pipefail

# Linting script for terraform-provider-graphql
# This script runs various code quality checks

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="terraform-provider-graphql"
LINT_TIMEOUT="5m"

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."

    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        exit 1
    fi

    # Check if golangci-lint is installed
    if ! command -v golangci-lint &> /dev/null; then
        log_info "Installing golangci-lint..."
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    fi

    log_info "Prerequisites check passed"
}

# Run go fmt
run_gofmt() {
    log_info "Running go fmt..."

    # Check for formatting issues
    unformatted_files=$(go fmt ./... 2>&1)

    if [ -n "$unformatted_files" ]; then
        log_error "Code formatting issues found:"
        echo "$unformatted_files"
        exit 1
    fi

    log_info "Code formatting check passed"
}

# Run go vet
run_govet() {
    log_info "Running go vet..."

    if ! go vet ./...; then
        log_error "go vet found issues"
        exit 1
    fi

    log_info "go vet check passed"
}

# Run golangci-lint
run_golangci_lint() {
    log_info "Running golangci-lint..."

    # Create .golangci.yml if it doesn't exist
    if [ ! -f ".golangci.yml" ]; then
        cat > .golangci.yml << 'EOF'
run:
  timeout: 5m
  go: "1.21"

linters:
  enable:
    - gofmt
    - goimports
    - govet
    - errcheck
    - staticcheck
    - gosimple
    - ineffassign
    - unused
    - misspell
    - gosec
    - gocyclo
    - dupl
    - gocritic
    - godot
    - godox
    - err113
    - mnd
    - goprintffuncname
    - nakedret
    - noctx
    - nolintlint
    - rowserrcheck
    - stylecheck
    - typecheck
    - unconvert
    - unparam
    - whitespace

linters-settings:
  gocyclo:
    min-complexity: 15
  dupl:
    threshold: 100
  gocritic:
    enabled-tags:
      - diagnostic
      - experimental
      - opinionated
      - performance
      - style

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - mnd
        - gocyclo
        - dupl
    - path: testdata/
      linters:
        - all
EOF
    fi

    if ! golangci-lint run --timeout="$LINT_TIMEOUT"; then
        log_error "golangci-lint found issues"
        exit 1
    fi

    log_info "golangci-lint check passed"
}

# Run go mod tidy
run_gomod_tidy() {
    log_info "Running go mod tidy..."

    if ! go mod tidy; then
        log_error "go mod tidy failed"
        exit 1
    fi

    log_info "go mod tidy check passed"
}

# Run go mod verify
run_gomod_verify() {
    log_info "Running go mod verify..."

    if ! go mod verify; then
        log_error "go mod verify failed"
        exit 1
    fi

    log_info "go mod verify check passed"
}

# Run security scan
run_security_scan() {
    log_info "Running security scan..."

    # Check if gosec is available
    if command -v gosec &> /dev/null; then
        if ! gosec ./...; then
            log_warn "Security scan found issues (non-blocking)"
        else
            log_info "Security scan passed"
        fi
    else
        log_warn "gosec not installed, skipping security scan"
    fi
}

# Run test coverage
run_test_coverage() {
    log_info "Running test coverage..."

    coverage_file="coverage.out"

    # Run tests with coverage
    if ! go test -v -coverprofile="$coverage_file" -covermode=atomic ./...; then
        log_error "Tests failed"
        exit 1
    fi

    # Check coverage threshold (80%)
    if [ -f "$coverage_file" ]; then
        coverage=$(go tool cover -func="$coverage_file" | grep total | awk '{print $3}' | sed 's/%//')
        log_info "Test coverage: ${coverage}%"

        if (( $(echo "$coverage < 80" | bc -l) )); then
            log_warn "Test coverage is below 80% (${coverage}%)"
        fi
    fi

    log_info "Test coverage check passed"
}

# Main linting process
main() {
    log_info "Starting linting process for ${PROJECT_NAME}"

    check_prerequisites
    run_gofmt
    run_govet
    run_golangci_lint
    run_gomod_tidy
    run_gomod_verify
    run_security_scan
    run_test_coverage

    log_info "All linting checks passed!"
}

# Run main function
main "$@"