#!/bin/bash

# SPY Strangle Bot Deployment Script
# Usage: ./scripts/deploy.sh [environment]
# Environments: staging, production

set -e

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
if [[ $? -ne 0 ]]; then
    log_error "Tests failed. Aborting deployment."
    exit 1
fi

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

# Ensure bind-mount targets exist
mkdir -p "$PROJECT_ROOT/logs"
touch "$PROJECT_ROOT/positions.json"

# Stop existing containers
log_info "Stopping existing containers..."
$COMPOSE down || true

# Start services
log_info "Starting services..."
if [[ "$ENVIRONMENT" == "production" ]]; then
    $COMPOSE -f docker-compose.yml -f docker-compose.prod.yml up -d --build
else
    $COMPOSE up -d --build
fi

# Wait for services to be healthy
log_info "Waiting for services to be ready..."
sleep 10

# Check if services are running
if $COMPOSE ps | grep -q "Up"; then
    log_info "Deployment to $ENVIRONMENT completed successfully!"

    # Show running containers
    log_info "Running containers:"
    $COMPOSE ps

    # Show logs
    log_info "Recent logs:"
    $COMPOSE logs --tail=20 strangle-bot
else
    log_error "Deployment failed. Services are not running properly."
    $COMPOSE logs
    exit 1
fi