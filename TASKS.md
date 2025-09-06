# SPY Strangle Bot - MVP Tasks

## Core MVP Features (Must Have)

### 1. Basic Trading Loop
- [ ] **Tradier API Integration**
  - [x] Authentication working
  - [x] Get SPY quotes 
  - [x] Get SPY option chains
  - [x] OTOCO order placement
  - [ ] Test with paper trading account
- [ ] **Entry Logic**  
  - [ ] Calculate IVR > 30 (simple 20-day lookback)
  - [ ] Find 45 DTE expiration (±5 days acceptable)
  - [ ] Select 16 delta strikes (or closest available)
  - [ ] Check minimum $2.00 credit requirement
  - [ ] Verify position sizing (max 35% allocation)
- [ ] **Exit Logic**
  - [x] OTOCO handles 50% profit automatically
  - [ ] Manual 21 DTE check (close regardless of P&L)
  - [ ] Emergency stop at 250% loss
- [ ] **Position Tracking**
  - [ ] Save positions to JSON file
  - [ ] Load positions on startup  
  - [ ] Calculate current P&L
  - [ ] Track days to expiration

### 2. Basic Risk Management
- [ ] **Position Sizing**
  - [ ] Calculate max position size based on account value
  - [ ] Enforce 35% allocation limit
  - [ ] Prevent overlapping positions (one at a time for MVP)
- [ ] **Hard Stops**
  - [ ] Close at 250% of credit received
  - [ ] Close at 5 DTE (assignment risk)
  - [ ] Close on any API/system error

### 3. Scheduler & Logging  
- [ ] **Cron Job Setup**
  - [ ] Run every 15 minutes during market hours
  - [ ] Skip weekends and holidays
  - [ ] Graceful shutdown handling
- [ ] **Basic Logging**
  - [ ] Entry/exit signals with reasoning
  - [ ] API errors and retries
  - [ ] Position P&L updates
  - [ ] Daily summary logs

## Testing & Validation

### 4. Paper Trading Validation
- [ ] **Test Entry Conditions**
  - [ ] Verify IVR calculation accuracy
  - [ ] Confirm strike selection logic  
  - [ ] Test position sizing math
  - [ ] Validate OTOCO order placement
- [ ] **Test Exit Conditions**
  - [ ] 50% profit target via OTOCO
  - [ ] 21 DTE manual close
  - [ ] Emergency stops trigger correctly
- [ ] **End-to-End Testing**
  - [ ] Complete 3 successful paper trades
  - [ ] No critical bugs in 1 week of running
  - [ ] All logs make sense and are useful

## Post-MVP Enhancements (Later)

### Phase 2: Reliability & Monitoring
- [ ] Better error handling with retries
- [ ] SQLite for position storage
- [ ] Structured logging with levels
- [ ] Email/Slack alerts on trades
- [ ] Basic web dashboard for monitoring

### Phase 3: Strategy Enhancements  
- [ ] "Football System" adjustments
- [ ] Multiple position management
- [ ] Different DTE/delta configurations
- [ ] Multi-ticker support (QQQ, IWM)
- [ ] Backtesting framework

## Implementation Priority (Week by Week)

### Week 1: Core Foundation
- [ ] **Get Paper Trading Working**
  - [ ] Verify Tradier sandbox credentials
  - [ ] Test OTOCO order with $1 credit strangle
  - [ ] Confirm orders appear in Tradier dashboard
- [ ] **Build IVR Calculator**
  - [ ] Simple 20-day historical volatility lookup
  - [ ] Current IV from option chain
  - [ ] Basic IVR = (current IV - 20d avg) / 20d avg  
- [ ] **Position Management**
  - [ ] Simple JSON file storage
  - [ ] Load/save position state on startup/shutdown
  - [ ] Calculate P&L from current quotes

### Week 2: Trading Logic  
- [ ] **Entry Scanner**
  - [ ] Check market hours (9:30 AM - 4:00 PM ET)
  - [ ] Calculate IVR and check > 30
  - [ ] Find closest expiration to 45 DTE
  - [ ] Get 16 delta strikes (or nearest available)
  - [ ] Validate minimum $2.00 credit
  - [ ] Place OTOCO order if all conditions met
- [ ] **Exit Monitor**
  - [ ] Check existing positions every 15 minutes  
  - [ ] Calculate days to expiration
  - [ ] Close position at 21 DTE (market order)
  - [ ] Emergency close at 250% loss

### Week 3: Polish & Test
- [ ] **Error Handling**
  - [ ] Retry API calls 3x with backoff
  - [ ] Log all errors with context
  - [ ] Graceful shutdown on critical errors
- [ ] **End-to-End Testing**
  - [ ] Run bot for 1 week without manual intervention
  - [ ] Complete at least 1 full trade cycle
  - [ ] Verify all logs are useful for debugging

## Success Criteria for MVP

### MVP Definition: Working Paper Trading Bot
A bot that can automatically:
1. Enter SPY short strangles when IVR > 30
2. Exit at 50% profit (via OTOCO) or 21 DTE  
3. Apply emergency stops (250% loss, 5 DTE)
4. Run unattended for 1 week without issues
5. Complete 3 successful trade cycles

### Must Have for Launch
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
- ❌ Interface abstractions (Broker/Strategy/Storage)
- ❌ Complex state machines (Football System) 
- ❌ Advanced order types (OCO, Bracket)
- ❌ Multiple position management
- ❌ SQLite/PostgreSQL database
- ❌ Comprehensive test coverage (>80%)
- ❌ Circuit breakers and observability  
- ❌ Web dashboards and monitoring
- ❌ Multi-asset support
- ❌ Backtesting framework

### Keep It Simple
- ✅ Direct struct implementations  
- ✅ Simple state: OPEN/CLOSED positions
- ✅ OTOCO orders only
- ✅ One position at a time
- ✅ JSON file storage
- ✅ Basic tests for math functions
- ✅ Simple retry logic with exponential backoff
- ✅ Console logging with timestamps
- ✅ SPY only
- ✅ Forward testing only