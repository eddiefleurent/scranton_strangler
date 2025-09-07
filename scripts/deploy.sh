#!/bin/bash

# SPY Strangle Bot Deployment Script
# Usage: ./scripts/deploy.sh [environment]
# Environments: staging, production

set -euo pipefail
IFS=$'\n\t'

ENVIRONMENT=${1:-staging}
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Validate environment
if [[ ! "$ENVIRONMENT" =~ ^(staging|production)$ ]]; then
    log_error "Invalid environment: $ENVIRONMENT. Must be 'staging' or 'production'"
    exit 1
fi

log_info "Starting deployment to $ENVIRONMENT environment..."

# Check if config file exists
CONFIG_FILE="$PROJECT_ROOT/config.yaml"
if [[ ! -f "$CONFIG_FILE" ]]; then
    log_error "Configuration file not found: $CONFIG_FILE"
    log_info "Copy config.yaml.example to config.yaml and update with your settings"
    exit 1
fi

# Build the application
log_info "Building Go application..."
cd "$PROJECT_ROOT"
go mod tidy
go test ./...

# Build binary
if [[ "$ENVIRONMENT" == "production" ]]; then
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o strangle-bot cmd/bot/main.go
else
    go build -o strangle-bot cmd/bot/main.go
fi

log_info "Binary built successfully"

# Detect compose command
if command -v docker compose &>/dev/null; then
    COMPOSE="docker compose"
else
    COMPOSE="docker-compose"
fi
COMPOSE=${COMPOSE:-docker-compose}  # Fallback in case neither condition is met

# Ensure bind-mount targets exist (see docker-compose.yml volumes section)
# Creates directories for: ./data:/data:rw and ./logs:/logs:rw
mkdir -p "$PROJECT_ROOT/data"
mkdir -p "$PROJECT_ROOT/data/logs"

# Seed positions.json in data directory if it doesn't exist
if [[ ! -f "$PROJECT_ROOT/data/positions.json" ]]; then
    echo '[]' > "$PROJECT_ROOT/data/positions.json"
    log_info "Created empty positions.json seed file in data directory"
fi

# Determine compose command with appropriate flags
if [[ "$ENVIRONMENT" == "production" ]]; then
    COMPOSE_CMD="$COMPOSE -f docker-compose.yml -f docker-compose.prod.yml"
else
    COMPOSE_CMD="$COMPOSE"
fi

# Stop existing containers
log_info "Stopping existing containers..."
$COMPOSE_CMD down || true

# Start services
log_info "Starting services..."
$COMPOSE_CMD up -d --build

# Wait for services to be healthy
log_info "Waiting for services to be healthy..."
deadline=$((SECONDS+120))
while :; do
  unhealthy=$($COMPOSE_CMD ps --format json | jq -r '.[] | select(.Health!="healthy") | .Name' | wc -l)
  [[ "$unhealthy" -eq 0 ]] && break
  [[ $SECONDS -ge $deadline ]] && break
  sleep 3
done

# Check health
if $COMPOSE_CMD ps --format json | jq -e '.[] | select(.Health!="healthy")' >/dev/null; then
    log_error "Deployment failed. One or more services are not healthy."
    $COMPOSE_CMD logs
    exit 1
else
    log_info "Deployment to $ENVIRONMENT completed successfully!"

    # Show running containers and recent logs
    log_info "Running containers:"
    $COMPOSE_CMD ps
    log_info "Recent logs:"
    $COMPOSE_CMD logs --tail=20 strangle-bot
fi