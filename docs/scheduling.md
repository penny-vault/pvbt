# Scheduling

Scheduling determines when the engine calls a strategy's `Compute` method.

## Tradecron

Schedules are defined using the `tradecron` package, which extends standard cron syntax with awareness of trading calendars. It knows about market holidays, trading hours, and special sessions.

The schedule is declared in the strategy's `Describe()` method using a tradecron expression:

```go
func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        Schedule: "@close @monthend",
    }
}
```

Expressions use standard cron syntax extended with market-aware directives.

### Common schedules

| Expression | When Compute runs |
|------------|-------------------|
| `@daily` | Every trading day at market open |
| `@monthend` | Last trading day of each month |
| `@close @monthend` | Last trading day of each month at market close |
| `@monthbegin` | First trading day of each month |
| `@weekbegin` | First trading day of each week |
| `@weekend` | Last trading day of each week |
| `@quarterbegin` | First trading day of each quarter |
| `@quarterend` | Last trading day of each quarter |
| `@close * * *` | Every trading day at market close |
| `@open * * *` | Every trading day at market open |
| `0 10 * * *` | Every trading day at 10:00 AM ET |
| `*/5 * * * *` | Every 5 minutes during trading hours |

Supported directives: `@daily`, `@open`, `@close`, `@weekbegin`, `@weekend`, `@monthbegin`, `@monthend`, `@quarterbegin`, `@quarterend`. These can be combined with standard cron fields for minute, hour, day-of-month, month, and day-of-week.

The `tradecron.RegularHours` constraint ensures the schedule never fires on weekends, holidays, or outside market hours. If a scheduled time falls on a holiday, it advances to the next valid trading day.

When constructing a `tradecron` schedule directly, two other sessions are available: `tradecron.ExtendedHours` widens the window to pre/post-market, and `tradecron.AllHours` drops the time-of-day constraint entirely so the schedule fires at its scheduled time on every trading day, early-close days included.

The schedule is required; the engine returns an error if none is set. All times are Eastern.

## Intra-day firings

A schedule may emit more than one timestamp per trading day. Cron expressions like `0 10,14 * * MON-FRI` (10:00 and 14:00 Eastern, weekdays) cause `Compute` to fire twice per trading day; the engine advances its simulation time to each firing in sequence and calls `Compute` once per firing.

Inside `Compute`:

- `engine.CurrentDate()` returns the trading-day boundary (used by daily housekeeping).
- `engine.Now()` returns the precise firing instant (10:00 ET or 14:00 ET in the example above).

Strategies that pull intraday data via `portfolio.MinuteBars(N)` or `portfolio.DailyAtTime(...)` anchor the lookback at `engine.Now()`, so each firing's window ends at exactly its firing moment.

Order fills during intra-day firings land at the next 1-minute bar's close, the same next-bar semantics used for daily strategies (next bar = next minute, not next day). Daily portfolio valuation, equity recording, and performance metrics remain anchored to once-per-day snapshots at the end-of-day boundary.

See [Data: Intraday 1-minute bars](data.md#intraday-1-minute-bars) for the data-fetch API and ClickHouse configuration.

## Example

```go
func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
    mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
    mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    riskOffDF, _ := s.RiskOff.At(ctx, data.MetricClose)
    portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)
    plan, _ := portfolio.EqualWeight(momentum)
    batch.RebalanceTo(ctx, plan...)
    return nil
}
```

This code operates on signals rather than individual assets or time steps. It expresses the strategy's logic in terms of DataFrames and portfolio operations. The engine handles the shape.
