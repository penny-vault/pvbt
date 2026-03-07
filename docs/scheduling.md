# Scheduling

Scheduling determines when the engine calls a strategy's `Compute` method.

## Tradecron

Schedules are defined using the `tradecron` package, which extends standard cron syntax with awareness of trading calendars. It knows about market holidays, trading hours, and special sessions.

```go
e.Schedule(tradecron.New("@monthend 0 16 * *", tradecron.RegularHours))
```

The first argument is a cron expression. The second specifies which trading sessions to consider.

### Common schedules

```go
// Last trading day of each month at 4:00 PM
tradecron.New("@monthend 0 16 * *", tradecron.RegularHours)

// Every trading day at market close
tradecron.New("@daily 0 16 * *", tradecron.RegularHours)

// Every trading day at market open
tradecron.New("@daily 30 9 * *", tradecron.RegularHours)

// First trading day of each quarter
tradecron.New("@quarterstart 0 16 * *", tradecron.RegularHours)

// Every hour during regular trading hours
tradecron.New("@hourly * * * *", tradecron.RegularHours)
```

The `tradecron.RegularHours` constraint ensures the schedule never fires on weekends, holidays, or outside market hours. If a scheduled time falls on a holiday, it advances to the next valid trading day.

### Changing the schedule

The schedule is set during Setup, but a strategy can modify it during computation through the engine reference. This is uncommon but useful for strategies that change behavior based on market conditions — for example, switching from monthly to daily computation during high-volatility periods.

## Example

```go
func (s *ADM) Compute(ctx context.Context, p portfolio.Portfolio) {
    mom1 := signal.Momentum(df, 1)
    mom3 := signal.Momentum(df, 3)
    mom6 := signal.Momentum(df, 6)

    momentum := mom1.Add(mom3).Add(mom6).DivScalar(3)
    symbols := momentum.Select(portfolio.MaxAboveZero(s.riskOff))
    p.RebalanceTo(portfolio.EqualWeight(symbols)...)
}
```

This code operates on signals rather than individual assets or time steps. It expresses the strategy's logic in terms of DataFrames and portfolio operations. The engine handles the shape.
