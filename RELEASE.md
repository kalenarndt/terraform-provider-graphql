# Release Guide

This guide explains how to set up and use the GitHub Actions workflow for building and releasing the Terraform GraphQL provider.

## Prerequisites

1. **GPG Key Setup**: You need a GPG key for signing releases
2. **GitHub Secrets**: Configure the required secrets in your repository
3. **GitHub Token**: Ensure the workflow has access to create releases

## Setup

### 1. Generate GPG Key

Run the setup script to generate a GPG key:

```bash
./scripts/setup-gpg.sh
```

This script will:
- Generate a new GPG key
- Export the private key for GitHub Secrets
- Export the public key for GitHub GPG keys
- Provide instructions for the next steps

### 2. Configure GitHub Secrets

Add the following secrets to your GitHub repository (Settings > Secrets and variables > Actions):

- `GPG_PRIVATE_KEY`: The contents of the private key file
- `GPG_PASSPHRASE`: The passphrase you used when creating the GPG key

### 3. Add GPG Key to GitHub

1. Go to your GitHub account settings
2. Navigate to "SSH and GPG keys"
3. Click "New GPG key"
4. Paste the contents of the public key file

## Workflow Overview

The GitHub Actions workflow includes several jobs:

### Test Job
- Runs all tests with coverage
- Uploads coverage to Codecov
- Ensures code quality

### Lint Job
- Runs golangci-lint
- Checks for code style and potential issues

### Build Job
- Builds the provider for all supported platforms
- Creates snapshot releases for testing
- Runs on every push to master/main

### Release Job
- Creates signed releases
- Only runs when you push a tag (e.g., `v1.0.0`)
- Signs artifacts with GPG
- Creates GitHub releases

## Making a Release

### 1. Prepare for Release

Ensure your code is ready:
- All tests pass
- Documentation is updated
- Version is updated in code if needed

### 2. Create and Push a Tag

```bash
# Create a new tag
git tag v1.0.0

# Push the tag to trigger the release
git push origin v1.0.0
```

### 3. Monitor the Release

1. Go to the "Actions" tab in your GitHub repository
2. Monitor the "Release" job
3. Check that the release is created successfully
4. Verify the artifacts are signed

## Supported Platforms

The provider is built for the following platforms:

- **Linux**: amd64, 386, arm, arm64
- **Windows**: amd64, 386, arm, arm64
- **macOS**: amd64, arm64
- **FreeBSD**: amd64, 386, arm, arm64

## Release Artifacts

Each release includes:

- Binary files for each platform
- SHA256 checksums
- GPG signatures for the checksums
- Source code archive
- Documentation files

## Troubleshooting

### Common Issues

1. **GPG Signing Fails**
   - Ensure the GPG_PRIVATE_KEY secret is correct
   - Check that the GPG_PASSPHRASE is set
   - Verify the GPG key is properly imported

2. **Build Fails**
   - Check that all tests pass locally
   - Ensure the Go version is compatible
   - Verify all dependencies are available

3. **Release Not Created**
   - Ensure you're pushing a tag (not just a commit)
   - Check that the tag follows the `v*` pattern
   - Verify the GitHub token has release permissions

### Debugging

To debug issues:

1. Check the GitHub Actions logs
2. Run GoReleaser locally: `goreleaser release --snapshot --clean`
3. Test the build locally: `go build -o terraform-provider-graphql`

## Local Development

For local development and testing:

```bash
# Build locally
go build -o terraform-provider-graphql

# Run tests
go test -v ./...

# Run linter
golangci-lint run

# Test GoReleaser locally
goreleaser release --snapshot --clean
```

## Version Management

The version is automatically determined from the Git tag. To update the version in your code:

1. Update any version constants in your code
2. Create a new tag: `git tag v1.1.0`
3. Push the tag: `git push origin v1.1.0`

The GoReleaser will automatically:
- Extract the version from the tag
- Build for all platforms
- Create a signed release
- Upload artifacts to GitHub


