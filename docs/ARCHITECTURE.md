# SPY Short Strangle Bot - Master Architecture

## Table of Contents
1. [System Overview](#system-overview)
2. [Architecture Design](#architecture-design)
3. [Development Phases](#development-phases)
4. [Implementation Details](#implementation-details)
5. [Deployment & Operations](#deployment--operations)

---

## System Overview

Automated trading bot for SPY short strangles via Tradier API. Built in Go for performance and reliability. No ML, just disciplined execution of a proven options strategy.

### Core Principles
- **KISS**: Simple is better than complex
- **Fail Safe**: When uncertain, do nothing
- **Paper First**: Prove it works before risking capital
- **Progressive Enhancement**: Start simple, add complexity only after proving stability

---

## Architecture Design

### High-Level Components

```
┌─────────────────────────────────────────────────────┐
│                   SCHEDULER                         │
│              (Cron - every 15 min)                  │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│                MARKET DATA SERVICE                  │
│        • Quotes  • Chains  • Greeks  • IVR          │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│                 STRATEGY ENGINE                     │
│     • Entry Signals  • Exit Signals  • Adjustments  │
└────────────────────┬────────────────────────────────┘
                     │
┌────────────────────▼────────────────────────────────┐
│                  RISK MONITOR                       │
│    • Position Sizing  • Loss Limits  • Allocation   │
└────────────────────┬────────────────────────────────┘
                     │
     ┌───────────────┴───────────────┐
     │                               │
┌────▼──────────┐          ┌────────▼────────┐
│ORDER EXECUTOR │          │POSITION MANAGER │
│ • Place       │◄─────────┤ • Track         │
│ • Modify      │          │ • Monitor       │
│ • Cancel      │          │ • Calculate P&L │
└───────────────┘          └─────────────────┘
```

### Component Responsibilities

#### 1. Market Data Service
- Fetches real-time quotes from Tradier
- Retrieves option chains with Greeks
- Calculates IV Rank (or fetches if available)
- Caches data to minimize API calls

#### 2. Strategy Engine
- **Entry Logic**: IVR > 30, 45 DTE, 16Δ strikes
- **Exit Logic**: 50% profit or 21 DTE remaining
- **Adjustment Logic**: Progressive management system
- Returns actionable signals, not orders

#### 3. Risk Monitor
- Enforces position sizing (35% max allocation)
- Tracks buying power usage
- Prevents over-leveraging
- Emergency stop conditions

#### 4. Order Executor
- Translates signals to Tradier API calls
- Handles multi-leg orders (strangles)
- Manages partial fills
- Implements retry logic

#### 5. Position Manager
- Maintains position state
- Calculates real-time P&L
- Tracks adjustments history
- Persists state between runs

### State Management

```
Bot States:
IDLE ──────► SCANNING ──────► ENTERING ──────► POSITIONED
                │                                    │
                └────────────────────────────────────┘
                                │
                        ADJUSTING / CLOSING
```

### Data Models

```go
Position {
    ID              string
    Symbol          string
    PutStrike       float64
    CallStrike      float64
    Expiration      time.Time
    EntryDate       time.Time
    CreditReceived  float64
    CurrentPnL      float64
    Status          string
    Adjustments     []Adjustment
}

Signal {
    Type        string  // "enter", "exit", "adjust"
    Action      string  // "open_strangle", "close_all", "roll_put", etc
    Urgency     string  // "immediate", "end_of_day", "monitor"
    Parameters  map
}
```

---

## Development Phases

### Phase 1: MVP (Weeks 1-4)
**Goal**: Prove core functionality works

#### Scope
- **IN**: SPY only, basic entry/exit, 16Δ strikes, paper trading
- **OUT**: Adjustments, multiple tickers, database, UI

#### Architecture (MVP)
```
spy-strangle-bot/
├── cmd/bot/main.go           # Entry point, scheduler
├── internal/
│   ├── broker/tradier.go     # API client
│   ├── strategy/strangle.go  # Core logic
│   ├── models/position.go    # Data structures
│   └── config/config.go      # Configuration
├── config.yaml               # Settings
└── positions.json            # State persistence
```

#### Success Metrics
- 30 days without crashes
- 10+ completed trades
- 70%+ win rate

---

### Phase 2: Full Management System (Weeks 5-8)
**Goal**: Implement complete adjustment system

#### New Components

##### Adjustment Manager
Implements the "Football System":

```
SECOND DOWN (First Test)
├── Detect: Price within 5-10 points of strike
├── Action: Roll untested side to current 16Δ
└── Result: Extended breakeven

THIRD DOWN (Continued Pressure)
├── Detect: Price through original strike
├── Action: Create straddle (roll untested to tested strike)
└── Result: Significant breakeven extension

FOURTH DOWN (Last Resort)
├── Option A: Inverted strangle
├── Option B: Hold and hope
└── Option C: Roll out in time
```

#### Enhanced Architecture
```diff
spy-strangle-bot/
├── internal/
+│   ├── adjustments/
+│   │   ├── detector.go      # When to adjust
+│   │   ├── executor.go      # How to adjust
+│   │   └── tracker.go       # Adjustment history
+│   ├── database/
+│   │   └── sqlite.go        # Position history
+│   └── analytics/
+│       └── performance.go   # P&L attribution
```

---

### Phase 3: Multi-Asset & Optimization (Weeks 9-12)
**Goal**: Scale beyond single ticker

#### Portfolio Management
```
Portfolio Allocator
├── SPY (35% max)
├── GLD (20% max)  
├── TLT (20% max)
└── EWZ (15% max)

Correlation Matrix → Position Sizing
```

#### Smart Entry System
```
IVR-Based DTE Selection:
if IVR < 30:  DTE = 60
if IVR 30-50: DTE = 45  
if IVR > 50:  DTE = 30

Delta Flexibility:
if regime == "low_vol": use 16Δ
if regime == "high_vol": use 30Δ
```

---

### Phase 4: Production (Weeks 13-16)
**Goal**: Live trading ready

#### Infrastructure Upgrades
- PostgreSQL for trade history
- Redis for position cache
- Grafana dashboard
- Alert system (Telegram/Discord)
- Docker deployment

#### Architecture (Production)
```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   PostgreSQL │────│     Bot       │────│    Redis      │
│  (History)   │    │   (Core)      │    │   (Cache)     │
└──────────────┘    └───────┬───────┘    └──────────────┘
                            │
                    ┌───────▼───────┐
                    │   Grafana      │
                    │  (Monitoring)  │
                    └────────────────┘
```

---

## Implementation Details

### Configuration Schema

```yaml
# config.yaml
environment:
  mode: "paper"  # paper | live
  log_level: "info"
  
broker:
  provider: "tradier"
  api_key: "${TRADIER_API_KEY}"  # From environment
  api_endpoint: "https://sandbox.tradier.com/v1/"
  account_id: "${TRADIER_ACCOUNT}"
  
strategy:
  symbol: "SPY"
  allocation_pct: 0.35
  
  entry:
    min_ivr: 30
    target_dte: 45
    dte_range: [40, 50]  # Acceptable range
    delta: 16
    min_credit: 2.00
    
  exit:
    profit_target: 0.50
    max_dte: 21
    
  adjustments:
    enabled: false  # Phase 2
    second_down_threshold: 10  # Points from strike
    
risk:
  max_contracts: 2
  max_daily_loss: 1000
  max_position_loss: 2.0  # 200% of credit
  
schedule:
  market_check_interval: "15m"
  after_hours_check: false
```

### API Integration

#### Tradier Endpoints Used
```
GET  /markets/quotes              # Get SPY price
GET  /markets/options/chains      # Get option chain
GET  /markets/options/expirations # Find target DTE
POST /accounts/{id}/orders        # Place orders
GET  /accounts/{id}/positions     # Get positions
GET  /accounts/{id}/orders        # Check order status
```

#### Rate Limiting
- 120 requests/minute (sandbox)
- 500 requests/minute (production)
- Implement exponential backoff
- Cache responses for 1 minute

### Order Execution Flow

```
1. Signal Generated
       ↓
2. Risk Check Pass?
   No → Log and Skip
   Yes ↓
3. Build Order
       ↓
4. Place Order
       ↓
5. Wait for Fill (max 5 min)
   Partial → Adjust or Cancel
   Filled → Update Position
       ↓
6. Persist State
```

### Error Handling Matrix

| Error Type | Response | Recovery |
|------------|----------|----------|
| Network Timeout | Retry 3x with backoff | Skip cycle |
| API Rate Limit | Wait and retry | Reduce frequency |
| Invalid Order | Log error | Manual review |
| Insufficient Funds | Alert | Halt trading |
| Position Not Found | Reconcile | Rebuild state |
| Critical Error | Shutdown | Alert operator |

---

## Deployment & Operations

### Development Environment
```bash
# Local development
go run cmd/bot/main.go --config=config.dev.yaml

# Run tests
go test ./...

# Build binary
go build -o strangle-bot cmd/bot/main.go
```

### Production Deployment

#### Option 1: Systemd (Linux VPS)
```ini
[Unit]
Description=SPY Strangle Bot
After=network.target

[Service]
Type=simple
User=trading
WorkingDirectory=/opt/strangle-bot
ExecStart=/opt/strangle-bot/strangle-bot
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

#### Option 2: Docker
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o strangle-bot cmd/bot/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/strangle-bot .
COPY config.yaml .
CMD ["./strangle-bot"]
```

### Monitoring & Alerting

#### Key Metrics
- Positions opened/closed per day
- Current P&L
- Win rate (rolling 30 days)
- API calls per minute
- Error rate

#### Log Levels
```
DEBUG: Detailed execution flow
INFO:  Normal operations
WARN:  Unusual but handled
ERROR: Needs attention
FATAL: Bot shutting down
```

#### Alert Triggers
- Position down >100% of credit
- No heartbeat for 30 minutes
- API errors >10 in 5 minutes
- Unexpected position state

### Backup & Recovery

#### State Backup
- Position file: Every update
- Database: Daily backup
- Configuration: Version controlled

#### Disaster Recovery
1. Stop bot immediately
2. Close all positions manually if needed
3. Restore from last known good state
4. Validate positions match broker
5. Resume in paper mode first

---

## Appendix: Decision Trees

### Entry Decision
```
Is market open?
  No → Wait
  Yes ↓
  
Have position?
  Yes → Skip to management
  No ↓
  
Is IVR > 30?
  No → Wait
  Yes ↓
  
Find 45 DTE expiration
  Not found → Try 40-50 DTE range
  Found ↓
  
Calculate 16Δ strikes
  Credit < $2 → Skip
  Credit >= $2 ↓
  
Risk check pass?
  No → Skip
  Yes ↓
  
PLACE ORDER
```

### Exit Decision
```
Have position?
  No → Skip
  Yes ↓
  
P&L >= 50% of credit?
  Yes → CLOSE
  No ↓
  
DTE <= 21?
  Yes → CLOSE
  No ↓
  
Loss > 200% of credit?
  Yes → CLOSE (Phase 2)
  No ↓
  
Continue monitoring
```