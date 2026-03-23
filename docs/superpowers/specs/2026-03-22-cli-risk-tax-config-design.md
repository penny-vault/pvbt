# CLI Configuration for Risk Rules and Tax Optimization

**Issue:** #32
**Date:** 2026-03-22
**Dependencies:** #3 (Risk management overlay), #10 (Tax optimization)

## Overview

Users configure risk management rules and tax optimization strategies through a TOML config file and optional CLI flags, without modifying strategy code. The engine reads this configuration and wires the appropriate middleware at startup.

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
volatility_scaler_lookback = 60   # enable volatility scaler (requires data source)

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

| Profile | max_position_size | max_position_count | drawdown_circuit_breaker | volatility_scaler_lookback |
|---|---|---|---|---|
| conservative | 0.20 | - | 0.10 | 60 |
| moderate | 0.25 | - | 0.15 | - |
| aggressive | 0.35 | - | 0.25 | - |

### Tax configuration

`[tax]` controls the tax loss harvester. When `enabled = true`, the harvester is added to the middleware stack with the specified parameters. The `[tax.substitutes]` table maps original ticker to substitute ticker; these are resolved to `asset.Asset` values via the engine's asset provider at initialization.

## CLI Flags

Only the most common knobs get cobra flags. Everything else is config-file-only:

- `--risk-profile` (string) -- shorthand for the risk profile
- `--tax` (bool) -- enable/disable tax optimization

These bind to Viper so they override config file values.

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
}

// TaxConfig holds tax middleware settings.
type TaxConfig struct {
    Enabled        bool
    LossThreshold  float64
    GainOffsetOnly bool
    Substitutes    map[string]string // ticker -> ticker, resolved to assets at engine init
}
```

Pointer fields distinguish "not set" (nil, defer to profile) from "explicitly set to zero."

`Load(cmd *cobra.Command) (*Config, error)` initializes Viper, binds flags, reads the config file, and unmarshals into the `Config` struct. It returns an error for unknown profile names or invalid values (negative thresholds, etc.).

## Engine Integration

New engine option: `WithMiddlewareConfig(cfg config.Config)`.

During engine initialization, after the data source is available, the engine:

1. Resolves the risk profile to its base set of middleware.
2. Applies overrides from explicitly-set config values (non-nil pointer fields).
3. Builds tax middleware if enabled, injecting the engine's data source automatically.
4. Resolves `Substitutes` ticker strings to `asset.Asset` via the asset provider.
5. Registers all middleware on the account via `acct.Use(...)`.

### Precedence

CLI flag > config file > strategy code > profile defaults

Config-driven middleware replaces any strategy-declared middleware when config is present. If no risk/tax config exists (no config file, no flags), no middleware is added and behavior is unchanged from today.

## CLI Integration

In `runBacktest` and `runLive`, after creating the account and before creating the engine:

```go
cfg, err := config.Load(cmd)
if err != nil {
    return fmt.Errorf("load config: %w", err)
}

engineOpts = append(engineOpts, engine.WithMiddlewareConfig(*cfg))
```

The `--config` flag is registered on the root command so it is available to all subcommands.

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

This lets users verify their middleware setup without running a backtest.

## Error Handling

- Unknown profile name: error at config load time.
- Invalid override values (negative thresholds, position size > 1.0, etc.): error at config load time.
- Substitute ticker that cannot be resolved to an asset: error at engine init, before the backtest starts.
- No config file found and no CLI flags set: not an error; no middleware is added.
