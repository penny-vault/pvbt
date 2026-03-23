# CLI Risk/Tax Configuration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to configure risk management rules and tax optimization strategies through a TOML config file and CLI flags, without modifying strategy code.

**Architecture:** New `config` package owns Viper initialization and TOML parsing, producing a `Config` struct. The engine receives this struct via `WithMiddlewareConfig` and constructs middleware during initialization. A `ClearMiddleware` method on `PortfolioManager` supports the replacement semantics. A `pvbt config` subcommand displays the resolved configuration.

**Tech Stack:** Go, Viper (already in go.mod), TOML, Cobra (already used)

**Spec:** `docs/superpowers/specs/2026-03-22-cli-risk-tax-config-design.md`

---

### Task 1: Add ClearMiddleware to PortfolioManager

**Files:**
- Modify: `portfolio/portfolio.go:186-189` (add to interface)
- Modify: `portfolio/account.go:1813-1815` (add method)
- Test: `portfolio/middleware_test.go`

- [ ] **Step 1: Write the failing test**

In `portfolio/middleware_test.go`, add a test that calls `ClearMiddleware` and verifies the middleware slice is empty:

```go
Describe("ClearMiddleware", func() {
    It("removes all registered middleware", func() {
        acct := portfolio.New(portfolio.WithCash(100000, time.Now()))
        acct.Use(mockMiddleware{})
        acct.ClearMiddleware()
        // Verify by executing a batch -- no middleware should run.
        // If middleware ran, it would annotate the batch.
        batch := acct.NewBatch(time.Now())
        Expect(batch.Annotations()).To(BeEmpty())
    })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./portfolio/...`
Expected: compilation error -- `ClearMiddleware` not defined

- [ ] **Step 3: Add ClearMiddleware to PortfolioManager interface**

In `portfolio/portfolio.go`, add to the `PortfolioManager` interface after the `Use` method:

```go
// ClearMiddleware removes all registered middleware from the processing
// chain. The engine calls this when config-driven middleware replaces
// strategy-declared middleware.
ClearMiddleware()
```

- [ ] **Step 4: Implement ClearMiddleware on Account**

In `portfolio/account.go`, add after the `Use` method:

```go
// ClearMiddleware removes all registered middleware.
func (a *Account) ClearMiddleware() {
	a.middleware = nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `ginkgo run -race ./portfolio/...`
Expected: PASS

- [ ] **Step 6: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add portfolio/portfolio.go portfolio/account.go portfolio/middleware_test.go
git commit -m "feat: add ClearMiddleware to PortfolioManager interface"
```

---

### Task 2: Create config package with types and validation

**Files:**
- Create: `config/config.go`
- Create: `config/profiles.go`
- Create: `config/config_suite_test.go`
- Create: `config/config_test.go`

- [ ] **Step 1: Create test suite file**

Create `config/config_suite_test.go` with the standard Ginkgo wiring:

```go
package config_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Config Suite")
}
```

- [ ] **Step 2: Write failing tests for Config types and validation**

Create `config/config_test.go` with tests for:
- `ValidateAndApplyDefaults` rejects unknown profile names
- `ValidateAndApplyDefaults` rejects negative `MaxPositionSize`
- `ValidateAndApplyDefaults` rejects `MaxPositionSize` > 1.0
- `ValidateAndApplyDefaults` rejects negative `DrawdownCircuitBreaker`
- `ValidateAndApplyDefaults` accepts valid config with profile and overrides
- `ValidateAndApplyDefaults` accepts config with `profile = "none"` and explicit rules
- `ValidateAndApplyDefaults` accepts empty config (no risk, no tax)
- `DefaultLossThreshold` is 0.05 when tax is enabled but threshold is 0
- `HasMiddleware` returns false for `profile = "none"` with no other overrides
- `HasMiddleware` returns true for `profile = "none"` with explicit risk overrides
- `HasMiddleware` returns true when tax is enabled

- [ ] **Step 3: Run tests to verify they fail**

Run: `ginkgo run -race ./config/...`
Expected: compilation errors -- package does not exist

- [ ] **Step 4: Implement config types**

Create `config/config.go`:

```go
package config

import "fmt"

// Config holds the resolved middleware configuration.
type Config struct {
	Risk RiskConfig
	Tax  TaxConfig
}

// RiskConfig holds risk middleware settings.
type RiskConfig struct {
	Profile                  string   `mapstructure:"profile"`
	MaxPositionSize          *float64 `mapstructure:"max_position_size"`
	MaxPositionCount         *int     `mapstructure:"max_position_count"`
	DrawdownCircuitBreaker   *float64 `mapstructure:"drawdown_circuit_breaker"`
	VolatilityScalerLookback *int     `mapstructure:"volatility_scaler_lookback"`
	GrossExposureLimit       *float64 `mapstructure:"gross_exposure_limit"`
	NetExposureLimit         *float64 `mapstructure:"net_exposure_limit"`
}

// TaxConfig holds tax middleware settings.
type TaxConfig struct {
	Enabled        bool              `mapstructure:"enabled"`
	LossThreshold  float64           `mapstructure:"loss_threshold"`
	GainOffsetOnly bool              `mapstructure:"gain_offset_only"`
	Substitutes    map[string]string `mapstructure:"substitutes"`
}

// DefaultLossThreshold is the loss threshold used when tax is enabled
// but no threshold is explicitly configured.
const DefaultLossThreshold = 0.05

// ValidateAndApplyDefaults checks that the config is internally consistent
// and applies default values where needed (e.g., default loss threshold).
func (cfg *Config) ValidateAndApplyDefaults() error {
	// Validate risk profile name.
	switch cfg.Risk.Profile {
	case "", "conservative", "moderate", "aggressive", "none":
		// valid
	default:
		return fmt.Errorf("config: unknown risk profile %q", cfg.Risk.Profile)
	}

	if cfg.Risk.MaxPositionSize != nil {
		if *cfg.Risk.MaxPositionSize < 0 || *cfg.Risk.MaxPositionSize > 1.0 {
			return fmt.Errorf("config: max_position_size must be between 0 and 1, got %f", *cfg.Risk.MaxPositionSize)
		}
	}

	if cfg.Risk.MaxPositionCount != nil && *cfg.Risk.MaxPositionCount < 0 {
		return fmt.Errorf("config: max_position_count must be non-negative, got %d", *cfg.Risk.MaxPositionCount)
	}

	if cfg.Risk.DrawdownCircuitBreaker != nil {
		if *cfg.Risk.DrawdownCircuitBreaker < 0 || *cfg.Risk.DrawdownCircuitBreaker > 1.0 {
			return fmt.Errorf("config: drawdown_circuit_breaker must be between 0 and 1, got %f", *cfg.Risk.DrawdownCircuitBreaker)
		}
	}

	if cfg.Risk.VolatilityScalerLookback != nil && *cfg.Risk.VolatilityScalerLookback < 1 {
		return fmt.Errorf("config: volatility_scaler_lookback must be at least 1, got %d", *cfg.Risk.VolatilityScalerLookback)
	}

	if cfg.Risk.GrossExposureLimit != nil && *cfg.Risk.GrossExposureLimit < 0 {
		return fmt.Errorf("config: gross_exposure_limit must be non-negative, got %f", *cfg.Risk.GrossExposureLimit)
	}

	if cfg.Risk.NetExposureLimit != nil && *cfg.Risk.NetExposureLimit < 0 {
		return fmt.Errorf("config: net_exposure_limit must be non-negative, got %f", *cfg.Risk.NetExposureLimit)
	}

	// Apply default loss threshold.
	if cfg.Tax.Enabled && cfg.Tax.LossThreshold == 0 {
		cfg.Tax.LossThreshold = DefaultLossThreshold
	}

	return nil
}

// HasMiddleware returns true if the config specifies any risk or tax middleware.
// Note: profile = "none" with no other overrides means "no middleware" -- it
// does not count as having middleware configured.
func (cfg *Config) HasMiddleware() bool {
	hasRiskProfile := cfg.Risk.Profile != "" && cfg.Risk.Profile != "none"
	hasRiskOverrides := cfg.Risk.MaxPositionSize != nil ||
		cfg.Risk.MaxPositionCount != nil ||
		cfg.Risk.DrawdownCircuitBreaker != nil ||
		cfg.Risk.VolatilityScalerLookback != nil ||
		cfg.Risk.GrossExposureLimit != nil ||
		cfg.Risk.NetExposureLimit != nil
	return hasRiskProfile || hasRiskOverrides || cfg.Tax.Enabled
}
```

- [ ] **Step 5: Implement risk profile baselines**

Create `config/profiles.go`:

```go
package config

// ProfileBaseline returns the default parameter values for a named profile.
// Fields that are nil are not part of the profile's baseline.
// These values must match the profiles in risk/profiles.go.
func ProfileBaseline(name string) RiskConfig {
	switch name {
	case "conservative":
		maxPos := 0.20
		drawdown := 0.10
		volLookback := 60
		return RiskConfig{
			Profile:                  name,
			MaxPositionSize:          &maxPos,
			DrawdownCircuitBreaker:   &drawdown,
			VolatilityScalerLookback: &volLookback,
		}
	case "moderate":
		maxPos := 0.25
		drawdown := 0.15
		return RiskConfig{
			Profile:                name,
			MaxPositionSize:        &maxPos,
			DrawdownCircuitBreaker: &drawdown,
		}
	case "aggressive":
		maxPos := 0.35
		drawdown := 0.25
		return RiskConfig{
			Profile:                name,
			MaxPositionSize:        &maxPos,
			DrawdownCircuitBreaker: &drawdown,
		}
	default:
		return RiskConfig{Profile: name}
	}
}

// ResolveProfile merges profile baselines with explicit overrides.
// Explicit values (non-nil fields on cfg.Risk) take precedence over
// profile defaults.
func (cfg *Config) ResolveProfile() RiskConfig {
	if cfg.Risk.Profile == "" || cfg.Risk.Profile == "none" {
		return cfg.Risk
	}

	baseline := ProfileBaseline(cfg.Risk.Profile)

	// Apply overrides: if the user set a value, it wins.
	if cfg.Risk.MaxPositionSize != nil {
		baseline.MaxPositionSize = cfg.Risk.MaxPositionSize
	}

	if cfg.Risk.MaxPositionCount != nil {
		baseline.MaxPositionCount = cfg.Risk.MaxPositionCount
	}

	if cfg.Risk.DrawdownCircuitBreaker != nil {
		baseline.DrawdownCircuitBreaker = cfg.Risk.DrawdownCircuitBreaker
	}

	if cfg.Risk.VolatilityScalerLookback != nil {
		baseline.VolatilityScalerLookback = cfg.Risk.VolatilityScalerLookback
	}

	if cfg.Risk.GrossExposureLimit != nil {
		baseline.GrossExposureLimit = cfg.Risk.GrossExposureLimit
	}

	if cfg.Risk.NetExposureLimit != nil {
		baseline.NetExposureLimit = cfg.Risk.NetExposureLimit
	}

	return baseline
}
```

- [ ] **Step 6: Add profile baseline cross-reference tests**

Add tests that verify `ProfileBaseline` values match the parameters used in `risk/profiles.go`. For example, `ProfileBaseline("conservative").MaxPositionSize` must equal 0.20 (matching `risk.Conservative`). This prevents silent drift between the config baselines and the risk package.

- [ ] **Step 7: Run tests to verify they pass**

Run: `ginkgo run -race ./config/...`
Expected: PASS

- [ ] **Step 8: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add config/
git commit -m "feat: add config package with types, validation, and profile resolution"
```

---

### Task 3: Add Viper-based config loading

**Files:**
- Create: `config/load.go`
- Modify: `config/config_test.go` (add loading tests)

- [ ] **Step 1: Write failing tests for config loading**

Add tests to `config/config_test.go`:
- `Load` reads a TOML file and produces a valid `Config`
- `Load` with empty path searches default locations (write a temp `pvbt.toml` in a temp dir)
- `Load` returns zero-value Config when no file is found (not an error)
- `Load` returns error for malformed TOML
- Pointer fields are nil when not set in TOML
- Pointer fields are non-nil when set in TOML (including zero values like `max_position_size = 0`)
- `LoadFromCommand` applies `--risk-profile` flag override on top of config file
- `LoadFromCommand` applies `--tax` flag override on top of config file
- `LoadFromCommand` re-validates after flag overrides (e.g., `--risk-profile=invalid` returns error)
- `LoadFromCommand` with no flags and no config file returns zero-value Config

Use `os.MkdirTemp` and write TOML files for each test case. For `LoadFromCommand` tests, create a cobra command with the expected flags registered, set flag values, and call `LoadFromCommand`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./config/...`
Expected: compilation error -- `Load` not defined

- [ ] **Step 3: Implement Load and LoadFromCommand**

Create `config/load.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Load reads configuration from the given path. If configPath is empty,
// it searches the default locations (./pvbt.toml, ~/.config/pvbt/config.toml).
// Returns a zero-value Config (no middleware) if no file is found.
func Load(configPath string) (*Config, error) {
	vp := viper.New()
	vp.SetConfigType("toml")

	if configPath != "" {
		vp.SetConfigFile(configPath)
	} else {
		vp.SetConfigName("pvbt")
		vp.AddConfigPath(".")

		home, err := os.UserHomeDir()
		if err == nil {
			vp.AddConfigPath(filepath.Join(home, ".config", "pvbt"))
		}
	}

	if err := vp.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return &Config{}, nil
		}

		return nil, fmt.Errorf("config: reading config file: %w", err)
	}

	var cfg Config
	if err := vp.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshaling config: %w", err)
	}

	if err := cfg.ValidateAndApplyDefaults(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadFromCommand reads configuration from Viper, binding CLI flags
// for --config, --risk-profile, and --tax. Delegates to Load for
// file discovery and parsing, then applies flag overrides.
func LoadFromCommand(cmd *cobra.Command) (*Config, error) {
	configPath, _ := cmd.Flags().GetString("config")

	cfg, err := Load(configPath)
	if err != nil {
		return nil, err
	}

	// Apply CLI flag overrides.
	if cmd.Flags().Changed("risk-profile") {
		profile, _ := cmd.Flags().GetString("risk-profile")
		cfg.Risk.Profile = profile
	}

	if cmd.Flags().Changed("tax") {
		taxEnabled, _ := cmd.Flags().GetBool("tax")
		cfg.Tax.Enabled = taxEnabled
	}

	// Re-validate after flag overrides.
	if err := cfg.ValidateAndApplyDefaults(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ConfigFilePath returns the path of the config file that was loaded,
// or empty string if no file was found. Used by the `pvbt config` command.
func ConfigFilePath(configPath string) string {
	vp := viper.New()
	vp.SetConfigType("toml")

	if configPath != "" {
		vp.SetConfigFile(configPath)
	} else {
		vp.SetConfigName("pvbt")
		vp.AddConfigPath(".")

		home, err := os.UserHomeDir()
		if err == nil {
			vp.AddConfigPath(filepath.Join(home, ".config", "pvbt"))
		}
	}

	if err := vp.ReadInConfig(); err != nil {
		return ""
	}

	return vp.ConfigFileUsed()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `ginkgo run -race ./config/...`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add config/load.go config/config_test.go
git commit -m "feat: add Viper-based TOML config loading with CLI flag binding"
```

---

### Task 4: Add WithMiddlewareConfig engine option and middleware construction

**Files:**
- Modify: `engine/option.go` (add WithMiddlewareConfig)
- Create: `engine/middleware_config.go` (middleware construction logic)
- Create: `engine/middleware_config_test.go`
- Modify: `engine/engine.go:37-74` (add middlewareConfig field)

- [ ] **Step 1: Write failing tests for middleware construction from config**

Create `engine/middleware_config_test.go` with tests:
- Config with `profile = "moderate"` produces MaxPositionSize and DrawdownCircuitBreaker middleware
- Config with profile + override applies the override value
- Config with `profile = "none"` and explicit rules produces only those rules
- Config with tax enabled produces TaxLossHarvester middleware
- Config with `volatility_scaler_lookback` produces VolatilityScaler (needs DataSource)
- Middleware ordering matches spec: VolatilityScaler, MaxPositionSize, MaxPositionCount, GrossExposureLimit, NetExposureLimit, DrawdownCircuitBreaker, Tax
- Empty config produces no middleware

Use a mock/stub `DataSource` and `AssetProvider` for tests. Verify middleware count and ordering by running them through a batch and checking annotations.

- [ ] **Step 2: Run tests to verify they fail**

Run: `ginkgo run -race ./engine/...`
Expected: compilation errors

- [ ] **Step 3: Add middlewareConfig field to Engine struct**

In `engine/engine.go`, add to the Engine struct fields:

```go
middlewareConfig *config.Config
```

Add the import for the config package.

- [ ] **Step 4: Add WithMiddlewareConfig option**

In `engine/option.go`:

```go
// WithMiddlewareConfig sets the middleware configuration. The engine
// constructs risk and tax middleware from this config during initialization.
// When set, config-driven middleware replaces any strategy-declared middleware.
func WithMiddlewareConfig(cfg config.Config) Option {
	return func(e *Engine) {
		e.middlewareConfig = &cfg
	}
}
```

- [ ] **Step 5: Implement middleware construction**

Create `engine/middleware_config.go` with:

```go
package engine

// buildMiddlewareFromConfig constructs risk and tax middleware from the
// engine's middleware config and registers them on the account.
// Called during Backtest/Live initialization after the data source
// and asset provider are available.
func (e *Engine) buildMiddlewareFromConfig() error
```

This function:
1. Calls `cfg.ResolveProfile()` to get the merged risk config
2. Constructs middleware in the fixed order from the spec
3. For `VolatilityScaler`, passes `e` (which implements `DataSource`)
4. For tax, resolves substitute tickers via `e.Asset()`, builds `HarvesterConfig`, constructs `TaxLossHarvester`
5. Calls `e.account.ClearMiddleware()` then `e.account.Use(...)` with the constructed middleware

- [ ] **Step 6: Run tests to verify they pass**

Run: `ginkgo run -race ./engine/...`
Expected: PASS

- [ ] **Step 7: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add engine/option.go engine/engine.go engine/middleware_config.go engine/middleware_config_test.go
git commit -m "feat: add WithMiddlewareConfig engine option and middleware construction"
```

---

### Task 5: Wire middleware config into Backtest and Live initialization

**Files:**
- Modify: `engine/backtest.go:34-60` (call buildMiddlewareFromConfig after account creation)
- Modify: `engine/live.go` (same integration point)
- Test: `engine/middleware_config_test.go` (add integration-level test)

- [ ] **Step 1: Write failing integration test**

Add a test that creates an engine with `WithMiddlewareConfig` containing a moderate profile, runs a short backtest with a trivial strategy, and verifies that the account has middleware registered (check via batch annotations showing risk middleware acted).

- [ ] **Step 2: Run test to verify it fails**

Run: `ginkgo run -race ./engine/...`
Expected: FAIL -- middleware not wired

- [ ] **Step 3: Wire into Backtest**

In `engine/backtest.go`, after the account is created (after `createAccount`) and after `strategy.Setup` is called, add:

```go
// Apply config-driven middleware (replaces any strategy-declared middleware).
if e.middlewareConfig != nil {
    if err := e.buildMiddlewareFromConfig(); err != nil {
        return nil, fmt.Errorf("engine: building middleware from config: %w", err)
    }
}
```

The exact insertion point is after step 4 (Setup) so that the asset registry and data providers are ready, but before the simulation loop starts.

- [ ] **Step 4: Wire into Live**

In `engine/live.go`, add the same `buildMiddlewareFromConfig` call after the account is created and assigned to `e.account`, and after `strategy.Setup` has been called. This mirrors the Backtest insertion point -- the asset registry and data providers must be ready before middleware construction.

- [ ] **Step 5: Run tests to verify they pass**

Run: `ginkgo run -race ./engine/...`
Expected: PASS

- [ ] **Step 6: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add engine/backtest.go engine/live.go engine/middleware_config_test.go
git commit -m "feat: wire config-driven middleware into Backtest and Live initialization"
```

---

### Task 6: Register CLI flags and load config in CLI layer

**Files:**
- Modify: `cli/run.go` (register persistent flags: --config, --risk-profile, --tax)
- Modify: `cli/backtest.go:61-154` (load config, pass to engine)
- Modify: `cli/live.go:34-end` (load config, pass to engine)
- Test: `cli/` (add test for flag registration if a CLI test file exists)

- [ ] **Step 1: Register persistent flags on root command**

In `cli/run.go`, inside `Run()`, after the existing `log-level` flag setup, register `--config` as a persistent flag on the root command (needed by all subcommands including `pvbt config`):

```go
rootCmd.PersistentFlags().String("config", "", "Path to config file (default: ./pvbt.toml or ~/.config/pvbt/config.toml)")
```

Register `--risk-profile` and `--tax` on the `backtest` and `live` subcommands only (not the root), since they are meaningless for `describe`, `snapshot`, and `study`. Add these in `newBacktestCmd` and `newLiveCmd`:

```go
cmd.Flags().String("risk-profile", "", "Risk profile (conservative, moderate, aggressive, none)")
cmd.Flags().Bool("tax", false, "Enable tax optimization")
```

- [ ] **Step 2: Load config in runBacktest**

In `cli/backtest.go`, in `runBacktest`, after `applyStrategyFlags` and before creating `engineOpts`, add:

```go
cfg, err := config.LoadFromCommand(cmd)
if err != nil {
    return fmt.Errorf("load config: %w", err)
}
```

Then append to `engineOpts`:

```go
if cfg.HasMiddleware() {
    engineOpts = append(engineOpts, engine.WithMiddlewareConfig(*cfg))
}
```

- [ ] **Step 3: Load config in runLive**

Same pattern in `cli/live.go`.

- [ ] **Step 4: Run tests**

Run: `make test`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cli/run.go cli/backtest.go cli/live.go
git commit -m "feat: register --config, --risk-profile, --tax flags and load config in CLI"
```

---

### Task 7: Add `pvbt config` subcommand

**Files:**
- Create: `cli/config_cmd.go`
- Modify: `cli/run.go` (register subcommand)

- [ ] **Step 1: Implement the config subcommand**

Create `cli/config_cmd.go`:

```go
package cli

// newConfigCmd returns a cobra command that displays the resolved
// middleware configuration after merging config file, profile defaults,
// and CLI flag overrides.
func newConfigCmd() *cobra.Command
```

The command:
1. Calls `config.LoadFromCommand(cmd)` to get the resolved config
2. Calls `config.ConfigFilePath(...)` to display which file was loaded
3. Calls `cfg.ResolveProfile()` to get the effective risk settings
4. Prints each section with `(override)`, `(profile default)`, or `(added)` annotations
5. Annotation logic: compare resolved values against `config.ProfileBaseline(profile)` -- if the value matches baseline, it is `(profile default)`; if it differs, `(override)`; if the baseline does not include that rule, `(added)`

- [ ] **Step 2: Register the subcommand**

In `cli/run.go`, add:

```go
rootCmd.AddCommand(newConfigCmd())
```

- [ ] **Step 3: Manual test**

Create a `pvbt.toml` in a temp directory and verify the output matches the expected format from the spec.

- [ ] **Step 4: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/config_cmd.go cli/run.go
git commit -m "feat: add pvbt config subcommand to display resolved middleware config"
```

---

### Task 8: End-to-end test and cleanup

**Files:**
- Create: `config/testdata/full.toml` (example config for tests)
- Modify: `config/config_test.go` (add end-to-end loading test with full.toml)

- [ ] **Step 1: Create test fixture**

Create `config/testdata/full.toml` with the full schema from the spec:

```toml
[risk]
profile = "moderate"
max_position_size = 0.15
max_position_count = 20
drawdown_circuit_breaker = 0.12
volatility_scaler_lookback = 60
gross_exposure_limit = 1.5
net_exposure_limit = 1.0

[tax]
enabled = true
loss_threshold = 0.05
gain_offset_only = false

[tax.substitutes]
SPY = "VOO"
QQQ = "QQQM"
IWM = "VTWO"
```

- [ ] **Step 2: Write end-to-end test**

Test that `Load("testdata/full.toml")` (relative to config package test directory) produces a `Config` where:
- `Risk.Profile` is `"moderate"`
- All risk override pointers are non-nil with correct values
- `Tax.Enabled` is true
- `Tax.LossThreshold` is 0.05
- `Tax.Substitutes` has 3 entries
- `ResolveProfile()` returns overridden values where set and profile defaults where not

- [ ] **Step 3: Run all tests**

Run: `make test`
Expected: PASS

- [ ] **Step 4: Run lint**

Run: `make lint`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add config/testdata/full.toml config/config_test.go
git commit -m "test: add end-to-end config loading test with full TOML fixture"
```

---

### Task 9: Update changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog entry**

Under the `[Unreleased]` section, add to `Added`:

```markdown
- Users can configure risk management rules and tax optimization through a TOML config file (`pvbt.toml`) and `--risk-profile`/`--tax` CLI flags, without modifying strategy code
- The `pvbt config` command displays the resolved middleware configuration
```

- [ ] **Step 2: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs: add changelog entry for CLI risk/tax configuration"
```
