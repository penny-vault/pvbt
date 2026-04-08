# CLAUDE.md

## Project

pvbt is a Go backtesting and live trading framework for quantitative strategies. Strategies declare parameters, schedules, and a Compute function; the engine handles data routing, scheduling, order execution, and performance reporting. The same code runs in backtest and live modes.

## Build, Test, Lint

```bash
make build     # go build -o pvbt .
make test      # ginkgo run -race ./...
make lint      # go vet, go fmt, golangci-lint run
```

Tests use Ginkgo/Gomega (BDD style). Every package has a `*_suite_test.go` that wires up the suite. Redirect zerolog output to `GinkgoWriter` so logs only appear on failure.

## Public API

Changes to the public API are breaking and must appear in the changelog. Everything else is internal.

**CLI surface** -- commands (`backtest`, `live`, `snapshot`, `study`, `describe`, `config`) and their flags (`--preset`, `--benchmark`, `--config`, `--risk-profile`, `--tax`, etc.)

**Strategy author surface:**
- `Strategy` interface (`Name`, `Setup`, `Compute`) and `Descriptor` interface (`Describe`)
- `Portfolio` (read-only query methods: `Holdings`, `Prices`, `ProjectedWeights`, margin methods, etc.)
- `Batch` (orders, annotations)
- `DataFrame` (construction via `NewDataFrame`, querying, `Correlation`, `Covariance`, `Std`, etc.)
- Broker options (`WithFractionalShares`, etc.)
- Middleware configuration (`risk.Conservative`, `risk.Moderate`, `tax.TaxEfficient`, etc.) and TOML config file (`pvbt.toml`)
- Weighting functions, universe declarations (`USTradable`, `SP500`, `Nasdaq100`, `NewStatic`), parameter/preset definitions

## Code Conventions

- **Error handling:** wrap errors with context via `fmt.Errorf("context: %w", err)`. Never silently swallow or degrade on errors. Never use `//nolint` directives; fix the underlying issue.
- **Variable names:** minimum 2 characters (enforced by varnamelen linter). Exceptions: `db`, `df`, `ii`, `sb`.
- **Logging:** use zerolog. The zerologlint linter enforces correct usage.

## Changelog

Follow [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. Entries must be:
- Complete sentences with a subject, in active voice
- Written from the user's perspective, not implementation details
- Related items combined into a single bullet
- "Changed" entries describe what is different relative to the previous release

Internal changes (report package internals, metric computation plumbing) only appear if they produce a user-visible effect, described in terms of that effect (e.g., "backtests run 9x faster" not "redesigned DataFrame internals").
