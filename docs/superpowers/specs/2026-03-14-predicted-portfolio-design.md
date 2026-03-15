# Predicted Portfolio Design

## Status

Draft

## Problem

Strategies that trade infrequently (e.g., monthly like ADM) leave users wondering
what the strategy would buy on the next trade date. The legacy system provided a
predicted portfolio showing what trades would occur using current prices. The new
engine has no equivalent.

## Decision

Add `Engine.PredictedPortfolio(ctx) (portfolio.Portfolio, error)` that runs the
strategy's Compute against a shadow copy of the current portfolio, using the next
scheduled trade date and forward-filled data. The strategy is completely unaware
it is a prediction run.

## Prerequisites

- DataFrame has a `Frequency` field (implemented).

## Method Signature

```go
func (eng *Engine) PredictedPortfolio(ctx context.Context) (portfolio.Portfolio, error)
```

Callable anytime after the engine has been initialized (post-Backtest or during
RunLive). Returns a Portfolio reflecting what the strategy would do on the next
scheduled trade date given currently available data.

## Algorithm

1. **Determine the next trade date.** Call `eng.schedule.Next(eng.currentDate)` to
   get the next scheduled trade date.

2. **Clone the current account.** Create a deep copy of the current Account so
   the prediction run does not mutate the real portfolio. The clone must include
   holdings, cash, prices, perfData, and metadata. It does NOT need tax lots,
   registered metrics, or the full transaction log (though keeping them is
   harmless).

3. **Set up the shadow environment.** Create a fresh `SimulatedBroker`. Wire it
   to the clone via `clone.SetBroker(shadowBroker)`. The engine's own broker
   field is not touched -- the shadow broker is only referenced by the cloned
   account. Save and restore `eng.currentDate` after the prediction run.

4. **Enable forward-fill mode.** Set a flag on the engine indicating that data
   fetched via `Fetch`/`FetchAt` should be forward-filled to the predicted
   date. When this flag is set, after a normal fetch:
   - If the DataFrame's last timestamp is before `eng.currentDate`, generate
     fill timestamps from the day after the last available date through
     `eng.currentDate`, spaced according to `df.Frequency()`.
   - For each fill timestamp, copy the last available row's values.
   - Append these rows to the DataFrame (via `AppendRow` or by constructing
     a new DataFrame with the extended data).

5. **Set the broker's price provider.** Call
   `shadowBroker.SetPriceProvider(eng, predictedDate)` so the simulated broker
   can look up prices for order fills.

6. **Run Compute.** Set `eng.currentDate = predictedDate` and call
   `eng.strategy.Compute(ctx, eng, clone)`. The strategy calls
   `universe.Window`/`universe.At` as normal, which flow through to
   `eng.Fetch`/`eng.FetchAt`, which forward-fill as described above.

7. **Restore state.** Reset `eng.currentDate` to its original value. Clear the
   forward-fill flag.

8. **Return the clone.** The clone now reflects the trades the strategy would
   have made. The caller can inspect holdings, transactions, annotations, and
   justifications.

## Forward-Fill Detail

The forward-fill logic lives in the engine's fetch path, not in the data
providers or DataFrame. It is activated only during a `PredictedPortfolio` call
via a boolean flag on the engine (`eng.predicting bool`).

The forward-fill is applied in `Fetch`/`FetchAt` after `fetchRange` returns
the assembled DataFrame (after the `Between` trim), not inside `fetchRange`
itself. This ensures the forward-filled rows are not clipped by the time
range filter.

After a normal `Fetch` or `FetchAt` returns a DataFrame:

```go
if eng.predicting && df.End().Before(eng.currentDate) {
    df = eng.forwardFillTo(df, eng.currentDate)
}
```

The `forwardFillTo` method:

1. Determines the fill frequency from `df.Frequency()`. If the frequency is
   `Tick`, return an error -- tick data has no regular interval and cannot be
   meaningfully forward-filled.
2. Generates timestamps from `df.End() + 1 frequency step` through `targetDate`,
   using the frequency to determine spacing (Daily = calendar days,
   Weekly = 7 days, Monthly = 1 month, Quarterly = 3 months, Yearly = 1 year).
3. For each generated timestamp, copies the values from the last row of the
   DataFrame.
4. Appends each row to the DataFrame via `df.AppendRow(timestamp, values)`.
5. Returns the extended DataFrame.

Note: The engine's `fetchRange` method currently hard-codes `Frequency: data.Daily`
in its `DataRequest`. All engine-fetched DataFrames will be Daily. The forward-fill
logic supports other frequencies for future use, but in practice Daily is the
common case.

## Account Cloning

Add a `Clone() *Account` method to Account that creates a deep copy suitable
for prediction:

```go
func (acct *Account) Clone() *Account
```

This copies: cash, holdings (deep copy of map), prices (shallow -- immutable
between steps), perfData (deep copy -- `AppendRow` mutates the slab),
benchmark, riskFree, metadata (deep copy of map), annotations (copy of slice).
Tax lots, registered metrics, and the transaction log are copied shallowly
(the prediction run may append to the transaction log, but that's fine since
it's the clone's copy).

Note: `PredictedPortfolio` only calls `Compute`, not the full step loop
(no `UpdatePrices` is called on the clone). However, perfData is deep-copied
as a safety measure in case future callers exercise that path.

## Engine State Management

The engine must save and restore its state around the prediction run:

```go
savedDate := eng.currentDate
eng.predicting = true
eng.currentDate = predictedDate
defer func() {
    eng.currentDate = savedDate
    eng.predicting = false
}()
```

The data cache (`eng.cache`) is shared between the real run and the prediction.
This is safe because the prediction only reads from it (forward-fill extends
the result after fetching, it does not modify the cache).

## Universe Membership on Future Dates

During a prediction run, universe methods like `ratedUniverse.Assets(predictedDate)`
query the provider for a future date. The provider should return the most recent
known membership -- this is the best available estimate. No special handling is
needed in the engine; the behavior depends on the provider implementation.
Static and index universes are unaffected since they resolve membership from
historical data.

## Error Handling

- If the engine has no schedule set, return an error.
- If Compute returns an error, return it wrapped with context.
- If data fetch or forward-fill fails, the error propagates through Compute.

## Files Changed

### Source

- `engine/engine.go` -- add `predicting bool` field, add `PredictedPortfolio`
  method, add `forwardFillTo` helper
- `engine/engine.go` -- modify `Fetch`/`FetchAt` to call forward-fill when
  `eng.predicting` is true
- `portfolio/account.go` -- add `Clone() *Account` method

### Tests

- `engine/predicted_portfolio_test.go` -- test PredictedPortfolio with a
  simple strategy, verify the returned portfolio has expected trades
- `engine/forward_fill_test.go` -- test forwardFillTo with various frequencies
- `portfolio/account_test.go` -- test Account.Clone preserves state
