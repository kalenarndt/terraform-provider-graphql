#!/bin/bash

set -euo pipefail

# Build script for terraform-provider-graphql
# This script ensures consistent builds across different environments

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="terraform-provider-graphql"
VERSION=${VERSION:-"dev"}
BUILD_DIR="./dist"
CGO_ENABLED=${CGO_ENABLED:-0}

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

    if ! command -v git &> /dev/null; then
        log_error "Git is not installed or not in PATH"
        exit 1
    fi

    log_info "Prerequisites check passed"
}

# Clean build directory
clean_build() {
    log_info "Cleaning build directory..."
    rm -rf "$BUILD_DIR"
    mkdir -p "$BUILD_DIR"
}

# Download dependencies
download_deps() {
    log_info "Downloading dependencies..."
    go mod download
    go mod verify
}

# Run tests
run_tests() {
    log_info "Running tests..."
    go test -v -coverprofile=coverage.out ./...

    if [ -f coverage.out ]; then
        coverage=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        log_info "Test coverage: ${coverage}%"
    fi
}

# Build provider
build_provider() {
    log_info "Building provider..."

    # Set build flags
    LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse HEAD 2>/dev/null || echo 'unknown')"

    # Build for multiple platforms
    platforms=(
        # "linux/amd64"
        # "linux/arm64"
        # "darwin/amd64"
        "darwin/arm64"
        # "windows/amd64"
        # "windows/386"
    )

    for platform in "${platforms[@]}"; do
        IFS='/' read -r os arch <<< "$platform"

        log_info "Building for ${os}/${arch}..."

        output_dir="${BUILD_DIR}/${PROJECT_NAME}_${VERSION}_${os}_${arch}"
        mkdir -p "$output_dir"

        # Skip darwin/386 as it's not supported
        if [[ "$os" == "darwin" && "$arch" == "386" ]]; then
            log_warn "Skipping darwin/386 (not supported)"
            continue
        fi

        env CGO_ENABLED=$CGO_ENABLED GOOS=$os GOARCH=$arch go build \
            -ldflags="$LDFLAGS" \
            -trimpath \
            -o "$output_dir/${PROJECT_NAME}" \
            .

        if [ $? -eq 0 ]; then
            log_info "Successfully built for ${os}/${arch}"
        else
            log_error "Failed to build for ${os}/${arch}"
            exit 1
        fi
    done
}

# Create checksums
create_checksums() {
    log_info "Creating checksums..."
    cd "$BUILD_DIR"
    find . -name "${PROJECT_NAME}_${VERSION}_*" -type f | while read -r file; do
        sha256sum "$file" >> "${PROJECT_NAME}_${VERSION}_SHA256SUMS"
    done
    cd - > /dev/null
}

# Main build process
main() {
    log_info "Starting build process for ${PROJECT_NAME} v${VERSION}"

    check_prerequisites
    clean_build
    download_deps
    run_tests
    build_provider
    create_checksums

    log_info "Build completed successfully!"
    log_info "Build artifacts are in: ${BUILD_DIR}"
}

# Run main function
main "$@"