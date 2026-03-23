# CLI Configuration for Risk Rules and Tax Optimization

**Issue:** #32
**Date:** 2026-03-22
**Dependencies:** #3 (Risk management overlay), #10 (Tax optimization)

## Overview

Users configure risk management rules and tax optimization strategies through a TOML config file and optional CLI flags, without modifying strategy code. The engine reads this configuration and constructs the appropriate middleware at startup.

## Config File Loading

Viper handles config file discovery and merging with CLI flags. TOML is the default format.

**Lookup order (first found wins):**

1. `--config path/to/file.toml` (explicit flag on root command)
2. `./pvbt.toml` (working directory)
3. `~/.config/pvbt/config.toml`

Viper binds to cobra flags so CLI flags override config file values automatically.

## TOML Schema

```toml
[risk]
profile = "moderate"              # conservative | moderate | aggressive | none
max_position_size = 0.15          # override profile default
max_position_count = 20           # override or add rule not in profile
drawdown_circuit_breaker = 0.12   # override profile default
volatility_scaler_lookback = 60   # enable volatility scaler
gross_exposure_limit = 1.5        # max gross exposure as multiple of NAV
net_exposure_limit = 1.0          # max net exposure as multiple of NAV

[tax]
enabled = true
loss_threshold = 0.05
gain_offset_only = false

[tax.substitutes]
SPY = "VOO"
QQQ = "QQQM"
IWM = "VTWO"
```

### Risk configuration

When `risk.profile` is set, the profile provides baseline middleware and parameter values. Explicitly set rule parameters override those defaults. Rules not included in the chosen profile can be added by setting their parameters (e.g., `max_position_count = 20` adds that rule to a `moderate` profile that does not normally include it).

Setting `profile = "none"` disables all risk middleware unless individual rules are explicitly configured.

Profile baselines:

| Profile | max_position_size | drawdown_circuit_breaker | volatility_scaler_lookback |
|---|---|---|---|
| conservative | 0.20 | 0.10 | 60 |
| moderate | 0.25 | 0.15 | - |
| aggressive | 0.35 | 0.25 | - |

### Tax configuration

`[tax]` controls the tax loss harvester. When `enabled = true`, the harvester is added to the middleware stack with the specified parameters. If `loss_threshold` is not set, it defaults to 0.05 (5%).

The `[tax.substitutes]` table maps original ticker to substitute ticker. The engine resolves these to `asset.Asset` values via its asset provider during initialization.

## CLI Flags

Only the most common knobs get cobra flags. Everything else is config-file-only:

- `--risk-profile` (string) -- shorthand for the risk profile
- `--tax` (bool) -- enable/disable tax optimization

These bind to Viper so they override config file values. The `--config` flag is registered as a persistent flag on the root command so it is available to all subcommands.

## New Package: `config`

```go
package config

// Config holds the resolved middleware configuration.
type Config struct {
    Risk RiskConfig
    Tax  TaxConfig
}

// RiskConfig holds risk middleware settings.
type RiskConfig struct {
    Profile                  string   // "conservative", "moderate", "aggressive", "none"
    MaxPositionSize          *float64 // nil = use profile default
    MaxPositionCount         *int     // nil = use profile default
    DrawdownCircuitBreaker   *float64 // nil = use profile default
    VolatilityScalerLookback *int     // nil = use profile default
    GrossExposureLimit       *float64 // nil = not applied
    NetExposureLimit         *float64 // nil = not applied
}

// TaxConfig holds tax middleware settings.
type TaxConfig struct {
    Enabled        bool
    LossThreshold  float64           // defaults to 0.05 if not set
    GainOffsetOnly bool
    Substitutes    map[string]string // ticker -> ticker, resolved to assets by engine
}
```

Pointer fields distinguish "not set" (nil, defer to profile) from "explicitly set to zero."

The config package provides two loading functions:

- `Load(configPath string) (*Config, error)` -- loads from a specific file path (or searches the default locations if empty). Used by the engine in programmatic/test contexts.
- `LoadFromCommand(cmd *cobra.Command) (*Config, error)` -- binds Viper to cobra flags, then delegates to `Load`. Used by the CLI layer.

Both return an error for unknown profile names or invalid values (negative thresholds, position size > 1.0, etc.).

## Engine Integration

New engine option: `WithMiddlewareConfig(cfg config.Config)`.

The config struct is a recipe, not a set of constructed middleware. The engine constructs all middleware instances during initialization, after its data source is available. The config package never touches middleware objects.

During engine initialization the engine:

1. Resolves the risk profile to its baseline parameter set.
2. Applies overrides from explicitly-set config values (non-nil pointer fields).
3. Constructs risk middleware instances in a fixed order (see Middleware Ordering below), passing the engine's data source to `VolatilityScaler` when needed.
4. If tax is enabled, resolves `Substitutes` ticker strings to `asset.Asset` via the asset provider, then constructs the `TaxLossHarvester` with the engine's data source.
5. Registers all constructed middleware on the account via `acct.Use(...)`.

### Replacement semantics

When `WithMiddlewareConfig` is present, the engine owns the entire middleware stack. If `WithAccount` is also provided, the engine uses that account for cash, metrics, and broker configuration but takes over middleware registration -- any middleware already on the account is cleared before config-driven middleware is applied. If `WithAccount` is not provided, the engine constructs the account internally as it does today.

If `WithMiddlewareConfig` is not provided, behavior is unchanged from today -- no middleware is added by the engine.

There is no merging between config-driven and strategy-declared middleware. Config fully replaces strategy middleware when present.

### Precedence

For determining parameter values within the config system:

CLI flag > config file > profile defaults

### Middleware Ordering

When the engine constructs risk middleware from config, it registers them in this fixed order:

1. `VolatilityScaler` (if enabled -- must run before position sizing)
2. `MaxPositionSize` (if enabled)
3. `MaxPositionCount` (if enabled)
4. `GrossExposureLimit` (if enabled)
5. `NetExposureLimit` (if enabled)
6. `DrawdownCircuitBreaker` (if enabled -- runs last as a circuit breaker)
7. Tax middleware (always runs after all risk middleware)

This matches the ordering used by the existing risk profiles where volatility scaling runs before position limits.

## CLI Integration

In `runBacktest` and `runLive`, config is loaded and appended to `engineOpts` before calling `engine.New`:

```go
cfg, err := config.LoadFromCommand(cmd)
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

engineOpts = append(engineOpts, engine.WithMiddlewareConfig(*cfg))
```

The `--config`, `--risk-profile`, and `--tax` flags are registered in `Run()` as persistent flags on the root command.

## `pvbt config` Subcommand

A new `pvbt config` subcommand displays the resolved configuration after merging config file, profile defaults, and CLI flag overrides. It does not require a strategy. Example output:

```
Config file: ./pvbt.toml

Risk:
  profile: moderate
  max_position_size: 0.15 (override)
  drawdown_circuit_breaker: 0.15 (profile default)

Tax:
  enabled: true
  loss_threshold: 0.05
  gain_offset_only: false
  substitutes:
    SPY -> VOO
    QQQ -> QQQM
```

Fields where the effective value differs from the profile default are annotated with `(override)`. Fields using the profile's value are annotated with `(profile default)`. Fields added beyond the profile (not present in the profile baseline) are annotated with `(added)`.

## Error Handling

- Unknown profile name: error at config load time.
- Invalid override values (negative thresholds, position size > 1.0, etc.): error at config load time.
- Substitute ticker that cannot be resolved to an asset: error at engine init, before the backtest starts.
- No config file found and no CLI flags set: not an error; no middleware is added.
