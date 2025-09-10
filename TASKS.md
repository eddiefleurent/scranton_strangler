# SPY Strangle Bot - MVP Tasks

## Core MVP Features (Must Have)

### 1. Basic Trading Loop
- [x] **Tradier API Integration**
  - [x] Authentication working
  - [x] Get SPY quotes
  - [x] Get SPY option chains
  - [x] OTOCO order placement
  - [x] Buy-to-close order placement
  - [x] Test with paper trading account
- [x] **Entry Logic**
  - [x] Calculate IV > 30 (absolute IV, simple 20-day lookback)
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
  - [x] Close at 21 DTE (forced exit)
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
- [x] **API Setup & Connection**
  - [x] Get Tradier sandbox API key
  - [x] Test basic API connectivity
  - [x] Verify account balance retrieval
  - [x] Test option chain data access
- [x] **Test Entry Conditions**
  - [x] Verify IV calculation accuracy (absolute IV, 20-day lookback)
  - [x] Confirm strike selection logic
  - [x] Test position sizing math
  - [x] Validate OTOCO order placement
- [x] **Test Exit Conditions**
  - [x] 50% profit target detection
  - [x] 21 DTE manual close
  - [x] Buy-to-close order execution
  - [x] Emergency stops trigger correctly
- [ ] **End-to-End Testing** (NEEDS VALIDATION)
  - [ ] Complete 3 successful paper trades
  - [ ] No critical bugs in 1 week of running
  - [ ] All logs make sense and are useful

### 5. Critical Test Coverage (MVP Blocker)
- [x] **Core Strategy Testing - 60% Coverage**
  - [x] Test `CheckEntryConditions()` - validates IV > 30%, DTE, delta logic
  - [x] Test `FindStrangleStrikes()` - strike selection and credit validation
  - [x] Test `CheckExitConditions()` - 50% profit, 21 DTE, 250% loss conditions
  - [x] Test `CalculatePnL()` - position value calculations with live quotes
  - [x] Test `GetCurrentIV()` - IV calculation with historical data (absolute IV, 20-day lookback)
- [x] **Broker API Integration Testing - 73.1% Coverage**
  - [x] Test `GetQuote()` - quote fetching with error handling
  - [x] Test `GetOptionChain()` - option data parsing and greeks
  - [x] Test `PlaceStrangleOrder()` - OTOCO order creation and validation
  - [x] Test `PlaceBuyToCloseOrder()` - exit order execution
  - [x] Test `GetOrderStatus()` - order fill verification and status tracking
- [x] **Main Bot Loop Testing - Refactored & Component Tested**
  - [x] Test `runTradingCycle()` - complete entry/exit workflow
  - [x] Test `executeEntry()` - position opening with risk checks
  - [x] Test `executeExit()` - position closing logic
  - [x] Test reconciliation and position monitoring
  - [x] Test error handling and graceful shutdown scenarios
- [x] **Order Management Testing - 68% Coverage**
  - [x] Test `PollOrderStatus()` - order fill verification with timeouts
  - [x] Test order failure handling and retry logic
  - [x] Test partial fill scenarios and position state updates
- [x] **Retry Client Testing - 91.5% Coverage**
  - [x] Test exponential backoff logic with API failures
  - [x] Test transient error detection and retry triggers
  - [x] Test timeout handling and circuit breaker behavior

### 6. Bug Fixes & Polish
- [x] **Historical IV Data Storage**
  - [x] Replace mock historical IV with real data collection
  - [x] Store daily IV readings for accurate IVR calculation
  - [x] Implement rolling 20-day IV history
- [x] **Order Fill Verification**
  - [x] Wait for order fills before updating position state
  - [x] Handle partial fills correctly
  - [x] Implement order status checking
- [x] **Error Recovery**
  - [x] Handle API downtime gracefully
  - [x] Recover from network interruptions
  - [x] Validate position state on startup

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

## Success Criteria for MVP

### MVP Definition: Working Paper Trading Bot
A bot that can automatically:
1. Enter SPY short strangles when IV > 30 (absolute IV)
2. Exit at 50% profit (via OTOCO) or 21 DTE
3. Apply emergency stops (250% loss, 21 DTE)
4. Run unattended for 1 week without issues
5. Complete 3 successful trade cycles