# Scheduling

Scheduling determines when the engine calls a strategy's `Compute` method.

## Tradecron

Schedules are defined using the `tradecron` package, which extends standard cron syntax with awareness of trading calendars. It knows about market holidays, trading hours, and special sessions.

```go
tc, err := tradecron.New("@close @monthend", tradecron.RegularHours)
e.Schedule(tc)
```

`New` returns a `(*TradeCron, error)`. The first argument is a cron expression with optional market-aware directives. The second specifies which trading sessions to consider.

### Common schedules

```go
// Last trading day of each month at market close
tradecron.New("@monthend", tradecron.RegularHours)

// Last trading day of each month at a specific time
tradecron.New("@close @monthend", tradecron.RegularHours)

// Every trading day at market close
tradecron.New("@close * * *", tradecron.RegularHours)

// Every trading day at market open
tradecron.New("@open * * *", tradecron.RegularHours)

// First trading day of each month
tradecron.New("@monthbegin", tradecron.RegularHours)

// First trading day of each week
tradecron.New("@weekbegin", tradecron.RegularHours)

// Last trading day of each week
tradecron.New("@weekend", tradecron.RegularHours)

// Every 5 minutes during trading hours
tradecron.New("*/5 * * * *", tradecron.RegularHours)
```

Supported directives: `@open`, `@close`, `@weekbegin`, `@weekend`, `@monthbegin`, `@monthend`. These can be combined with standard cron fields for minute, hour, day-of-month, month, and day-of-week.

The `tradecron.RegularHours` constraint ensures the schedule never fires on weekends, holidays, or outside market hours. If a scheduled time falls on a holiday, it advances to the next valid trading day.

### Changing the schedule

The schedule is set during Setup, but a strategy can modify it during computation through the engine reference. This is uncommon but useful for strategies that change behavior based on market conditions — for example, switching from monthly to daily computation during high-volatility periods.

## Example

```go
func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
    mom1 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(1))
    mom3 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(3))
    mom6 := signal.Momentum(ctx, s.RiskOn, portfolio.Months(6))

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    symbols := portfolio.MaxAboveZero(s.RiskOff.Assets(e.CurrentDate())).Select(momentum)
    p.RebalanceTo(ctx, portfolio.EqualWeight(symbols)...)
}
```

This code operates on signals rather than individual assets or time steps. It expresses the strategy's logic in terms of DataFrames and portfolio operations. The engine handles the shape.
