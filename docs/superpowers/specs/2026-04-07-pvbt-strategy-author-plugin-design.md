# pvbt-strategy-author Claude Code Plugin

## Goal

Ship a Claude Code plugin to the marketplace that helps authors write and review pvbt quantitative trading strategies. The plugin targets strategy authors working in their own repositories (not pvbt contributors), so it must carry its pvbt-specific knowledge with it and not assume the pvbt source tree is available at runtime.

The plugin delivers two pieces of value:

1. **Design-phase help** during brainstorming: extract strategy intent from natural-language descriptions, fill in sensible defaults, and only pause on genuine ambiguity or risk.
2. **Review-phase help** after code is written: catch correctness bugs, non-idiomatic pvbt usage, and quant red flags such as survivorship and lookahead bias.

## Non-goals

- Code generation is not a separate plugin component. After the design phase produces a filled-out spec, the main Claude context generates code using the plugin's shipped reference material.
- No slash commands. Auto-activation via description fields is the only entry point in the first release. If discoverability proves weak, commands can be added later.
- No CI integration or automated pull request review.
- No multiple specialized reviewer agents. One reviewer with three distinct passes is simpler and matches the shape of `effective-go-reviewer`.
- The plugin is not a replacement for pvbt documentation. Upstream `docs/*` remains authoritative; plugin references are derived.

## Repository and distribution

- Standalone repository, separate from `penny-vault/pvbt`.
- Published to the Claude Code marketplace as `pvbt-strategy-author`.
- Versioning independent of pvbt, but README declares minimum and recommended pvbt versions.
- Semantic versioning:
  - Major: pvbt has a breaking API change that forces plugin updates.
  - Minor: pvbt adds a capability the plugin should surface, or plugin gains a meaningful new check.
  - Patch: plugin-only fixes (prompt wording, reference doc corrections, bug fixes).

## Plugin layout

```
pvbt-strategy-author/
  .claude-plugin/
    plugin.json
  README.md
  agents/
    pvbt-strategy-reviewer.md
  skills/
    pvbt-strategy-design/
      SKILL.md
  references/
    strategy-api.md
    universes.md
    data-frames.md
    scheduling.md
    portfolio-and-batch.md
    signals-and-weighting.md
    parameters-and-presets.md
    testing-strategies.md
    common-pitfalls.md
```

The agent and the skill are thin orchestrators. All pvbt-specific knowledge lives in `references/` and is loaded on demand. This keeps prompts short, lets reference content evolve without touching orchestration, and makes audit and review of the plugin tractable.

## Reviewer agent

### Activation

The agent description must name pvbt explicitly so Claude invokes it only when Go code imports `github.com/penny-vault/pvbt/...` or implements the `engine.Strategy` interface. The description includes usage examples showing it being invoked after strategy code is written or modified.

### Review protocol

1. Identify strategy files in the working set. A strategy file imports pvbt and defines a type implementing `Name()`, `Setup()`, and `Compute()`.
2. Read only the reference files relevant to what the strategy actually uses. A strategy with a static universe does not need `universes.md` loaded; a strategy using `IndexUniverse` does. This keeps each review bounded in context size.
3. Perform three review passes, report three sections in the output:
   - **Correctness**
     - Interface fully implemented. No misspelled method names. Return type signatures match.
     - Errors are wrapped with context and returned. Errors are not silently swallowed or downgraded to nil returns.
     - The portfolio is treated as read-only. Orders go through `batch.RebalanceTo` or other batch methods.
     - Warmup is declared in `Describe()` when the strategy needs historical data.
     - Schedule is declared and valid.
     - Resources such as contexts are threaded correctly.
   - **Idiom**
     - Built-in signal and weighting functions used where available: `EqualWeight`, `MaxAboveZero`, `df.Pct`, `df.RiskAdjustedPct`, `portfolio.Months`, and similar.
     - Declarative `Describe()` preferred over imperative configuration in `Setup`.
     - Universe `Window` and `At` used instead of manual date arithmetic.
     - Presets declared via struct tags for named variants rather than hand-maintained configuration branches.
     - Logging uses zerolog pulled from the context.
   - **Quant red flags**
     - Survivorship bias: a static universe used as if it represented historical reality.
     - Lookahead bias: decisions at time `t` that use data only available after `t`.
     - Leaked state between `Compute` calls: mutable fields on the strategy struct that accumulate across invocations without intent.
     - Insufficient warmup relative to the declared lookback.
     - Missing benchmark or risk-free asset when the strategy relies on metrics that need them.
     - Parameter counts and preset counts indicative of overfitting.
4. Each finding cites `file:line` and links to the specific reference document that explains the principle. Findings are ordered by severity within each section.

### Agent memory

The reviewer uses agent memory to record recurring patterns and anti-patterns it sees across reviews in a given project. This builds institutional knowledge that makes subsequent reviews more targeted without bloating the base prompt.

## Design skill

### Role

The skill is a mental model plus a default table plus a gap detector. It does not run its own brainstorming flow. The `superpowers:brainstorming` skill continues to own the design conversation. The pvbt design skill changes brainstorming's behavior from "ask about every slot" to "fill what you can from the description, ask only about gaps and risks."

### Activation

The skill's frontmatter description matches when the user is designing, describing, or drafting a pvbt quantitative trading strategy. It fires alongside brainstorming.

### Contents

1. **Strategy schema.** The slots that make up any pvbt strategy:
   - Universe (static, index-tracking, or rated)
   - Schedule (tradecron expression)
   - Signal (data metric plus computation)
   - Selection (which subset of the universe gets held)
   - Weighting (how capital is distributed among selected assets)
   - Warmup (historical data required before first compute)
   - Benchmark and risk-free asset
   - Parameters to expose to the CLI
   - Presets for named configurations
   - Risk management rules
2. **Extraction rules.** How to map natural-language phrasing to slot values. Examples:
   - "monthly" maps to `@monthend`
   - "quarterly" maps to `@quarterend`
   - "N-month lookback" implies warmup of approximately `N * 21` trading days
   - "rotate into the best" implies `MaxAboveZero` selection plus `EqualWeight`
   - "equal weight across top K" implies `TopK` selection plus `EqualWeight`
   - "momentum" without further qualification means total return over the lookback
   - "risk-adjusted" or "Sharpe-like" means `RiskAdjustedPct`
3. **Default table.** What each slot falls back to when the author does not specify it. Defaults appear in the design doc with a "(default)" marker so the author can override. Example defaults:
   - Benchmark: first asset in the primary universe
   - Risk-free asset: auto-resolved DGS3MO
   - Weighting: `EqualWeight`
   - Warmup: derived from the longest lookback mentioned
4. **Ambiguity triggers.** The short list of situations worth pausing the brainstorm to ask about:
   - Signal computation is vague (could be raw return, risk-adjusted, or something else)
   - Selection rule is under-specified (how many assets get held)
   - Rebalance cadence is not stated or implied
   - Exit rules are mentioned but not detailed
5. **Red-flag triggers.** Situations worth warning about even if the author did not ask:
   - A historical backtest described with a hand-picked ticker list (survivorship risk)
   - A decision point at market close using data that settles post-close (lookahead risk)
   - A strategy that needs many parameters to function (overfitting risk)
   - A universe-selection scheme incompatible with the declared warmup

### Output contract

When brainstorming writes its design document, the pvbt design skill ensures a "pvbt strategy spec" section is present and filled in. The section contains one line per schema slot with either the author's stated value or the default, plus any flagged ambiguities or red flags. The main Claude context then uses this section plus the `references/` files to generate code, without needing a separate code-generation agent.

## Reference content strategy

- **Curated, not verbatim.** Each file is scoped to a single task an agent or skill will perform. The audience is a capable Claude instance, not a human reader. Examples, constraints, and gotchas are prioritized over exposition.
- **Source of truth remains upstream.** pvbt's `docs/*` tree is authoritative. Plugin references are derived and may lag. Each reference file has a header line naming the pvbt version it was last verified against.
- **Cross-linked.** References use relative markdown links so Claude can navigate between them without re-reading the full set.
- **Update workflow.** After each pvbt minor release, the plugin maintainer sweeps `references/`, diffs against upstream `docs/*`, updates wording and examples, bumps the header version line, and publishes a new plugin version.

## plugin.json metadata

- `name`: `pvbt-strategy-author`
- `description`: one sentence that names pvbt, so Claude activates the plugin only in pvbt contexts.
- `version`: semantic version.
- `author`: penny-vault maintainer.
- `homepage`: link to the plugin repository.
- `keywords`: `pvbt`, `penny-vault`, `quantitative-trading`, `backtesting`.

## Open questions

None at this time. The design is ready for a plan.

## Risks and mitigations

- **Reference drift.** pvbt evolves faster than the plugin. Mitigation: version header on each reference file, scheduled sweep after every pvbt minor release, minimum-version declaration in the README.
- **Over-activation.** The skill or agent fires outside pvbt contexts. Mitigation: description fields name pvbt explicitly and require pvbt imports or the `engine.Strategy` interface as a signal.
- **Under-activation.** Authors do not realize the plugin applies. Mitigation: README includes explicit "how to invoke" examples; consider adding slash commands in a later version if telemetry suggests discovery is weak.
- **Annoying brainstorm.** The skill asks too many questions, defeating its purpose. Mitigation: the extraction-first contract is load-bearing and tested by dogfooding before the first release.
