# Compute Error Propagation Design

## Status

Draft

## Problem

The `Strategy.Compute()` method returns void. When a strategy encounters an error
during computation (failed data fetch, invalid state, etc.), it can only log and
return early. The engine has no way to know a computation failed other than
observing that no rebalance happened. In backtesting, this silently produces
corrupted results.

## Decision

Change `Compute()` to return `error`. The engine halts the backtest on error.
Live trading logs and continues.

## Interface Change

```go
type Strategy interface {
    Name() string
    Setup(eng *Engine)
    Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio) error
}
```

The parameter name `portfolio` is intentional despite shadowing the package name;
Go permits this and the domain term is the clearest choice.

## Engine Behavior

### Backtest

Halt and return the error wrapped with strategy name and date context:

```go
if err := eng.strategy.Compute(stepCtx, eng, acct); err != nil {
    return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
        eng.strategy.Name(), date, err)
}
```

### RunLive

Log the error and continue to the next scheduled date, consistent with the
existing log-and-continue pattern for fetch errors:

```go
if err := eng.strategy.Compute(stepCtx, eng, acct); err != nil {
    zerolog.Ctx(stepCtx).Error().Err(err).Msg("strategy compute failed")
    continue
}
```

## Files Changed

### Source

- `engine/strategy.go` -- add `error` return to `Compute` in the `Strategy` interface
- `engine/backtest.go` -- check error from `Compute`, wrap and return
- `engine/live.go` -- check error from `Compute`, log and continue
- `examples/momentum-rotation/main.go` -- add `error` return, `return nil`

### Tests

- `engine/fetch_test.go` -- update 5 test strategy `Compute` signatures
- `engine/example_test.go` -- update 2 test strategy `Compute` signatures
- `engine/backtest_test.go` -- update 1 test strategy `Compute` signature
- `cli/cli_test.go` -- update 1 test strategy `Compute` signature

### Documentation

- `engine/doc.go` -- update code samples
- `README.md` -- update code sample
- `docs/overview.md` -- update code samples
- `docs/scheduling.md` -- update code sample
- `docs/universes.md` -- update code sample
- `docs/portfolio.md` -- update code samples
- `docs/superpowers/specs/2026-03-14-rated-universe-design.md` -- update code sample
