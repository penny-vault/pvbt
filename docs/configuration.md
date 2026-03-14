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

Strategy metadata like name, description, and schedule are part of the Go code rather than configuration:

```go
func (s *ADM) Name() string { return "adm" }

func (s *ADM) Setup(e *engine.Engine) {
    tc, _ := tradecron.New("@monthend", tradecron.RegularHours)
    e.Schedule(tc)
    e.SetBenchmark(e.Asset("VFINX"))
    e.RiskFreeAsset(e.Asset("DGS3MO"))
}
```

The schedule, benchmark, and risk-free asset are set in `Setup`. The strategy name comes from the `Name()` method.

Strategies can optionally implement the `Descriptor` interface to provide additional metadata (shortcode, description, source URL, version):

```go
func (s *ADM) Describe() engine.StrategyDescription {
    return engine.StrategyDescription{
        ShortCode:   "adm",
        Description: "A market timing strategy using momentum scores.",
        Source:      "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/",
        Version:     "1.1.0",
    }
}
```

## Serialization

`engine.DescribeStrategy` produces a `StrategyInfo` struct that serializes to JSON. It collects everything: name and description from the strategy, schedule/benchmark/risk-free from the engine, parameters from struct tags, and suggestions grouped by preset name.

```go
info := engine.DescribeStrategy(eng)
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
  "benchmark": "VFINX",
  "riskFree": "DGS3MO",
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

Call `DescribeStrategy` after the engine has run `Setup` (i.e., after `Backtest` or `RunLive` initialization). Before Setup, schedule/benchmark/risk-free fields will be empty.
