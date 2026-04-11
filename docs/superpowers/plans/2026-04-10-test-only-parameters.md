# Test-Only Strategy Parameters Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `testonly` struct tag that lets a strategy author hide a parameter from every user-facing surface (CLI flags, `describe`, TUI, presets, study sweeps) and reject any external attempt to set it, while still letting tests assign the field directly.

**Architecture:** A single boolean tag detected by one helper, `engine.IsTestOnlyField`, that returns true when the `testonly` tag is present and parses as `true`. Filtering happens at every place that walks a strategy struct's fields: `engine.StrategyParameters` (which feeds `DescribeStrategy` and preset lookup), the three field-walking functions in `cli/flags.go`, and `engine.ApplyParams`. The field stays exported so test code constructs strategies with literal field assignment.

**Tech Stack:** Go 1.22+, Ginkgo/Gomega for tests, cobra for CLI flags, reflection-based struct tag parsing.

**Spec:** [`docs/superpowers/specs/2026-04-10-test-only-parameters-design.md`](../specs/2026-04-10-test-only-parameters-design.md)

---

## File Overview

- **Modified `engine/parameter.go`** -- adds `IsTestOnlyField` helper; `StrategyParameters` skips test-only fields.
- **Modified `engine/parameter_test.go`** -- tests for `IsTestOnlyField` and the `StrategyParameters` filter.
- **Modified `engine/apply_params.go`** -- `ApplyParams` rejects merged param names that correspond to test-only fields.
- **Modified `engine/apply_params_test.go`** -- tests for the new error.
- **Modified `engine/descriptor_test.go`** -- defense-in-depth test that `DescribeStrategy` omits test-only fields.
- **Modified `cli/flags.go`** -- `registerStrategyFlags`, `applyStrategyFlags`, and `collectParamSweeps` skip test-only fields.
- **Modified `cli/cli_test.go`** -- tests for the three flag-walker functions.
- **Modified `CHANGELOG.md`** -- single bullet under `## [Unreleased]` `### Added`.

---

## Task 1: Add `IsTestOnlyField` helper

**Files:**
- Modify: `engine/parameter.go`
- Modify: `engine/parameter_test.go`

- [ ] **Step 1: Write failing tests for the helper**

Append to `engine/parameter_test.go` (add `"strconv"` to the import block if missing):

```go
type testOnlyTagStrategy struct {
    Visible    int  `pvbt:"visible" desc:"v" default:"1"`
    HiddenTrue int  `pvbt:"hidden-true" testonly:"true"`
    HiddenFalse int `pvbt:"hidden-false" testonly:"false"`
}

func (s *testOnlyTagStrategy) Name() string                                                                                                            { return "testOnlyTag" }
func (s *testOnlyTagStrategy) Setup(_ *engine.Engine)                                                                                                  {}
func (s *testOnlyTagStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
    return nil
}

var _ = Describe("IsTestOnlyField", func() {
    fieldByName := func(name string) reflect.StructField {
        t := reflect.TypeOf(testOnlyTagStrategy{})
        field, ok := t.FieldByName(name)
        Expect(ok).To(BeTrue())
        return field
    }

    It("returns false when the testonly tag is absent", func() {
        Expect(engine.IsTestOnlyField(fieldByName("Visible"))).To(BeFalse())
    })

    It("returns true when the testonly tag is \"true\"", func() {
        Expect(engine.IsTestOnlyField(fieldByName("HiddenTrue"))).To(BeTrue())
    })

    It("returns false when the testonly tag is \"false\"", func() {
        Expect(engine.IsTestOnlyField(fieldByName("HiddenFalse"))).To(BeFalse())
    })

    It("panics when the testonly tag has an unparseable value", func() {
        type bad struct {
            Field int `pvbt:"x" testonly:"banana"`
        }
        field, ok := reflect.TypeOf(bad{}).FieldByName("Field")
        Expect(ok).To(BeTrue())
        Expect(func() { engine.IsTestOnlyField(field) }).To(PanicWith(ContainSubstring("invalid testonly tag")))
    })
})
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run: `ginkgo run --focus "IsTestOnlyField" ./engine/...`
Expected: compilation error -- `engine.IsTestOnlyField undefined`.

- [ ] **Step 3: Implement `IsTestOnlyField`**

Open `engine/parameter.go`. Add `"fmt"` and `"strconv"` to the import block (currently `"reflect"` and `"strings"`), then append this function below `ParameterName`:

```go
// IsTestOnlyField reports whether the given strategy struct field is marked as
// test-only via a `testonly:"true"` struct tag. Test-only fields are hidden
// from every user-facing surface (CLI flags, describe output, TUI, presets,
// study sweeps) and cannot be set through ApplyParams. They remain exported
// so that test code can assign them directly.
//
// The tag value must parse as a Go boolean. An unparseable value is a
// programming error in the strategy source code, so this function panics
// rather than silently treating it as false.
func IsTestOnlyField(field reflect.StructField) bool {
    raw, ok := field.Tag.Lookup("testonly")
    if !ok {
        return false
    }

    val, err := strconv.ParseBool(raw)
    if err != nil {
        panic(fmt.Sprintf("strategy field %s: invalid testonly tag %q: %v", field.Name, raw, err))
    }

    return val
}
```

- [ ] **Step 4: Run the tests and verify they pass**

Run: `ginkgo run --focus "IsTestOnlyField" ./engine/...`
Expected: PASS for all four `It` blocks.

- [ ] **Step 5: Run the linter**

Run: `make lint`
Expected: no new findings.

- [ ] **Step 6: Commit**

```bash
git add engine/parameter.go engine/parameter_test.go
git commit -m "feat(engine): add IsTestOnlyField helper for testonly tag

Detects the new testonly:\"true\" struct tag on strategy parameter
fields. Panics on unparseable values since they are programming errors
in strategy source, not user input."
```

---

## Task 2: Skip test-only fields in `StrategyParameters`

**Files:**
- Modify: `engine/parameter.go:81-108`
- Modify: `engine/parameter_test.go`

- [ ] **Step 1: Write a failing test**

Append to `engine/parameter_test.go`:

```go
var _ = Describe("StrategyParameters with testonly fields", func() {
    It("omits fields tagged testonly:\"true\"", func() {
        strategy := &testOnlyTagStrategy{}
        params := engine.StrategyParameters(strategy)

        // Visible and HiddenFalse remain; HiddenTrue is filtered.
        Expect(params).To(HaveLen(2))
        Expect(findParam(params, "visible")).NotTo(BeNil())
        Expect(findParam(params, "hidden-false")).NotTo(BeNil())
        Expect(findParam(params, "hidden-true")).To(BeNil())
    })
})
```

- [ ] **Step 2: Run the test and verify it fails**

Run: `ginkgo run --focus "StrategyParameters with testonly" ./engine/...`
Expected: FAIL -- `params` has length 3, includes `hidden-true`.

- [ ] **Step 3: Implement the filter**

Edit `engine/parameter.go` inside the `StrategyParameters` field loop. The current loop body starts at line 81 with `field := paramType.Field(ii)`. After the existing `if !field.IsExported() { continue }` check and the existing Strategy-typed-field skip, add a new skip for test-only fields. Replace:

```go
		// Skip Strategy-typed fields -- these are children, not parameters.
		if field.Type.Implements(strategyType) ||
			(field.Type.Kind() == reflect.Pointer && field.Type.Elem().Implements(strategyType)) {
			continue
		}

		name := ParameterName(field)
```

with:

```go
		// Skip Strategy-typed fields -- these are children, not parameters.
		if field.Type.Implements(strategyType) ||
			(field.Type.Kind() == reflect.Pointer && field.Type.Elem().Implements(strategyType)) {
			continue
		}

		// Skip fields marked test-only -- they must not appear on any
		// user-facing surface.
		if IsTestOnlyField(field) {
			continue
		}

		name := ParameterName(field)
```

- [ ] **Step 4: Run the test and verify it passes**

Run: `ginkgo run --focus "StrategyParameters with testonly" ./engine/...`
Expected: PASS.

- [ ] **Step 5: Run the full engine suite to catch regressions**

Run: `ginkgo run -race ./engine/...`
Expected: PASS. The pre-existing `extracts exported fields with correct metadata` test in `parameter_test.go` still expects `HaveLen(9)` because `paramTestStrategy` has no `testonly` fields -- this should remain green.

- [ ] **Step 6: Commit**

```bash
git add engine/parameter.go engine/parameter_test.go
git commit -m "feat(engine): filter test-only fields from StrategyParameters

Test-only fields are now omitted from parameter discovery, which
cascades through DescribeStrategy and preset lookups so the describe
command, TUI, and study reports never see them."
```

---

## Task 3: Add a defense-in-depth test that `DescribeStrategy` omits test-only fields

**Files:**
- Modify: `engine/descriptor_test.go`

This task adds no production code; the cascade through `StrategyParameters` already covers `DescribeStrategy`. The test exists so a future refactor that bypasses `StrategyParameters` would be caught.

- [ ] **Step 1: Add a strategy type and a test**

Append to `engine/descriptor_test.go`:

```go
// testOnlyDescriptorStrategy has a mix of regular and test-only fields and
// implements Descriptor so it can be passed to DescribeStrategy.
type testOnlyDescriptorStrategy struct {
    Lookback int       `pvbt:"lookback" desc:"Lookback" default:"6"`
    Now      time.Time `pvbt:"now" testonly:"true"`
}

func (s *testOnlyDescriptorStrategy) Name() string                                                                                                            { return "TestOnlyDescriptor" }
func (s *testOnlyDescriptorStrategy) Setup(_ *engine.Engine)                                                                                                  {}
func (s *testOnlyDescriptorStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
    return nil
}
func (s *testOnlyDescriptorStrategy) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{ShortCode: "tod"}
}

var _ = Describe("DescribeStrategy with testonly fields", func() {
    It("omits test-only fields from StrategyInfo.Parameters", func() {
        info := engine.DescribeStrategy(&testOnlyDescriptorStrategy{})

        Expect(info.Parameters).To(HaveLen(1))
        Expect(info.Parameters[0].Name).To(Equal("lookback"))
    })
})
```

Add `"time"` to the import block at the top of `engine/descriptor_test.go` if not already present.

- [ ] **Step 2: Run the test and verify it passes**

Run: `ginkgo run --focus "DescribeStrategy with testonly" ./engine/...`
Expected: PASS (no production change needed -- the filter from Task 2 already covers this path).

- [ ] **Step 3: Commit**

```bash
git add engine/descriptor_test.go
git commit -m "test(engine): verify DescribeStrategy hides test-only fields

Defense-in-depth test so a future refactor that bypasses
StrategyParameters would be caught."
```

---

## Task 4: Skip test-only fields in `cli/flags.go` field walkers

`cli/flags.go` iterates strategy struct fields directly in three functions: `registerStrategyFlags`, `applyStrategyFlags`, and `collectParamSweeps`. None of them go through `StrategyParameters`, so each needs its own `IsTestOnlyField` check. (`collectParamSweeps` is defended in practice by the missing-flag check, but the explicit skip keeps the three walkers symmetrical.)

**Files:**
- Modify: `cli/flags.go:37-41`, `cli/flags.go:127-131`, `cli/flags.go:228-232`
- Modify: `cli/cli_test.go`

- [ ] **Step 1: Write failing tests for the three walkers**

Append to `cli/cli_test.go`:

```go
type testOnlyFlagStrategy struct {
    Lookback int `pvbt:"lookback" desc:"lookback" default:"30"`
    Seed     int `pvbt:"seed" testonly:"true"`
    Window   int `pvbt:"window" desc:"window" default:"5"`
}

func (s *testOnlyFlagStrategy) Name() string           { return "testOnlyFlag" }
func (s *testOnlyFlagStrategy) Setup(e *engine.Engine) {}
func (s *testOnlyFlagStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
    return nil
}

var _ = Describe("registerStrategyFlags with testonly fields", func() {
    It("does not register a cobra flag for a test-only field", func() {
        cmd := &cobra.Command{Use: "test"}
        strategy := &testOnlyFlagStrategy{}

        registerStrategyFlags(cmd, strategy)

        Expect(cmd.Flags().Lookup("lookback")).NotTo(BeNil())
        Expect(cmd.Flags().Lookup("window")).NotTo(BeNil())
        Expect(cmd.Flags().Lookup("seed")).To(BeNil())
    })
})

var _ = Describe("applyStrategyFlags with testonly fields", func() {
    It("leaves a test-only field untouched even if a flag is registered out of band", func() {
        cmd := &cobra.Command{Use: "test"}
        strategy := &testOnlyFlagStrategy{}

        // Manually register a flag for "seed" to simulate a flag being
        // registered out of band (e.g., by another command). The
        // test-only check inside applyStrategyFlags must still skip the
        // field, leaving it at its zero value.
        cmd.Flags().Int("seed", 999, "")

        applyStrategyFlags(cmd, strategy)
        Expect(strategy.Seed).To(Equal(0))
    })
})

var _ = Describe("collectParamSweeps with testonly fields", func() {
    It("does not collect a sweep for a test-only field even if a flag is registered out of band", func() {
        cmd := &cobra.Command{Use: "test"}
        strategy := &testOnlyFlagStrategy{}

        registerStrategyFlags(cmd, strategy)

        // Manually register a "seed" flag with range syntax. Without the
        // explicit IsTestOnlyField check inside collectParamSweeps, this
        // would be picked up as a parameter sweep.
        cmd.Flags().String("seed", "1:5:1", "")

        sweeps := collectParamSweeps(cmd, strategy)
        for _, sweep := range sweeps {
            Expect(sweep.Field).NotTo(Equal("seed"))
        }
    })
})
```

- [ ] **Step 2: Run the tests and verify they fail**

Run: `ginkgo run --focus "with testonly fields" ./cli/...`
Expected:
- `registerStrategyFlags with testonly fields` FAILS -- a cobra `Int` flag named `seed` is registered because nothing currently filters on `testonly`.
- `applyStrategyFlags with testonly fields` FAILS -- the manually registered `seed` flag is found, parsed, and `strategy.Seed` becomes 999 instead of 0.
- `collectParamSweeps with testonly fields` FAILS -- a `ParamSweep` with `Field == "seed"` is returned because the loop reaches the manually registered flag and parses `"1:5:1"` as a range.

- [ ] **Step 3: Implement the skip in `registerStrategyFlags`**

Edit `cli/flags.go`. Inside the `registerStrategyFlags` loop body, after the existing `if !field.IsExported() { continue }` check at line 39-41, add a new skip:

Replace:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		name := engine.ParameterName(field)
```

(at the top of the `registerStrategyFlags` loop) with:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		// Skip fields marked test-only -- they must not be exposed as
		// CLI flags.
		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)
```

- [ ] **Step 4: Implement the skip in `applyStrategyFlags`**

Inside the `applyStrategyFlags` loop body (around line 127-131), make the same change. Replace:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		name := engine.ParameterName(field)
```

with:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)
```

- [ ] **Step 5: Implement the skip in `collectParamSweeps`**

Inside the `collectParamSweeps` loop body (around line 228-232), make the same change. Replace:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		name := engine.ParameterName(field)
```

with:

```go
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)
```

- [ ] **Step 6: Run the focused tests and verify they pass**

Run: `ginkgo run --focus "with testonly fields" ./cli/...`
Expected: PASS for all three new `It` blocks.

- [ ] **Step 7: Run the full cli suite**

Run: `ginkgo run -race ./cli/...`
Expected: PASS. Existing `registerStrategyFlags` tests over `testStrategy` and `universeStrategy` should be unaffected (neither uses `testonly`).

- [ ] **Step 8: Commit**

```bash
git add cli/flags.go cli/cli_test.go
git commit -m "feat(cli): skip test-only fields in flag registration

registerStrategyFlags, applyStrategyFlags, and collectParamSweeps now
skip any field marked testonly:\"true\". The three field walkers stay
symmetrical even though collectParamSweeps was already defended by the
missing-flag check."
```

---

## Task 5: Reject test-only param names in `ApplyParams`

`ApplyParams` builds a merged map of preset values plus explicit params, then calls `applyParamValue` for each. Test-only fields are filtered from `StrategyParameters`, so the preset path can never produce a test-only name -- but the explicit `params` argument is supplied verbatim by the caller and could still contain one. This task adds an explicit reject.

**Files:**
- Modify: `engine/apply_params.go`
- Modify: `engine/apply_params_test.go`

- [ ] **Step 1: Write a failing test**

Append to `engine/apply_params_test.go`:

```go
// applyParamsTestOnlyStrategy has one regular field and one test-only field.
// It implements Descriptor so the preset path is also exercisable.
type applyParamsTestOnlyStrategy struct {
    Window int `pvbt:"window" desc:"Rolling window" default:"12"`
    Seed   int `pvbt:"seed" testonly:"true"`
}

func (ap *applyParamsTestOnlyStrategy) Name() string           { return "ApplyParamsTestOnly" }
func (ap *applyParamsTestOnlyStrategy) Setup(_ *engine.Engine) {}
func (ap *applyParamsTestOnlyStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
    return nil
}
func (ap *applyParamsTestOnlyStrategy) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{ShortCode: "apto"}
}

var _ = Describe("ApplyParams with testonly fields", func() {
    It("returns an error when explicit params target a test-only field", func() {
        strategy := &applyParamsTestOnlyStrategy{}
        eng := engine.New(strategy)

        err := engine.ApplyParams(eng, "", map[string]string{"seed": "42"})
        Expect(err).To(HaveOccurred())
        Expect(err.Error()).To(ContainSubstring("seed"))
        Expect(err.Error()).To(ContainSubstring("test-only"))
        Expect(strategy.Seed).To(Equal(0))
    })

    It("still applies non-test-only params alongside the rejected one", func() {
        strategy := &applyParamsTestOnlyStrategy{}
        eng := engine.New(strategy)

        err := engine.ApplyParams(eng, "", map[string]string{
            "window": "24",
            "seed":   "42",
        })
        Expect(err).To(HaveOccurred())
        // The whole call fails -- no params should be partially applied.
        Expect(strategy.Window).To(Equal(0))
        Expect(strategy.Seed).To(Equal(0))
    })

    It("allows direct struct assignment of a test-only field", func() {
        strategy := &applyParamsTestOnlyStrategy{Seed: 99}
        eng := engine.New(strategy)

        err := engine.ApplyParams(eng, "", map[string]string{"window": "24"})
        Expect(err).NotTo(HaveOccurred())
        Expect(strategy.Window).To(Equal(24))
        Expect(strategy.Seed).To(Equal(99))
    })
})
```

- [ ] **Step 2: Run the new tests and verify they fail**

Run: `ginkgo run --focus "ApplyParams with testonly" ./engine/...`
Expected: FAIL -- the first two tests pass without error and `Seed` becomes 42.

- [ ] **Step 3: Add the test-only set helper**

Edit `engine/apply_params.go`. Add `"reflect"` to the imports if it is not already present, then add this unexported helper at the bottom of the file:

```go
// testOnlyParamNames returns the set of parameter names (as kebab-case)
// belonging to fields marked testonly:"true" on the given strategy.
func testOnlyParamNames(strategy Strategy) map[string]struct{} {
    names := make(map[string]struct{})

    val := reflect.ValueOf(strategy)
    if val.Kind() == reflect.Ptr {
        val = val.Elem()
    }

    if val.Kind() != reflect.Struct {
        return names
    }

    targetType := val.Type()
    for ii := 0; ii < targetType.NumField(); ii++ {
        field := targetType.Field(ii)
        if !field.IsExported() {
            continue
        }

        if !IsTestOnlyField(field) {
            continue
        }

        names[ParameterName(field)] = struct{}{}
    }

    return names
}
```

- [ ] **Step 4: Reject test-only names in `ApplyParams`**

Edit `engine/apply_params.go` inside `ApplyParams`. After the merged map has been built and before the `for paramName, paramValue := range merged { ... applyParamValue(...) }` loop, add the reject. Replace:

```go
	// Explicit params override preset values.
	for paramName, paramValue := range params {
		merged[paramName] = paramValue
	}

	// Apply all merged parameters via applyParamValue.
	for paramName, paramValue := range merged {
		if err := applyParamValue(eng.strategy, paramName, paramValue); err != nil {
			return fmt.Errorf("ApplyParams: applying param %q=%q: %w", paramName, paramValue, err)
		}
	}
```

with:

```go
	// Explicit params override preset values.
	for paramName, paramValue := range params {
		merged[paramName] = paramValue
	}

	// Reject any merged param that targets a test-only field. Test-only
	// parameters are deliberately invisible to user surfaces and must not
	// be settable via ApplyParams; tests should construct strategies with
	// direct field assignment instead.
	testOnly := testOnlyParamNames(eng.strategy)
	for paramName := range merged {
		if _, isTestOnly := testOnly[paramName]; isTestOnly {
			return fmt.Errorf("ApplyParams: parameter %q is test-only and cannot be set via ApplyParams; assign the field directly on the strategy struct", paramName)
		}
	}

	// Apply all merged parameters via applyParamValue.
	for paramName, paramValue := range merged {
		if err := applyParamValue(eng.strategy, paramName, paramValue); err != nil {
			return fmt.Errorf("ApplyParams: applying param %q=%q: %w", paramName, paramValue, err)
		}
	}
```

The reject loop runs *before* the apply loop so that a single test-only name aborts the call without partially applying the other params -- this is what the second test in Step 1 verifies.

- [ ] **Step 5: Run the focused tests and verify they pass**

Run: `ginkgo run --focus "ApplyParams with testonly" ./engine/...`
Expected: PASS for all three `It` blocks.

- [ ] **Step 6: Run the full engine suite**

Run: `ginkgo run -race ./engine/...`
Expected: PASS. Existing `ApplyParams` tests in the file should be unaffected since they use strategies without `testonly` fields.

- [ ] **Step 7: Commit**

```bash
git add engine/apply_params.go engine/apply_params_test.go
git commit -m "feat(engine): reject test-only parameter names in ApplyParams

ApplyParams now returns an error if the merged params map (preset
values plus explicit overrides) targets a field marked testonly:\"true\".
The reject runs before any field is set, so a single test-only name
aborts the whole call without partially applying other params. Tests
must assign test-only fields directly on the strategy struct."
```

---

## Task 6: Sanity check -- test-only field readable inside `Compute`

This task verifies that a test-only field set via direct struct assignment remains a normal Go field that `Compute` can read. The point is to prove the marker only changes user-surface visibility and does not interfere with normal field reads. Calling `Compute` directly is sufficient -- a full backtest setup adds nothing.

**Files:**
- Modify: `engine/parameter_test.go`

- [ ] **Step 1: Add a sanity test that observes the test-only field inside Compute**

Append to `engine/parameter_test.go`:

```go
// sanityTestOnlyStrategy records the value of a test-only field the first
// time Compute is called. This proves direct struct assignment of a
// testonly field is visible to Compute even though the field is hidden
// from every user surface.
type sanityTestOnlyStrategy struct {
    Window   int `pvbt:"window" desc:"window" default:"5"`
    Injected int `pvbt:"injected" testonly:"true"`

    observed int
    seen     bool
}

func (s *sanityTestOnlyStrategy) Name() string           { return "sanityTestOnly" }
func (s *sanityTestOnlyStrategy) Setup(_ *engine.Engine) {}
func (s *sanityTestOnlyStrategy) Compute(_ context.Context, _ *engine.Engine, _ portfolio.Portfolio, _ *portfolio.Batch) error {
    if !s.seen {
        s.observed = s.Injected
        s.seen = true
    }
    return nil
}

var _ = Describe("test-only field accessibility", func() {
    It("is observable inside Compute when set via direct struct assignment", func() {
        strategy := &sanityTestOnlyStrategy{Injected: 42}
        Expect(strategy.Injected).To(Equal(42))

        // Calling Compute directly is sufficient to prove the field is
        // readable. The point of this test is the marker does not block
        // normal Go field reads -- it only filters discovery surfaces.
        err := strategy.Compute(context.Background(), nil, nil, nil)
        Expect(err).NotTo(HaveOccurred())
        Expect(strategy.seen).To(BeTrue())
        Expect(strategy.observed).To(Equal(42))

        // And the parameter is correctly hidden from discovery.
        params := engine.StrategyParameters(strategy)
        Expect(findParam(params, "injected")).To(BeNil())
        Expect(findParam(params, "window")).NotTo(BeNil())
    })
})
```

- [ ] **Step 2: Run the test and verify it passes**

Run: `ginkgo run --focus "test-only field accessibility" ./engine/...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add engine/parameter_test.go
git commit -m "test(engine): sanity-check direct assignment of testonly fields

Confirms a testonly field set via direct struct literal assignment is
observable inside Compute, proving the marker only filters discovery
surfaces and does not block normal Go field reads."
```

---

## Task 7: Update the changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add a single bullet under `## [Unreleased]` `### Added`**

Edit `CHANGELOG.md`. The current `### Added` block under `## [Unreleased]` already has several bullets. Append one more bullet at the end of that list:

```markdown
- Strategy authors can mark a parameter field as test-only with `testonly:"true"`. Test-only parameters are hidden from `pvbt describe`, the TUI, CLI flags, presets, and study sweeps, and `engine.ApplyParams` rejects any attempt to set them. Tests assign test-only fields directly on the strategy struct.
```

- [ ] **Step 2: Run the linter and the full test suite once more**

Run: `make lint && make test`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: changelog entry for test-only strategy parameters"
```

---

## Final verification

- [ ] Run `make lint` -- expect no findings.
- [ ] Run `make test` -- expect all packages green.
- [ ] Run `git log --oneline main..HEAD` -- expect 7 commits (one per task).
