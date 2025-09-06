# SPY Strangle Bot - MVP Tasks

## Core MVP Features (Must Have)

### 1. Basic Trading Loop
- [ ] **Tradier API Integration**
  - [x] Authentication working
  - [x] Get SPY quotes 
  - [x] Get SPY option chains
  - [x] OTOCO order placement
  - [x] Buy-to-close order placement
  - [ ] Test with paper trading account
- [x] **Entry Logic**  
  - [x] Calculate IVR > 30 (simple 20-day lookback)
  - [x] Find 45 DTE expiration (±5 days acceptable)
  - [x] Select 16 delta strikes (or closest available)
  - [x] Check minimum $2.00 credit requirement
  - [x] Verify position sizing (max 35% allocation)
- [x] **Exit Logic**
  - [x] OTOCO handles 50% profit automatically
  - [x] Manual 21 DTE check (close regardless of P&L)
  - [x] Emergency stop at 250% loss
- [x] **Position Tracking**
  - [x] Save positions to JSON file
  - [x] Load positions on startup  
  - [x] Calculate current P&L
  - [x] Track days to expiration

### 2. Basic Risk Management
- [x] **Position Sizing**
  - [x] Calculate max position size based on account value
  - [x] Enforce 35% allocation limit
  - [x] Prevent overlapping positions (one at a time for MVP)
- [x] **Hard Stops**
  - [x] Close at 250% of credit received
  - [x] Close at 5 DTE (assignment risk)
  - [x] Close on any API/system error

### 3. Scheduler & Logging  
- [x] **Cron Job Setup**
  - [x] Run every 15 minutes during market hours
  - [x] Skip weekends and holidays
  - [x] Graceful shutdown handling
- [x] **Basic Logging**
  - [x] Entry/exit signals with reasoning
  - [x] API errors and retries
  - [x] Position P&L updates
  - [x] Daily summary logs

## Testing & Validation

### 4. Paper Trading Validation
- [ ] **API Setup & Connection**
  - [ ] Get Tradier sandbox API key
  - [ ] Test basic API connectivity
  - [ ] Verify account balance retrieval
  - [ ] Test option chain data access
- [ ] **Test Entry Conditions**
  - [ ] Verify IVR calculation accuracy
  - [ ] Confirm strike selection logic  
  - [ ] Test position sizing math
  - [ ] Validate OTOCO order placement
- [ ] **Test Exit Conditions**
  - [ ] 50% profit target detection
  - [ ] 21 DTE manual close
  - [ ] Buy-to-close order execution
  - [ ] Emergency stops trigger correctly
- [ ] **End-to-End Testing**
  - [ ] Complete 3 successful paper trades
  - [ ] No critical bugs in 1 week of running
  - [ ] All logs make sense and are useful

### 5. Critical Test Coverage (MVP Blocker)
- [ ] **Core Strategy Testing - 0% Coverage**
  - [ ] Test `CheckEntryConditions()` - validates IVR > 30, DTE, delta logic
  - [ ] Test `FindStrangleStrikes()` - strike selection and credit validation  
  - [ ] Test `CheckExitConditions()` - 50% profit, 21 DTE, 250% loss conditions
  - [ ] Test `CalculatePnL()` - position value calculations with live quotes
  - [ ] Test `GetCurrentIVR()` - IV rank calculation with historical data
- [ ] **Broker API Integration Testing - 0% Coverage** 
  - [ ] Test `GetQuote()` - quote fetching with error handling
  - [ ] Test `GetOptionChain()` - option data parsing and greeks
  - [ ] Test `PlaceStrangleOrder()` - OTOCO order creation and validation
  - [ ] Test `PlaceBuyToCloseOrder()` - exit order execution
  - [ ] Test `GetOrderStatus()` - order fill verification and status tracking
- [ ] **Main Bot Loop Testing - 0% Coverage**
  - [ ] Test `runTradingCycle()` - complete entry/exit workflow 
  - [ ] Test `executeEntry()` - position opening with risk checks
  - [ ] Test `executeExit()` - position closing logic
  - [ ] Test `checkExistingPosition()` - position monitoring and P&L updates
  - [ ] Test error handling and graceful shutdown scenarios
- [ ] **Order Management Testing - 0% Coverage**
  - [ ] Test `PollOrderStatus()` - order fill verification with timeouts
  - [ ] Test order failure handling and retry logic
  - [ ] Test partial fill scenarios and position state updates
- [ ] **Retry Client Testing - 0% Coverage**
  - [ ] Test exponential backoff logic with API failures
  - [ ] Test transient error detection and retry triggers
  - [ ] Test timeout handling and circuit breaker behavior

### 6. Bug Fixes & Polish
- [ ] **Historical IV Data Storage**
  - [ ] Replace mock historical IV with real data collection
  - [ ] Store daily IV readings for accurate IVR calculation
  - [ ] Implement rolling 20-day IV history
- [ ] **Order Fill Verification** 
  - [ ] Wait for order fills before updating position state
  - [ ] Handle partial fills correctly
  - [ ] Implement order status checking
- [ ] **Error Recovery**
  - [ ] Handle API downtime gracefully
  - [ ] Recover from network interruptions
  - [ ] Validate position state on startup

## Post-MVP Enhancements (Later)

### Phase 2: Reliability & Monitoring
- [ ] Better error handling with retries
- [ ] SQLite for position storage
- [ ] Structured logging with levels
- [ ] **Trade Monitoring & Alerting**
  - [ ] Discord webhook notifications for trade events (entry/exit/adjustments/alerts)
  - [ ] Simple event logging to append-only JSON file (trades.log)
  - [ ] Basic web dashboard with HTMX for position viewing
  - [ ] Real-time position status endpoint for dashboard polling
  - [ ] Configuration for notification levels and webhook URL

### Phase 3: Strategy Enhancements  
- [ ] "Football System" adjustments
- [ ] Multiple position management
- [ ] Different DTE/delta configurations
- [ ] Multi-ticker support (QQQ, IWM)
- [ ] Backtesting framework

## Implementation Priority (Week by Week)

### Week 1: Critical Test Foundation (MVP Blocker - 29.5% Overall Coverage)
- [ ] **Strategy Function Tests (Currently 0% Coverage)**
  - [ ] Create test suite for `CheckEntryConditions()` with mock data
  - [ ] Create test suite for `FindStrangleStrikes()` with various market scenarios
  - [ ] Create test suite for `CheckExitConditions()` covering all exit triggers
  - [ ] Create test suite for `GetCurrentIVR()` with historical IV data
- [ ] **Broker Integration Tests (Currently 0% Coverage)**
  - [ ] Create mock broker tests for all API methods 
  - [ ] Test error handling for API failures and timeouts
  - [ ] Test order placement validation and response parsing
  - [ ] Test quote and option chain data processing
- [ ] **Main Bot Loop Tests (Currently 0% Coverage)**
  - [ ] Create integration tests for complete trading cycle
  - [ ] Test position monitoring and P&L calculation logic
  - [ ] Test entry/exit execution with mocked broker responses

### Week 2: Core Functionality with Test Coverage
- [ ] **Entry Logic (Test While Building)**
  - [ ] Check market hours with comprehensive test cases
  - [ ] Calculate IVR with unit tests for edge cases
  - [ ] Find DTE/strike logic with boundary condition tests
  - [ ] OTOCO order placement with mock broker verification
- [ ] **Exit Logic (Test While Building)**
  - [ ] Monitor existing positions with test scenarios
  - [ ] Test all exit condition triggers (profit, time, loss)
  - [ ] Test emergency exit scenarios and error recovery
- [ ] **Order Management (Currently 0% Coverage)**
  - [ ] Create tests for order polling and status checking
  - [ ] Test timeout handling and retry scenarios
  - [ ] Test partial fill detection and position updates

### Week 3: Integration Testing & Polish
- [ ] **Error Handling (Test All Scenarios)**
  - [ ] Test retry client with various failure modes
  - [ ] Test API downtime recovery with circuit breaker
  - [ ] Test graceful shutdown with position preservation
- [ ] **End-to-End Testing**
  - [ ] Complete trade cycle tests in sandbox environment  
  - [ ] Test bot restart/recovery after crashes
  - [ ] Validate all error scenarios are properly logged

## Success Criteria for MVP

### MVP Definition: Working Paper Trading Bot
A bot that can automatically:
1. Enter SPY short strangles when IVR > 30
2. Exit at 50% profit (via OTOCO) or 21 DTE  
3. Apply emergency stops (250% loss, 5 DTE)
4. Run unattended for 1 week without issues
5. Complete 3 successful trade cycles

### Must Have for Launch
- [ ] **Minimum 60% Test Coverage** (Currently 29.5% - CRITICAL GAP)
  - [ ] Core strategy functions fully tested (0% → 80%+ target)
  - [ ] Broker integration tested with mocks (0% → 70%+ target)  
  - [ ] Main bot loop tested (0% → 60%+ target)
  - [ ] Order management tested (0% → 70%+ target)
  - [ ] Retry logic tested (0% → 80%+ target)
- [ ] Tradier API integration working in sandbox
- [ ] IVR calculation (simple 20-day method)
- [ ] Entry logic: find strikes, check credit, place OTOCO
- [ ] Exit logic: 21 DTE monitor, emergency stops
- [ ] JSON position persistence
- [ ] Basic error handling with retries
- [ ] Market hours checking
- [ ] Position sizing (max 35% allocation)

### Success Metrics
- [ ] 3 completed paper trades (entry to exit)
- [ ] No unhandled crashes for 1 week
- [ ] All trades respect risk limits
- [ ] Logs provide clear audit trail
- [ ] Can restart bot and resume correctly

---

## Removed Complexity (Post-MVP)

### Unnecessary for MVP
- ❌ Advanced order types (OCO, Bracket orders)
- ❌ Multiple position management
- ❌ SQLite/PostgreSQL database
- ❌ Comprehensive test coverage (>80%)
- ❌ Circuit breakers and observability
- ❌ Web dashboards and monitoring
- ❌ Multi-asset support
- ❌ Backtesting framework

### Keep It Simple
- ✅ Interface abstractions (Broker/Strategy/Storage) - implemented for MVP
- ✅ Simple state machine (OPEN/CLOSED positions)
- ✅ OTOCO orders (in-scope) vs OCO/Bracket (post-MVP)
- ✅ One position at a time
- ✅ JSON file storage
- ✅ Basic tests for math functions
- ✅ Simple retry logic with exponential backoff
- ✅ Console logging with timestamps
- ✅ SPY only
- ✅ Forward testing only

## CRITICAL TEST COVERAGE ANALYSIS (Current: 29.5%)

### 🚨 IMMEDIATE MVP BLOCKERS - 0% Coverage:
1. **Core Strategy Logic** (`internal/strategy/strangle.go`)
   - `CheckEntryConditions()` - Entry signal validation
   - `FindStrangleStrikes()` - Strike selection and credit validation  
   - `CheckExitConditions()` - All exit condition logic
   - `GetCurrentIVR()` - IV rank calculation
   - `CalculatePnL()` - Position value calculations

2. **Broker API Integration** (`internal/broker/tradier.go`)
   - `GetQuote()` - Quote fetching with error handling
   - `GetOptionChain()` - Option data parsing and validation
   - `PlaceStrangleOrder()` - OTOCO order creation
   - `GetOrderStatus()` - Order fill verification

3. **Main Bot Loop** (`cmd/bot/main.go`)
   - `runTradingCycle()` - Complete trading workflow
   - `executeEntry()` - Position opening logic
   - `executeExit()` - Position closing logic
   - `checkExistingPosition()` - Position monitoring

4. **Order Management** (`internal/orders/manager.go`)
   - `PollOrderStatus()` - Order status polling
   - Order timeout and retry handling
   - Partial fill scenarios

5. **Retry Client** (`internal/retry/client.go`)
   - Exponential backoff implementation
   - Error classification and retry logic
   - Timeout and circuit breaker behavior

### ✅ Well-Tested Components (>70% Coverage):
- State Machine (73.1%) - Position state transitions
- Mock Data Provider (90.5%) - Testing infrastructure
- Storage Interface (57.0%) - Position persistence

### 🎯 Test Coverage Targets for MVP Launch:
- **Overall Coverage**: 29.5% → **60%+** (minimum)
- **Strategy Functions**: 0% → **80%+** (critical path)
- **Broker Integration**: 0% → **70%+** (API reliability)
- **Main Bot Loop**: 0% → **60%+** (core functionality)
- **Orders & Retry**: 0% → **70%+** (resilience)

### 📋 Testing Priority Order:
1. **Week 1**: Strategy function unit tests (highest impact)
2. **Week 2**: Broker integration tests with mocks 
3. **Week 3**: Main bot loop integration tests
4. **Week 4**: Order management and retry logic tests
5. **Week 5**: End-to-end testing in sandbox environment

**Bottom Line**: The current 29.5% test coverage is insufficient for a reliable trading bot. The 0% coverage on core strategy and broker functions represents significant risk. Testing must be prioritized before any production deployment.