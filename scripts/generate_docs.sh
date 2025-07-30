#!/bin/bash

set -euo pipefail

# Documentation generation script for terraform-provider-graphql
# This script generates documentation for the provider

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="terraform-provider-graphql"
DOCS_DIR="./docs"
WEBSITE_DIR="./docsite"

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

    # Check if tfplugindocs is installed
    if ! command -v tfplugindocs &> /dev/null; then
        log_info "Installing tfplugindocs..."
        go install github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest
    fi

    log_info "Prerequisites check passed"
}

# Generate provider documentation
generate_provider_docs() {
    log_info "Generating provider documentation..."

    # Create docs directory if it doesn't exist
    mkdir -p "$DOCS_DIR"

    # Generate documentation using tfplugindocs
    tfplugindocs generate --provider-name graphql --rendered-provider-name "GraphQL"

    if [ $? -eq 0 ]; then
        log_info "Provider documentation generated successfully"
    else
        log_error "Failed to generate provider documentation"
        exit 1
    fi
}

# Generate website documentation
generate_website_docs() {
    log_info "Generating website documentation..."

    if [ -d "$WEBSITE_DIR" ]; then
        cd "$WEBSITE_DIR"

        # Install dependencies if needed
        if [ -f "package.json" ]; then
            log_info "Installing website dependencies..."
            npm install
        fi

        # Build website
        if [ -f "package.json" ] && grep -q "build" package.json; then
            log_info "Building website..."
            npm run build
        fi

        cd - > /dev/null
        log_info "Website documentation generated successfully"
    else
        log_warn "Website directory not found, skipping website generation"
    fi
}

# Validate documentation
validate_docs() {
    log_info "Validating documentation..."

    # Check if required documentation files exist
    required_files=(
        "docs/index.md"
        "docs/data-sources/query.md"
        "docs/resources/mutation.md"
    )

    for file in "${required_files[@]}"; do
        if [ ! -f "$file" ]; then
            log_error "Required documentation file missing: $file"
            exit 1
        fi
    done

    log_info "Documentation validation passed"
}

# Update README
update_readme() {
    log_info "Updating README..."

    # Check if README.md exists
    if [ -f "README.md" ]; then
        # Add documentation section if it doesn't exist
        if ! grep -q "## Documentation" README.md; then
            cat >> README.md << 'EOF'

## Documentation

For detailed documentation, see the [docs](./docs) directory:

- [Provider Configuration](./docs/index.md)
- [Data Sources](./docs/data-sources/)
- [Resources](./docs/resources/)

To generate documentation locally:

```bash
./scripts/generate_docs.sh
```
EOF
        fi
        log_info "README updated successfully"
    else
        log_warn "README.md not found, skipping README update"
    fi
}

# Main documentation generation process
main() {
    log_info "Starting documentation generation for ${PROJECT_NAME}"

    check_prerequisites
    generate_provider_docs
    generate_website_docs
    validate_docs
    update_readme

    log_info "Documentation generation completed successfully!"
    log_info "Documentation is available in: ${DOCS_DIR}"
}

# Run main function
main "$@"