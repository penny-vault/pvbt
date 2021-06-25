# Defensive Asset Allocation

The **Defensive Asset Allocation** strategy was developed by [Wouter Keller](https://papers.ssrn.com/sol3/cf_dev/AbsByAuth.cfm?per_id=1935527) and [JW Keuning](https://papers.ssrn.com/sol3/cf_dev/AbsByAuth.cfm?per_id=2530815). It is based off their paper: [Breadth Momentum and the Canary Universe: Defensive Asset Allocation (DAA)](https://papers.ssrn.com/sol3/papers.cfm?abstract_id=3212862). The strategy uses a momentum approach that is heavily weighted towards more recent months. It also uses a system the authors call “breadth momentum” to determine how much to shift to defensive positions.

## Rules

The strategy has three main groups of ETFS it allocats:

- **Risky**: SPY, IWM, QQQ, VGK, EWJ, EEM, VNQ, DBC,GLD, TLT, HYG, LQD
- **Protective**: SHY, IEF, LQD
- **Canary**: EEM, AGG

1. On the last trading day of the month, calculate a momentum score for all the assets above
   - Momentum Score = (12*(p0/p1)) + (4*(p0/p3)) + (2*(p0/p6)) + (p0/p12) – 19
   - p0 = today’s price, p1 = price at close of last month, etc…
2. The number of “canary” assets with a positive momentum score will determine our portfolio allocation.
   - n = # of canary assets with a negative momentum score
   - If n=2
      - 100% of the portfolio is invested the protective asset with the highest momentum score
   - if n=1
      - 50% of the portfolio is invested in the protective asset with the highest momentum score
      - 50% of the portfolio is invested equally in the 6 “risky” assets with the highest momentum score.
   - if n=0,
      - 100% of the portfolio is invested equally in the 6 “risky” assets with the highest momentum score.
3. Hold all positions until the close of the following month.

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
