# SPY Short Strangle Bot - Master Architecture

## Table of Contents
1. [System Overview](#system-overview)
2. [Architecture Design](#architecture-design)
3. [Architecture Assessment](#architecture-assessment)
4. [Development Phases](#development-phases)
5. [Implementation Details](#implementation-details)
6. [Critical Improvements Required](#critical-improvements-required)
7. [Deployment & Operations](#deployment--operations)

---

## System Overview

Automated trading bot for SPY short strangles via Tradier API. Built in Go for performance and reliability. No ML, just disciplined execution of a proven options strategy.

### Core Principles
- **KISS**: Simple is better than complex
- **Fail Safe**: When uncertain, do nothing
- **Paper First**: Prove it works before risking capital
- **Progressive Enhancement**: Start simple, add complexity only after proving stability

### Architecture Score: 7.5/10
**Current State**: Solid foundation; production hardening in progress
- ✅ Clean separation of concerns
- ✅ Configuration-driven design
- ✅ Interface-based design (Broker, Storage, Strategy, Orders)
- ✅ Unit tests for strategy/state machine/storage
- ⚠️ Further hardening: integration tests, sandbox E2E, observability

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

### Layered Architecture View
```text
┌─────────────────────────────────────────────────────────────┐
│                    Application Layer                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Scheduler   │  │ Trading     │  │ Signal      │        │
│  │ (main.go)   │  │ Loop        │  │ Handling    │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                     Business Logic Layer                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Strategy    │  │ Risk        │  │ Position    │        │
│  │ Engine      │  │ Management  │  │ Manager     │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                   Infrastructure Layer                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │ Broker API  │  │ Storage     │  │ Config      │        │
│  │ (Tradier)   │  │ (JSON)      │  │ (YAML)      │        │
│  └─────────────┘  └─────────────┘  └─────────────┘        │
└─────────────────────────────────────────────────────────────┘
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
- **OTOCO Orders**: Planned feature - currently unsupported for multi-leg orders (see `internal/broker/tradier.go`)
- **OCO Orders**: Emergency exits and rolling scenarios
- Handles multi-leg orders (strangles)
- Manages partial fills and order status
- Implements retry logic with exponential backoff

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

## Architecture Assessment

### Design Patterns Analysis

#### Well-Implemented Patterns
1. **Strategy Pattern** - Trading logic encapsulated in `strategy/strangle.go`
2. **Adapter Pattern** - Broker API abstracted in `broker/tradier.go`
3. **Repository Pattern** - Data persistence in `storage/storage.go`
4. **Configuration Pattern** - Environment-aware config in `config/config.go`

#### Anti-Patterns Identified
1. **God Object (Partial)** - Bot struct in `main.go` manages too many concerns
2. **Missing Interface Segregation** - TradierAPI is a large struct without interfaces
3. **Hard Dependencies** - No dependency injection, making testing difficult

### Dependency Analysis

#### Current Issues
```
cmd/bot/main.go
├── internal/broker (Concrete TradierClient)
├── internal/config (Concrete Config)
└── internal/strategy (Concrete StrangleStrategy)
    └── internal/broker (Circular dependency risk)
```

#### Required Interface Design
```go
// Core interfaces needed for proper abstraction
type Broker interface {
    GetQuote(symbol string) (*Quote, error)
    GetOptionChain(symbol, exp string, withGreeks bool) ([]Option, error)
    PlaceStrangleOrder(params OrderParams) (*Order, error)
    GetAccountBalance() (float64, error)
}

type Storage interface {
    GetCurrentPosition() *Position
    SetCurrentPosition(pos *Position) error
    ClosePosition(pnl float64) error
}

type Strategy interface {
    CheckEntryConditions() (bool, string)
    CheckExitConditions() (bool, string)
    FindStrangleStrikes() (*StrangleOrder, error)
}

type RiskManager interface {
    ValidatePosition(pos *Position) error
    CheckAllocationLimit(size float64) bool
    CalculatePositionSize(credit float64) int
}
```

### Critical Issues Summary

| Issue | Impact | Priority |
|-------|--------|----------|
| ~~No interface-based design~~ | ~~Testing impossible~~ | ~~Critical~~ → **Resolved in PR #2** |
| ~~Zero test coverage~~ | ~~No confidence in calculations~~ | ~~Critical~~ → **Resolved in PR #2** |
| File-based state without ACID | Data corruption risk | Critical |
| No retry/circuit breaker | API failures cascade | High |
| Plain text credentials | Security vulnerability | High |
| No structured logging | Debugging difficult | Medium |

---

## Order Types & Automation Strategy

### OTOCO Orders (One-Triggers-One-Cancels-Other) - **Planned Feature**
**Status**: Currently unsupported for multi-leg strangle orders
- **Limitation**: Tradier API does not support OTOCO for multi-leg orders
- **Current Behavior**: `use_otoco: true` in config has no runtime effect
- **Fallback**: System automatically uses regular multi-leg orders with monitoring
- **Implementation**: See `internal/broker/tradier.go` - returns `ErrOTOCOUnsupported`
- **Future**: Planned implementation may use separate entry + linked exit orders

### OCO Orders (One-Cancels-Other) 
**Use Cases**:
1. **Second Down Rolling**: Close at 70% profit OR roll untested side
2. **Third Down Management**: Take 25% profit OR continue to Fourth Down  
3. **Fourth Down Stops**: Profit target OR 200% loss limit
4. **Hard Stops**: 250% loss OR 5 DTE emergency close
5. **Delta Management**: Close profitable leg OR roll when |delta| > 0.5

### Advanced Order Types (Future Phases)

#### Bracket Orders
**Use Case**: Complete automation of entry, profit, and stop
- **Entry**: Sell strangle at limit price
- **Profit**: 50% target (GTC)
- **Stop**: 250% loss limit (GTC)
- **Benefit**: Single order handles entire lifecycle

#### Conditional Orders  
**Use Cases**:
1. **Volatility Triggers**: Enter only if IVR > threshold
2. **Price Triggers**: Roll only if SPY breaches specific levels  
3. **Time Triggers**: Auto-close at 21 DTE regardless of P&L
4. **Greek Triggers**: Adjust when delta > |0.5|

#### Multi-Conditional OCO (MOCO)
**Advanced Management**: Multiple exit conditions
- **Example**: Close at 50% profit OR 21 DTE OR 200% loss OR delta > 1.0
- **Benefit**: Comprehensive automation without monitoring

### Regular Orders
**Fallback**: When advanced orders not available
- Standard multileg strangle orders
- Individual leg adjustments  
- Manual monitoring required

### Automated Order Flow Decision Tree
```
Position Entry:
  use_otoco=true? → OTOCO (entry + 50% exit) ⭐ CURRENT RUNTIME
  use_bracket=true? → Bracket (entry + profit + stop) [PLANNED/IGNORED]
  Default → Regular strangle + monitoring [PLANNED/IGNORED]

Position Management:
  First Down → Monitor via OTOCO exit order ⭐ CURRENT RUNTIME
  Second Down → OCO (70% profit OR roll untested) ⭐ CURRENT RUNTIME
  Third Down → OCO (25% profit OR continue) [PLANNED/IGNORED]
  Fourth Down → OCO (any profit OR 200% stop) [PLANNED/IGNORED]
  
Hard Stops (Immediate):
  Loss > 250%? → Market close order
  DTE < 5? → Market close order  
  |Delta| > 1.0? → Market close order
  Black swan? → Market close order

Next Trade:
  Position closed? → Wait 1 day, scan for new entry
  Multiple losses? → Reduce position size 50%
  System error? → Stop trading, manual review
```

### Strategy Alignment
- **Risk Controls**: Escalation and stop-loss thresholds supported
- **50% Profit Target**: Monitored and exited via programmatic monitoring and single-leg orders
- **Rolling Management**: OCO supports "football system" adjustments
- **Manual Override**: Always available for black swan events

---

## Development Phases

### Phase 1: MVP with Critical Fixes (Weeks 1-4)
**Goal**: Prove core functionality with proper architecture

#### Architecture Requirements
- ✅ Interface-based design for all components
- ✅ Comprehensive unit test coverage (>80%)
- ✅ State machine for position management
- ✅ Retry logic with exponential backoff
- ✅ Circuit breaker for API resilience

#### Scope
- **IN**: SPY only, basic entry/exit, 16Δ strikes, paper trading, testing infrastructure
- **OUT**: Adjustments, multiple tickers, production database, UI

#### Enhanced MVP Architecture
```
spy-strangle-bot/
├── cmd/bot/main.go           # Entry point with dependency injection
├── internal/
│   ├── interfaces/           # All interface definitions
│   │   ├── broker.go
│   │   ├── storage.go
│   │   └── strategy.go
│   ├── broker/
│   │   ├── tradier.go        # Implements Broker interface
│   │   └── tradier_test.go
│   ├── strategy/
│   │   ├── strangle.go       # Implements Strategy interface
│   │   └── strangle_test.go
│   ├── storage/
│   │   ├── json_storage.go   # Implements Storage interface
│   │   └── storage_test.go
│   ├── models/
│   │   └── position.go
│   ├── state/
│   │   └── machine.go        # Position state machine
│   └── config/
│       └── config.go
├── config.yaml
├── positions.json            # Temporary, migrate to SQLite
└── go.mod
```

#### State Machine Implementation
```go
type PositionStateMachine struct {
    current     PositionState
    transitions map[PositionState][]PositionState
    storage     Storage
    mu          sync.RWMutex
}

func (sm *PositionStateMachine) TransitionTo(newState PositionState) error {
    sm.mu.Lock()
    defer sm.mu.Unlock()

    if !sm.isValidTransition(sm.current, newState) {
        return ErrInvalidStateTransition
    }

    // Get current position, update its state, and persist via atomic file write (temp file + fsync + rename)
    position := sm.storage.GetCurrentPosition()
    if position == nil {
        return fmt.Errorf("no current position to update")
    }

    if err := position.TransitionState(newState, "state_machine_transition"); err != nil {
        return fmt.Errorf("failed to transition position state: %w", err)
    }

    if err := sm.storage.SetCurrentPosition(position); err != nil {
        return fmt.Errorf("failed to persist state: %w", err)
    }

    sm.current = newState
    return nil
}
```

#### Resilience Patterns
```go
// Retry with exponential backoff
func (b *ResilientBroker) GetQuoteWithRetry(symbol string) (*Quote, error) {
    var q *Quote
    err := retry.Do(
        func() error {
            quote, err := b.broker.GetQuote(symbol)
            if err != nil {
                return err
            }
            q = quote
            return nil
        },
        retry.Attempts(3),
        retry.Delay(time.Second),
        retry.DelayType(retry.BackOffDelay),
    )
    return q, err
}

// Circuit breaker implementation - IMPLEMENTED ✅
type CircuitBreakerBroker struct {
    broker  Broker
    breaker *gobreaker.CircuitBreaker
}

// CircuitBreakerSettings configures circuit breaker behavior
type CircuitBreakerSettings struct {
    MaxRequests  uint32        // Max requests when half-open
    Interval     time.Duration // Reset counts interval
    Timeout      time.Duration // Open circuit duration
    MinRequests  uint32        // Min requests before tripping
    FailureRatio float64       // Failure ratio threshold
}

// Production settings: 60s interval, 30s timeout
// Test settings: 1s interval, 2s timeout (for fast CI)
//
// Usage in main.go:
// tradierClient := broker.NewTradierClient(...)
// bot.broker = broker.NewCircuitBreakerBroker(tradierClient)
```

#### Success Metrics
- **100% unit test coverage for financial calculations** ✅ *Achieved in PR #2*
- 30 days without crashes
- 10+ completed trades
- 70%+ win rate

---

### Phase 2: Production Readiness (Weeks 5-8)
**Goal**: Implement adjustments and production features

#### New Components

##### SQLite Storage Migration
```sql
-- SQLite schema for ACID properties
CREATE TABLE positions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol TEXT NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('idle','scanning','entering','positioned','adjusting','closing')),
    entry_date TIMESTAMP NOT NULL,
    put_strike REAL NOT NULL,
    call_strike REAL NOT NULL,
    expiration DATE NOT NULL,
    quantity INTEGER NOT NULL,
    credit_received REAL NOT NULL,
    current_value REAL,
    realized_pnl REAL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE adjustments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    position_id INTEGER REFERENCES positions(id),
    adjustment_type TEXT NOT NULL,
    executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

##### Observability Stack
```go
// Structured logging with zap
logger, _ := zap.NewProduction()
defer logger.Sync()

logger.Info("executing strategy",
    zap.String("symbol", symbol),
    zap.Float64("ivr", ivr),
    zap.Int("dte", dte),
)

// Prometheus metrics
var (
    ordersPlaced = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "trading_orders_placed_total",
        },
        []string{"type", "status"},
    )
)
```

##### Security Enhancements
```go
// Encrypted credential storage
type SecureConfig struct {
    cipher cipher.Block
}

func (s *SecureConfig) GetAPIKey() (string, error) {
    // Decrypt API key from secure storage
}
```

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
+│   ├── storage/
+│   │   ├── sqlite.go        # SQLite implementation
+│   │   └── migrations/      # Database migrations
+│   ├── resilience/
+│   │   ├── retry.go         # Retry logic
+│   │   └── breaker.go       # Circuit breaker
+│   └── monitoring/
+│       ├── logger.go        # Structured logging
+│       └── metrics.go       # Prometheus metrics
```

---

### Phase 3: Multi-Asset & Optimization (Weeks 9-12)
**Goal**: Scale beyond single ticker with production database

#### PostgreSQL Migration
```sql
-- Production-ready PostgreSQL schema
CREATE TYPE position_status AS ENUM (
    'idle', 'scanning', 'entering', 
    'positioned', 'adjusting', 'closing'
);

CREATE TABLE positions (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    symbol VARCHAR(10) NOT NULL,
    status position_status NOT NULL,
    entry_date TIMESTAMPTZ NOT NULL,
    put_strike DECIMAL(10,2) NOT NULL,
    call_strike DECIMAL(10,2) NOT NULL,
    expiration DATE NOT NULL,
    quantity INTEGER NOT NULL,
    credit_received DECIMAL(10,2) NOT NULL,
    current_value DECIMAL(10,2),
    realized_pnl DECIMAL(10,2),
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Performance indexes
CREATE INDEX idx_positions_status ON positions(status);
CREATE INDEX idx_positions_symbol ON positions(symbol);
CREATE INDEX idx_positions_expiration ON positions(expiration);
```

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

### Phase 4: Production Deployment (Weeks 13-16)
**Goal**: Live trading ready with full monitoring

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
  use_otoco: false  # OTOCO orders are currently ignored/not implemented
  
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
  after_hours_check: false  # Enable after-hours position monitoring
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

### After-Hours Functionality

The bot supports optional after-hours position monitoring via the `after_hours_check` configuration flag:

- **Default Behavior**: When `false`, the bot only runs during regular trading hours (09:45-15:45 ET, Monday-Friday)
- **After-Hours Mode**: When `true`, the bot continues monitoring existing positions even outside regular hours
- **Functionality During After-Hours**:
  - Monitors existing positions for exit conditions (stop losses, profit targets)
  - **Does NOT** attempt new position entries
  - **Does NOT** perform position adjustments (Phase 2 feature)
  - Logs after-hours activity for transparency

#### Use Cases
- Risk management for overnight positions
- Monitoring positions during extended hours trading
- Emergency exit capabilities outside normal hours

#### Rate Limiting Implementation
```go
type RateLimitedClient struct {
    client     *http.Client
    limiter    *rate.Limiter  // Market Data: 60/min sandbox, 120/min prod
    lastRequest time.Time
    cache      *cache.Cache    // 1-minute TTL
}

func (c *RateLimitedClient) GetQuote(symbol string) (*Quote, error) {
    // Check cache first
    if cached, found := c.cache.Get(cacheKey(symbol)); found {
        return cached.(*Quote), nil
    }
    
    // Rate limit API call
    if err := c.limiter.Wait(context.Background()); err != nil {
        return nil, err
    }
    
    quote, err := c.client.GetQuote(symbol)
    if err != nil {
        return nil, err
    }
    
    c.cache.Set(cacheKey(symbol), quote, time.Minute)
    return quote, nil
}
```

### Order Execution Flow

```
1. Signal Generated
       ↓
2. Risk Check Pass?
   No → Log and Skip
   Yes ↓
3. Build Order
       ↓
4. Place Order (with retry)
       ↓
5. Wait for Fill (max 5 min)
   Partial → Adjust or Cancel
   Filled → Update Position
       ↓
6. Persist State (atomic file write; transactions arrive with SQLite in Phase 2)
```

### Error Handling Matrix

| Error Type | Response | Recovery | Implementation |
|------------|----------|----------|----------------|
| Network Timeout | Retry 3x with backoff | Skip cycle | Exponential backoff |
| API Rate Limit | Wait and retry | Reduce frequency | Rate limiter |
| Invalid Order | Log error | Manual review | Validation layer |
| Insufficient Funds | Alert | Halt trading | Pre-flight check |
| Position Not Found | Reconcile | Rebuild state | State recovery |
| Critical Error | Shutdown | Alert operator | Circuit breaker |

---

## Critical Improvements Required

### Immediate (MVP Blockers)
1. **Interface-Based Design** ⚠️
   - Define core interfaces (Broker, Storage, Strategy)
   - Implement dependency injection
   - Enable mock implementations

2. **Testing Infrastructure** ⚠️
   - Unit tests for all financial calculations
   - Integration tests for broker API
   - Mock data providers

3. **State Management** ⚠️
   - Implement position state machine
   - Add transaction support
   - Ensure consistency between memory and storage

4. **Error Resilience** ⚠️
   - Add retry logic with exponential backoff
   - Implement circuit breaker pattern
   - Categorize errors (transient vs permanent)

### Short-term (Production Requirements)
1. **Database Migration**
   - Move from JSON to SQLite (Phase 2)
   - Add ACID properties
   - Implement migrations

2. **Observability**
   - Structured logging (zap/logrus)
   - Metrics collection (Prometheus)
   - Health check endpoints

3. **Security**
   - Encrypt credentials
   - API request signing
   - Audit logging

### Long-term (Scalability)
1. **Multi-Asset Support**
2. **Event-Driven Architecture**
3. **PostgreSQL Migration**
4. **Web Dashboard**

---

## Deployment & Operations

### Development Environment
```bash
# Local development with tests
go test ./... -v
go run cmd/bot/main.go --config=config.dev.yaml

# Build with version info
go build -ldflags "-X main.version=$(git describe --tags)" \
    -o strangle-bot cmd/bot/main.go
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
StandardOutput=append:/var/log/strangle-bot/output.log
StandardError=append:/var/log/strangle-bot/error.log

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
- State transitions per hour

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
- Invalid state transition attempted

### Backup & Recovery

#### State Backup
- Position file: Every update (Phase 1)
- SQLite: Continuous backup (Phase 2)
- PostgreSQL: Daily backup with WAL (Phase 3)
- Configuration: Version controlled

#### Disaster Recovery
1. Stop bot immediately
2. Close all positions manually if needed
3. Restore from last known good state
4. Validate positions match broker
5. Resume in paper mode first
6. Verify state machine consistency

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
  
PLACE ORDER (with retry)
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

### State Transition Rules
```
IDLE → SCANNING: Market open & no position
SCANNING → ENTERING: Valid entry signal
ENTERING → POSITIONED: Order filled
POSITIONED → ADJUSTING: Adjustment trigger
POSITIONED → CLOSING: Exit signal
ADJUSTING → POSITIONED: Adjustment complete
CLOSING → IDLE: Position closed
ANY → IDLE: Error recovery
```

---

## Architecture Roadmap

### Week 1-2: Foundation
- [ ] Create interface definitions
- [ ] Implement dependency injection
- [ ] Write unit tests for core logic
- [ ] Add integration tests

### Week 3-4: Resilience
- [ ] Implement state machine
- [ ] Add retry logic
- [ ] Implement circuit breaker
- [ ] Enhanced error handling

### Week 5-6: Storage & Monitoring
- [ ] Migrate to SQLite
- [ ] Add structured logging
- [ ] Implement metrics
- [ ] Create health checks

### Week 7-8: Security & Testing
- [ ] Encrypt credentials
- [ ] Add comprehensive tests
- [ ] Performance testing
- [ ] 30-day paper trading

### Week 9+: Production & Scale
- [ ] PostgreSQL migration
- [ ] Multi-asset support
- [ ] Web dashboard
- [ ] Live trading preparation