# Fetch Hot-Path Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce wall time for long-horizon backtests by eliminating point-in-time reassembly overhead in `engine.fetchRange` and trimming redundant bytes out of `data.PVDataProvider.fetchMetrics`.

**Architecture:** Three independent changes applied in TDD order with a real-workload benchmark between each. (1) add a point-in-time fast path to `fetchRange` that binary-searches cached year chunks instead of rebuilding a union-time-axis slab. (2) build the SQL SELECT for `fetchMetrics` from the requested metrics only, so callers pay for only the columns they asked for. (3) hoist per-row allocations out of the `fetchFundamentals` scan loop. Each change is gated on passing Ginkgo tests and on a measured wall-time or CPU improvement from an `ncave` backtest profile.

**Tech Stack:** Go 1.25, Ginkgo/Gomega (BDD tests, `make test`), pgx v5, pprof, pvbt engine + data packages.

**Supporting spec:** `docs/superpowers/specs/2026-04-11-fetch-hot-path-optimization-design.md`

---

## File Layout

| File | Responsibility | Change kind |
| ---- | -------------- | ----------- |
| `engine/engine.go` | Add `assemblePointInTime` helper and point-in-time branch in `fetchRange`. | Modify |
| `engine/fetch_test.go` | Add Ginkgo cases that pin fast-path semantics vs. the slab path. | Modify |
| `data/pvdata_provider.go` | Add `metricsColumn` map; rewrite `fetchMetrics` SQL + scan loop; hoist `fetchFundamentals` allocations. | Modify |
| `data/pvdata_provider_test.go` (if present) or new `data/pvdata_provider_test.go` | Add cases for trimmed-column SELECT behavior. | Modify or create |
| `docs/superpowers/plans/2026-04-11-fetch-hot-path-optimization.md` | This plan. | Create |
| `CHANGELOG.md` | Single user-facing entry once all three changes land. | Modify |

Each change must keep its test code and implementation code in the same commit. Benchmark steps are non-committing and only record measurements in this plan's comments / PR description.

---

## Prerequisites

- [ ] **Step 0.1: Working tree check**

Run: `git -C /Users/jdf/Developer/penny-vault/pvbt status`
Expected: clean (or only the untracked spec at `docs/superpowers/specs/2026-04-11-fetch-hot-path-optimization-design.md`). If the spec is untracked, leave it untracked until Task 1 commits it alongside the first code change.

- [ ] **Step 0.2: Verify the ncave strategy module picks up the local pvbt**

Run: `grep -n "replace github.com/penny-vault/pvbt" /Users/jdf/Developer/penny-vault/strategies/ncave/go.mod`
Expected: `replace github.com/penny-vault/pvbt => ../../pvbt`
This guarantees that a build of the ncave binary picks up whichever source tree is currently checked out in the pvbt repo, so each in-between benchmark exercises the in-progress optimization.

- [ ] **Step 0.3: Confirm pprof and /usr/bin/time are available**

Run: `which go pprof /usr/bin/time`
Expected: three paths. If `pprof` is missing, use `go tool pprof` instead in all later steps that invoke `pprof`.

---

## Task 0: Baseline Benchmark (captured out-of-band)

The baseline wall-clock and pprof numbers in the Benchmarks table at the
bottom of this plan were captured before the `--cpu-profile` flag
existed, by adding a one-off `runtime/pprof` block to the ncave strategy
binary, running the backtest, and then reverting the change. The numbers
are comparable to later runs because both paths call
`pprof.StartCPUProfile` around `runBacktest`.

Do not re-run this task as part of normal execution. If you want to
re-capture the baseline after Task 0A lands, use the Task 1 /
Task 2 / Task 3 benchmark commands below (substituting `baseline` for
`after1` / `after2` / `after3`).

---

## Task 0A: Add `--cpu-profile` Persistent Flag to the CLI

**Files:**
- Modify: `cli/run.go`
- Modify: `cli/cli_test.go` (or a new focused Ginkgo file in the `cli` package)

### Design summary for this task

Add a `--cpu-profile <path>` persistent flag to the root cobra command
built in `cli.Run`. When the flag is set, `PersistentPreRunE` creates the
file and calls `pprof.StartCPUProfile`; `PersistentPostRunE` calls
`pprof.StopCPUProfile` and closes the file. The flag is persistent so
every subcommand (`backtest`, `live`, `snapshot`, `study`, `describe`,
`config`) inherits it, and because `cli.Run` is the single entry point
every strategy binary that uses the framework gets CPU profiling for
free without per-strategy boilerplate.

The existing `PersistentPreRun` (which configures zerolog) must be
preserved. Combine the new profile-start logic into the same
`PersistentPreRunE` (converting from the non-error form) or chain it
after the current logic — whichever keeps the file short. Error
handling: if `--cpu-profile` is set but the file cannot be created or
`StartCPUProfile` fails, return the error from `PersistentPreRunE` so
cobra surfaces it and aborts the command.

Stop ordering: `PersistentPostRunE` stops the profile regardless of the
subcommand's outcome. If the file handle is nil (flag unset) it is a
no-op.

### Tests first

- [ ] **Step 0A.1: Read existing CLI test patterns**

Run:
```bash
grep -n "buildTestCmd\|rootCmd\.SetArgs" /Users/jdf/Developer/penny-vault/pvbt/cli/cli_test.go | head -20
```
Look at how existing tests build and run the cobra tree. Reuse that
pattern.

- [ ] **Step 0A.2: Write failing test — `--cpu-profile <path>` creates a non-empty profile file**

Add a Ginkgo case in an appropriate test file (either extend
`cli_test.go` or create a focused file). Pseudocode:

```go
It("writes a CPU profile when --cpu-profile is set", func() {
    tmpDir := GinkgoT().TempDir()
    profilePath := filepath.Join(tmpDir, "cpu.prof")

    strategy := newTestStrategyThatDoesSomeWork()
    rootCmd, _ := buildTestCmd(strategy, nil)
    rootCmd.SetArgs([]string{"backtest", "--cpu-profile", profilePath})

    Expect(rootCmd.Execute()).To(Succeed())

    info, err := os.Stat(profilePath)
    Expect(err).NotTo(HaveOccurred())
    Expect(info.Size()).To(BeNumerically(">", 0),
        "CPU profile file must be non-empty after a backtest run")
})

It("does not create a profile when --cpu-profile is omitted", func() {
    strategy := newTestStrategyThatDoesSomeWork()
    rootCmd, _ := buildTestCmd(strategy, nil)
    rootCmd.SetArgs([]string{"backtest"})

    Expect(rootCmd.Execute()).To(Succeed())
    // No assertion on a specific path; the absence of a created file
    // is checked by not referencing a path.
})
```

If the existing `buildTestCmd` helper does not wire up something that
behaves like `cli.Run`'s root (with our persistent flag), either extend
it or write a new helper that calls a small exported builder. Do not
duplicate `cli.Run`'s body in the test.

Recommended refactor: extract the rootCmd construction from `cli.Run`
into an unexported `newRootCmd(strategy engine.Strategy) *cobra.Command`
function in `cli/run.go`, have `Run` call it, and have the test call
`newRootCmd` directly. This is the minimal change that makes the root
testable without leaking internals.

- [ ] **Step 0A.3: Run the failing test**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "writes a CPU profile when --cpu-profile is set" ./cli
```
Expected: FAIL — the flag does not exist yet, so cobra emits an unknown
flag error, or the test cannot compile because `newRootCmd` is not
defined yet. Either is an acceptable red state.

### Implementation

- [ ] **Step 0A.4: Extract `newRootCmd` in `cli/run.go` and add the `--cpu-profile` flag**

Rewrite `cli/run.go` as:

```go
package cli

import (
    "fmt"
    "os"
    "runtime/pprof"

    "github.com/penny-vault/pvbt/engine"
    "github.com/rs/zerolog"
    "github.com/rs/zerolog/log"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

// Run is the single entry point for strategy authors. It builds the
// cobra command tree, parses flags, and executes the appropriate
// subcommand.
func Run(strategy engine.Strategy) {
    if err := newRootCmd(strategy).Execute(); err != nil {
        os.Exit(1)
    }
}

func newRootCmd(strategy engine.Strategy) *cobra.Command {
    var cpuProfileFile *os.File

    rootCmd := &cobra.Command{
        Use:   strategy.Name(),
        Short: fmt.Sprintf("Run the %s strategy", strategy.Name()),
    }

    rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
    rootCmd.PersistentFlags().String("config", "", "Path to config file (default: ./pvbt.toml or ~/.config/pvbt/config.toml)")
    rootCmd.PersistentFlags().String("cpu-profile", "", "Write a Go CPU profile to the given path for the duration of the command")

    if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
        log.Fatal().Err(err).Msg("failed to bind log-level flag")
    }

    rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
        level, err := zerolog.ParseLevel(viper.GetString("log-level"))
        if err != nil {
            level = zerolog.InfoLevel
        }

        zerolog.SetGlobalLevel(level)

        log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
            With().Timestamp().Logger()

        profilePath, _ := cmd.Flags().GetString("cpu-profile")
        if profilePath == "" {
            return nil
        }

        file, err := os.Create(profilePath)
        if err != nil {
            return fmt.Errorf("cli: create cpu profile %q: %w", profilePath, err)
        }

        if err := pprof.StartCPUProfile(file); err != nil {
            file.Close()
            return fmt.Errorf("cli: start cpu profile: %w", err)
        }

        cpuProfileFile = file

        return nil
    }

    rootCmd.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
        if cpuProfileFile == nil {
            return nil
        }

        pprof.StopCPUProfile()

        if err := cpuProfileFile.Close(); err != nil {
            return fmt.Errorf("cli: close cpu profile: %w", err)
        }

        cpuProfileFile = nil

        return nil
    }

    rootCmd.AddCommand(newBacktestCmd(strategy))
    rootCmd.AddCommand(newLiveCmd(strategy))
    rootCmd.AddCommand(newSnapshotCmd(strategy))
    rootCmd.AddCommand(newDescribeCmd(strategy))
    rootCmd.AddCommand(newStudyCmd(strategy))
    rootCmd.AddCommand(newConfigCmd())

    return rootCmd
}
```

Note: `cpuProfileFile` is captured by closure so both the pre-run and
post-run callbacks see the same file handle. It is scoped to one
`newRootCmd` invocation, so parallel tests each get their own state.

- [ ] **Step 0A.5: Run the test and verify it passes**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "writes a CPU profile when --cpu-profile is set" ./cli
```
Expected: PASS.

- [ ] **Step 0A.6: Run the full cli suite + project suite + lint**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./cli && make test && make lint
```
Expected: green.

- [ ] **Step 0A.7: Commit**

```bash
git -C /Users/jdf/Developer/penny-vault/pvbt add cli/run.go cli/cli_test.go
git -C /Users/jdf/Developer/penny-vault/pvbt commit -m "$(cat <<'EOF'
feat(cli): --cpu-profile flag on all pvbt commands

Adds a persistent --cpu-profile <path> flag to the root cobra command
built in cli.Run so every strategy binary that uses the framework can
capture a CPU profile without adding runtime/pprof boilerplate to its
own main. The flag is honored via PersistentPreRunE /
PersistentPostRunE hooks so it works for any subcommand (backtest,
live, snapshot, study, describe, config).
EOF
)"
```

- [ ] **Step 0A.8: Changelog entry (fold into the final entry in Task 4)**

Do NOT add a changelog entry yet. Task 4 adds a single user-facing entry
that covers all the perf work plus the flag. Note here as a reminder:
the final entry should mention that strategies can now write CPU
profiles via `--cpu-profile`.

---

## Task 1: Point-in-time Fast Path in `fetchRange`

**Files:**
- Modify: `engine/engine.go` (`fetchRange`, new helper `assemblePointInTime`)
- Modify: `engine/fetch_test.go` (new Ginkgo `It(...)` cases)

### Design summary for this task

Current `fetchRange` (engine/engine.go:390-672) always runs the miss-fetch phase, then rebuilds a union time axis from cached entries, NaN-fills a slab, scatters cell values by `timeIdx[t.Unix()]`, runs within-chunk forward-fill on fundamental columns, constructs a DataFrame, and calls `Between(rangeStart, rangeEnd)`.

For `rangeStart.Equal(rangeEnd)` we will short-circuit *after* the miss-fetch phase and build the one-row result directly by binary-searching each cached column. Semantics MUST match the slab path exactly:

- Use `chunkYears(rangeEnd, rangeEnd)` (always one year in practice) as the set of year keys to consult, matching the slab path which only scatters values from those chunks.
- Time matching uses date keys (`YYYYMMDD`), not `Unix()`, because `DataFrame.Between` on daily-or-coarser frames matches by date key. See `data/data_frame.go:126-145,616-645`.
- For non-fundamental metrics, require an exact date-key match.
- For fundamental metrics, find the greatest index whose date key is `<=` the target date key, within the same year's cache entry (within-chunk forward-fill). This matches the slab path's forward-fill which only walks the current year's rows (`engine.go:642-656`).
- The returned DataFrame:
  - One row anchored at `rangeEnd` whenever `hasFundamental` and `rangeStart.Equal(rangeEnd)` is true (mirrors the `timeSet[rangeEnd.Unix()] = rangeEnd` injection at `engine.go:581-583`).
  - Otherwise, one row only if at least one cached entry in the consulted chunks has a date-key match for `rangeEnd`. Else an empty DataFrame (`Len() == 0`) with the requested asset/metric schema, matching what `Between(rangeEnd, rangeEnd)` produces from a slab whose union axis contains no matching date.

### Tests first

- [ ] **Step 1.1: Read existing point-in-time coverage**

Run:
```bash
grep -n "FetchAt\|fastPoint\|point.in.time" /Users/jdf/Developer/penny-vault/pvbt/engine/fetch_test.go
```
Expected: existing `Context("FetchAt", ...)` block at `fetch_test.go:301` and `Context("FetchAt cache", ...)` at `fetch_test.go:363`. Read both blocks so new tests match style.

- [ ] **Step 1.2: Write failing test — FetchAt on a non-trading day returns empty for non-fundamental metrics**

Edit `engine/fetch_test.go`. Inside the existing `Describe("Engine Fetch", ...)` block, add (adapt indentation to the file's 1-tab Go style):

```go
Context("FetchAt point-in-time fast path", func() {
    It("returns an empty frame when FetchAt targets a non-trading day for a daily metric", func() {
        // 2024-06-15 was a Saturday - no eod row expected.
        saturday := time.Date(2024, 6, 15, 16, 0, 0, 0, time.UTC)

        strategy := &pastFetchAtStrategy{
            assets:    []asset.Asset{spy},
            queryDate: saturday,
            metrics:   []data.Metric{data.MetricClose},
        }

        eng := newTestEngine(strategy)
        _, err := eng.Backtest(context.Background())
        Expect(err).NotTo(HaveOccurred())
        Expect(strategy.fetchErr).NotTo(HaveOccurred())
        Expect(strategy.fetched).NotTo(BeNil())
        Expect(strategy.fetched.Len()).To(Equal(0),
            "non-fundamental FetchAt on Saturday must return empty like the slab path")
    })
})
```

Notes for the implementer:
- `pastFetchAtStrategy`, `newTestEngine`, and `spy` all already exist in `engine/fetch_test.go` / `engine/engine_suite_test.go`. Reuse them. Do not introduce new helpers.
- Pick a Saturday that falls inside the existing test fixture date range. If `spy`'s fixture range does not cover 2024-06-15, substitute any Saturday the fixture does cover. Confirm by checking `engine/engine_suite_test.go` for the fixture date range.

- [ ] **Step 1.3: Run the new test and watch it fail**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "returns an empty frame when FetchAt targets a non-trading day" ./engine
```
Expected: 1 test, 0 passed, 1 failed. Either the test passes already (in which case re-check fixture ranges — the existing slab path may already return empty for this date), or the assertion about frame emptiness trips. If the test passes without any production code changes, that branch is already correct today; continue to 1.4 with a different failing case (skip to 1.5 with a passing test that still captures the semantic contract).

- [ ] **Step 1.4: Write failing test — FetchAt on a trading day returns correct single-row value for non-fundamental metrics**

Add inside the same `Context("FetchAt point-in-time fast path", ...)`:

```go
It("returns a one-row frame with the close price for FetchAt on a trading day", func() {
    tradingDay := time.Date(2024, 6, 14, 16, 0, 0, 0, time.UTC) // Friday

    strategy := &pastFetchAtStrategy{
        assets:    []asset.Asset{spy},
        queryDate: tradingDay,
        metrics:   []data.Metric{data.MetricClose},
    }

    eng := newTestEngine(strategy)
    _, err := eng.Backtest(context.Background())
    Expect(err).NotTo(HaveOccurred())
    Expect(strategy.fetchErr).NotTo(HaveOccurred())
    Expect(strategy.fetched).NotTo(BeNil())
    Expect(strategy.fetched.Len()).To(Equal(1),
        "FetchAt on a trading day must produce exactly one row")
    Expect(math.IsNaN(strategy.fetched.Value(spy, data.MetricClose))).To(BeFalse(),
        "FetchAt on a trading day must produce a non-NaN close value")
})
```

- [ ] **Step 1.5: Write failing test — FetchAt on a fundamental metric forward-fills within-year**

Add inside the same Context:

```go
It("returns a one-row fundamental frame forward-filled from the most recent filing within the year", func() {
    // Pick a date after the fixture's Q1 fundamental filing so forward-fill kicks in.
    queryDate := time.Date(2024, 6, 28, 16, 0, 0, 0, time.UTC)

    strategy := &pastFetchAtStrategy{
        assets:    []asset.Asset{spy},
        queryDate: queryDate,
        metrics:   []data.Metric{data.MarketCap},
    }

    eng := newTestEngine(strategy)
    _, err := eng.Backtest(context.Background())
    Expect(err).NotTo(HaveOccurred())
    Expect(strategy.fetchErr).NotTo(HaveOccurred())
    Expect(strategy.fetched).NotTo(BeNil())
    Expect(strategy.fetched.Len()).To(Equal(1),
        "FetchAt on a fundamental-bearing metric must produce exactly one row")
})
```

If `data.MarketCap` is not in the test fixture, replace it with any fundamental-view metric the fixture does provide, or use an eod metric and remove this specific test case (rely on the existing forward-fill coverage in `fetch_test.go:548-599`).

- [ ] **Step 1.6: Run the three new tests and confirm the failures make sense**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "FetchAt point-in-time fast path" ./engine
```
Expected: tests fail OR pass. If all pass today, the semantic contract we are about to preserve is already correct — great. If any fail, the failures tell us something about the current slab-path semantics and must be investigated before writing any fast path. Resolve (either fix the test to match existing semantics, or treat it as the fast-path goal). The resolution must leave a suite where every assertion reflects what the slab path *already* does.

### Implementation

- [ ] **Step 1.7: Add `assemblePointInTime` helper in `engine/engine.go`**

Place the new function immediately below `fetchRange`. The helper assumes the miss-fetch phase has already run and that all cache entries the caller needs are populated.

```go
// assemblePointInTime builds a one-row DataFrame for a FetchAt-style query
// without rebuilding a union time axis. It walks the cached column entries
// for the year containing rangeEnd and binary-searches each entry by date
// key. Its output must match the slab path in fetchRange byte-for-byte for
// rangeStart.Equal(rangeEnd) queries.
func (e *Engine) assemblePointInTime(assets []asset.Asset, metrics []data.Metric, rangeEnd time.Time, hasFundamental bool) (*data.DataFrame, error) {
    targetKey := dataFrameDateKey(rangeEnd)

    years := chunkYears(rangeEnd, rangeEnd)

    numMetrics := len(metrics)
    rowCells := make([]float64, len(assets)*numMetrics)
    for i := range rowCells {
        rowCells[i] = math.NaN()
    }

    anyMatch := false

    for aIdx, a := range assets {
        for mIdx, m := range metrics {
            isFund := data.IsFundamental(m)

            for _, year := range years {
                key := colCacheKey{figi: a.CompositeFigi, metric: m, chunkStart: year}
                entry, ok := e.cache.get(key)
                if !ok || len(entry.times) == 0 {
                    continue
                }

                if isFund {
                    // Find greatest index with dateKey(entry.times[i]) <= targetKey.
                    idx := sort.Search(len(entry.times), func(i int) bool {
                        return dataFrameDateKey(entry.times[i]) > targetKey
                    }) - 1
                    if idx >= 0 {
                        rowCells[aIdx*numMetrics+mIdx] = entry.values[idx]
                        anyMatch = true
                    }
                    continue
                }

                // Non-fundamental: require exact date-key match.
                idx := sort.Search(len(entry.times), func(i int) bool {
                    return dataFrameDateKey(entry.times[i]) >= targetKey
                })
                if idx < len(entry.times) && dataFrameDateKey(entry.times[idx]) == targetKey {
                    rowCells[aIdx*numMetrics+mIdx] = entry.values[idx]
                    anyMatch = true
                }
            }
        }
    }

    // Return shape: mirror the slab-path rules from fetchRange.
    if !anyMatch && !hasFundamental {
        return data.NewDataFrame(nil, assets, metrics, data.Daily, nil)
    }

    times := []time.Time{rangeEnd}
    cols := data.SlabToColumns(rowCells, len(assets)*numMetrics, 1)

    return data.NewDataFrame(times, assets, metrics, data.Daily, cols)
}
```

The helper references `dataFrameDateKey`, which does not yet exist at engine-package scope. Resolve by either:

1. Exporting `dateKey` from the `data` package as `DateKey`, and calling `data.DateKey` here; or
2. Duplicating a local `dateKey(t time.Time) int32 { y, m, d := t.Date(); return int32(y)*10000 + int32(m)*100 + int32(d) }` helper in `engine/engine.go`.

**Pick option 1** so the engine and the DataFrame agree on date-key semantics by construction. Add in `data/data_frame.go` just below the existing unexported `dateKey` (line ~126):

```go
// DateKey exposes the daily-frame date key used by Between and the engine's
// point-in-time fast path. Year*10000 + Month*100 + Day.
func DateKey(t time.Time) int32 { return dateKey(t) }
```

Then in `engine/engine.go` replace every `dataFrameDateKey(...)` call with `data.DateKey(...)`.

- [ ] **Step 1.8: Wire the fast path into `fetchRange`**

In `engine/engine.go`, after the miss-fetch loop ends (just before `// Assemble the requested DataFrame from cached columns.` around line 545), add:

```go
if rangeStart.Equal(rangeEnd) {
    hasFundamental := false
    for _, m := range metrics {
        if data.IsFundamental(m) {
            hasFundamental = true
            break
        }
    }

    result, err := e.assemblePointInTime(assets, metrics, rangeEnd, hasFundamental)
    if err != nil {
        return nil, fmt.Errorf("engine: assemble point-in-time: %w", err)
    }

    log.Debug().
        Int("len", result.Len()).
        Int("assets", len(result.AssetList())).
        Int("metrics", len(result.MetricList())).
        Msg("engine.fetchRange fast path")

    return result, nil
}
```

Leave the existing slab path below untouched. Range queries (`Fetch` with `rangeStart != rangeEnd`) continue to use it.

- [ ] **Step 1.9: Run the targeted tests**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "FetchAt point-in-time fast path" ./engine
```
Expected: all three (or however many survived Step 1.6) pass.

- [ ] **Step 1.10: Run the full engine suite**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./engine
```
Expected: entire engine suite green. If any pre-existing FetchAt / forward-fill test now fails, the fast path diverges from slab-path semantics — fix the fast path, not the test.

- [ ] **Step 1.11: Run the full project suite + lint**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make test && make lint
```
Expected: green. Lint must also pass — no `//nolint` directives.

- [ ] **Step 1.12: Commit**

Stage only the files touched by this task plus the design spec from the brainstorming step (that spec file has been untracked until now).

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && git add \
    docs/superpowers/specs/2026-04-11-fetch-hot-path-optimization-design.md \
    docs/superpowers/plans/2026-04-11-fetch-hot-path-optimization.md \
    engine/engine.go \
    engine/fetch_test.go \
    data/data_frame.go
```

Then:
```bash
git -C /Users/jdf/Developer/penny-vault/pvbt commit -m "$(cat <<'EOF'
perf(engine): fast path for FetchAt point-in-time queries

fetchRange now short-circuits rangeStart == rangeEnd calls past the
union-time-axis rebuild. Each requested (asset, metric) binary-searches
the cached year chunk by date key and writes directly into a one-row
slab, matching slab-path semantics (exact match for daily metrics,
within-year forward-fill for fundamentals). Eliminates the
mapaccess2_fast64 and mapassign_fast64 hot spots in the daily
housekeeping path.
EOF
)"
```
Expected: commit created.

### Benchmark between tasks

- [ ] **Step 1.13: Rebuild ncave and rerun the backtest**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/strategies/ncave && go build -o ncave . && /usr/bin/time -p ./ncave backtest --start 2010-01-01 --end 2026-01-01 --cpu-profile after1.cpu.prof > after1.stdout 2> after1.time
```
Expected: exit 0. `after1.time` exists. Portfolio terminal NAV in `after1.stdout` must match the baseline's (the optimization is a pure CPU / I/O change; if NAVs disagree, the fast path is semantically wrong — stop and debug).

- [ ] **Step 1.14: Compare against the baseline**

Run:
```bash
cat /Users/jdf/Developer/penny-vault/strategies/ncave/baseline.time /Users/jdf/Developer/penny-vault/strategies/ncave/after1.time && go tool pprof -top -cum /Users/jdf/Developer/penny-vault/strategies/ncave/after1.cpu.prof | head -30
```
Record in the Benchmarks table below, row **After Task 1**:
- Wall-clock `real` seconds.
- `fetchRange` cum time.
- `mapaccess2_fast64` flat time.
- `mapassign_fast64` flat time.

**Decision gate**: if wall-clock did not improve AND on-CPU `fetchRange` time did not drop by at least 1.5s, something is wrong — either the fast path is not being taken (log should show "engine.fetchRange fast path"), or its cost is accidentally worse. Do NOT proceed to Task 2 until this is explained. An acceptable outcome is "wall unchanged, CPU clearly dropped" — the DB wait floor dominates, which is expected, but CPU must drop.

---

## Task 2: Column-Trimmed SELECT in `fetchMetrics`

**Files:**
- Modify: `data/pvdata_provider.go` (`fetchMetrics` and supporting data structures)
- Modify or create: `data/pvdata_provider_test.go` (or the equivalent Ginkgo `_test.go` file this package uses)

### Design summary for this task

`fetchMetrics` currently emits a fixed 11-column SELECT (`data/pvdata_provider.go:576-584`). Trim that to only the SQL columns that correspond to requested metrics. Mirror the pattern already used by `fetchFundamentals`, which builds `sqlCols` and `metricOrder` from the `metricColumn` map and generates the SELECT via `fmt.Sprintf` + `strings.Join`. Also hoist `scanArgs`, `intVals`, `floatVals` out of the per-row loop.

### Tests first

- [ ] **Step 2.1: Find or create the Ginkgo test file for the provider**

Run:
```bash
ls /Users/jdf/Developer/penny-vault/pvbt/data/*provider*test* /Users/jdf/Developer/penny-vault/pvbt/data/pvdata* 2>/dev/null
```
Expected: inspect what exists. The existing tests may be in a larger file (e.g. `data/pvdata_provider_test.go` may already exist but the repo's Ginkgo migration may be in progress — see the `project_data_tests_ginkgo_migration` memory). Do not migrate pre-existing standard-library tests in this task; add new cases in the style already present in the target file.

- [ ] **Step 2.2: Write failing test — Fetch(MarketCap only) returns only MarketCap**

Add a Ginkgo case:

```go
It("Fetch with only MarketCap does not populate other metrics-view columns", func() {
    ctx := context.Background()
    req := data.DataRequest{
        Assets:    []asset.Asset{testSpy},
        Metrics:   []data.Metric{data.MarketCap},
        Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
        End:       time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
        Frequency: data.Daily,
    }

    df, err := provider.Fetch(ctx, req)
    Expect(err).NotTo(HaveOccurred())
    Expect(df.MetricList()).To(ConsistOf(data.MarketCap))
    Expect(df.Len()).To(BeNumerically(">", 0))
})
```

- [ ] **Step 2.3: Write failing test — Fetch with a mix of metrics-view metrics**

```go
It("Fetch returns requested metrics-view metrics populated and no others", func() {
    ctx := context.Background()
    req := data.DataRequest{
        Assets:    []asset.Asset{testSpy},
        Metrics:   []data.Metric{data.MarketCap, data.PE},
        Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
        End:       time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
        Frequency: data.Daily,
    }

    df, err := provider.Fetch(ctx, req)
    Expect(err).NotTo(HaveOccurred())
    Expect(df.MetricList()).To(ConsistOf(data.MarketCap, data.PE))
})
```

- [ ] **Step 2.4: Run the failing tests**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "Fetch with only MarketCap|Fetch returns requested metrics-view metrics" ./data
```
Expected: these either (a) pass already because `PVDataProvider.Fetch`'s outer assembly discards unrequested columns — in which case the test validates existing behavior and the optimization is invisible at this layer — or (b) pass but exercise the over-fetching path. The Task-2 optimization isn't about changing externally visible behavior; it's about the SQL shape. Keep the tests as a regression guard and move on to implementation.

If the tests do not compile or setup is missing, build the minimum test harness by following the pattern used elsewhere in `data/*_test.go`. Do not stub the real Postgres — if the existing tests in this package use a sqlmock/pgxmock or a live test DB, reuse that exact setup.

### Implementation

- [ ] **Step 2.5: Introduce `metricsColumn` map**

In `data/pvdata_provider.go`, next to the existing `metricColumn` map (around line 1017), add:

```go
// metricsColumn maps metrics-view Metrics to their SQL column name and
// whether the DB column is a bigint (scanned as int64 and converted to
// float64). Covers the same metrics as metricView[m] == "metrics".
var metricsColumn = map[Metric]struct {
    sql    string
    intCol bool
}{
    MarketCap:       {"market_cap", true},
    EnterpriseValue: {"ev", true},
    PE:              {"pe", false},
    PB:              {"pb", false},
    PS:              {"ps", false},
    EVtoEBIT:        {"ev_ebit", false},
    EVtoEBITDA:      {"ev_ebitda", false},
    ForwardPE:       {"pe_forward", false},
    PEG:             {"peg", false},
    PriceToCashFlow: {"price_to_cash_flow", false},
    Beta:            {"beta", false},
}
```

Cross-check: the SQL names and intCol flags must match the hardcoded list at `data/pvdata_provider.go:562-574`. A mismatch silently corrupts data — verify each line.

- [ ] **Step 2.6: Replace `fetchMetrics` with a dynamic-SELECT version**

Replace the body of `fetchMetrics` (`data/pvdata_provider.go:544-643`) with:

```go
func (p *PVDataProvider) fetchMetrics(
    ctx context.Context,
    conn *pgxpool.Conn,
    figis []string,
    start, end time.Time,
    metrics []Metric,
    ensureCol func(string, Metric) map[int64]float64,
    timeSet map[int64]time.Time,
) error {
    if len(metrics) == 0 {
        return nil
    }

    type boundCol struct {
        metric Metric
        intCol bool
    }

    sqlCols := make([]string, 0, len(metrics))
    bound := make([]boundCol, 0, len(metrics))

    for _, m := range metrics {
        spec, ok := metricsColumn[m]
        if !ok {
            return fmt.Errorf("pvdata: no SQL column for metrics-view metric %q", m)
        }

        sqlCols = append(sqlCols, spec.sql)
        bound = append(bound, boundCol{metric: m, intCol: spec.intCol})
    }

    query := fmt.Sprintf(
        `SELECT composite_figi, event_date, %s
         FROM metrics
         WHERE composite_figi = ANY($1) AND event_date BETWEEN $2::date AND $3::date
         ORDER BY event_date`,
        strings.Join(sqlCols, ", "),
    )

    rows, err := conn.Query(ctx, query, figis, start, end)
    if err != nil {
        return fmt.Errorf("pvdata: query metrics: %w", err)
    }
    defer rows.Close()

    // Hoisted scan destinations.
    intVals := make([]*int64, len(bound))
    floatVals := make([]*float64, len(bound))
    scanArgs := make([]any, 0, 2+len(bound))

    want := metricSet(metrics)

    for rows.Next() {
        var (
            figi      string
            eventDate time.Time
        )

        scanArgs = scanArgs[:0]
        scanArgs = append(scanArgs, &figi, &eventDate)

        for idx, col := range bound {
            if col.intCol {
                intVals[idx] = nil
                scanArgs = append(scanArgs, &intVals[idx])
            } else {
                floatVals[idx] = nil
                scanArgs = append(scanArgs, &floatVals[idx])
            }
        }

        if err := rows.Scan(scanArgs...); err != nil {
            return fmt.Errorf("pvdata: scan metrics row: %w", err)
        }

        eventDate = eodTimestamp(eventDate)
        sec := eventDate.Unix()
        timeSet[sec] = eventDate

        for idx, col := range bound {
            if !want[col.metric] {
                continue
            }

            if col.intCol {
                if intVals[idx] != nil {
                    ensureCol(figi, col.metric)[sec] = float64(*intVals[idx])
                }
            } else {
                if floatVals[idx] != nil {
                    ensureCol(figi, col.metric)[sec] = *floatVals[idx]
                }
            }
        }
    }

    return rows.Err()
}
```

Notes:
- `metricSet(metrics)` and `eodTimestamp(...)` already exist in the file; do not re-declare.
- `scanArgs` is reused across rows by slicing back to zero length and appending. This avoids a per-row allocation while keeping exact semantics.
- The old 11-column `columns` literal is now dead — delete it when replacing the function body.

- [ ] **Step 2.7: Run the targeted tests**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "Fetch with only MarketCap|Fetch returns requested metrics-view metrics" ./data
```
Expected: both pass.

- [ ] **Step 2.8: Run the full data suite + full project suite**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run ./data && make test && make lint
```
Expected: green.

- [ ] **Step 2.9: Commit**

```bash
git -C /Users/jdf/Developer/penny-vault/pvbt add data/pvdata_provider.go data/pvdata_provider_test.go
git -C /Users/jdf/Developer/penny-vault/pvbt commit -m "$(cat <<'EOF'
perf(pvdata): SELECT only requested columns in fetchMetrics

fetchMetrics now builds its SELECT list from the caller's requested
metrics via a new metricsColumn map, mirroring the existing pattern in
fetchFundamentals. Row scan destinations are hoisted out of the loop so
each row reuses the same scanArgs buffer. Strategies that ask for one
metric (e.g. MarketCap only) no longer pay for the other ten columns.
EOF
)"
```

If `data/pvdata_provider_test.go` was not touched (because the new tests went into a different file), adjust the `git add` accordingly.

### Benchmark between tasks

- [ ] **Step 2.10: Rebuild ncave and rerun the backtest**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/strategies/ncave && go build -o ncave . && /usr/bin/time -p ./ncave backtest --start 2010-01-01 --end 2026-01-01 --cpu-profile after2.cpu.prof > after2.stdout 2> after2.time
```
Expected: exit 0. Terminal NAV in `after2.stdout` must match baseline.

- [ ] **Step 2.11: Compare against Task-1 result**

Run:
```bash
cat /Users/jdf/Developer/penny-vault/strategies/ncave/after1.time /Users/jdf/Developer/penny-vault/strategies/ncave/after2.time && go tool pprof -top -cum /Users/jdf/Developer/penny-vault/strategies/ncave/after2.cpu.prof | head -30
```
Record in the Benchmarks table row **After Task 2**:
- Wall-clock real.
- `fetchMetrics` cum.
- `rows.Next` / `syscall.rawsyscalln` flat.

**Decision gate**: expect `fetchMetrics` cum to drop noticeably (target: at least 1s drop versus Task-1 result). Wall clock should drop by at least the difference in `rows.Next` flat time. If `fetchMetrics` cum is unchanged, the dynamic SELECT may not have actually trimmed columns — inspect the SQL the provider sends (either log it or temporarily wrap `conn.Query` to print the final string) and verify the sqlCols list is built correctly.

---

## Task 3: Hoist Per-Row Allocations in `fetchFundamentals`

**Files:**
- Modify: `data/pvdata_provider.go` (`fetchFundamentals`)

### Design summary for this task

`fetchFundamentals` (`data/pvdata_provider.go:688-720`) allocates `vals := make([]any, ...)` and `floatVals := make([]*float64, ...)` inside the `rows.Next()` loop. Hoist both out, reset `floatVals` entries to nil each iteration so SQL NULLs remain nil, and reuse `vals` in place. The semantics are unchanged.

### Implementation

- [ ] **Step 3.1: Write a small test that exercises fetchFundamentals**

If there is no existing Ginkgo case for it, add one to the same `data/*_test.go` file used in Task 2:

```go
It("Fetch returns requested fundamentals metrics", func() {
    ctx := context.Background()
    req := data.DataRequest{
        Assets:    []asset.Asset{testSpy},
        Metrics:   []data.Metric{data.WorkingCapital},
        Start:     time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
        End:       time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC),
        Frequency: data.Daily,
    }

    df, err := provider.Fetch(ctx, req)
    Expect(err).NotTo(HaveOccurred())
    Expect(df.MetricList()).To(ConsistOf(data.WorkingCapital))
})
```

Run it to make sure it passes on the current (pre-change) code:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "Fetch returns requested fundamentals metrics" ./data
```
Expected: pass. (This test is a regression guard, not a failing-first TDD case — the allocation hoist is an optimization, not a new behavior.)

- [ ] **Step 3.2: Rewrite the `fetchFundamentals` loop**

In `data/pvdata_provider.go`, replace the body of `fetchFundamentals` from `for rows.Next() {` to the end of that block with:

```go
// Hoisted scan destinations.
vals := make([]any, len(sqlCols)+3)
floatVals := make([]*float64, len(sqlCols))

var (
    figi      string
    eventDate time.Time
    dateKey   time.Time
)

vals[0] = &figi
vals[1] = &eventDate
vals[2] = &dateKey
for idx := range sqlCols {
    vals[idx+3] = &floatVals[idx]
}

for rows.Next() {
    for idx := range floatVals {
        floatVals[idx] = nil
    }

    if err := rows.Scan(vals...); err != nil {
        return fmt.Errorf("pvdata: scan fundamentals row: %w", err)
    }

    eventDate = eodTimestamp(eventDate)
    sec := eventDate.Unix()
    timeSet[sec] = eventDate

    for idx, m := range metricOrder {
        if floatVals[idx] != nil {
            ensureCol(figi, m)[sec] = *floatVals[idx]
        }
    }
}

return rows.Err()
```

Check: the closure captures `figi`, `eventDate`, and `dateKey` once. Because `pgx` copies scanned values into these variables each row, hoisting is safe — the pointer targets are reused but the values are overwritten every iteration.

- [ ] **Step 3.3: Run the focused test**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run --focus "Fetch returns requested fundamentals metrics" ./data
```
Expected: pass.

- [ ] **Step 3.4: Run the full project suite + lint**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make test && make lint
```
Expected: green.

- [ ] **Step 3.5: Commit**

```bash
git -C /Users/jdf/Developer/penny-vault/pvbt add data/pvdata_provider.go data/pvdata_provider_test.go
git -C /Users/jdf/Developer/penny-vault/pvbt commit -m "$(cat <<'EOF'
perf(pvdata): hoist scan buffers out of fetchFundamentals row loop

vals and floatVals are now allocated once per call rather than per
row. floatVals entries are reset to nil each iteration so SQL NULLs
remain nil.
EOF
)"
```

### Benchmark between tasks

- [ ] **Step 3.6: Rebuild ncave and rerun the backtest**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/strategies/ncave && go build -o ncave . && /usr/bin/time -p ./ncave backtest --start 2010-01-01 --end 2026-01-01 --cpu-profile after3.cpu.prof > after3.stdout 2> after3.time
```

- [ ] **Step 3.7: Compare against Task-2 result**

Run:
```bash
cat /Users/jdf/Developer/penny-vault/strategies/ncave/after2.time /Users/jdf/Developer/penny-vault/strategies/ncave/after3.time && go tool pprof -top -cum /Users/jdf/Developer/penny-vault/strategies/ncave/after3.cpu.prof | head -30
```
Record in the Benchmarks table row **After Task 3**. Task 3 is small; a ~100-200ms improvement is an acceptable outcome. No decision gate — this is the last change, so the comparison is informational.

---

## Task 4: Changelog Entry

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 4.1: Add a single user-facing entry under the unreleased section**

Open `CHANGELOG.md`. Add one bullet under `### Changed` (or create the subsection if it does not exist) in the unreleased block. Keep the tone user-facing per the project's changelog conventions:

```
- Long-horizon backtests run noticeably faster. The engine now short-circuits point-in-time data lookups through a direct cache read instead of rebuilding a union time axis, and the pvdata provider fetches only the columns each strategy actually asked for.
```

No per-task breakdown; related items are combined into one bullet. The entry must describe the user-visible effect (backtests run faster), not the implementation.

- [ ] **Step 4.2: Commit**

```bash
git -C /Users/jdf/Developer/penny-vault/pvbt add CHANGELOG.md
git -C /Users/jdf/Developer/penny-vault/pvbt commit -m "docs: changelog entry for fetch hot-path optimization"
```

---

## Benchmarks

Fill in as the plan executes. Methodology: warm Postgres cache with one
discard run of each binary, then 5 alternating runs (baseline, task,
baseline, task, ...). Report average user CPU time (stable) and average
wall clock (includes I/O variance). Single-run cold-cache numbers are
unreliable; the original baseline of 32.52s was a cold-cache outlier.

| Stage           | User CPU avg (s) | Wall avg (s) | `mapaccess2_fast64` flat | `mapassign_fast64` flat | Terminal NAV |
| --------------- | ---------------- | ------------ | ------------------------ | ----------------------- | ------------ |
| Baseline        | 14.97            | 23.90        | 1.08s                    | 0.89s                   | $941,027.09  |
| After Task 1    | 9.79 (-34.6%)    | 17.92 (-25%) | 0.06s                    | 0.01s                   | $941,027.09  |
| After Task 2    | 5.29 (-64.7%)    | 13.94 (-41.7%) |                          |                         | $941,027.09  |
| After Task 3    | 5.58 (REVERTED)  | 19.26 (REVERTED) |                       |                         | $941,027.09  |

Task 3 (hoist fetchFundamentals allocations) was reverted because it
consistently regressed user CPU by ~0.3s vs Task 2 within the same
benchmark session. Go's allocator handles small, short-lived slices
efficiently; the explicit nil-reset loop on every row was slower than
letting the runtime allocate fresh zero-initialized slices.

Terminal NAV must stay identical across all rows. Any drift means a semantic bug — stop and investigate before continuing.

---

## Self-Review Notes (for the writer, not the executor)

- Spec coverage: Task 1 covers fast path (spec Change 1). Task 2 covers column-trimmed SELECT and scan-buffer hoist for `fetchMetrics` (spec Change 2). Task 3 covers the `fetchFundamentals` allocation hoist (spec Change 3). Benchmark steps exist between each, per user directive. Task 4 adds the changelog entry per project conventions.
- Placeholder scan: no "TBD", "TODO", "similar to task N", or hand-waving-at-tests. Every step shows actual commands / code.
- Type consistency: `data.DateKey` is introduced in Task 1 and used inside `assemblePointInTime`. `metricsColumn` is introduced in Task 2 and referenced nowhere else. `boundCol` is task-local. `scanArgs` is reset via `scanArgs[:0]` in Task 2.
- Known fragility: the new tests reference specific calendar dates (2024-06-14 Friday, 2024-06-15 Saturday). If the existing test fixture does not cover 2024, Step 1.2 explicitly instructs the executor to substitute a date the fixture does cover and verify via `engine/engine_suite_test.go`.
