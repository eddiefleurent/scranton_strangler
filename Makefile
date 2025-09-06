# SPY Strangle Bot - Makefile

.PHONY: all help build build-prod test test-coverage lint run clean \
        docker-build docker-run deploy-staging deploy-prod logs stop \
        dev-setup test-api security-scan build-test-helper

all: build

# Default target
help:
	@echo "Targets:"
	@echo "  build/test/lint/run/clean"
	@echo "  docker-build/docker-run/logs/stop"
	@echo "  deploy-staging/deploy-prod"
	@echo "  test-coverage/security-scan/build-test-helper"
	@echo "  dev-setup"

# Go binary name
BINARY_NAME=strangle-bot
BINARY_PATH=./$(BINARY_NAME)

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) cmd/bot/main.go

# Build for production (optimized)
build-prod:
	@echo "Building $(BINARY_NAME) for production..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o $(BINARY_NAME) cmd/bot/main.go

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run

# Run the bot locally
run: build
	@echo "Starting $(BINARY_NAME)..."
	$(BINARY_PATH) --config=config.yaml

# Clean build artifacts
clean:
	@echo "Cleaning..."
	go clean -testcache
	rm -f $(BINARY_NAME) coverage.out coverage.html *.test *.prof

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker build -t strangle-bot:latest .

docker-run:
	@echo "Starting services with Docker Compose..."
	docker compose up -d

# Deployment commands
deploy-staging:
	@echo "Deploying to staging..."
	./scripts/deploy.sh staging

deploy-prod:
	@echo "Deploying to production..."
	./scripts/deploy.sh production

# Utility commands
logs:
	@echo "Showing container logs..."
	docker compose logs -f strangle-bot

stop:
	@echo "Stopping all containers..."
	docker compose down

# Development helpers
dev-setup:
	@echo "Setting up development environment..."
	@if [ -e config.yaml ]; then \
		echo "config.yaml already exists; skipping copy"; \
	else \
		cp config.yaml.example config.yaml && echo "Created config.yaml — please update with your settings"; \
	fi

# API testing
test-api:
	@echo "Testing Tradier API connection..."
	cd scripts/test_tradier && go run test_tradier.go

# Build test helper
build-test-helper:
	@echo "Building test helper..."
	go build -tags test -o scripts/test_helper/test_helper scripts/test_helper/test_helper.go

# Security scan
security-scan:
	@echo "Running security scan..."
	gosec ./...
	govulncheck ./...