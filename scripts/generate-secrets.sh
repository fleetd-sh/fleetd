#!/bin/bash

# Generate secure secrets for fleetd
# This script generates cryptographically secure random secrets

set -e

echo "🔐 fleetd Secret Generator"
echo "=========================="
echo ""

# Function to generate secure random string
generate_secret() {
    local length=$1
    local type=$2

    if [ "$type" = "base64" ]; then
        openssl rand -base64 $length | tr -d '\n'
    else
        openssl rand -hex $length | tr -d '\n'
    fi
}

# Check if .env file exists
if [ -f ".env" ]; then
    echo "⚠️  Warning: .env file already exists!"
    read -p "Do you want to backup existing .env and create a new one? (y/n): " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Exiting without changes."
        exit 0
    fi

    # Backup existing .env
    backup_file=".env.backup.$(date +%Y%m%d_%H%M%S)"
    cp .env "$backup_file"
    echo "✅ Backed up existing .env to $backup_file"
fi

# Copy from example if it exists
if [ -f ".env.example" ]; then
    cp .env.example .env
    echo "✅ Created .env from .env.example"
else
    touch .env
    echo "✅ Created new .env file"
fi

# Generate secure secrets
echo ""
echo "🔑 Generating secure secrets..."
echo ""

# PostgreSQL password (32 bytes base64)
POSTGRES_PASSWORD=$(generate_secret 32 base64)
echo "POSTGRES_PASSWORD=$POSTGRES_PASSWORD"

# JWT Secret (64 bytes base64 for HS256)
JWT_SECRET=$(generate_secret 64 base64)
echo "JWT_SECRET=$JWT_SECRET"

# API Key (32 bytes hex)
API_KEY=$(generate_secret 32 hex)
echo "API_KEY=fld_$API_KEY"

# Valkey/Redis password (24 bytes base64)
VALKEY_PASSWORD=$(generate_secret 24 base64)
echo "VALKEY_PASSWORD=$VALKEY_PASSWORD"

# Grafana admin password (20 bytes base64)
GRAFANA_PASSWORD=$(generate_secret 20 base64)
echo "GRAFANA_PASSWORD=$GRAFANA_PASSWORD"

# Session secret (32 bytes base64)
SESSION_SECRET=$(generate_secret 32 base64)
echo "SESSION_SECRET=$SESSION_SECRET"

echo ""
echo "📝 Writing secrets to .env file..."

# Function to update or add environment variable
update_env() {
    local key=$1
    local value=$2

    if grep -q "^$key=" .env; then
        # Update existing
        if [[ "$OSTYPE" == "darwin"* ]]; then
            # macOS
            sed -i '' "s|^$key=.*|$key=$value|" .env
        else
            # Linux
            sed -i "s|^$key=.*|$key=$value|" .env
        fi
    else
        # Add new
        echo "$key=$value" >> .env
    fi
}

# Update .env file with generated secrets
update_env "POSTGRES_PASSWORD" "$POSTGRES_PASSWORD"
update_env "JWT_SECRET" "$JWT_SECRET"
update_env "API_KEY" "fld_$API_KEY"
update_env "VALKEY_PASSWORD" "$VALKEY_PASSWORD"
update_env "GRAFANA_PASSWORD" "$GRAFANA_PASSWORD"
update_env "SESSION_SECRET" "$SESSION_SECRET"

echo ""
echo "✅ Secrets successfully generated and saved to .env"
echo ""
echo "⚠️  IMPORTANT SECURITY NOTES:"
echo "   1. NEVER commit .env file to version control"
echo "   2. Keep .env file permissions restrictive (chmod 600 .env)"
echo "   3. Store production secrets in a secure secret manager"
echo "   4. Rotate secrets regularly"
echo "   5. Use different secrets for each environment"
echo ""

# Set restrictive permissions
chmod 600 .env
echo "✅ Set restrictive permissions on .env (600)"

# Verify .gitignore
if ! grep -q "^\.env$\|^\.env\*" .gitignore 2>/dev/null; then
    echo ""
    echo "⚠️  WARNING: .env is not in .gitignore!"
    echo "   Adding .env* to .gitignore..."
    echo ".env*" >> .gitignore
    echo "!.env.example" >> .gitignore
fi

echo ""
echo "🎉 Secret generation complete!"
echo ""
echo "To use these secrets with Docker Compose:"
echo "  docker-compose up"
echo ""
echo "To use these secrets with the application:"
echo "  source .env && just platform-api-dev"
echo ""
