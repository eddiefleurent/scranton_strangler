# SPY Strangle Bot - Makefile

.PHONY: help build test lint run clean docker-build docker-run deploy-staging deploy-prod

# Default target
help:
	@echo "Available commands:"
	@echo "  build           - Build the Go binary"
	@echo "  test            - Run all tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  lint            - Run linter"
	@echo "  run             - Run the bot locally"
	@echo "  clean           - Clean build artifacts"
	@echo "  docker-build    - Build Docker image"
	@echo "  docker-run      - Run with Docker Compose"
	@echo "  deploy-staging  - Deploy to staging environment"
	@echo "  deploy-prod     - Deploy to production environment"
	@echo "  logs            - Show container logs"
	@echo "  stop            - Stop all containers"

# Go binary name
BINARY_NAME=strangle-bot
BINARY_PATH=./$(BINARY_NAME)

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	go mod tidy
	go build -o $(BINARY_NAME) cmd/bot/main.go

# Build for production (optimized)
build-prod:
	@echo "Building $(BINARY_NAME) for production..."
	go mod tidy
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
	go clean
	rm -f $(BINARY_NAME)
	rm -f bot
	rm -f main
	rm -f test_otoco
	rm -f test-tradier
	rm -f coverage.out coverage.html
	rm -f *.test
	rm -f *.prof

# Docker commands
docker-build:
	@echo "Building Docker image..."
	docker build -t strangle-bot:latest .

docker-run:
	@echo "Starting services with Docker Compose..."
	docker-compose up -d

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
	docker-compose logs -f strangle-bot

stop:
	@echo "Stopping all containers..."
	docker-compose down

# Development helpers
dev-setup:
	@echo "Setting up development environment..."
	cp config.yaml.example config.yaml
	@echo "Please update config.yaml with your settings"

# API testing
test-api:
	@echo "Testing Tradier API connection..."
	cd scripts/test_otoco && go run test_otoco.go

# Security scan
security-scan:
	@echo "Running security scan..."
	gosec ./...
	govulncheck ./...