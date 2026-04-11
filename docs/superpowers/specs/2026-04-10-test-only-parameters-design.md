# Test-Only Strategy Parameters

## Goal

Allow a strategy author to mark a parameter as test-only so that it is invisible to every user-facing surface (CLI flags, `describe` output, TUI, presets, study sweeps) and cannot be set from any external input. The field remains an exported Go field so test code can set it by direct struct assignment.

## Motivation

Strategies sometimes need parameters that exist purely to make the strategy testable (for example: a fixed clock, an injection point that forces a specific code path, a stub data hook). Today every exported field on a strategy struct automatically becomes a user parameter -- it appears in `describe` output, gets a cobra flag, and shows up in the TUI parameter list. Strategy authors who want testability either pollute their public surface or resort to package-private state with bespoke test hooks. A `testonly` tag fixes this by letting a single declaration mark a field as hidden from users while still letting tests set it.

## Tag

A new struct tag, `testonly`, is added to the recognized strategy tag set. The value must parse as a Go boolean. Presence with `"true"` marks the field as test-only; presence with `"false"` is equivalent to omitting the tag. Any other value is a programming error and panics the first time the strategy is reflected over.

```go
type MyStrategy struct {
    Lookback float64   `pvbt:"lookback" desc:"Lookback in months" default:"6.0"`
    Now      time.Time `pvbt:"now" testonly:"true"`
}
```

The `testonly:"true"` form matches the existing pvbt tag style (`pvbt:"name"`, `desc:"text"`, `default:"value"`, `suggest:"..."`), and leaves the `"false"` form available so a derived strategy could in principle un-mark an inherited field.

## Detection helper

A single helper in `engine/parameter.go` is the source of truth:

```go
func IsTestOnlyField(field reflect.StructField) bool {
    raw, ok := field.Tag.Lookup("testonly")
    if !ok {
        return false
    }
    val, err := strconv.ParseBool(raw)
    if err != nil {
        panic(fmt.Sprintf("strategy field %s: invalid testonly tag %q: %v",
            field.Name, raw, err))
    }
    return val
}
```

A panic is appropriate because an invalid `testonly` value is a bug in strategy source code, caught the first time the strategy struct is reflected over (during `Setup` or describe). It is not user input, so there is no benign "soft" recovery path.

## Filter points

A test-only parameter must be filtered everywhere that walks strategy fields. There are four such places:

1. **`engine/parameter.go` `StrategyParameters()`** -- skip fields where `IsTestOnlyField` is true. This single change cascades to:
   - `engine/descriptor.go` `DescribeStrategy()`, which feeds the `describe` command, study reports, and the TUI parameter list (all consume `StrategyInfo`).
   - `cli/preset.go` preset lookup, so no `suggest` value can target a test-only field.
2. **`cli/flags.go` `registerStrategyFlags()`** -- skip test-only fields, so no cobra flag is created for them. (`flags.go` iterates struct fields directly rather than going through `StrategyParameters`, so it needs its own check via `IsTestOnlyField`.)
3. **`cli/flags.go` `applyStrategyFlags()`** -- skip test-only fields. Defensive: no flag should exist, but the field iteration here mirrors `registerStrategyFlags` and must stay consistent.
4. **`engine/apply_params.go` `ApplyParams()`** -- if the merged params map (preset values plus explicit params) contains the name of any test-only parameter, return an explicit error rather than silently setting it. This blocks `--params foo=bar` from the CLI and `Params:` entries in study sweep configs.

`applyParamValue` itself is unchanged. It is a low-level field setter; gating happens at the boundaries that decide whether a value should reach it.

## Tests use direct assignment

Test-only parameters remain exported Go fields. Tests construct the strategy and set them directly:

```go
strategy := &MyStrategy{Now: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)}
```

No new test API is introduced. The `testonly` tag exists to hide the field from users, not to add a new way of setting it.

## Tests for the feature

The following tests must be added in their respective packages:

- `engine/parameter_test.go`: a strategy with a `testonly:"true"` field is omitted from `StrategyParameters` output.
- `engine/parameter_test.go`: `IsTestOnlyField` returns true for `"true"`, false for `"false"`, false when the tag is absent, and panics for an unparseable value.
- `engine/descriptor_test.go`: `DescribeStrategy` omits test-only fields from `StrategyInfo.Parameters`.
- `engine/apply_params_test.go`: `ApplyParams` returns an error whose message names the offending parameter when the explicit `params` argument contains a test-only parameter name. (The preset path is structurally safe -- presets are built from `suggest:` tags on fields that have themselves been filtered out of `StrategyParameters` -- so no preset test is needed.)
- `cli/cli_test.go`: `registerStrategyFlags` does not register a cobra flag for a test-only field, and `applyStrategyFlags` is a no-op for it.
- `engine/parameter_test.go` (or a new integration-style test in the engine suite): a strategy with a test-only field set via direct struct assignment runs through a full backtest and the assigned value is observed by `Compute`.

## Out of scope

- A runtime "unhide test params" mode that exposes them as flags under a debug build. If such a thing is ever wanted, it can be added later as a separate feature.
- Loading test-only params from `pvbt.toml`, environment variables, or any other external source.
- Refactoring `cli/flags.go` to consume `StrategyParameters()` instead of duplicating struct field iteration. That cleanup is worthwhile but unrelated to this feature.
- Validation that test-only params have safe defaults; the existing `default` tag handling applies unchanged.

## Public-API impact

Per `CLAUDE.md`, the following changes are user-visible and must appear in the changelog:

- The `testonly` struct tag is added to the recognized strategy tag set.
- `engine.IsTestOnlyField` is added as a public helper.
- `engine.ApplyParams` returns a new error class when asked to set a parameter that has been declared test-only.
- The `describe` command, the TUI parameter list, and study sweep configurations no longer list or accept test-only parameters.
