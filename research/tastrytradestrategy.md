# Tastytrade's SPY Short Strangle Strategy

A short strangle on SPY involves selling an out-of-the-money (OTM) put and call, profiting if SPY stays between the strikes as time decay and volatility contraction work in your favor.

[Tastytrade](https://tastylive.com) emphasizes using defined probabilities and risk management. Core principles include targeting about 1-standard-deviation strikes (~16Δ, ~84% OTM) for ~68–70% theoretical probability of profit, with higher win rates (≈80–83%) realized in backtests.

They advise trading in reasonably elevated volatility (e.g. VIX>20 or IV Rank in mid/high range) – strangles are profitable in all regimes, but P/L is much higher when vol is high.

## Entry Criteria

**Implied Volatility (IV):** Favor moderate-to-high IV. Studies show short strangles work in all IV environments, but returns are substantially greater when IV/VIX is elevated. (In practice Tasty suggests avoiding very low-IV regimes; e.g. treat IVR<30 as "low" and IVR>40 as "high" for DTE selection below).

**Days to Expiration (DTE):** Target ~45 DTE to balance theta vs. gamma. Adjust based on IV: use 60 DTE strangles in very low-IV (IVR<30) and 30 DTE strangles in very high-IV (IVR>40). (This IV-based DTE "modification" keeps win-rate and P/L roughly constant).

**Strike Selection:** Aim for ~1σ away in each direction, roughly 16Δ on both put and call (about 84% OTM). This yields a ~68% chance SPY stays within the strikes (before accounting for credit) – about 70–75% actual POP after credit. (Greater width → lower max return but higher POP; Tasty prefers the credit/P.O.P balance of 1σ strangles.)

**Minimum Premium:** Tasty hasn't published a fixed "min credit", but implicitly targets enough premium so that rolling and exit targets are meaningful. For example, on a 00 SPY one might look for multi-dollar total credit (00+ per strangle) – sufficient to justify the trade and allow 50% profit exits.

**Other Factors:** No earnings/events risk in SPY; trade only on regular cycles. Use IV Rank (or percentile) to time entries, and avoid extremely low-vol windows if possible.

The table below summarizes these setup parameters:

| Parameter         | Tastytrade Guideline                                                                 | Source                      |
|-------------------|--------------------------------------------------------------------------------------|-----------------------------|
| Underlying        | SPY (S&P 500 ETF)                                                                   | –                          |
| Underlying Outlook| Neutral/bidirectional (sell premium)                                                | Tastytrade focus for short strangle (neutral) |
| Implied Volatility| Prefer moderate-to-high (e.g. VIX>20). Avoid lowest IV ranks.                       | Market Measures study      |
| DTE (Expiration)  | ~45 days; extend to 60 DTE if IVR <30; shorten to 30 DTE if IVR >40                  | Stated guidelines and research |
| Strikes           | ~1σ away (~16Δ call and 16Δ put, ~84% OTM)                                          | Tastylive strategy notes   |
| Target Credit/POP | ~70% theoretical POP (68% chance in-range) (credit boosts to ~70–75%)               | –                          |
| Position Sizing   | Moderate – typically allocate 20–35% of capital to strategy; <50% max. Compute contracts by: Account × alloc% ÷ BPR per strangle | Market Measures allocation study |
| Diversification   | Consider adding other assets (e.g. GLD, TLT) to halve P/L swings                     | Diversification study      |

## Trade Management

**Profit Taking:** Primary profit target is 50% of max credit. In practice, Tasty recommends closing ~half the max-profit when the strangle's mark drops to 50% of the initial credit. This typically occurs well before expiration. (Historical median P/L for a 45-DTE 16Δ SPY strangle is ~50% of credit by 21 DTE, so Tasty often takes profits at 50% or around 21 DTE.)

**Duration-Based Exit:** Consistent with the above, Tasty now advises managing out at ~21 days before expiration to reduce gamma risk. Studies show 21-DTE exits (or 50% profit) dramatically lower P/L volatility with only modest reduction in returns. In summary: exit when (a) you hit 50% credit, or (b) ~21 DTE remain, whichever comes first.

**Adjustments (Rolling):** If SPY moves and one short option goes in-the-money ("tested side"), Tasty's go-to adjustment is to roll the untested (opposite) side closer to the stock to collect extra credit and rebalance delta. This "reduces risk first" by adding premium on the side still out-of-the-money. Repeated rolls toward an inverted strangle (or eventually a straddle) are possible as needed. The aim is to stay delta-neutral (keeping net delta within ~±0.2), scaling back delta by 25–50% each roll. Tasty emphasizes rolling the untested side as it increases credit and cuts risk, rather than immediately buying back the losing side.

**Stop-Losses:** Tasty's research finds no benefit to strict stop-loss rules. Implementing fixed loss-exits (e.g. closing at 25–75% of credit lost) does reduce volatility, but yields nearly identical cumulative returns to simply holding/managed exit. In other words, stop-losses sacrifice return without substantially reducing drawdowns. Thus, Tasty instead relies on the above profit-taking and rolling mechanics rather than an absolute stop-loss threshold. (Practitioners sometimes manually close if loss exceeds ~100–200% of credit, but this is not prescribed in official content.)

**Risk Reducing:** If a strangle becomes deep in-the-money and unwound, consider rolling to the next expiration (resetting to ~45 DTE again) or converting to an iron condor by buying wings. Note, however, that buying protective wings is expensive: one study showed purchasing 10Δ wings on a 20Δ SPY strangle cut total P/L by ~42%. (This means the trade-off for capping risk is a sharply lower expected return.)

## Risk Management

**Capital Allocation:** Tasty recommends a moderate allocation to any short-vol strategy. In a multi-year SPY strangle study, allocating about 30–35% of account capital to the strategy (with strict 50% profit-taking) produced the best risk-adjusted results. (Too high allocation (e.g. 50%) led to deep drawdowns, while very low (<15%) underutilized opportunity.) In practice, keep overall buying power used to <60%, with many citing ~30–35% as reasonable. (For example, on a M account with a 40 DTE SPY strangle BPR ~00k, 35% alloc ⇒ 50k BPR ⇒ ~3–4 contracts.)

**Position Sizing:** Size trades so that no single strangle risks an outsized share of capital. Using portfolio margin, a typical SPY 16Δ strangle (~around 50 DTE) might use ~15–20% of SPY's price in buying power. In cash-secured accounts, reduce size accordingly. The formula given was: Contracts = (Account Size × Allocation %) ÷ (BPR per strangle).

**Diversification:** A purely SPY short strangle book can see large swings. Tasty's research illustrates that a diversified strangle portfolio (e.g. across SPY, GLD, TLT, EUR) cuts P/L volatility roughly in half versus SPY-only. Thus, even if focused on SPY, be mindful of concentration risk.

**Hedging and Defined-Risk:** In rare cases, traders may choose to hedge tail risk (e.g. convert to iron condor) but must accept reduced returns. Tasty notes that defining risk on outlier moves is costly, so their primary stance is to remain unhedged and manage via rolling.

## Exit Rules

Tasty's exit guidelines are tightly linked to its profit-taking and duration rules:

**Profit Target:** Close the position when it has reached ~50% of its initial credit. In practice, this often means taking profits as soon as a strangle's mark is half of what was received.

**Time Exit:** If 50% profit isn't hit, close by ~21 DTE. (Studies show that, by 21 DTE, the median P/L of a 45-DTE 16Δ SPY strangle is ~50% of credit, so this acts as a firm deadline.)

**Duration:** Thus a strangle is rarely held all the way to expiration. Exiting or rolling by 21 DTE helps avoid the steep gamma swings of the final weeks.

**Max Loss:** There is no simple "stop-loss" percentage recommended by Tasty. Instead, traders manage increasing loss by rolling as above. However, if a strangle becomes extremely unprofitable (e.g. one side deeply ITM near expiration), traders may choose to close or re-establish in a fresh cycle. Tasty's content suggests letting winners run (to the 50% target) but not symmetrically enforcing a fixed multiple loss limit, given the poor trade-off of stops.

## Backtesting & Performance

Tastytrade has published various backtests on SPY short strangles:

**Return & Win-Rate:** One study (2005–2017) found that selling 45-DTE 16Δ strangles each cycle yielded roughly 0.07% daily return on capital (≈25%/yr). After accounting for a typical 30–35% allocation, this equates to ~9–10% annual return (before fees/taxes) – roughly comparable to long SPY. Historical win rates were high; one analysis showed ~83% of 16Δ SPY strangles finished profitable, far above the 68% initial POP.

**Profit Levels:** The median 45-DTE 16Δ strangle achieves ~50% of max-profit by 21 DTE. This supports the 50% profit exit strategy.

**Volatility Effects:** Consistently, higher IV periods generated far more credit. For example, SPY strangles sold when VIX>20 had ~4–5× the average profit (in dollars) than in low-vol times. In backtests segmented by IV Rank, high-IV trades performed markedly better, confirming that selling in elevated volatility is advantageous.

**Diversification:** A 2008–present study showed that adding short strangles in other uncorrelated markets (e.g. gold, bonds) cut portfolio swings ~50% without hurting profitability.

**Capital vs. Buy-Hold:** In one 11-year test, short strangles (with management) roughly matched SPY buy-and-hold returns, but with much lower volatility when managed properly. Over-allocating capital (e.g. 50%) led to worse drawdowns.

(Note: These results are from Tasty's research segments and slides. Actual results depend on exact rules and market conditions.)

## Evolution of the Approach

Tastytrade's guidance has refined over time. Early on, the standard was simply "sell 45 DTE 1σ strangles and hold to expiration." Over the years, research added nuance:

**Profit Taking:** The 50%-profit close was adopted early (cited around 2015) and persists.

**Duration Management:** Originally strangles were often held to expiry; newer research (2022–2023) strongly favors exiting/rolling by ~21 DTE to tame late-gamma risk.

**IV-based DTE:** Since 2018, Tasty suggests dynamically adjusting DTE by IV Rank, lengthening in calm markets and shortening in wild markets.

**Strike/Delta:** The focus on ~16Δ (~1σ) has been consistent (Tasty long emphasized 84% OTM). They noted a very-high-POP approach (e.g. 30Δ strangles) historically win more often, but Tasty's books emphasize 16Δ for balance of premium vs risk.

**Risk Measures:** Tasty's research increasingly quantifies risk–reward (volatility, drawdown). For example, recent work shows diversification benefits and quantifies the cost of hedging, informing more disciplined sizing.

## Summary of Tastytrade SPY Short Strangle Parameters

| Parameter       | Tastytrade Guidance                                                                 |
|-----------------|-------------------------------------------------------------------------------------|
| Entry IV        | Prefer mid–high IV (sell premium in higher-vol regimes yields 4–5× profits vs low IV). |
| Entry DTE       | ~45 DTE; extend to 60 DTE if IVR<30, shorten to 30 DTE if IVR>40.                    |
| Strikes         | ~1σ away (~16Δ OTM each side, ~84% OTM); yields ≈70% POP after credit.             |
| Max Credit Goal | Aim for a meaningful credit (e.g. multi-$ per spread on SPY) so 50% profit exit is worthwhile. |
| Profit Target   | Close at 50% of initial credit (typically by ~21 DTE).                              |
| Manage At       | 21 DTE or 50% profit (whichever first); avoid holding to last few days.            |
| Adjustment      | Roll the untested side closer when challenged; repeatedly roll shorter strikes first (adding credit, reducing delta). |
| Stop-Loss       | No fixed stop recommended; Tasty found stop-loss rules yield no net gain.          |
| Allocation      | Moderate – roughly 30–35% of account (<=50%) to strangles; never use >60% BPR.      |
| Hedging         | Not default. Buying protective wings (iron condor) cuts profit by ~42%; generally sell naked and manage. |

Each recommendation above is drawn from Tastytrade's official research and content.