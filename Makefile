SHELL := /bin/bash
# SPY Strangle Bot - Makefile

.PHONY: all help build build-prod test test-coverage test-integration lint run clean \
        deploy-unraid unraid-logs unraid-status unraid-restart \
        dev-setup test-api test-paper security-scan build-test-helper tools check liquidate

all: build

# Default target
help:
	@printf "Targets:\n  build/test/test-integration/test-coverage/lint/run/clean\n  deploy-unraid/unraid-logs/unraid-status/unraid-restart\n  security-scan/build-test-helper\n  dev-setup/check/liquidate\n"

# Go binary name and paths
BINARY_NAME=scranton-strangler
BIN_DIR=bin
BINARY_PATH=./$(BIN_DIR)/$(BINARY_NAME)

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/bot

# Build for production (optimized)
build-prod:
	@echo "Building $(BINARY_NAME) for production..."
	@echo "⏱️  Starting build timer..."
	@START_TIME=$$(date +%s); \
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -trimpath -buildvcs=false -ldflags="-w -s" -o $(BINARY_NAME) ./cmd/bot; \
	END_TIME=$$(date +%s); \
	DURATION=$$((END_TIME - START_TIME)); \
	echo "⏱️  Build completed in $${DURATION} seconds"

# Run tests
test:
	@echo "Running tests..."
	go test -race -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -n1
	@echo "Coverage report generated: coverage.html"

# Run end-to-end integration tests
test-integration:
	@echo "Running end-to-end integration tests..."
	@if [ -z "$$TRADIER_API_KEY" ] || [ -z "$$TRADIER_ACCOUNT_ID" ]; then \
		if [ ! -f "config.yaml" ]; then \
			echo "⏭️  Skipping integration tests; no credentials found"; \
			echo "   Set TRADIER_API_KEY and TRADIER_ACCOUNT_ID environment variables or create config.yaml"; \
			exit 0; \
		else \
			echo "📝 Using credentials from config.yaml"; \
		fi; \
	else \
		echo "📝 Using credentials from environment variables"; \
	fi
	@echo "🧪 Testing complete trading cycle with real Tradier sandbox"
	@mkdir -p data
	go run cmd/integration/main.go

# Run linter
lint: tools
	@echo "Running linter..."
	golangci-lint run
	go vet ./...

# Run the bot locally
run: build
	@echo "Starting $(BINARY_NAME)..."
	$(BINARY_PATH) --config=config.yaml

# Clean build artifacts
clean:
	@echo "Cleaning..."
	go clean -testcache
	rm -rf $(BIN_DIR) coverage.out coverage.html *.test *.prof

# Unraid deployment
deploy-unraid:
	@echo "Deploying binary to Unraid..."
	@if [ ! -f ./deploy.sh ]; then \
		echo "Error: deploy.sh not found. Please ensure the deployment script exists."; \
		exit 1; \
	fi
	@if [ ! -x ./deploy.sh ]; then \
		echo "Error: deploy.sh is not executable. Please run 'chmod +x ./deploy.sh'."; \
		exit 1; \
	fi
	./deploy.sh

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

# Paper trading integration test
test-paper:
	@echo "Running end-to-end paper trading test..."
	cd scripts/test_paper_trading && go run main.go

# Liquidate all positions via market orders
liquidate:
	@echo "🚨 LIQUIDATING ALL POSITIONS 🚨"
	@echo "This will close ALL open positions using aggressive market pricing"
	@echo "Uses credentials from environment variables or config.yaml"
	@read -p "Are you sure? [y/N] " confirm && [ "$$confirm" = "y" ] || exit 1
	cd scripts && go run liquidate_positions.go

# Build test helper
build-test-helper:
	@echo "Building test helper..."
	@mkdir -p $(BIN_DIR)
	go build -tags test -o $(BIN_DIR)/test_helper scripts/test_helper/test_helper.go

# Security scan
security-scan: tools
	@echo "Running security scan..."
	@PATH="$(shell go env GOPATH)/bin:$$PATH" gosec ./...
	@PATH="$(shell go env GOPATH)/bin:$$PATH" govulncheck ./...

# Check: lint, test, integration test, security scan, and build
check: lint test test-integration security-scan build
	@echo "✅ All checks passed: lint, test, integration test, security scan, and build completed successfully"

# Install security tools
tools:
	@echo "Installing security tools..."
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

unraid-logs:
	@echo "Showing Unraid bot logs..."
	ssh unraid 'tail -f /mnt/user/appdata/scranton-strangler/logs/bot.log'

unraid-status:
	@echo "Checking Unraid bot status..."
	ssh unraid 'pgrep -f scranton-strangler && echo "✅ Bot is running" || echo "❌ Bot not running"'
	ssh unraid 'test -f /mnt/user/appdata/scranton-strangler/data/positions.json && echo "✅ Positions file exists" || echo "❌ Positions file missing"'

unraid-restart:
	@echo "Restarting Unraid bot..."
	ssh unraid '/mnt/user/appdata/scranton-strangler/stop-service.sh && /mnt/user/appdata/scranton-strangler/start-service.sh'