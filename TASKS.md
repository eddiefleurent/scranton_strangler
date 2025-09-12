# SPY Strangle Bot - Outstanding Tasks

## 🚨 CRITICAL: Enhanced Risk Management System Implementation

### HIGH PRIORITY: Enhanced Position Monitoring System
- [ ] **URGENT: Implement high-frequency position monitoring**
  - [ ] **Profit Target**: Place single GTC limit order to close strangle at 50% credit
  - [ ] **Stop-Loss Monitoring**: Real-time P&L tracking with conditional market order execution
  - [x] **Monitoring Frequency**: 1-minute intervals configured (config updated)
  - [x] **SPY Extended Hours**: Documented 4:00-4:15 PM ET only (no pre-market)
  - [ ] **Trigger Logic**: When position P&L reaches -200% of credit, place immediate market order
  - [ ] **Order Management**: Automatically cancel profit target when stop-loss executes
  - [ ] **Extended Hours Orders**: Market orders only for emergency exits (liquidity constraints)
  - [x] **Documentation**: After-hours constraints and schedule clarified
  - [ ] **Implementation**: Still needs code changes for 1-minute monitoring loop

### URGENT: Enhanced Position Monitor Implementation
- [ ] **Create Enhanced Position Monitor Component**
  - [ ] Implement 1-minute P&L calculation loop during market hours (9:30-4:00 PM)
  - [ ] Add 1-minute monitoring during SPY extended hours (4:00-4:15 PM only)
  - [ ] Calculate real-time position P&L using current option quotes
  - [ ] Compare P&L against configurable stop-loss threshold (-200% default)
  - [ ] Trigger immediate market order when threshold breached
  - [ ] Log all monitoring events and threshold checks for debugging
  
### URGENT: After-Hours Configuration & Testing  
- [x] **Update Configuration Schema**
  - [x] Change default `market_check_interval` from "15m" to "1m" in config.yaml.example
  - [x] Set `after_hours_check: true` as recommended default
  - [ ] Add config validation for 1-minute minimum interval during trading hours
  - [x] Document SPY-specific extended hours limitations (4:00-4:15 PM only)

- [ ] **After-Hours Order Execution Logic**
  - [ ] Implement extended hours market state detection
  - [ ] Force market orders for after-hours exits (no limit orders due to liquidity)
  - [ ] Add bid-ask spread validation before placing after-hours orders  
  - [ ] Implement wider spread tolerance for after-hours execution
  - [ ] Log warnings for after-hours order placement and execution quality

### URGENT: Conditional Order Engine
- [ ] **Implement Conditional Order Execution System**
  - [ ] Add `ConditionalOrderEngine` to execute orders based on position metrics
  - [ ] Support P&L-based triggers for stop-loss execution  
  - [ ] Place market orders for immediate execution when stop-loss triggered
  - [ ] Integrate with existing retry logic and order status polling
  - [ ] Add failure handling and retry logic for conditional orders

### URGENT: Order Lifecycle Manager
- [ ] **Create Automatic Order Management System**
  - [ ] Track linked orders (profit target + stop-loss monitoring) per position
  - [ ] Automatically cancel profit target GTC order when stop-loss executes
  - [ ] Automatically stop monitoring when profit target fills
  - [ ] Handle order cancellation failures and edge cases
  - [ ] Update position data with order execution results and timestamps

### URGENT: Stop Loss Implementation Review - **SOLVED BY ENHANCED MONITORING**
- [x] **Analyzed current stop loss mechanism gaps**  
  - [x] **IDENTIFIED**: Only bot monitoring every 15 minutes during market hours (9:45-3:45 PM)
  - [x] **RISK CONFIRMED**: 18+ hours daily with zero stop-loss protection
  - [x] **RISK CONFIRMED**: No protection if bot crashes or during network outages
  - [x] **SOLUTION**: Enhanced monitoring system with 1-minute P&L tracking and conditional execution
  - [x] **RESEARCH**: P&L-based triggers preferred over standing stop-loss orders (avoid immediate fills)
  - [x] **IMPLEMENTATION**: Covered in Enhanced Position Monitoring section above

### URGENT: After-Hours Testing & Validation
- [ ] **Create Test Files** (after implementation is complete)
  - [ ] Create `internal/config/after_hours_test.go` - Configuration behavior tests
  - [ ] Create `internal/broker/after_hours_test.go` - Order restriction tests
  
- [ ] **Extended Hours Order Testing** (needs implementation first)
  - [ ] Test order placement during 4:00-4:15 PM window using sandbox
  - [ ] Verify market orders execute properly during extended hours
  - [ ] Test bid-ask spread validation and wider tolerance handling
  - [ ] Validate order rejection handling for pre-market SPY options (should fail gracefully)
  - [ ] Test after-hours quote retrieval and P&L calculation accuracy

- [ ] **Configuration Testing** (needs implementation first)
  - [ ] Test bot behavior with `after_hours_check: true` during extended hours
  - [ ] Test bot skips new entries after 3:45 PM (trading_end)
  - [ ] Test bot continues monitoring existing positions during 4:00-4:15 PM
  - [ ] Test bot stops monitoring after 4:15 PM (no SPY options trading)
  - [ ] Test 1-minute interval during both regular and extended hours

- [ ] **Market State Detection Tests** (partially implemented in test files)
  - [ ] Mock Tradier market clock responses for different states (open/closed/extended)
  - [ ] Test fallback to config-based hours when market clock API fails
  - [ ] Test timezone handling for extended hours (EST/EDT transitions)
  - [ ] Test holiday handling with extended hours enabled

## Current Focus: End-to-End Validation

### Paper Trading Validation (NEEDS COMPLETION)
- [ ] **End-to-End Testing**
  - [ ] Complete 3 successful paper trades with after-hours monitoring enabled
  - [ ] Test overnight gap scenarios during paper trading
  - [ ] No critical bugs in 1 week of running with 1-minute monitoring
  - [ ] All logs make sense and are useful (including after-hours events)

## Post-MVP Enhancements

### Phase 2: Reliability & Monitoring
- [ ] Better error handling with retries
- [ ] SQLite for position storage
- [ ] Structured logging with levels
- [ ] **Trade Monitoring & Alerting**
  - [ ] Discord webhook notifications for trade events (entry/exit/adjustments/alerts)
  - [ ] Simple event logging to append-only JSON file (trades.log)
  - [ ] Health check endpoint for monitoring systems
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

**Status**: Core functionality complete, dashboard implemented. Ready for extended paper trading validation.