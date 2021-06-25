# Protective Asset Allocation

The Protective Asset Allocation strategy was developed by [Wouter Keller](https://papers.ssrn.com/sol3/cf_dev/AbsByAuth.cfm?per_id=1935527) and [JW Keuning](https://papers.ssrn.com/sol3/cf_dev/AbsByAuth.cfm?per_id=2530815). It’s based off their paper [Protective Asset Allocation (PAA): A Simple Momentum-Based Alternative for Term Deposits](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=2759734). The strategy uses dual momentum to determine what assets to hold but has a very aggressive portfolio protection mechanism in case of a market crash. Their goal was to make an “appealing alternative for a 1-year term deposit.”

## Rules

1. On the last trading day of each month, calculate a momentum score for 12 asset classes.
   - MOM = p0/SMA(12) – 1
   - p0 = price on last trading day of the month
   - SMA(12) = 12-month simple moving average
2. Determine the percent of the portfolio that is allocated to the crash protection asset. This is done by determining how many “good” and “bad” assets are in our list of assets. n=number of assets with a MOM > 0.
   - If n<=6, the entire portfolio is invested in the crash protection asset.
   - If n>=7
     - the percent of the portfolio invested in the crash protection asset = (12- n ) / 6
     - the remaining portfolio is divided equally between the top 6 assets with the highest positive MOM scores.
3. Hold positions until the last trading day of the next month. Rebalance the entire portfolio even if there is not a change in positions.

## Assets Typically Held

| Ticker | Name                                                | Sector                              |
| ------ | --------------------------------------------------- | ----------------------------------- |
| IEF    | iShares 7-10 Year Treasury Bond ETF                 | Bond, U.S., Intermediate-Term       |
| IWM    | iShares Russell 2000 ETF                            | Equity, U.S., Small Cap             |
| QQQ    | Invesco QQQ                                         | Equity, U.S., Large Cap             |
| VNQ    | Vanguard Real Estate Index Fund ETF                 | Real Estate, U.S.                   |
| SPY    | SPDR S&P 500 ETF                                    | Equity, U.S., Large Cap             |
| VGK    | Vanguard FTSE Europe ETF                            | Equity, Europe, Large Cap           |
| EEM    | iShares MSCI Emerging Markets ETF                   | Equity, Emerging Markets, Large Cap |
| EWJ    | iShares MSCI Japan ETF                              | Equity, Japan, Large Cap            |
| DBC    | Invesco DB Commodity Index Tracking Fund            | Commodity, Diversified              |
| TLT    | iShares 20+ Year Treasury Bond ETF                  | Bond, U.S., Long-Term               |
| GLD    | SPDR Gold Trust                                     | Commodity, Gold                     |
| HYG    | iShares iBoxx $ High Yield Corporate Bond ETF       | Bond, U.S., Intermediate-Term       |
| LQD    | iShares iBoxx $ Investment Grade Corporate Bond ETF | Bond, U.S., All-Term                |
