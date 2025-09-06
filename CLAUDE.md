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
cd scripts
go run test_tradier_api.go

# Run all tests
go test ./...

# Run specific package tests with verbose output
go test ./internal/models -v
go test ./internal/strategy -v
go test ./internal/storage -v

# Run tests with coverage
go test ./... -cover
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
└── config/config.go        # Configuration management

cmd/bot/main.go             # Application entry point and scheduler
scripts/test_tradier_api.go # API connection testing utility
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
cd scripts && go run test_tradier_api.go
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