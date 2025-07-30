#!/bin/bash

# Script to help set up GPG signing for Terraform provider releases
# Run this script to generate a GPG key and get the necessary secrets for GitHub

set -e

echo "🔐 Setting up GPG signing for Terraform provider releases"
echo "=========================================================="

# Check if GPG is installed
if ! command -v gpg &> /dev/null; then
    echo "❌ GPG is not installed. Please install GPG first."
    echo "   On Ubuntu/Debian: sudo apt-get install gnupg"
    echo "   On macOS: brew install gnupg"
    echo "   On Windows: Install GPG4Win"
    exit 1
fi

# Generate GPG key
echo "📝 Generating GPG key..."
echo "Please provide the following information:"
echo "  - Name: Your Name"
echo "  - Email: your-email@example.com"
echo "  - Comment: Terraform Provider Signing Key"
echo "  - Passphrase: (create a strong passphrase)"

gpg --full-generate-key

# Get the key ID
KEY_ID=$(gpg --list-secret-keys --keyid-format LONG | grep sec | tail -1 | awk '{print $2}' | cut -d'/' -f2)

echo ""
echo "✅ GPG key generated successfully!"
echo "Key ID: $KEY_ID"

# Export the private key
echo ""
echo "📤 Exporting private key for GitHub Secrets..."
gpg --armor --export-secret-key "$KEY_ID" > private-key.gpg

# Export the public key
echo "📤 Exporting public key..."
gpg --armor --export "$KEY_ID" > public-key.gpg

echo ""
echo "🔑 GPG Setup Complete!"
echo "======================"
echo ""
echo "📋 Next steps:"
echo "1. Add the following secrets to your GitHub repository:"
echo "   - GPG_PRIVATE_KEY: (copy the contents of private-key.gpg)"
echo "   - GPG_PASSPHRASE: (the passphrase you used)"
echo ""
echo "2. Add the public key to your GitHub account:"
echo "   - Go to Settings > SSH and GPG keys"
echo "   - Click 'New GPG key'"
echo "   - Paste the contents of public-key.gpg"
echo ""
echo "3. Clean up the temporary files:"
echo "   rm private-key.gpg public-key.gpg"
echo ""
echo "4. Test the release process:"
echo "   - Create a tag: git tag v1.0.0"
echo "   - Push the tag: git push origin v1.0.0"
echo ""

# Show the keys for easy copying
echo "📄 Private Key (for GPG_PRIVATE_KEY secret):"
echo "--------------------------------------------"
cat private-key.gpg
echo ""
echo "📄 Public Key (for GitHub GPG keys):"
echo "------------------------------------"
cat public-key.gpg