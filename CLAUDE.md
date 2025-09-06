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
go run test_tradier.go

# Run tests (when implemented)
go test ./...
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
├── broker/tradier.go     # Tradier API client implementation
├── strategy/strangle.go  # Core trading logic and signal generation
├── models/position.go    # Position and order data structures
├── config/config.go      # Configuration management
├── storage/storage.go    # Position persistence layer
└── mock/mock_data.go     # Mock data for testing

cmd/bot/main.go           # Application entry point and scheduler
scripts/test_tradier.go   # API connection testing utility
```

## Key Implementation Notes

### Tradier API Integration
- Sandbox endpoint: `https://sandbox.tradier.com/v1/`
- Rate limits: 120 req/min (sandbox), 500 req/min (production)
- Implements exponential backoff for retries
- Caches responses for 1 minute to minimize API calls

### State Management
Position state is persisted to `positions.json` after each update. States flow: `IDLE → SCANNING → ENTERING → POSITIONED → ADJUSTING/CLOSING`

### Configuration
Uses `config.yaml` for all settings. Copy from `config.yaml.example` and update with your Tradier credentials. Environment variables are supported via `${VAR_NAME}` syntax.

## Development Phases

Currently in **Phase 1 (MVP)**: Basic entry/exit logic with paper trading
- Phase 2: Full adjustment system ("Football System")
- Phase 3: Multi-asset portfolio support
- Phase 4: Production deployment with monitoring

## Testing Approach

When testing, verify:
1. API connectivity with `scripts/test_tradier.go`
2. Entry signals trigger correctly when IVR > 30
3. Position sizing respects allocation limits
4. Exit conditions work at 50% profit or 21 DTE
5. State persists correctly across restarts