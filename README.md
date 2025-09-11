# SPY Short Strangle Trading Bot

Automated options trading bot implementing the SPY short strangle strategy with disciplined risk management.

## Quick Start

### 1. Get Tradier API Access

1. Sign up at [Tradier Developer](https://developer.tradier.com/)
2. Get your **sandbox** API token (free)
3. Note your account ID from the dashboard

### 2. Test API Connection

```bash
# Set your API key
export TRADIER_API_KEY='your_sandbox_token_here'

# Run the test script
go run ./scripts/test_tradier/test_tradier.go
```

You should see:
- ✓ Market status
- ✓ SPY quote
- ✓ Option expirations
- ✓ Option chain data

### 3. Configure the Bot

```bash
# Copy example config
cp config.yaml.example config.yaml

# Edit with your credentials
vim config.yaml
```

Key settings to update:
- `api_key`: Your Tradier sandbox token
- `account_id`: Your Tradier account ID
- `max_contracts`: Start with 1 for testing

### 4. Run the Bot

```bash
# Build and run locally
make build
make run

# Or deploy to Unraid server
make deploy-unraid
```

## Project Structure

```
scranton_strangler/
├── cmd/bot/          # Main application entry
├── internal/         # Core business logic
│   ├── broker/       # Tradier API client
│   ├── strategy/     # Trading strategy logic
│   └── models/       # Data structures
├── scripts/          # Utility scripts
├── docs/             # Documentation
│   ├── ARCHITECTURE.md    # System design
│   └── SPY_SHORT_STRANGLE_MASTER_STRATEGY.md
└── config.yaml       # Your configuration (git ignored)
```

## Data Formats

### Timestamp Format
All timestamps in position data (`data/positions.json`) use **UTC with Z suffix**:
- Format: `YYYY-MM-DDTHH:MM:SSZ` (ISO 8601 UTC)
- Example: `2024-12-20T21:00:00Z` (December 20, 2024 at 9:00 PM UTC)
- **Not accepted**: Local time with Z suffix (e.g., `2024-12-20T16:00:00Z` for 4:00 PM ET)
- **Not accepted**: Explicit offsets (e.g., `2024-12-20T16:00:00-05:00`)

### IV Rank Format
IV Rank (IVR) values are stored as **integers from 0-100**:
- Format: Whole numbers (0-100)
- Example: `18` (not `0.18`)
- This matches the strategy thresholds and documentation
- Values represent percentage points (18 = 18% IV rank)

## Strategy Overview

The bot implements a mechanical short strangle strategy on SPY:

1. **Entry**: Sell 16Δ put and call when IVR > 30
2. **Exit**: Close at 50% profit or 21 DTE (automatic with OTOCO orders)
3. **Risk**: Max 35% account allocation
4. **Management**: Progressive adjustments (Phase 2)

### OTOCO Order Support

The bot can use OTOCO (One-Triggers-One-Cancels-Other) orders for automatic profit taking:
- When enabled via `use_otoco: true` in config
- Places exit order immediately when opening position
- Exit order stays active (GTC) until filled at 50% profit
- No need to monitor positions for profit target

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for full system design.

## Development Phases

- **Phase 1 (MVP)**: Basic entry/exit logic with paper trading ✅
- **Phase 2**: Full adjustment system ("Football System")
- **Phase 3**: Multi-asset portfolio support
- **Phase 4**: Production hardening and monitoring

## Safety First

⚠️ **IMPORTANT**:
- Always start with paper trading
- Test for minimum 30 days
- Verify all trades match expected behavior
- Never trade with money you can't afford to lose

## Unraid Deployment

The bot is designed for simple binary deployment to Unraid servers. No Docker containers or runtime dependencies required.

### Prerequisites

1. **SSH Key Authentication**: Ensure passwordless SSH access to your Unraid server:
   ```bash
   ssh unraid "echo 'Connection successful'"
   ```

2. **Configuration**: Create your trading configuration:
   ```bash
   make dev-setup  # Creates config.yaml from example
   # Edit config.yaml with your Tradier API credentials
   ```

### Deploy to Unraid

```bash
make deploy-unraid
```

**What happens automatically:**
- Builds Go binary for Linux
- Creates `/mnt/user/appdata/scranton-strangler/` directory structure
- Copies binary and config to Unraid
- Creates start/stop service scripts
- Adds to Unraid's boot sequence for auto-start
- Starts the bot immediately

### Management Commands

```bash
make unraid-logs      # View bot logs
make unraid-status    # Check if bot is running
make unraid-restart   # Restart the bot service
```

## Monitoring

The bot logs all activity to `bot.log`:
- Entry signals
- Orders placed
- Position updates
- P&L tracking
- Errors and warnings

## Common Issues

### API Key Not Working
- Ensure you're using the sandbox key (not production)
- Check the key hasn't expired
- Verify you're hitting sandbox URL

### No Options Data
- Markets may be closed
- SPY options should always be available
- Check your API rate limits

### Position Not Opening
- Verify IVR is above threshold (30)
- Check minimum credit requirement ($2)
- Ensure market hours (9:30 AM - 4:00 PM ET)

## Implementation Status

**Current Implementation** (Phase 1 MVP):
- ✅ Tradier API integration with rate limiting
- ✅ IVR calculation using VXX proxy
- ✅ Complete option chain processing
- ✅ Entry signal generation (IVR > 30%, 45 DTE, 16Δ)
- ✅ Order execution via Tradier API
- ✅ Position state machine with comprehensive tracking
- ✅ Paper trading mode for safe testing
- ✅ Unraid deployment with auto-start

**Next Steps** (Phase 2):
- [ ] Advanced adjustment logic ("Football System")
- [ ] Production hardening and monitoring
- [ ] Multi-asset portfolio support

## Resources

- [Tradier API Docs](https://documentation.tradier.com/brokerage-api)
- [Strategy Guide](docs/SPY_SHORT_STRANGLE_MASTER_STRATEGY.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Tasks](TASKS.md)

## License

Private - Not for distribution