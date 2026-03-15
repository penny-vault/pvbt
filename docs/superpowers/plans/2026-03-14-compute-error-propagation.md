# Compute Error Propagation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Change `Strategy.Compute()` to return `error` so the engine can detect and handle strategy failures.

**Architecture:** The `Strategy` interface gains an `error` return on `Compute()`. The backtest loop wraps and returns the error, halting the run. The live trading loop logs the error and continues. All existing implementations return `nil`.

**Tech Stack:** Go, zerolog

**Spec:** `docs/superpowers/specs/2026-03-14-compute-error-propagation-design.md`

---

## Chunk 1: Interface and Engine

### Task 1: Change Strategy interface

**Files:**
- Modify: `engine/strategy.go:25-29`

- [ ] **Step 1: Update the Strategy interface**

Change the interface to return `error` from `Compute` and rename parameters:

```go
// Strategy is the interface that all strategies must implement.
type Strategy interface {
	Name() string
	Setup(eng *Engine)
	Compute(ctx context.Context, eng *Engine, portfolio portfolio.Portfolio) error
}
```

- [ ] **Step 2: Commit**

```bash
git add engine/strategy.go
git commit -m "feat: add error return to Strategy.Compute interface"
```

Note: The codebase will not compile until all implementations are updated. This is expected.

### Task 2: Update backtest loop to check Compute error

**Files:**
- Modify: `engine/backtest.go:167`

- [ ] **Step 1: Wrap the Compute call with error handling**

Replace line 167:

```go
e.strategy.Compute(stepCtx, e, acct)
```

with:

```go
if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
    return nil, fmt.Errorf("engine: strategy %q compute on %v: %w",
        e.strategy.Name(), date, err)
}
```

- [ ] **Step 2: Commit**

```bash
git add engine/backtest.go
git commit -m "feat: halt backtest on strategy Compute error"
```

### Task 3: Update live trading loop to check Compute error

**Files:**
- Modify: `engine/live.go:169`

- [ ] **Step 1: Wrap the Compute call with error handling**

Replace line 169:

```go
e.strategy.Compute(stepCtx, e, acct)
```

with:

```go
if err := e.strategy.Compute(stepCtx, e, acct); err != nil {
    zerolog.Ctx(stepCtx).Error().Err(err).Msg("strategy compute failed")
    continue
}
```

- [ ] **Step 2: Commit**

```bash
git add engine/live.go
git commit -m "feat: log and continue on strategy Compute error in live trading"
```

### Task 4: Update all test strategy implementations

**Files:**
- Modify: `engine/backtest_test.go:69,123`
- Modify: `engine/fetch_test.go:52,74,97,121,146`
- Modify: `engine/example_test.go:31,83`
- Modify: `engine/live_test.go:45`
- Modify: `engine/parameter_test.go:46`
- Modify: `engine/descriptor_test.go:47,70`
- Modify: `engine/hydrate_test.go:60`
- Modify: `cli/cli_test.go:39`

Each Compute method needs its signature changed to return `error` and a `return nil` added.

- [ ] **Step 1: Update engine/backtest_test.go**

`backtestStrategy.Compute` (line 69) -- change signature to return `error`. This function has two bare `return` statements (lines 71 and 75) that must become `return nil`. Add `return nil` at the end of the function body.

`noScheduleStrategy.Compute` (line 123) -- change:
```go
func (s *noScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}
```
to:
```go
func (s *noScheduleStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error { return nil }
```

- [ ] **Step 2: Update engine/fetch_test.go**

All 5 strategies (`fetchStrategy`, `fetchAtStrategy`, `doubleFetchStrategy`, `fetchThenFetchAtStrategy`, `futureFetchAtStrategy`) -- change signature to return `error`, add `return nil` at end of each function body.

- [ ] **Step 3: Update engine/example_test.go**

`BuyAndHold.Compute` (line 31) and `MomentumStrategy.Compute` (line 83) -- change signature to return `error`. Change bare `return` statements to `return nil`. Add `return nil` at end.

- [ ] **Step 4: Update engine/live_test.go**

Change:
```go
func (s *liveStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}
```
to:
```go
func (s *liveStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error { return nil }
```

- [ ] **Step 5: Update engine/parameter_test.go**

Change:
```go
func (s *paramTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}
```
to:
```go
func (s *paramTestStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error { return nil }
```

- [ ] **Step 6: Update engine/descriptor_test.go**

Change both `descriptorStrategy.Compute` (line 47) and `plainStrategy.Compute` (line 70) from:
```go
func (s *descriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) {}
```
to:
```go
func (s *descriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error { return nil }
```
(Same pattern for `plainStrategy`.)

- [ ] **Step 7: Update engine/hydrate_test.go**

Change signature to return `error`, add `return nil` after `s.computeCalled = true`.

- [ ] **Step 8: Update cli/cli_test.go**

Change:
```go
func (s *testStrategy) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {}
```
to:
```go
func (s *testStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error { return nil }
```

- [ ] **Step 9: Update examples/momentum-rotation/main.go**

Change signature to return `error`. This function has four bare `return` statements (lines 43, 46, 56, 64) that must become `return nil`. Add `return nil` at the end of the function body.

- [ ] **Step 10: Run tests to verify compilation and passing**

Run: `go build ./... && go test ./...`
Expected: All tests pass.

- [ ] **Step 11: Commit**

```bash
git add engine/backtest_test.go engine/fetch_test.go engine/example_test.go \
  engine/live_test.go engine/parameter_test.go engine/descriptor_test.go \
  engine/hydrate_test.go cli/cli_test.go examples/momentum-rotation/main.go
git commit -m "refactor: update all Strategy.Compute implementations to return error"
```

### Task 5: Add test for backtest halting on Compute error

**Files:**
- Modify: `engine/backtest_test.go`

- [ ] **Step 1: Write a test strategy that returns an error**

Add to `engine/backtest_test.go`, near the other strategy type definitions:

```go
// failingStrategy always returns an error from Compute.
type failingStrategy struct{}

func (s *failingStrategy) Name() string { return "failing" }

func (s *failingStrategy) Setup(eng *engine.Engine) {
	tc, err := tradecron.New("0 16 * * 1-5", tradecron.RegularHours)
	if err != nil {
		panic(fmt.Sprintf("failingStrategy.Setup: tradecron.New: %v", err))
	}
	eng.Schedule(tc)
}

func (s *failingStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio) error {
	return fmt.Errorf("simulated compute failure")
}
```

- [ ] **Step 2: Write the test**

Add inside the existing `Context("validation", ...)` block in `engine/backtest_test.go`:

```go
It("halts when strategy Compute returns an error", func() {
    dataStart := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
    df := makeDailyTestData(dataStart, 400, testAssets, metrics)
    provider := data.NewTestProvider(metrics, df)

    strategy := &failingStrategy{}
    eng := engine.New(strategy,
        engine.WithDataProvider(provider),
        engine.WithAssetProvider(assetProvider),
        engine.WithInitialDeposit(10_000),
    )

    start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
    end := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC)

    _, err := eng.Backtest(context.Background(), start, end)
    Expect(err).To(HaveOccurred())
    Expect(err.Error()).To(ContainSubstring("simulated compute failure"))
})
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./engine/ -run "halts when strategy Compute returns an error" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add engine/backtest_test.go
git commit -m "test: verify backtest halts on strategy Compute error"
```

## Chunk 2: Documentation

### Task 6: Update documentation code samples

**Files:**
- Modify: `engine/doc.go:76,212`
- Modify: `README.md:43`
- Modify: `docs/overview.md:53,142,238`
- Modify: `docs/scheduling.md:55`
- Modify: `docs/universes.md:80`
- Modify: `docs/portfolio.md:164,470,578`
- Modify: `docs/superpowers/specs/2026-03-14-rated-universe-design.md:169`

All `Compute` signatures in documentation must be updated to include the `error` return and use descriptive parameter names (`eng` instead of `e`, `portfolio` instead of `p`).

- [ ] **Step 1: Update engine/doc.go**

Two signatures (lines 76 and 212). Change from:
```go
//	func (s *ADM) Compute(ctx context.Context, e *engine.Engine, p portfolio.Portfolio) {
```
to:
```go
//	func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
```

Note: `README.md` line 43 has a pre-existing bug -- the signature is missing the `*Engine` parameter:
```go
func (s *ADM) Compute(ctx context.Context, p portfolio.Portfolio) {
```
Fix this to match the actual interface:
```go
func (s *ADM) Compute(ctx context.Context, eng *engine.Engine, portfolio portfolio.Portfolio) error {
```

- [ ] **Step 2: Update README.md**

Also check if Setup signature matches the actual interface and fix if needed.

- [ ] **Step 3: Update docs/overview.md**

Three signatures (lines 53, 142, 238). Same pattern: rename params, add `error` return.

- [ ] **Step 4: Update docs/scheduling.md**

One signature (line 55). Same pattern.

- [ ] **Step 5: Update docs/universes.md**

One signature (line 80). Same pattern.

- [ ] **Step 6: Update docs/portfolio.md**

Three signatures (lines 164, 470, 578). Same pattern.

- [ ] **Step 7: Update docs/superpowers/specs/2026-03-14-rated-universe-design.md**

One signature (line 169). Same pattern.

- [ ] **Step 8: Run tests to verify nothing broke**

Run: `go build ./... && go test ./...`
Expected: All tests pass.

- [ ] **Step 9: Commit**

```bash
git add engine/doc.go README.md docs/overview.md docs/scheduling.md \
  docs/universes.md docs/portfolio.md \
  docs/superpowers/specs/2026-03-14-rated-universe-design.md
git commit -m "docs: update Compute signatures to include error return"
```
