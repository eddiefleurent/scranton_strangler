# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

SPY Short Strangle Trading Bot - An automated options trading system implementing mechanical short strangle strategies on SPY via the Tradier API. Built in Go for performance and reliability.

## Build and Development Commands

```bash
# Build the bot
go build -o strangle-bot cmd/bot/main.go

# Run the bot
./strangle-bot --config=config.yaml

# Test Tradier API connection
export TRADIER_API_KEY='your_sandbox_token_here'
export TRADIER_ACCOUNT_ID='your_account_id_here'
cd scripts/test_tradier
go run test_tradier.go

# Run all tests
go test ./...

# Run specific package tests with verbose output
go test ./internal/models -v
go test ./internal/strategy -v
go test ./internal/storage -v
go test ./internal/broker -v

# Run tests with coverage
go test ./... -cover

# Run CI pipeline locally
go mod download
go mod verify
go vet ./...
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go build -o strangle-bot cmd/bot/main.go
./strangle-bot --help

# Run linting (local development only)
go vet ./...
go fmt ./...
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
- **Entry**: IVR > 30, 45 DTE (±5), 16Δ strikes, minimum $2 credit
- **Exit**: 50% profit target or 21 DTE remaining
- **Risk**: Maximum 35% account allocation

## Project Structure

```
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

### Configuration
Uses `config.yaml` for all settings. Copy from `config.yaml.example` and update with your Tradier credentials. Key configuration sections:
- **Environment**: `mode` (paper/live), `log_level`
- **Broker**: API credentials, endpoints, OTOCO order support
- **Strategy**: Entry/exit rules, DTE targets, delta, credit requirements
- **Risk**: Position sizing, loss limits, allocation limits
- **Schedule**: Market hours, check intervals

## Development Phases

Currently in **Phase 1 (MVP)**: Basic entry/exit logic with paper trading
- Phase 2: Full adjustment system ("Football System")
- Phase 3: Multi-asset portfolio support
- Phase 4: Production deployment with monitoring

## Testing Approach

The project includes comprehensive test coverage:

### Unit Tests
```bash
go test ./internal/models -v      # State machine tests
go test ./internal/strategy -v    # Strategy logic tests  
go test ./internal/storage -v     # Storage interface tests
go test ./internal/broker -v      # Broker interface tests
```

### Integration Tests
```bash
# Test API connectivity
export TRADIER_API_KEY='your_sandbox_token'
export TRADIER_ACCOUNT_ID='your_account_id'
cd scripts/test_tradier && go run test_tradier.go

# Test with specific flags
go run test_tradier.go --sandbox=true    # Use sandbox (default)
go run test_tradier.go --sandbox=false   # Use production (careful!)
```

### Key Test Scenarios
1. State machine transitions (all valid paths)
2. Position state persistence across restarts
3. Entry signal generation when IVR > 30
4. Exit conditions at 50% profit target
5. Risk limits enforcement
6. Configuration loading and validation

## Implementation Status

**Current Implementation** (Phase 1):
- ✅ Interface-based architecture with dependency injection
- ✅ Position state machine with validation
- ✅ Comprehensive test coverage
- ✅ Mock implementations for testing
- ✅ Configuration management
- ✅ Basic strategy logic structure

**In Progress**:
- 🔧 IVR calculation implementation
- 🔧 Complete option chain processing
- 🔧 Order execution via Tradier API

## CI/CD Pipeline

### Prerequisites
```bash
# One-time setup: Install golangci-lint (recommended method using install script)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin latest

# Alternative: Install via go install (binary installation is preferred)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### GitHub Actions
The project uses GitHub Actions for CI/CD with the following jobs:
- **Test**: Unit tests with race detection and coverage reporting
- **Lint**: Code quality checks with golangci-lint (timeout: 5m) using latest version
- **Security Scan**: Vulnerability scanning with gosec v2.24.0 and SARIF upload

Triggers:
- Push to `main` or `feature/*` branches
- Pull requests to `main`

All jobs use concurrency control to cancel in-progress runs when new commits are pushed.

### Common CI Issues & Fixes
When running CI checks, watch for these common issues:

1. **Undefined package errors** (`typecheck`):
   ```bash
   # Error: undefined: yaml
   # Fix: Run go mod tidy to resolve dependencies
   go mod tidy
   ```

2. **Unchecked errors** (`errcheck`):
   ```go
   // Bad
   resp.Body.Close()
   
   // Good  
   defer func() {
       if err := resp.Body.Close(); err != nil {
           // Handle error appropriately
       }
   }()
   ```

3. **Missing package comments** (`stylecheck`):
   ```go
   // Package broker provides trading API clients for executing options trades.
   package broker
   ```

4. **HTTP context usage** (`noctx`):
   ```go
   // Use http.NewRequestWithContext instead of http.NewRequest
   req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
   ```

5. **Magic numbers/strings** (`goconst`):
   ```go
   const (
       optionTypePut  = "put"
       optionTypeCall = "call"
   )
   ```

### Dependency Management
**Important**: Always run `go mod tidy` after adding new dependencies or when encountering "undefined" errors in CI. This ensures all dependencies are properly downloaded and resolved.

### Module Information
- **Module**: `github.com/eddiefleurent/scranton_strangler`
- **Go Version**: 1.25.1
- **Key Dependencies**: `gopkg.in/yaml.v3` for configuration

### Local Development Setup
1. Clone the repository
2. Run `go mod download` to fetch dependencies
3. Copy `config.yaml` and update with your Tradier credentials
4. Test connection: `cd scripts/test_tradier && go run test_tradier.go`
5. Run tests: `go test ./...`