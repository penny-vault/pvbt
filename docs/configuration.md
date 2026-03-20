# Configuration

Strategy parameters are defined as exported struct fields with struct tags. The engine populates them via reflection before calling Setup. No external configuration files are needed.

## Struct tags

Four tags control how a field is exposed:

| Tag | Purpose | Example |
|-----|---------|---------|
| `pvbt` | CLI flag name (defaults to lowercase field name) | `pvbt:"riskOn"` |
| `desc` | Description for help text | `desc:"ETFs to invest in"` |
| `default` | Default value (parsed from string) | `default:"VOO,SCZ"` |
| `suggest` | Named presets (pipe-delimited `name=value`) | `suggest:"Classic=VFINX,PRIDX"` |

## Supported field types

| Go type | Default format | Example |
|---------|---------------|---------|
| `float64` | Decimal number | `default:"0.05"` |
| `int` | Integer | `default:"12"` |
| `string` | Plain text | `default:"momentum"` |
| `bool` | `true` or `false` | `default:"true"` |
| `time.Duration` | Go duration string | `default:"720h"` |
| `asset.Asset` | Ticker symbol | `default:"SPY"` |
| `universe.Universe` | Comma-separated tickers | `default:"VOO,SCZ,EFA"` |

## Example

```go
type ADM struct {
    RiskOn    universe.Universe `pvbt:"riskOn"    desc:"ETFs to invest in"        default:"VOO,SCZ" suggest:"Classic=VFINX,PRIDX|Modern=SPY,QQQ"`
    RiskOff   universe.Universe `pvbt:"riskOff"   desc:"Out-of-market asset"      default:"TLT"     suggest:"Classic=VUSTX|Modern=TLT"`
    Lookback  int               `pvbt:"lookback"  desc:"Momentum lookback months"  default:"6"`
}
```

This defines three CLI flags: `--riskOn`, `--riskOff`, and `--lookback`. Each has a description and default value. The `suggest` tags define two named presets ("Classic" and "Modern") that users can select as starting points.

## How hydration works

Before calling `Setup`, the engine reflects over the strategy struct and processes each exported field with a `default` tag:

1. If the field is already non-zero (set by the caller or CLI flags), it is not overwritten.
2. Otherwise, the `default` tag value is parsed into the field's type.
3. For `asset.Asset` fields, the ticker is resolved via `e.Asset()`.
4. For `universe.Universe` fields, the comma-separated tickers are resolved and wrapped in a `StaticUniverse` via `e.Universe()`.

## CLI integration

The CLI uses the `pvbt` and `desc` tags to register cobra flags automatically. When a user passes `--riskOn "SPY,QQQ"`, the field is populated before hydration runs, so the `default` tag is skipped.

## Metadata

Strategy metadata like name, description, and schedule are part of the Go code rather than configuration. The preferred approach is to declare schedule and benchmark in `Describe()`:

```go
func (s *ADM) Name() string { return "adm" }

func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        ShortCode:   "adm",
        Description: "A market timing strategy using momentum scores.",
        Source:      "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/",
        Version:     "1.1.0",
        Schedule:    "@monthend",
        Benchmark:   "SPY",
    }
}
```

The engine reads `Schedule` and `Benchmark` from `Describe()` during initialization. If declared there, the strategy does not need to call `eng.Schedule()` or `eng.SetBenchmark()` in `Setup`.

The imperative approach still works for backward compatibility or for cases where the schedule/benchmark depends on runtime state:

```go
func (s *ADM) Setup(e *engine.Engine) {
    tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
    e.Schedule(tc)
    e.SetBenchmark(e.Asset("VFINX"))
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}
```

Values set in `Setup` override those from `Describe()`.

## Serialization

`engine.DescribeStrategy` produces a `StrategyInfo` struct that serializes to JSON. It takes a `Strategy` (not an engine) and does not require `Setup` to have run. It collects name and description from the strategy, schedule and benchmark from `Describe()`, and parameters and suggestions from struct tags.

```go
info := engine.DescribeStrategy(strategy)
data, _ := json.MarshalIndent(info, "", "  ")
```

This produces:

```json
{
  "name": "adm",
  "shortcode": "adm",
  "description": "A market timing strategy using momentum scores.",
  "source": "https://engineeredportfolio.com/...",
  "version": "1.1.0",
  "schedule": "@monthend",
  "benchmark": "SPY",
  "parameters": [
    {"name": "riskOn", "description": "ETFs to invest in", "type": "universe.Universe", "default": "VOO,SCZ"},
    {"name": "riskOff", "description": "Out-of-market asset", "type": "universe.Universe", "default": "TLT"},
    {"name": "lookback", "description": "Momentum lookback months", "type": "int", "default": "6"}
  ],
  "suggestions": {
    "Classic": {"riskOn": "VFINX,PRIDX", "riskOff": "VUSTX"},
    "Modern": {"riskOn": "SPY,QQQ", "riskOff": "TLT"}
  }
}
```

## CLI flags

The `describe` command prints human-readable output by default. Pass `--json` for JSON output:

```
pvbt adm describe          # human-readable table
pvbt adm describe --json   # JSON output
```

The `--preset` flag applies a named parameter preset to backtest, live, or snapshot runs:

```
pvbt adm backtest --preset Classic
```

This populates strategy fields from the matching `suggest` tag values before running.
