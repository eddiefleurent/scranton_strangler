# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SPY Short Strangle Trading Bot - An automated options trading system implementing mechanical short strangle strategies on SPY via the Tradier API. Built in Go for performance and reliability.

## Build and Development Commands

The project includes a comprehensive Makefile for common development tasks:

```bash
# Build the bot
make build              # Standard build
make build-prod        # Production build (optimized)

# Run the bot
make run               # Build and run with config.yaml

# Testing
make test              # Run all tests
make test-coverage     # Run tests with coverage report
make test-api          # Test Tradier API connection

# Code quality
make lint              # Run golangci-lint
make security-scan     # Run security scans (gosec, govulncheck)
make check             # Run lint, test, security scan, and build in sequence

# Unraid deployment
make deploy-unraid     # Deploy binary to Unraid
make unraid-logs       # Show bot logs on Unraid
make unraid-status     # Check bot status on Unraid
make unraid-restart    # Restart bot on Unraid

# Development setup
> **SECURITY WARNING**: Add config.yaml to .gitignore and NEVER commit it. Populate secrets from environment variables or CI secrets using `envsubst < config.yaml.template > config.yaml` or similar secure injection methods. Also protect `data/positions.json` and `logs/` as they may contain sensitive trading information.
make dev-setup        # Create config.yaml from example

# Emergency liquidation
make liquidate        # Close ALL positions using market orders (requires confirmation)

# Cleanup
make clean            # Remove build artifacts

# Help
make help             # Show all available targets
```

### Direct Go Commands (when needed)

```bash
# Dependency management
go mod download       # Download dependencies
go mod tidy          # Clean up module dependencies
go mod verify        # Verify dependencies

# Testing specific packages
go test ./internal/models -v      # State machine tests
go test ./internal/strategy -v    # Strategy logic tests  
go test ./internal/storage -v     # Storage interface tests
go test ./internal/broker -v      # Broker interface tests

# Formatting
go fmt ./...         # Format all Go files

# Environment setup for API testing
export TRADIER_API_KEY='your_sandbox_token_here'
export TRADIER_ACCOUNT_ID='your_account_id_here'
```

## Architecture Overview

The bot follows a component-based architecture with clear separation of concerns:

- **Scheduler**: Runs every 15 minutes via cron
- **Market Data Service**: Fetches quotes, option chains, Greeks, and calculates IVR
- **Strategy Engine**: Generates entry/exit/adjustment signals based on configurable rules
- **Risk Monitor**: Enforces position sizing and allocation limits
- **Order Executor**: Translates signals to Tradier API calls with retry logic
- **Position Manager**: Tracks state, P&L, and persists to `positions.json`

### Core Strategy Rules
- **Entry**: IV > 30%, 45 DTE (±5), 16Δ strikes, minimum $2 credit
- **Exit**: 50% profit target or 21 DTE remaining
- **Risk**: Maximum 35% account allocation

## Unraid Deployment

```bash
make deploy-unraid     # Deploy binary to Unraid
make unraid-logs       # Show bot logs on Unraid
make unraid-status     # Check bot status on Unraid
make unraid-restart    # Restart bot on Unraid
```

The deployment script handles building, transferring files, setting up auto-start, and creating the necessary directory structure at `/mnt/user/appdata/scranton-strangler/`.

## Project Structure

```text
internal/
├── broker/
│   ├── interface.go          # Broker interface definition
│   ├── tradier.go           # Tradier API client implementation
│   └── interface_test.go    # Interface tests
├── strategy/
│   ├── strangle.go          # Core trading logic and signal generation
│   └── strangle_test.go     # Strategy tests
├── models/
│   ├── position.go          # Position and order data structures
│   ├── state_machine.go     # Position state machine implementation
│   └── state_machine_test.go # State machine tests
├── storage/
│   ├── interface.go         # Storage interface definition
│   ├── storage.go          # JSON file storage implementation
│   ├── mock_storage.go     # Mock storage for testing
│   └── interface_test.go   # Storage tests
├── orders/
│   └── manager.go          # Order management logic
├── retry/
│   └── client.go           # HTTP retry client
├── config/config.go        # Configuration management
└── mock/                   # Mock implementations for testing

cmd/bot/main.go                    # Application entry point and scheduler
scripts/test_tradier/test_tradier.go # API connection testing utility
```

## Key Implementation Notes

### Tradier API Integration
- Sandbox endpoint: `https://sandbox.tradier.com/v1/`
- Rate limits: 120 req/min (sandbox), 500 req/min (production)
- Implements exponential backoff for retries
- Caches responses for 1 minute to minimize API calls

### State Management  
Position state follows the Football System via state machine: `idle → open → first_down → second_down → third_down → fourth_down → closed`

The state machine enforces:
- Max 3 adjustments per position
- Max 1 time roll per position  
- Valid state transitions only
- Automatic validation

See `docs/STATE_MACHINE.md` for detailed documentation.

### Position Reconciliation & Sync Management
The bot includes robust position synchronization to prevent broker/storage mismatches:

**Reconciliation Process:**
- Runs automatically every trading cycle
- Additional reconcile check before opening new trades
- Detects positions closed manually outside the bot
- Recovers positions that filled but weren't tracked due to timeouts

**Timeout Handling:**
- Enhanced timeout logic checks broker positions before closing locally
- Prevents premature position closure when orders actually filled
- Handles frequent redeploys and network interruptions gracefully

**Emergency Tools:**
- `make liquidate` - Force close all positions via market orders
- `scripts/liquidate_positions.go` - Direct API liquidation utility
- Automatic position limit enforcement prevents over-allocation

**Common Sync Issues Fixed:**
- Order timeout during polling (bot thinks order failed, but it filled)
- Frequent bot restarts killing order polling goroutines
- Manual position management via broker interface
- Network interruptions during order status checks

### Configuration
Uses `config.yaml` for all settings. Copy from `config.yaml.example` and update with your Tradier credentials. Key configuration sections:
- **Environment**: `mode` (paper/live), `log_level`
- **Broker**: API credentials, endpoints, OTOCO order support
- **Strategy**: Entry/exit rules, DTE targets, delta, credit requirements
  - Note: profit_target is a ratio (0–1), whereas fields with "Pct" are percentage points (e.g., 30 for 30%).
- **Risk**: Position sizing, loss limits, allocation limits
- **Schedule**: Market hours, check intervals

## Testing

```bash
# Run all tests with coverage
make test-coverage

# Test specific packages
go test ./internal/models -v      # State machine tests
go test ./internal/strategy -v    # Strategy logic tests  
go test ./internal/storage -v     # Storage interface tests
go test ./internal/broker -v      # Broker interface tests

# Test Tradier API connection
make test-api
```

## CI/CD Pipeline

GitHub Actions runs on push to `main`/`feature/*` and PRs:
- **Test**: Unit tests with race detection and coverage
- **Lint**: golangci-lint checks (install with `make tools`)
- **Security Scan**: gosec vulnerability scanning

**Key CI Fix**: Run `go mod tidy` after adding dependencies or if you see "undefined" package errors.