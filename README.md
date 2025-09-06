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
go run ./scripts/test_tradier
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
# Build the bot
go build -o strangle-bot cmd/bot/main.go

# Run in paper trading mode
./strangle-bot --config=config.yaml
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

- **Phase 1 (Current)**: Basic entry/exit, paper trading
- **Phase 2**: Full adjustment system
- **Phase 3**: Multi-asset portfolio
- **Phase 4**: Production deployment

## Safety First

⚠️ **IMPORTANT**: 
- Always start with paper trading
- Test for minimum 30 days
- Verify all trades match expected behavior
- Never trade with money you can't afford to lose

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

## Next Steps

1. [x] Test Tradier API connection
2. [ ] Implement IVR calculation
3. [ ] Build entry scanner
4. [ ] Create position monitor
5. [ ] Add exit logic
6. [ ] Start paper trading
7. [ ] Run for 30 days
8. [ ] Evaluate results

## Resources

- [Tradier API Docs](https://documentation.tradier.com/brokerage-api)
- [Strategy Guide](docs/SPY_SHORT_STRANGLE_MASTER_STRATEGY.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Tasks](TASKS.md)

## License

Private - Not for distribution