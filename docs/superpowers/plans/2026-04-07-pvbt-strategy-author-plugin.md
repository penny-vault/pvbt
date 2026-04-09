# pvbt-strategy-author Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Claude Code plugin that helps authors design, write, and review pvbt quantitative trading strategies, then publish it to the marketplace.

**Architecture:** Standalone plugin repository with one agent (pvbt-strategy-reviewer), one skill (pvbt-strategy-design), and nine curated reference documents. The agent and skill are thin orchestrators; all pvbt-specific knowledge lives in the references and is loaded on demand.

**Tech Stack:** Markdown, JSON, YAML frontmatter. No build system. Git for version control. The plugin is validated by installing and dogfooding it against the Claude Code CLI.

**Working directory:** This plan executes against a **new, empty sibling directory** of the pvbt checkout: `/Users/jdf/Developer/penny-vault/pvbt-strategy-author/`. All relative paths below are relative to that directory. Paths prefixed with `pvbt:` refer to the authoritative pvbt source tree at `/Users/jdf/Developer/penny-vault/pvbt/`.

**Reference style:** Each reference file starts with a version header line: `_Last verified against pvbt 0.6.0._` Content is written for Claude, not humans — prefer working code examples over prose, omit narrative framing, include exact type and function names. Cross-link between files with relative paths.

**Commit cadence:** One commit per phase, not per task. This avoids nuisance commits while keeping history meaningful.

---

## Phase 1: Scaffold the repository

### Task 1: Initialize plugin repository

**Files:**
- Create: `.gitignore`
- Create: `LICENSE`
- Create: `README.md` (stub)
- Create: `.claude-plugin/plugin.json`

- [ ] **Step 1: Create the repository directory and initialize git**

```bash
mkdir -p /Users/jdf/Developer/penny-vault/pvbt-strategy-author
cd /Users/jdf/Developer/penny-vault/pvbt-strategy-author
git init -b main
```

- [ ] **Step 2: Create `.gitignore`**

```
.DS_Store
*.swp
.claude/settings.local.json
```

- [ ] **Step 3: Copy the Apache 2.0 LICENSE from pvbt**

```bash
cp /Users/jdf/Developer/penny-vault/pvbt/LICENSE /Users/jdf/Developer/penny-vault/pvbt-strategy-author/LICENSE
```

- [ ] **Step 4: Create `.claude-plugin/plugin.json`**

```json
{
  "name": "pvbt-strategy-author",
  "version": "0.1.0",
  "description": "Design and review pvbt quantitative trading strategies. Activates when writing Go code that imports github.com/penny-vault/pvbt.",
  "author": {
    "name": "penny-vault"
  },
  "homepage": "https://github.com/penny-vault/pvbt-strategy-author",
  "keywords": ["pvbt", "penny-vault", "quantitative-trading", "backtesting", "go"]
}
```

- [ ] **Step 5: Create stub `README.md`**

```markdown
# pvbt-strategy-author

A Claude Code plugin for authoring and reviewing [pvbt](https://github.com/penny-vault/pvbt) quantitative trading strategies.

Full documentation lives in [README.md](./README.md) after Phase 5 is complete.
```

- [ ] **Step 6: Validate plugin.json is valid JSON**

Run: `python3 -c 'import json; json.load(open(".claude-plugin/plugin.json"))' && echo OK`
Expected output: `OK`

- [ ] **Step 7: Commit Phase 1**

```bash
git add .
git commit -m "chore: scaffold plugin repository"
```

---

## Phase 2: Reference documents

Each reference task follows the same shape:
1. Write the acceptance criteria (questions the doc must answer).
2. Read the authoritative upstream pvbt docs for the topic.
3. Write the reference file with the structured outline.
4. Verify every acceptance question is answered.

Do not commit between reference tasks. Phase 2 commits once at the end.

### Task 2: `references/strategy-api.md`

**Files:**
- Create: `references/strategy-api.md`

- [ ] **Step 1: Acceptance criteria**

The doc must answer:
- What is the `engine.Strategy` interface? What three methods does it require?
- What does `Setup` do vs. `Compute`? When is each called?
- What is `Describe()`? What fields does `engine.StrategyDescription` contain?
- How does a minimal `main()` look? What does `cli.Run` provide?
- How are parameters declared via struct tags (`pvbt`, `desc`, `default`, `suggest`)?
- What import paths are canonical?
- What types is `Compute` given? Which are read-only?

- [ ] **Step 2: Read upstream**

Read:
- `pvbt:engine/strategy.go`
- `pvbt:docs/strategy-guide.md` (full file)
- `pvbt:docs/overview.md`

- [ ] **Step 3: Write `references/strategy-api.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Strategy API

## Interface
[Strategy interface verbatim with a one-line description]

## Lifecycle
[Setup once, Compute per scheduled tick, Describe declarative. Order of calls.]

## Minimal strategy
[A complete minimal strategy: package, imports, struct with one param, Name/Setup/Describe/Compute, main. Copy-paste ready.]

## StrategyDescription
[Schedule, Benchmark, Warmup, RiskFreeAsset fields with one-line semantics each.]

## Parameters
[Struct tags table: pvbt, desc, default, suggest. One example per tag.]

## What you get from cli.Run
[Subcommands: backtest, live, snapshot, describe, study, config. One line each.]

## Read-only vs. mutable
[portfolio.Portfolio is read-only. portfolio.Batch is the only way to place orders.]

## See also
[Links to scheduling.md, universes.md, portfolio-and-batch.md, parameters-and-presets.md]
```

- [ ] **Step 4: Verify acceptance**

Re-read the file. Tick off each question from step 1. If any is missing, extend the file until all are answered.

### Task 3: `references/scheduling.md`

**Files:**
- Create: `references/scheduling.md`

- [ ] **Step 1: Acceptance criteria**

The doc must answer:
- What is a tradecron expression? What fields does it have?
- What are the common named schedules (`@daily`, `@monthend`, `@monthbegin`, `@weekend`, `@weekbegin`, `@quarterend`, `@quarterbegin`)?
- What are `@open` and `@close`?
- What time zone does tradecron use? (Eastern)
- How is warmup declared? How is it measured? (trading days)
- What happens if an asset lacks enough warmup history? (strict vs. permissive mode)
- When is `Compute` called relative to the schedule?

- [ ] **Step 2: Read upstream**

Read:
- `pvbt:docs/scheduling.md`
- `pvbt:docs/strategy-guide.md` (scheduling and warmup sections)
- `pvbt:tradecron/*.go` (skim for directive names if doc is unclear)

- [ ] **Step 3: Write `references/scheduling.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Scheduling and Warmup

## tradecron expressions
[5-field cron with market-aware extensions. Table of common schedules with exact strings.]

## Time zone
[Eastern time, always. Implications for data providers.]

## Warmup
[Declared in Describe().Warmup as integer trading days. Engine validates before first Compute.]

## Strict vs. permissive mode
[Default is strict: fails fast if warmup is insufficient. Permissive shifts start date forward.]

## When Compute is called
[Scheduled tick time. For @monthend, that's the close of the last trading day of the month.]

## Common pitfalls
[Using @daily when you mean @monthend, underestimating warmup, mismatched time zones between schedule and data provider.]
```

- [ ] **Step 4: Verify acceptance**

Tick off each question. Extend as needed.

### Task 4: `references/universes.md`

**Files:**
- Create: `references/universes.md`

- [ ] **Step 1: Acceptance criteria**

- What is a Universe?
- How do you build a static universe? As a CLI field vs. in `Setup`?
- What is `USTradable()`? When should an author use it?
- What are index universes (`eng.IndexUniverse("SPX")`, etc.)? What indexes are available?
- What is a rated universe?
- How do `Window` and `At` differ?
- How does a universe handle historical membership changes? Why does this matter for survivorship?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/universes.md`
- `pvbt:universe/doc.go`
- `pvbt:universe/*.go` (scan for public API)
- `pvbt:docs/strategy-guide.md` (universe section)

- [ ] **Step 3: Write `references/universes.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Universes

## What a universe is
[One line: a group of assets, possibly time-varying.]

## Kinds
### Static
[Fixed asset list. CLI flag form and Setup form. Example of each.]

### Index-tracking
[eng.IndexUniverse with index code. List of supported indexes (SPX, NDX, DJI, RUT...). Handles historical membership.]

### USTradable convenience
[universe.USTradable() returns a curated daily-refreshed universe of liquid US stocks. Use this as the starting point for broad US equity strategies.]

### Rated
[eng.RatedUniverse with provider and rating predicate. Example.]

## Fetching data
### Window
[Signature, semantics, example. Returns a DataFrame spanning [t-window, t].]

### At
[Single point in time. Example.]

## Survivorship bias
[Why static universes are dangerous for long backtests. Index universes are the fix.]

## See also
[data-frames.md, common-pitfalls.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 5: `references/data-frames.md`

**Files:**
- Create: `references/data-frames.md`

- [ ] **Step 1: Acceptance criteria**

- What is a `data.DataFrame` indexed by?
- How do you read a single value? A column? Most recent row?
- What slicing operations exist?
- What arithmetic operations exist? How does alignment work?
- What financial calculations are built in? (`Pct`, `RiskAdjustedPct`, `Diff`, `Log`, `CumSum`, `CumMax`, `Shift`, `Covariance`)
- What aggregations are available? (`Mean`, `Sum`, `MaxAcrossAssets`, `MinAcrossAssets`, `Variance`, `Std`)
- What rolling windows and resampling operations exist?
- How is error propagation handled through chains?
- When should you prefer `RiskAdjustedPct` over `Pct`?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/data.md`
- `pvbt:docs/strategy-guide.md` (DataFrame section)
- `pvbt:data/dataframe.go` (skim public methods)

- [ ] **Step 3: Write `references/data-frames.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# DataFrames

## Shape
[Indexed by (time, asset, metric). Column-major.]

## Reading values
[Value, ValueAt, Column, Times, AssetList, MetricList examples.]

## Slicing
[Assets, Metrics, Between, Last, At, Filter. Chaining and Err().]

## Arithmetic
[Add, Sub, Mul, Div, AddScalar, MulScalar. Broadcasting rules.]

## Financial calculations
[Pct, RiskAdjustedPct, Diff, Log, CumSum, CumMax, Shift, Covariance. One-line semantics each.]

## Aggregations
[Mean, Sum, MaxAcrossAssets, MinAcrossAssets, Variance, Std. How each reshapes the DataFrame.]

## Rolling windows
[df.Rolling(n).Mean() and friends.]

## Resampling
[Downsample, Upsample.]

## Error propagation
[Operations return a new DataFrame. Errors propagate; check Err() before using the result.]

## When to use RiskAdjustedPct vs. Pct
[RiskAdjustedPct subtracts the risk-free return. Use it when the signal should not reward raw duration.]

## See also
[signals-and-weighting.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 6: `references/portfolio-and-batch.md`

**Files:**
- Create: `references/portfolio-and-batch.md`

- [ ] **Step 1: Acceptance criteria**

- What is `portfolio.Portfolio`? What methods are exposed?
- What is `portfolio.Batch`? What methods does it expose?
- How does `RebalanceTo` work? What is an `Allocation`?
- How does `batch.Order` work? What modifiers are supported (`Limit`, `Stop`, `GoodTilCancel`, `OnTheOpen`, `OnTheClose`, `WithBracket`, `OCO`, etc.)?
- How do you short? How do negative weights in `RebalanceTo` work?
- How do you read margin state (`MarginRatio`, `MarginDeficiency`, `BuyingPower`)?
- What is `MarginCallHandler`?
- Why is Portfolio read-only in `Compute`?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/portfolio.md`
- `pvbt:docs/strategy-guide.md` (trading section)
- `pvbt:portfolio/*.go` (skim public API)

- [ ] **Step 3: Write `references/portfolio-and-batch.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Portfolio and Batch

## Portfolio (read-only)
[Cash, Value, Position, PositionValue, Holdings, MarginRatio, MarginDeficiency, BuyingPower. One line each.]

## Batch
[RebalanceTo (declarative) and Order (imperative). Never write to Portfolio directly; the engine does that from accumulated batch operations.]

## Declarative: RebalanceTo
[Three-step pipeline: select, weight, execute. Selector examples (MaxAboveZero, TopN, BottomN, CountWhere). Weighting (EqualWeight). Execute (batch.RebalanceTo).]

## Imperative: Order
[Signature. Modifier table copied from upstream. Bracket/OCO examples.]

## Short selling
[Declarative: negative weights in Allocation. Imperative: Sell when holding zero/negative. Cover: Buy to close.]

## Margin
[MarginRatio, MarginDeficiency, BuyingPower semantics. When a margin call triggers. Default auto-liquidation.]

## MarginCallHandler
[Interface. Example that covers all shorts. Note: OnMarginCall batch bypasses risk middleware.]

## Allocation
[Struct definition. Members map, weights sum to 1.0 for long-only, can exceed for leverage, can be negative for shorts.]

## See also
[signals-and-weighting.md, common-pitfalls.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 7: `references/signals-and-weighting.md`

**Files:**
- Create: `references/signals-and-weighting.md`

- [ ] **Step 1: Acceptance criteria**

- What is a Selector? Which built-in selectors exist (`MaxAboveZero`, `TopN`, `BottomN`, `CountWhere`)?
- How does a selector mark chosen assets? (inserts a `Selected` column)
- What weighting functions exist? (`EqualWeight` for now; list any others)
- How do you hand-roll a signal when the built-ins don't fit?
- When should you reach for `portfolio.Months(n)` vs. raw integer days?
- What's the recommended pattern for a lookback-based momentum signal?
- How do you combine multiple metrics into one score?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/signals.md` (if present)
- `pvbt:portfolio/select*.go`, `pvbt:portfolio/weight*.go`
- `pvbt:docs/strategy-guide.md` (signals and weighting sections)

- [ ] **Step 3: Write `references/signals-and-weighting.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Signals and Weighting

## Pipeline
[select -> weight -> execute]

## Built-in selectors
### MaxAboveZero
[Signature, behavior, example. Note the fallback DataFrame parameter.]

### TopN, BottomN
[Signatures and examples.]

### CountWhere
[For canary-style signals. Example with NaN filtering.]

## Built-in weighting
### EqualWeight
[Signature and example.]

## Writing a custom signal
[When to hand-roll. Example: normalized z-score of a custom metric. Emphasize reuse of DataFrame methods rather than raw loops.]

## Recommended idioms
[Momentum: df.Pct(n).Last(). Risk-adjusted: df.RiskAdjustedPct(n).Last(). Rank: df.Rolling(n).Mean() then TopN.]

## Lookback helpers
[portfolio.Months(n) vs raw integers. Prefer the helper.]

## Combining metrics
[Arithmetic across DataFrames is aligned by (asset, metric). Example: 60% momentum + 40% low-vol.]

## See also
[data-frames.md, portfolio-and-batch.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 8: `references/parameters-and-presets.md`

**Files:**
- Create: `references/parameters-and-presets.md`

- [ ] **Step 1: Acceptance criteria**

- Which Go types can become CLI parameters?
- What struct tags are recognized (`pvbt`, `desc`, `default`, `suggest`)?
- How do you declare a preset? What does `describe --json` emit for presets?
- How do you pick a preset at runtime? (`--preset Name`)
- How should you name parameters for readability?
- When should you expose a parameter at all vs. hard-coding it?
- How does overfitting risk relate to parameter count?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/strategy-guide.md` (parameters section)
- `pvbt:docs/configuration.md`
- `pvbt:cli/*.go` (skim for tag parsing)

- [ ] **Step 3: Write `references/parameters-and-presets.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Parameters and Presets

## Supported types
[Table: int, float64, string, bool, time.Duration, universe.Universe. CLI example for each.]

## Struct tags
[pvbt, desc, default, suggest. Example block with all four.]

## Naming
[Kebab-case at the CLI, exported in Go. pvbt tag overrides the derived name.]

## Presets
[Declared via suggest tag. Multiple presets per field. Resolved by --preset flag.]

## When to expose a parameter
[Expose only values the user will plausibly tune: lookback, universe, rebalance threshold. Do not expose internal magic numbers.]

## Overfitting and parameter count
[Every exposed parameter is a tuning knob and a potential source of overfitting. Prefer fewer parameters with sensible defaults and well-named presets over many parameters with no guidance.]

## describe output
[./strategy describe and describe --json. Use these to verify parameter declarations.]

## See also
[strategy-api.md, common-pitfalls.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 9: `references/testing-strategies.md`

**Files:**
- Create: `references/testing-strategies.md`

- [ ] **Step 1: Acceptance criteria**

- How do you capture a snapshot file with `./strategy snapshot`?
- How do you replay a snapshot in a unit test?
- What Ginkgo patterns are conventional for strategy tests?
- How do you write a regression test that runs a short backtest and asserts on the final equity curve?
- How do you mock external data providers? (answer: use snapshots, not mocks)
- How do you structure tests so they don't hit the internet?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/strategy-guide.md` (testing with snapshots section)
- `pvbt:engine/*_test.go` (scan for patterns)
- pvbt CLAUDE.md (Ginkgo conventions)

- [ ] **Step 3: Write `references/testing-strategies.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Testing Strategies

## Snapshot-based testing
[Capture with ./strategy snapshot. Store the resulting .db under testdata/.]

## Replaying a snapshot
[Use data.NewSnapshotProvider or equivalent to replay. Concrete code example.]

## Ginkgo conventions
[BDD style. _suite_test.go per package. Redirect zerolog to GinkgoWriter. Example Describe/Context/It.]

## Regression test pattern
[Short backtest over a fixed window, assert on final NAV within a tolerance.]

## Do not mock data providers
[Use snapshots. Mocks diverge from reality.]

## Offline guarantee
[Tests must never touch the network. Snapshot files are committed to the repo (or fetched by a Makefile target before tests run).]

## See also
[common-pitfalls.md]
```

- [ ] **Step 4: Verify acceptance**

### Task 10: `references/common-pitfalls.md`

**Files:**
- Create: `references/common-pitfalls.md`

- [ ] **Step 1: Acceptance criteria**

- What is survivorship bias and how does it manifest in pvbt strategies?
- What is lookahead bias and how does it manifest?
- What are the symptoms of leaked state between Compute calls?
- What are common warmup mistakes?
- What are common schedule / time zone mistakes?
- What does over-parameterization look like?
- What silent failures should authors avoid?
- What logging mistakes are common?

- [ ] **Step 2: Read upstream**

- `pvbt:docs/strategy-guide.md` (any pitfalls section)
- pvbt CLAUDE.md (error handling rules)
- `pvbt:docs/common-pitfalls.md` (if present)

- [ ] **Step 3: Write `references/common-pitfalls.md`**

Structure:
```
_Last verified against pvbt 0.6.0._

# Common Pitfalls

## Survivorship bias
Symptom: static universe with a curated ticker list used for historical backtests. Fix: use eng.IndexUniverse or USTradable.

## Lookahead bias
Symptom: the strategy runs at @monthend and uses data that has not settled by the close. Fix: move the schedule to @monthbegin (next month) or use metrics that are final at close.

## Leaked state between Compute calls
Symptom: strategy struct fields hold stateful accumulators that were never reset. Compute must be idempotent given the same (portfolio, batch) inputs. Fix: prefer deriving everything from the portfolio query API. If you must hold state, document why and reset it explicitly.

## Insufficient warmup
Symptom: the lookback is 252 trading days but Warmup is 126. The engine may run but the signal is zero or NaN at the start. Fix: Warmup should be at least as large as the longest lookback.

## Time zone mismatches
Symptom: schedule fires at the wrong time. All schedules are Eastern. Data providers must also be Eastern.

## Over-parameterization
Symptom: dozens of knobs that were added during backtest tuning. Every knob is an overfitting opportunity. Fix: remove any parameter you cannot justify from first principles.

## Silent failures
Symptom: errors in Compute are logged and nil is returned, letting the backtest continue with stale or missing data. Fix: return the error. The engine handles it.

## Logging
Symptom: fmt.Println or log package instead of zerolog. Fix: zerolog.Ctx(ctx) inside Compute.

## See also
[All other reference files cross-link here.]
```

- [ ] **Step 4: Verify acceptance**

### Task 11: Commit Phase 2

- [ ] **Step 1: Verify all nine reference files exist and start with a version header**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt-strategy-author
ls references/
head -1 references/*.md
```

Expected: nine files, each beginning with `_Last verified against pvbt 0.6.0._`.

- [ ] **Step 2: Commit Phase 2**

```bash
git add references/
git commit -m "docs: add curated pvbt reference material"
```

---

## Phase 3: Design skill

### Task 12: `pvbt-strategy-design` skill

**Files:**
- Create: `skills/pvbt-strategy-design/SKILL.md`

- [ ] **Step 1: Acceptance scenarios**

Write down three natural-language strategy descriptions and what the skill should do with each. These become the validation bar.

Scenario A (complete description, no gaps):
> "Monthly momentum rotation across SPY, EFA, EEM, 6-month lookback, fall back to SHY."

Expected: extract all slots, flag no gaps, one warning about the default risk-free asset not being explicitly stated (informational, not blocking).

Scenario B (partial description, one real gap):
> "I want to buy the cheapest 10% of US stocks by P/E and rebalance quarterly."

Expected: extract universe (USTradable), schedule (@quarterend), signal (PE ratio), selection (BottomN at 10% of universe size), weighting (EqualWeight default), warmup (derived). Gap to ask: lookback for the P/E measure? Daily snapshot? Trailing 12-month? Red flag: must check lookahead if using trailing fundamentals.

Scenario C (description with a red flag):
> "Historical backtest of holding AAPL, MSFT, GOOG, and AMZN equal-weighted from 2000 to today."

Expected: extract everything cleanly. Red flag: survivorship bias — the asset list was selected with knowledge of their future performance. Flag this before proceeding.

- [ ] **Step 2: Write `skills/pvbt-strategy-design/SKILL.md`**

Structure:
```yaml
---
name: pvbt-strategy-design
description: Use when designing, brainstorming, or drafting a pvbt quantitative trading strategy. Extracts strategy intent from the author's description, fills in sensible defaults, and only pauses the brainstorm for genuine ambiguity or risk.
---
```

Body:
```
# pvbt-strategy-design

This skill supplements `superpowers:brainstorming` with pvbt-specific knowledge.
It does NOT run its own flow; brainstorming still owns the conversation.
This skill changes brainstorming's behavior from "ask about every slot" to
"fill what you can from the description, ask only about gaps and risks".

## When this skill fires

Fires alongside brainstorming when the user describes a pvbt strategy.
A typical cue is the user saying "I want to build a strategy that ...".

## Strategy schema

Every pvbt strategy fills these slots:
1. Universe: static / index-tracking / rated.
2. Schedule: tradecron expression.
3. Signal: data metric plus computation.
4. Selection: which subset of the universe gets held.
5. Weighting: how capital is distributed among selected assets.
6. Warmup: historical data required before first Compute.
7. Benchmark and risk-free asset.
8. CLI parameters to expose.
9. Presets for named configurations.
10. Risk management rules.

## Extraction rules

Natural language -> slot value:
- "daily" -> @daily
- "weekly" / "every week" -> @weekbegin or @weekend (ask only if ambiguous)
- "monthly" / "every month" -> @monthend
- "quarterly" -> @quarterend
- "annually" -> @quarterend repeated; confirm
- "N-month lookback" -> warmup ~= N * 21 trading days
- "N-day lookback" -> warmup ~= N trading days
- "rotate into the best" -> MaxAboveZero selection + EqualWeight
- "top K" -> TopN selection + EqualWeight
- "bottom K" / "cheapest" -> BottomN selection + EqualWeight
- "momentum" unqualified -> total return over the stated lookback
- "risk-adjusted momentum" -> RiskAdjustedPct
- "US stocks" / "liquid US stocks" -> USTradable()
- "S&P 500" -> eng.IndexUniverse("SPX")
- "Nasdaq 100" -> eng.IndexUniverse("NDX")

## Default table

Apply when the author did not specify. Mark each default in the design doc as `(default)` so the author can override.

| Slot | Default |
|------|---------|
| Benchmark | First asset in the primary universe if short; SPY if broad US equity |
| Risk-free asset | Auto-resolved DGS3MO |
| Weighting | EqualWeight |
| Warmup | Derived from the longest declared lookback |
| Schedule | @monthend if "rebalance" is mentioned without a cadence |
| Selection | MaxAboveZero if "rotate" is the verb; TopN(1) if "pick the best" |
| Parameters | Lookback, risk-on universe, risk-off universe — the stated knobs of the idea |
| Presets | None unless named variants are mentioned |

## Ambiguity triggers

Pause the brainstorm and ask exactly one question when:
1. Signal math is vague — could be raw return, risk-adjusted, or price-vs-MA.
2. Selection count is undeclared for a "top N" style strategy.
3. Rebalance cadence is neither stated nor implied.
4. Exit rules are mentioned but not detailed (stop-loss, trailing drawdown).
5. The universe is described but could be static or index-tracking (ask which).

Do not ask about slots where a default is safe. Note the default in the design doc instead.

## Red flag triggers

Warn the author proactively, without waiting to be asked, when:
1. The description names specific historical tickers for a backtest (survivorship bias).
2. The trigger fires at a time when the data it needs has not settled (lookahead bias).
3. The parameter count exceeds three or four without clear justification (overfitting risk).
4. The warmup implied by the lookback exceeds the backtest window.
5. The universe is a static list that excludes common failure cases.

For each red flag, cite the relevant reference file:
`references/common-pitfalls.md#survivorship-bias`, etc.

## Output contract

When `superpowers:brainstorming` writes the design doc at
`docs/superpowers/specs/YYYY-MM-DD-<topic>-design.md`, this skill ensures the doc
contains a section titled `## pvbt strategy spec` with one line per schema slot:

```
## pvbt strategy spec

- **Universe:** USTradable() (default)
- **Schedule:** @quarterend
- **Signal:** trailing 12-month P/E ratio (asked: lookback window was not specified)
- **Selection:** BottomN(N), where N = 10% of the universe size
- **Weighting:** EqualWeight (default)
- **Warmup:** 252 trading days
- **Benchmark:** SPY (default)
- **Risk-free asset:** DGS3MO (default)
- **Parameters:** PercentileCut (default 0.10), RebalanceSchedule
- **Presets:** none
- **Risk management:** none (note: consider a stop-loss for single-name concentration)

### Flagged risks
- Lookahead bias: trailing P/E uses reported earnings; verify data provider supplies only post-announcement values.
```

The main Claude context then uses this section plus `references/` to generate code.
No separate code-generation agent exists.

## See also

- references/strategy-api.md
- references/scheduling.md
- references/universes.md
- references/signals-and-weighting.md
- references/common-pitfalls.md
```

- [ ] **Step 3: Verify the skill handles the three acceptance scenarios**

Dry-run each scenario by reading the skill as Claude would. For each scenario, confirm:
- All slots that the skill says it can extract are actually extractable from the text.
- The ambiguity triggers fire exactly where expected.
- The red flag triggers fire exactly where expected.
- The output contract produces a well-formed `## pvbt strategy spec` block.

If any scenario produces the wrong behavior, revise the extraction rules, defaults, or triggers until all three scenarios pass.

- [ ] **Step 4: Commit Phase 3**

```bash
git add skills/
git commit -m "feat: add pvbt-strategy-design skill"
```

---

## Phase 4: Reviewer agent

### Task 13: `pvbt-strategy-reviewer` agent

**Files:**
- Create: `agents/pvbt-strategy-reviewer.md`

- [ ] **Step 1: Acceptance scenarios**

Write three known-bad strategy snippets and the findings a correct reviewer must produce.

Scenario A (correctness bug):
```go
func (s *Foo) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    df, err := s.Assets.Window(ctx, data.Months(6), data.MetricClose)
    if err != nil {
        log.Error().Err(err).Msg("data fetch failed")
        return nil
    }
    // ...
}
```

Expected finding: Correctness — error is logged and nil returned, silencing the failure. Fix: return the wrapped error.

Scenario B (idiom issue):
```go
func (s *Foo) Compute(ctx context.Context, eng *engine.Engine, port portfolio.Portfolio, batch *portfolio.Batch) error {
    df, err := s.Assets.Window(ctx, data.Months(6), data.MetricClose)
    if err != nil { return err }
    last := df.Last()
    first := df.At(df.Times()[0])
    returns := make(map[asset.Asset]float64)
    for _, a := range df.AssetList() {
        returns[a] = (last.Value(a, data.MetricClose) / first.Value(a, data.MetricClose)) - 1.0
    }
    // ... hand-rolled selection and weighting
}
```

Expected finding: Idiom — hand-rolled return calculation, selection, and weighting. Fix: `df.Pct(df.Len()-1).Last()`, then `MaxAboveZero` + `EqualWeight`.

Scenario C (quant red flag):
```go
type Foo struct {
    Assets universe.Universe `pvbt:"assets" default:"AAPL,MSFT,GOOG,AMZN,META"`
}

func (s *Foo) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{ Schedule: "@monthend", Warmup: 252 }
}
```

Expected finding: Quant red flag — hand-picked mega-cap list used as a static universe implies survivorship bias in any historical backtest. Fix: use USTradable with a liquidity filter or eng.IndexUniverse("SPX").

- [ ] **Step 2: Write `agents/pvbt-strategy-reviewer.md`**

Structure:
```yaml
---
name: pvbt-strategy-reviewer
description: Use this agent when Go code implementing a pvbt strategy has been written or modified. It reviews recently changed strategy code for correctness, pvbt idioms, and quant red flags such as survivorship bias and lookahead bias. Invoke after strategy edits. Examples:\n\n- user: "Write a momentum rotation strategy across SPY, EFA, EEM."\n  assistant: [writes strategy]\n  Since pvbt strategy code was written, use the pvbt-strategy-reviewer agent to review it.\n  assistant: "Let me use the pvbt-strategy-reviewer agent to check this against pvbt best practices."\n\n- user: "I refactored the signal calculation in my pvbt strategy."\n  assistant: [reads the change]\n  Since pvbt strategy code was modified, use the pvbt-strategy-reviewer agent.\n  assistant: "I'll run the pvbt-strategy-reviewer agent on the refactored code."
tools: Bash, Glob, Grep, Read, WebFetch
model: opus
---
```

Body:
```
You are an expert pvbt strategy reviewer. You know the pvbt author-facing API
deeply: the Strategy interface, Universe, DataFrame, Portfolio, Batch, schedules,
warmup, signals, weighting, parameters, presets, and the common pitfalls that
trip up quantitative strategy authors. You never use emoji.

Your purpose is to review recently written or modified pvbt strategy code. You
focus on changed code, not the entire codebase.

## Knowledge base

Your authoritative pvbt knowledge lives in the plugin's `references/` directory.
Read only the references that apply to what the strategy actually uses. Do not
read the entire reference set on every review.

- references/strategy-api.md
- references/scheduling.md
- references/universes.md
- references/data-frames.md
- references/portfolio-and-batch.md
- references/signals-and-weighting.md
- references/parameters-and-presets.md
- references/testing-strategies.md
- references/common-pitfalls.md

## Identification

A strategy file imports `github.com/penny-vault/pvbt/engine` and defines a type
with methods `Name`, `Setup`, and `Compute`. It typically also defines
`Describe` and a `main` that calls `cli.Run`. If no such file exists in the
changed set, report "no pvbt strategy found in the changed code" and stop.

## Review protocol

Perform three distinct passes and report findings in three sections.

### Pass 1: Correctness

Check:
- The Strategy interface is fully and correctly implemented (method names,
  signatures, receiver types).
- Errors from data fetches, rebalances, and order placement are wrapped and
  returned. Errors are not silently swallowed or downgraded to nil.
- The portfolio is treated as read-only. Orders go through batch.RebalanceTo
  or batch.Order only.
- Warmup is declared in Describe() when the strategy needs historical data.
  The warmup value is at least as large as the longest lookback used in Compute.
- Schedule is declared and syntactically valid.
- Context is threaded through to data fetches and batch operations.
- Logging uses zerolog pulled from the context, not fmt or the standard log.

Cite each finding as `file:line`. Link to the exact section of the relevant
reference file.

### Pass 2: pvbt idiom

Check:
- Built-in signal and weighting functions are used where they apply:
  EqualWeight, MaxAboveZero, TopN, BottomN, df.Pct, df.RiskAdjustedPct,
  portfolio.Months. Hand-rolled equivalents are findings.
- Configuration is declarative via Describe() rather than imperative in Setup,
  unless there is a reason.
- Universe operations use Window or At, not manual date arithmetic.
- Named variants are expressed as presets via struct tags, not branches in Setup.
- Signal lookbacks use portfolio.Months or data.Months helpers.

Cite each finding as `file:line`.

### Pass 3: Quant red flags

Check:
- Survivorship bias: a static universe containing named tickers used as if it
  represented historical reality. Suggest USTradable or an index universe.
- Lookahead bias: decisions at time t that use data only available after t.
  Pay special attention to schedules near market close and to trailing
  fundamental metrics.
- Leaked state: mutable fields on the strategy struct that accumulate across
  Compute calls without explicit intent.
- Insufficient warmup: Describe().Warmup less than the longest lookback used.
- Missing benchmark or risk-free asset when the strategy relies on metrics that
  need them.
- Over-parameterization: more than three or four exposed parameters without
  justification suggests overfitting.

Cite each finding as `file:line`.

## Output format

Open with a one-line summary.

Then three sections: `## Correctness`, `## Idiom`, `## Quant red flags`.
Under each section, list findings in severity order:
```
- **<short title>** [file:line]
  <what is wrong, one sentence>
  <concrete fix, with a code block if it helps>
  Reference: [<ref-file>#<anchor>]
```

If a section has no findings, write `No findings.` under the heading.

Close with a "Good practices observed" section listing any notably idiomatic
code, no more than three bullets.

## Memory

Update your agent memory with patterns you see across reviews in this
project: recurring anti-patterns, project-specific idioms, and strategy
families you have reviewed before. Do not bloat memory with per-review detail.

## What you do not do

- Do not rewrite the strategy from scratch. Your job is to find and explain,
  not to replace.
- Do not suggest style changes that are personal preference. Stick to findings
  grounded in a reference file.
- Do not cross-examine unrelated code. Review changed code.
```

- [ ] **Step 3: Verify the agent handles the three acceptance scenarios**

Dry-run each scenario by reading the agent prompt as Claude would against each
snippet. Confirm the agent produces:
- Scenario A: a Correctness finding about silent error swallowing.
- Scenario B: Idiom findings about hand-rolled return calculation and missing
  built-in selection/weighting.
- Scenario C: a Quant red flag about survivorship bias in the static mega-cap
  list.

If any expected finding is missing, revise the relevant pass of the agent
prompt until all three scenarios produce the expected findings.

- [ ] **Step 4: Commit Phase 4**

```bash
git add agents/
git commit -m "feat: add pvbt-strategy-reviewer agent"
```

---

## Phase 5: README and polish

### Task 14: Write the public README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write the full README**

Overwrite the stub `README.md` with a complete user-facing document:

```markdown
# pvbt-strategy-author

A Claude Code plugin for authoring and reviewing [pvbt](https://github.com/penny-vault/pvbt) quantitative trading strategies.

## What it does

- **Design** a new strategy by describing it in natural language. The `pvbt-strategy-design` skill extracts intent, fills sensible defaults, and only asks about genuine gaps or risks.
- **Review** an existing strategy with the `pvbt-strategy-reviewer` agent. It checks correctness, pvbt idiom, and quant red flags such as survivorship and lookahead bias.

## Requirements

- Claude Code.
- pvbt 0.6.0 or newer.

## Install

Install from the Claude Code marketplace:

    /plugin install pvbt-strategy-author

Or install from source:

    git clone https://github.com/penny-vault/pvbt-strategy-author
    /plugin install ./pvbt-strategy-author

## Usage

### Designing a strategy

Describe your strategy idea in natural language. The skill activates automatically:

> "I want a monthly momentum rotation across SPY, EFA, EEM with a 6-month lookback, falling back to SHY when nothing is positive."

The skill extracts your intent, fills defaults, and only asks clarifying questions for genuine ambiguity.

### Reviewing a strategy

After writing strategy code, the reviewer agent runs automatically on changed files. You can also invoke it explicitly:

> "Review my strategy."

The reviewer returns a structured report with three sections: Correctness, Idiom, and Quant red flags.

## How it works

- `agents/pvbt-strategy-reviewer.md` — the reviewer agent definition.
- `skills/pvbt-strategy-design/SKILL.md` — the design skill definition.
- `references/` — curated pvbt reference material consumed by the agent and skill on demand.

The plugin carries its pvbt knowledge with it. It does not require the pvbt source tree to be available at runtime.

## Versioning

This plugin uses semantic versioning. Major releases track breaking pvbt API changes. Minor releases add capabilities. Each reference file declares the pvbt version it was last verified against.

## License

Apache 2.0. See [LICENSE](./LICENSE).
```

- [ ] **Step 2: Commit Phase 5**

```bash
git add README.md
git commit -m "docs: write public README"
```

---

## Phase 6: Dogfood validation

### Task 15: Validate against a real strategy

**Files:**
- Create: `validation/momentum-rotation.go` (disposable)
- Create: `validation/notes.md`

- [ ] **Step 1: Write a minimal real strategy for validation**

Create `validation/momentum-rotation.go` based on the canonical example from `pvbt:docs/strategy-guide.md`. It should be the ideal, idiomatic version.

- [ ] **Step 2: Run the reviewer against the ideal strategy**

Dispatch a fresh Claude Code session (or subagent) with the `pvbt-strategy-reviewer` agent on `validation/momentum-rotation.go`.

Expected output: no findings in any of the three sections, or only a single informational "good practices observed" note.

- [ ] **Step 3: Write a degraded version**

Modify `validation/momentum-rotation.go` to introduce one issue from each pass:
- Correctness: log-and-return-nil on the data fetch error.
- Idiom: hand-roll the momentum calculation instead of using `df.Pct`.
- Quant red flag: change the universe to a static mega-cap list.

- [ ] **Step 4: Re-run the reviewer**

Dispatch again. Expected output: three findings, one per pass, each citing the correct file:line and reference section.

- [ ] **Step 5: Record results in `validation/notes.md`**

Structure:
```
# Validation notes

## Ideal strategy run
<date>
<findings summary>

## Degraded strategy run
<date>
<findings summary>

## Adjustments made to the plugin
<list of any reference or prompt edits prompted by validation>
```

- [ ] **Step 6: Commit Phase 6**

```bash
git add validation/
git commit -m "test: add dogfood validation against real strategy"
```

---

## Phase 7: Marketplace submission

### Task 16: Prepare for marketplace submission

**Files:**
- Modify: `README.md` (add marketplace badge after acceptance)
- Modify: `.claude-plugin/plugin.json` (bump to 0.1.0 release tag)

- [ ] **Step 1: Push the repository to GitHub**

```bash
gh repo create penny-vault/pvbt-strategy-author --public --source=. --remote=origin --push
```

- [ ] **Step 2: Tag the 0.1.0 release**

```bash
git tag -a v0.1.0 -m "initial release"
git push origin v0.1.0
```

- [ ] **Step 3: Submit to the Claude Code marketplace**

Follow the marketplace submission process (docs vary — check the current submission guide at the time of release). Include:
- Plugin repository URL.
- One-paragraph description.
- Screenshot or sample output from the reviewer.
- Minimum Claude Code version.

- [ ] **Step 4: Record the submission outcome in CHANGELOG.md**

Create a top-level `CHANGELOG.md`:
```
# Changelog

## 0.1.0 — 2026-04-07

Initial release.

- The pvbt-strategy-design skill extracts strategy intent from natural-language descriptions, fills sensible defaults, and pauses the brainstorm only for genuine ambiguity or flagged risk.
- The pvbt-strategy-reviewer agent reviews strategy code for correctness, pvbt idiom, and quant red flags such as survivorship and lookahead bias.
- Nine curated reference documents derived from pvbt 0.6.0 ship with the plugin and are loaded on demand by the agent and skill.
```

- [ ] **Step 5: Commit Phase 7**

```bash
git add CHANGELOG.md README.md .claude-plugin/plugin.json
git commit -m "chore: prepare 0.1.0 release"
git push
```

---

## Self-review checklist

Before declaring the plan done, verify:

- [ ] Every slot in the spec's "Reviewer agent" section is covered by a specific check in Task 13.
- [ ] Every slot in the spec's "Design skill" section is covered by a specific rule or default in Task 12.
- [ ] Every reference file in the spec's "Plugin layout" appears as a task.
- [ ] Every acceptance criterion question has a clear home in the corresponding reference file.
- [ ] No step contains placeholder text like "TBD", "fill in later", or "similar to above".
- [ ] Commit cadence is one per phase (Phases 1–7), not one per task.
- [ ] Plugin version in `plugin.json` is `0.1.0` and every reference file header declares `pvbt 0.6.0`. The two values are different concepts — the plugin version is the plugin's own semver, and the reference version is the upstream pvbt release the docs were verified against.
