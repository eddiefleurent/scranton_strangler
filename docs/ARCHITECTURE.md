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
- Extracts SPY ATM implied volatility from option chain
- Caches data to minimize API calls

#### 2. Strategy Engine
- **Entry Logic**: SPY ATM IV ≥ 15% (configurable), 45 DTE, 16Δ strikes
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
- **OTOCO Orders**: Unsupported for multi-leg strangles (API limitation - different option_symbols)
- **OCO Orders**: Per-leg profit targets and stop-losses using GTC limit orders
- **Automated Risk Management**: Separate OCO orders per option leg for complete coverage
- Handles multi-leg orders (strangles) with individual leg management
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
    State           PositionState
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
```text
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
    // Account operations
    GetAccountBalance() (float64, error)
    GetOptionBuyingPower() (float64, error)
    GetPositions() ([]PositionItem, error)

    // Market data
    GetQuote(symbol string) (*QuoteItem, error)
    GetExpirations(symbol string) ([]string, error)
    GetOptionChain(symbol, expiration string, withGreeks bool) ([]Option, error)
    GetMarketClock(delayed bool) (*MarketClockResponse, error)
    IsTradingDay(delayed bool) (bool, error)

    // Order placement
    PlaceStrangleOrder(symbol string, putStrike, callStrike float64, expiration string,
        quantity int, limitPrice float64, preview bool, duration string, tag string) (*OrderResponse, error)
    PlaceStrangleOTOCO(symbol string, putStrike, callStrike float64, expiration string,
        quantity int, credit, profitTarget float64, preview bool) (*OrderResponse, error)

    // Order status
    GetOrderStatus(orderID int) (*OrderResponse, error)
    GetOrderStatusCtx(ctx context.Context, orderID int) (*OrderResponse, error)

    // Position closing
    CloseStranglePosition(symbol string, putStrike, callStrike float64, expiration string,
        quantity int, maxDebit float64, tag string) (*OrderResponse, error)
    PlaceBuyToCloseOrder(optionSymbol string, quantity int,
        maxPrice float64) (*OrderResponse, error)
}

type Storage interface {
    // Position management
    GetCurrentPosition() *Position
    SetCurrentPosition(pos *Position) error
    ClosePosition(finalPnL float64, reason string) error
    AddAdjustment(adj Adjustment) error

    // Data persistence
    Save() error
    Load() error

    // Historical data and analytics
    GetHistory() []Position
    HasInHistory(id string) bool
    GetStatistics() *Statistics
    GetDailyPnL(date string) float64
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
| ~~No retry/circuit breaker~~ | ~~API failures cascade~~ | ~~High~~ → Resolved in PR #2 (tune thresholds/alerts) |
| Plain text credentials | Security vulnerability | High |
| No structured logging | Debugging difficult | Medium |

---

## Order Types & Automation Strategy

### OTOCO Orders (One-Triggers-One-Cancels-Other) - **Unsupported for Multi-Leg**
**Status**: Cannot be used for short strangles due to API limitations
- **Root Cause**: Tradier OTOCO requires same `option_symbol` but strangles use different symbols per leg
- **Current Behavior**: `PlaceStrangleOTOCO()` returns `ErrOTOCOUnsupported`
- **Alternative**: Enhanced monitoring system with automated order management
- **Implementation**: See `internal/broker/tradier.go:1191`

### Enhanced Monitoring System - **Recommended Solution**
**Architecture**: Hybrid approach combining GTC orders with intelligent position monitoring
- **Profit Target**: Single GTC limit order to close entire strangle at 50% of credit
- **Stop-Loss Protection**: Real-time P&L monitoring with conditional market order execution
- **Monitoring Frequency**: Every 1-minute during market hours, 5-minute during extended hours
- **Trigger Logic**: When position P&L reaches -200% of credit, place immediate market order
- **Order Management**: System automatically cancels profit target when stop-loss executes
- **24/7 Protection**: Continuous monitoring provides protection when bot is active
- **Fallback**: Manual monitoring alerts for system failures

**Implementation Components**:
1. **Enhanced Position Monitor**: High-frequency P&L calculation and threshold checking
2. **Conditional Order Engine**: Trigger market orders based on position metrics
3. **Order Lifecycle Manager**: Automatic cancellation of linked orders upon execution
4. **Extended Hours Support**: Reduced frequency monitoring during pre/post market

**Football System Use Cases**:
1. **Second Down Rolling**: Close at 70% profit OR roll untested side
2. **Third Down Management**: Take 25% profit OR continue to Fourth Down  
3. **Fourth Down Stops**: Profit target OR 250% loss limit
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
- **Example**: Close at 50% profit OR 21 DTE OR 250% loss OR delta > 1.0
- **Benefit**: Comprehensive automation without monitoring

### Regular Orders
**Fallback**: When advanced orders not available
- Standard multileg strangle orders
- Individual leg adjustments  
- Manual monitoring required

### Automated Order Flow Decision Tree
```text
Position Entry:
  use_otoco=true? → OTOCO (entry + 50% exit) [UNSUPPORTED/IGNORED]
  use_bracket=true? → Bracket (entry + profit + stop) [PLANNED/IGNORED]
  Default → Regular strangle + monitoring [CURRENT RUNTIME]

Position Management:
  First Down → Monitor via regular orders (OTOCO unsupported)
  Second Down → OCO (70% profit OR roll untested) ⭐ CURRENT RUNTIME
  Third Down → OCO (25% profit OR continue) [PLANNED/IGNORED]
  Fourth Down → OCO (any profit OR 250% stop) [PLANNED/IGNORED]
  
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
```text
spy-strangle-bot/
├── cmd/bot/main.go           # Entry point with dependency injection
├── internal/
│   ├── broker/
│   │   ├── interface.go      # Broker interface + wrappers
│   │   ├── tradier.go        # Implements Broker interface
│   │   ├── interface_test.go
│   │   └── tradier_test.go
│   ├── strategy/
│   │   ├── strangle.go       # Implements Strategy interface
│   │   └── strangle_test.go
│   ├── storage/
│   │   ├── interface.go      # Storage interface
│   │   ├── storage.go        # JSON storage implementation
│   │   ├── interface_test.go
│   │   ├── storage_test.go
│   │   └── mock_storage.go
│   ├── models/
│   │   ├── position.go
│   │   ├── state_machine.go   # Position state machine
│   │   └── state_machine_test.go
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   ├── orders/
│   │   ├── manager.go         # Order execution manager
│   │   └── manager_test.go
│   ├── retry/
│   │   ├── client.go          # Retry client wrapper
│   │   └── client_retry_test.go
│   └── mock/
│       ├── mock_data.go       # Mock data providers
│       └── mock_data_test.go
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
    state TEXT NOT NULL CHECK(state IN ('idle','submitted','open','closed','error','first_down','second_down','third_down','fourth_down','adjusting','rolling')),
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
CREATE TYPE position_state AS ENUM (
    'idle', 'submitted', 'open',
    'closed', 'error', 'first_down',
    'second_down', 'third_down', 'fourth_down',
    'adjusting', 'rolling'
);

CREATE TABLE positions (
    id UUID DEFAULT gen_random_uuid() PRIMARY KEY,
    symbol VARCHAR(10) NOT NULL,
    state position_state NOT NULL,
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
CREATE INDEX idx_positions_state ON positions(state);
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
- Alert system (Telegram/Discord)
- Unraid deployment optimization

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

### Position Synchronization & State Management

The bot implements robust position synchronization to prevent broker/storage mismatches that can lead to over-allocation and trading errors.

#### Reconciliation System

```go
// Main reconciliation flow
func (b *Bot) reconcilePositions(storedPositions []models.Position) []models.Position {
    // 1. Get current broker positions
    brokerPositions := b.broker.GetPositions()
    
    // 2. Check stored positions against broker (detect manual closes)
    activePositions := []models.Position{}
    for _, position := range storedPositions {
        if b.isPositionOpenInBroker(&position, brokerPositions) {
            activePositions = append(activePositions, position)
        } else {
            b.storage.ClosePositionByID(position.ID, finalPnL, "manual_close")
        }
    }
    
    // 3. Detect orphaned broker positions (timeout-related sync issues)
    orphanedStrangles := b.findOrphanedStrangles(brokerPositions, activePositions)
    for _, orphan := range orphanedStrangles {
        recoveryPos := b.createRecoveryPosition(orphan)
        b.storage.AddPosition(recoveryPos)
        activePositions = append(activePositions, *recoveryPos)
    }
    
    return activePositions
}
```

#### Enhanced Timeout Handling

**Problem Solved**: Order polling goroutines would timeout after 5 minutes and prematurely close positions locally, while orders actually filled on the broker side.

```go
func (m *Manager) handleOrderTimeout(positionID string) {
    position := m.storage.GetPositionByID(positionID)
    
    // NEW: Check broker before closing locally
    brokerPositions, err := m.broker.GetPositions()
    if err == nil {
        isOpenInBroker := m.isPositionOpenInBroker(position, brokerPositions)
        if isOpenInBroker {
            // Order actually filled! Recover the position
            position.TransitionState(models.StateOpen, "order_filled")
            m.storage.UpdatePosition(position)
            return // Exit early - position recovered
        }
    }
    
    // Original timeout handling only if truly not filled
    m.storage.ClosePositionByID(positionID, 0, "order_timeout")
}
```

#### Reconciliation Triggers

1. **Every Trading Cycle**: `reconcilePositions()` runs at start of each 15-minute cycle
2. **Before New Trades**: Additional reconcile check before opening positions to prevent exceeding limits
3. **Bot Restart**: Automatic reconciliation on startup to handle deployment interruptions

#### Emergency Liquidation

```bash
# Force close all positions via market orders
make liquidate

# Direct API utility
go run scripts/liquidate_positions.go
```

The liquidation tool:
- Fetches current broker positions
- Places market orders for immediate execution
- Provides emergency position closure capability

#### Common Sync Issues Fixed

| Issue | Root Cause | Solution |
|-------|------------|----------|
| **Over-allocation** | Bot loses track of filled orders | Reconcile before new trades |
| **Timeout Premature Close** | 5min polling timeout vs slow sandbox fills | Check broker before local close |
| **Redeploy Orphans** | Killed goroutines lose order tracking | Startup reconciliation |
| **Manual Intervention** | User closes positions via broker UI | Detect manual closes in reconcile |

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
  use_otoco: false  # OTOCO orders are unsupported at runtime (paper-only flags tolerated)
  
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
  max_position_loss: 2.5  # 250% of credit
  
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

### Trading Hours & Order Duration Configuration

#### Trading Schedule
The bot operates on a configurable schedule optimized for options liquidity:

- **Default Trading Window**: 09:45–15:45 America/New_York (Mon–Fri, excluding U.S. market holidays)
  - Starts 15 minutes after market open for price stability
  - Stops 15 minutes before close to avoid end-of-day volatility
- **Check Interval**: Every 15 minutes via cron scheduler
- **Time Zone Aware**: Handles daylight saving time transitions automatically

#### Order Duration Types
The Tradier API integration supports multiple order duration types:

| Duration | Description | Use Case |
|----------|-------------|----------|
| **GTC** | Good Till Cancelled - remains active until filled or manually cancelled | **Default for limit orders** - ensures orders persist across trading sessions |
| **Day** | Expires at end of regular trading day (4:00 PM ET) | Quick fills or day-specific strategies |
| **Pre** | Extended hours pre-market (4:00 AM - 9:30 AM ET) | Pre-market trading (rarely used for options) |
| **Post** | Extended hours post-market (4:00 PM - 8:00 PM ET) | After-hours trading (rarely used for options) |

**Note**: Options typically have limited liquidity outside regular market hours. The bot defaults to GTC orders for strangle positions to ensure fills during regular trading hours across multiple sessions if needed.

#### After-Hours Functionality

The bot supports optional after-hours position monitoring via the `after_hours_check` configuration flag:

- **Default Behavior**: When `false`, the bot runs during the configured trading window only
- **After-Hours Mode**: When `true`, the bot continues monitoring existing positions even outside regular hours
- **SPY Options Extended Hours**: Limited to 4:00-4:15 PM ET (15 minutes post-close only)
- **Functionality During After-Hours**:
  - Monitors existing positions for exit conditions (stop losses, profit targets)
  - Emergency market orders for critical stop-loss breaches
  - **Does NOT** attempt new position entries
  - **Does NOT** perform position adjustments (Phase 2 feature)
  - Logs after-hours activity for transparency

##### Recommended Configuration
```yaml
schedule:
  market_check_interval: "1m"     # 1-minute for stop-loss monitoring
  trading_start: "09:45"          # Conservative entry window
  trading_end: "15:45"            # Conservative exit window  
  after_hours_check: true         # Monitor during 4:00-4:15 PM window ONLY
```

##### Monitoring Schedule Clarification
**IMPORTANT**: The bot monitors positions only when markets are open and trading is possible:

| Time Period | Monitoring Status | Reason |
|------------|------------------|--------|
| **9:30 AM - 4:00 PM ET** | ✅ Active (1-min intervals) | Regular trading hours |
| **4:00 PM - 4:15 PM ET** | ✅ Active (1-min intervals) | SPY extended hours window |
| **4:15 PM - 9:30 AM ET** | ❌ STOPPED | Market closed - no trading possible |
| **Weekends** | ❌ STOPPED | Market closed |

**Key Points**:
- Bot **STOPS at 4:15 PM** - cannot exit positions when market is closed
- Bot **RESUMES at 9:30 AM** next trading day
- Overnight gap risk exists but cannot be mitigated until market opens
- First cycle at 9:30 AM checks for adverse overnight moves

##### SPY Options After-Hours Constraints
- **Limited Window**: Only 4:00-4:15 PM ET (no pre-market)
- **Reduced Liquidity**: Wider bid-ask spreads, potential for poor fills
- **Order Types**: Market orders recommended for emergency exits only
- **No 24/7 Monitoring**: Positions cannot be adjusted when markets are closed

##### Use Cases
- Risk management for overnight positions
- Emergency exits during 4:00-4:15 PM window
- Stop-loss protection against after-hours news events
- Profit target monitoring during extended session

#### Rate Limiting Implementation
```go
type RateLimitedClient struct {
    client     *http.Client
    limiter    *rate.Limiter  // Market Data: 120/min sandbox, 500/min prod (defaults; overridable)
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

### Immediate (MVP Verification)
1. Interface-Based Design — Completed in PR #2 (verify DI wiring in cmd/bot)
2. Testing Infrastructure — 95%+ coverage (expand E2E sandbox runs)
3. State Management — Position state machine done (add invariants/guards)
4. Error Resilience — Retry + circuit breaker done (tune thresholds/alerts)

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

### Integrated Web Dashboard (Current Implementation)

#### HTMX Dashboard Architecture
The bot includes an integrated web dashboard served alongside the main trading application:

- **Single Binary Deployment**: Dashboard runs as HTTP server within the main bot process
- **Port Configuration**: Uses configurable esoteric port (default: 9847) for Unraid compatibility
- **Technology Stack**: Go net/http + HTML templates + HTMX for dynamic updates
- **Zero Build Step**: Pure server-side rendered HTML with minimal JavaScript

#### Dashboard Components

##### Backend (internal/dashboard/)
```go
type DashboardServer struct {
    storage  storage.Interface  // Existing storage layer integration
    broker   broker.Interface   // Real-time position data
    config   *config.Config     // Dashboard configuration
    server   *http.Server       // HTTP server instance
}

// HTTP Routes
GET  /                 # Main dashboard page
GET  /api/positions    # HTMX endpoint for positions table
GET  /api/stats        # HTMX endpoint for statistics overview  
GET  /api/position/{id} # Position detail modal/panel
GET  /health           # Health check endpoint
```

##### Frontend Templates (web/templates/)
- **dashboard.html**: Main layout with auto-refreshing sections
- **positions.html**: Active positions table (HTMX partial)
- **stats.html**: Performance statistics cards (HTMX partial)
- **position-detail.html**: Individual position details modal

##### Key Features
- **Real-time Updates**: HTMX polling every 15-30 seconds (`hx-trigger="every 15s"`)
- **Active Positions View**: 
  - Position ID, symbol, entry date
  - Strike prices (put/call), days to expiration
  - Current P&L, profit percentage, profit target progress
  - Position state (Open, First Down, etc.)
  - Exit condition indicators
- **Statistics Dashboard**:
  - Win rate, current streak, total P&L
  - Daily P&L tracking, average win/loss
  - Account allocation usage
- **Mobile Responsive**: Clean CSS grid/flexbox layout
- **Interactive Elements**: Click position rows for adjustment history details

#### Configuration Integration
```yaml
dashboard:
  enabled: true
  port: 9847              # Esoteric port for Unraid
  host: "0.0.0.0"         # Bind to all interfaces
  refresh_interval: "15s" # HTMX polling interval
  auth:
    enabled: false        # Optional basic auth
    username: ""
    password: ""
```

#### Deployment Integration

**Unraid Deployment**: 
- Dashboard accessible at `http://your-unraid-ip:9847`
- Single binary includes both bot and dashboard
- No additional containers or services required

**Makefile Updates**:
```bash
# Updated deployment includes dashboard port mapping
make deploy-unraid     # Deploys bot with integrated dashboard
make unraid-dashboard  # Show dashboard URL and status
```

#### Implementation Benefits
- **Zero Infrastructure**: No separate web server or database required
- **Real-time Monitoring**: Live position updates without manual refresh
- **Mobile Access**: Monitor trades from anywhere with responsive design
- **Minimal Overhead**: ~200 lines of Go code, negligible performance impact
- **Development Speed**: HTMX eliminates complex JavaScript build processes

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

#### Unraid Deployment
```bash
# Single command deploys bot with integrated dashboard
make deploy-unraid

# Access dashboard at http://unraid-ip:9847
# Monitor via: make unraid-dashboard
# View logs: make unraid-logs
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
  
Loss > 250% of credit?
  Yes → CLOSE (Phase 2)
  No ↓
  
Continue monitoring
```

### State Transition Rules
```text
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