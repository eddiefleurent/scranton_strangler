# SPY Short Strangle Master Strategy

## Quick Reference Card
- **Product**: SPY (S&P 500 ETF)
- **Strategy**: Short Strangle (sell OTM put + OTM call)
- **Target Win Rate**: 80-90% (with proper management)
- **Typical Duration**: MaxDTE-30 days (from 45 DTE entry, where MaxDTE = 21)
- **Profit Target**: 50% of initial credit
- **Capital Allocation**: 30-35% of account max

---

## Entry Checklist

### Pre-Entry Requirements
1. **Check IV Rank (IVR)**
   - IVR > 30: Proceed normally
   - IVR > 40: High IV - consider shorter DTE (30 days)
   - IVR < 30: Low IV - consider longer DTE (60 days) or skip

2. **Market Conditions**
   - No major events in next 48 hours
   - VIX ideally > 20 (higher premiums)
   - SPY not at extreme technical levels

### Entry Parameters

| Parameter | Primary Setup | Alternative Setup |
|-----------|--------------|-------------------|
| **DTE** | 45 days | 30 days (high IV) / 60 days (low IV) |
| **Put Strike** | 16Δ (~84% OTM) | 30Δ (~70% OTM) for more premium |
| **Call Strike** | 16Δ (~84% OTM) | 30Δ (~70% OTM) for more premium |
| **Min Credit** | $2+ per strangle | $3+ for 30Δ setup |
| **Position Size** | Account × 35% ÷ BPR | Never exceed 50% allocation |

### Strike Selection Decision Tree
```
If Conservative/New to Strategy:
  → Use 16Δ strikes (1 standard deviation)
  → Wider breakevens, less management
  → Lower premium but higher win rate

If Experienced/Active Management:
  → Use 30Δ strikes
  → More premium collected
  → Requires more active management
  → Tighter breakevens
```

---

## Management Rules (The Football System)

### First Down (Initial Position)
**Goal**: Reach 50% profit without breaching strikes
- Monitor daily but don't overtrade
- Let theta work (expect ~2-3% daily decay)
- Target: Close at 50% of max profit

### Second Down (One Side Tested)
**Trigger**: Stock approaches one strike (within 5-10 points)
**Action**: Roll the untested side
1. Close the profitable side (usually 70%+ profit)
2. Sell new strike at current 30Δ or 16Δ
3. Collect additional premium
4. Extends breakeven on tested side

### Third Down (Continued Pressure)
**Trigger**: Stock continues toward/through original strike
**Action**: Create a straddle
1. Close untested side again (70%+ profit)
2. Roll to same strike as tested side (straddle)
3. Significantly extends breakeven
4. Look for ANY bounce to exit at 25% profit

### Fourth Down (Risk Management)
**Trigger**: Approaching new breakeven after adjustments
**Three Options**:

**Option A - Field Goal (Inverted Strangle)**
- Roll untested strike BELOW tested strike
- Creates inverted strangle
- Goal: Exit at breakeven or small profit
- Risk: Assignment if held too long

**Option B - Go For It (Hold & Hope)**
- Keep straddle position
- Wait for mean reversion
- Works if move was overdone
- Risk: Continued losses if trend continues

**Option C - Punt (Roll Out in Time)**
- Roll entire position to next month
- Collect additional premium
- Reset strikes if possible
- Adds 30-45 days to trade

---

## Exit Rules

### Standard Exits
1. **Profit Target Hit**: 50% of initial credit (GTC limit order when using OTOCO)
2. **Time Exit**: MaxDTE (21) remaining (avoid gamma risk)
3. **Whichever comes first**

### Profit Taking Mechanics
- **With OTOCO**: Automatically places GTC limit order at entry
- **Without OTOCO**: Monitor daily for 50% profit opportunity
- **Target**: Buy to close at 50% of credit received
- **Example**: Collected $3.00 credit → Exit at $1.50 debit

### Emergency Exits (Manual Intervention Only)
- Loss exceeds EscalateLossPct (2.0 = 200%) of collected premium (escalate/prepare for action)
- Loss reaches StopLossPct (2.5 = 250%) of collected premium (position must be immediately closed)
- Black swan event (market drops/rises >8% in day)
- Major unexpected news event
- Assignment risk becomes imminent

### Automated Management Policy
- **Research**: Tastytrade studies show mechanical stops reduce returns (https://www.tastytrade.com/tools/options/backtesting and https://www.tastytrade.com/tools/options/backtesting/research)
- **Philosophy**: Manage through rolling and adjustments with hard limits
- **Automated System**: Must have definitive stop conditions
- **Never**: Let losses exceed defined thresholds without action
- **Rationale**: Balance management benefits with automation requirements

---

## Automated Management Rules

### Management Sequence (Football System - Automated)

#### First Down (Initial Position)
- **Goal**: 50% profit via OTOCO exit order
- **Monitor**: Position delta, time decay, price movement
- **Action**: None - let theta work
- **Trigger Next**: Stock within 5 points of either strike

#### Second Down (Strike Challenged)
- **Trigger**: SPY within 5 points of put or call strike
- **Action**: Roll untested side closer (1st adjustment)
- **OCO Order**: Close untested side at 70% profit OR roll to new strike
- **Count**: Strike adjustments = 1
- **Trigger Next**: Original strike breached

#### Third Down (Strike Breached) 
- **Trigger**: SPY breaks through original strike price
- **Action**: Roll untested side to same strike (2nd adjustment = straddle)
- **OCO Order**: Take 25% total profit OR continue to Fourth Down
- **Count**: Strike adjustments = 2
- **Limit**: Hold straddle maximum 7 days
- **Trigger Next**: Loss exceeds 150% of credit

#### Fourth Down (Critical Decision) - AUTOMATED STOPS
- **Trigger**: Loss > 150% of credit received
- **Three Actions** (Bot selects based on conditions):

**Option A - Field Goal (Inverted Strangle)**
- Roll untested strike below tested strike (3rd adjustment)
- **Count**: Strike adjustments = 3 (FINAL ADJUSTMENT)
- **STOP**: Close at any profit OR at EscalateLossPct (200%) loss (whichever first)
- **Time Limit**: 5 days maximum

**Option B - Go For It (Hold Straddle)** 
- Keep current straddle, wait for recovery (no additional roll)
- **Count**: Strike adjustments remain at 2
- **STOP**: Close at 25% profit OR EscalateLossPct (200%) loss (whichever first)
- **Time Limit**: 3 days maximum

**Option C - Punt (Time Roll)**
- Roll entire position to next expiration (time adjustment, not strike)
- **Count**: Strike adjustments reset to 0, but trade marked as "punted"
- **STOP**: Close at 50% profit OR MaxDTE (21) OR EscalateLossPct (200%) total loss
- **LIMIT**: Maximum 1 punt per original trade
- **RESET**: New expiration cycle allows fresh First→Fourth Down sequence

### Hard Stop Conditions (Non-Negotiable)

1. **Maximum Loss Stop**: StopLossPct (250%) of credit received
2. **Time Stop**: 5 DTE remaining (assignment risk)
3. **Delta Stop**: Position delta > |1.0| (directional risk too high)
4. **Management Stop**: Completed Fourth Down without recovery
5. **Black Swan Stop**: SPY moves >8% in single day
6. **Liquidity Stop**: Bid-ask spread >$0.50 on either leg

### Automated Decision Matrix

| Condition | Days Remaining | Loss Level | Action |
|-----------|---------------|------------|---------|
| Normal | >MaxDTE (21) | <50% | Continue monitoring |
| Strike approached | >MaxDTE (21) | <100% | Roll untested side |
| Strike breached | >14 DTE | <150% | Create straddle |
| Critical loss | >7 DTE | >150% | Execute Fourth Down |
| **HARD STOP** | Any | >StopLossPct (250%) | **Close immediately** |
| **HARD STOP** | <5 DTE | Any | **Close immediately** |

### Emergency Exit Triggers (Immediate Close)
- SPY gap >5% overnight
- VIX spike >50% in single day  
- Major market circuit breaker triggered
- Broker margin call received
- System error/connectivity loss

---

## Position Sizing & Risk Management

### Capital Allocation Formula
```
Number of Contracts = (Account Size × Allocation %) ÷ BPR per Strangle

Example:
$50,000 account × 35% = $17,500 allocated
$17,500 ÷ $15,000 BPR = 1 contract
```

### Risk Guidelines
- **Maximum Allocation**: 35% of account (never exceed 50%)
- **Buying Power Usage**: Keep under 60% total
- **Per Trade Risk**: Max 5% of account per strangle
- **Diversification**: Consider adding GLD, TLT, EWZ strangles

### Realistic Risk Scenarios
- **Normal Loss**: 50-100% of credit collected
- **Bad Loss**: EscalateLossPct (200%) of credit (manageable)
- **Worst Case**: 500-700% of credit (black swan)
- **Protect Against**: Never let loss exceed StopLossPct (250%) without action

---

## IV Rank Strategy

### Entry Timing by IVR
| IVR Range | Action | DTE | Notes |
|-----------|--------|-----|-------|
| 0-20 | Skip or 60 DTE | 60 | Very low premium environment |
| 20-30 | Proceed cautiously | 60 | Below average opportunity |
| 30-50 | Standard entry | 45 | Normal conditions |
| 50-70 | Excellent entry | 45 | Above average opportunity |
| 70+ | Premium entry | 30 | High IV, shorten DTE |

### Finding IVR
- TastyWorks platform
- ThinkOrSwim

---

## Adjustment Examples

### Scenario 1: SPY Drops Toward Put
**Starting Position**: Short 410 put / 450 call at 45 DTE
1. SPY drops to 415 → Roll call from 450 to 430
2. SPY drops to 410 → Roll call to 410 (straddle)
3. SPY drops to 405 → Roll call to 400 (inverted)
4. Exit when possible at 25% profit or better

### Scenario 2: SPY Rallies Toward Call
**Starting Position**: Short 410 put / 450 call at 45 DTE
1. SPY rises to 445 → Roll put from 410 to 430
2. SPY rises to 450 → Roll put to 450 (straddle)
3. SPY rises to 455 → Roll put to 460 (inverted)
4. Exit when possible at 25% profit or better

---

## Common Mistakes to Avoid

1. **Over-allocating capital** - Stick to 35% max
2. **Holding past MaxDTE (21)** - Gamma risk increases exponentially
3. **Not taking 50% profits** - Greed kills returns
4. **Using on single stocks** - Stick to ETFs for stability
5. **Ignoring IVR** - Trade when premium is worth it
6. **Aggressive rolling** - Don't chase, defend systematically
7. **Setting stop losses** - Roll and manage instead
8. **Trading around events** - Avoid FOMC, CPI releases

---

## Daily Routine

### Morning (9:30 AM)
1. Check SPY price vs strikes
2. Calculate current P&L
3. Check days remaining
4. Review IVR for new entries

### Midday Check
1. Assess if management needed
2. Look for 50% profit targets
3. Check for 70% profit on untested sides

### End of Day
1. Final P&L calculation
2. Plan next day's potential adjustments
3. Set alerts at key levels

---

## Track Record Expectations

### Based on Backtesting (2005-2023)
- **Win Rate**: 83% for 16Δ strangles
- **Average Winner**: 50% of max profit
- **Average Loser**: 150% of credit collected
- **Expected Annual Return**: 25-30% (at 35% allocation)
- **Worst Drawdown**: -20% (March 2020)
- **Recovery Time**: 3-6 months from major losses

### Monthly Targets
- 3-4 successful trades
- 2-3% monthly return on account
- Maximum 1 losing trade per month

---

## Advanced Tips

1. **IV Crush Plays**: Enter before known IV expansion events, exit after
2. **Earnings Avoidance**: SPY has no earnings but watch for Fed meetings
3. **Technical Levels**: Avoid entering at major support/resistance
4. **Correlation Trading**: When SPY strangles tested, consider opposing positions in TLT
5. **Delta Neutral**: Keep position delta between -0.2 and +0.2
6. **Volatility Skew**: Put side usually offers more premium - this is normal

---

## Emergency Protocols

### Black Swan Event Checklist
1. Immediately assess total portfolio exposure
2. Close untested sides for whatever profit available
3. Convert tested side to spread if possible (buy protective option)
4. Consider taking the loss rather than hoping
5. Don't average down into disaster

### Assignment Prevention
- Close or roll if strike breached with <7 DTE
- Never hold through expiration if ITM
- Roll to next month if necessary
- Convert to spread as last resort

---

## Record Keeping

Track for each trade:
- Entry date and DTE
- Strikes and deltas at entry
- Initial credit collected
- IVR at entry
- All adjustments made
- Exit date and profit/loss
- Days held
- Maximum adverse excursion
- Notes on what worked/didn't work

---

## Quick Decision Tree

```
Is IVR > 30?
  No → Wait or use 60 DTE
  Yes ↓
  
Sell 45 DTE strangle at 16Δ
  ↓
Monitor daily
  ↓
At 50% profit?
  Yes → CLOSE
  No ↓
  
At MaxDTE (21)?
  Yes → CLOSE
  No ↓
  
Strike being tested?
  Yes → Roll untested side
  No → Continue monitoring
```

---

## Final Rules for Success

1. **Patience**: Wait for good IVR setups
2. **Discipline**: Take 50% profits religiously
3. **Defense**: Manage early and often
4. **Size**: Never over-allocate
5. **Time**: Exit by MaxDTE (21) always
6. **Simplicity**: Stick to SPY until experienced
7. **Records**: Track everything for improvement
8. **Learning**: Each trade teaches something
9. **Emotions**: Stay mechanical, not emotional
10. **Consistency**: Small wins compound over time

---

*Remember: This strategy has high win rate but losses can be larger than wins. Success comes from discipline, proper sizing, and consistent management. Start small with 1 contract until comfortable with the mechanics.*