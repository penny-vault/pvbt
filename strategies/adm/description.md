# Accelerating Dual Momentum

The Accelerating Dual Momentum strategy was developed by the site [EngineeredPortfolio.com](https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/). The portfolio uses a Dual Momentum approach to compare absolute and relative momentum. The backtest time period is shorter than traditional dual momentum strategies. This makes the portfolio more responsive but also trades more often.

## Rules

This strategy allocates 100% of of the portfolio to one asset each month. It uses the average of a 1-, 3-, and 6-month momentum score to choose between investing in the S&P 500, international small-cap stocks, or long-term bonds.

1. On the last trading day of each month, calculate the “momentum score” for the S&P 500 (SPY) and the international small cap equities (SCZ). The momentum score is the average of the 1, 3, and 6-month total return for each asset.
2. If the momentum score of SCZ > SPY and is greater than 0, invest in SCZ.
3. If the momentum score of SPY > SCZ and is greater than 0, invest in SPY.
4. If neither momentum score is greater than 0, invest in long-term US Treasureies (TLT)

## Assets Typically Held

| Ticker | Name                            | Sector                           |
| ------ | ------------------------------- | -------------------------------- |
| SPY    | SPDR S&P 500 ETF                | Equity, U.S., Large Cap          |
| SCZ    | iShares MSCI EAFE Small-Cap ETF | Equity, International, Small Cap |
| TLT    | iShares 20+ Year Treasury Bond  | Bond, U.S., Long-Term            |
