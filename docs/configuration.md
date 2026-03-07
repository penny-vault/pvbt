# Configuration

A strategy is more than its Go code. It's a package: metadata that describes it, parameters that make it configurable, and suggested configurations that make it easy to use. This information lives in a TOML file alongside the Go source, and the engine reads it before calling Setup.

## Strategy package structure

```
strategies/adm/
    strategy.toml
    README.md
    adm.go
```

The Go file contains the strategy logic. The README provides long-form documentation and is automatically included when the strategy is published or displayed in a catalog. The TOML file is the focus of this section.

## TOML structure

A complete TOML file for Accelerating Dual Momentum:

```toml
name = "Accelerating Dual Momentum"
shortcode = "adm"
description = "A market timing strategy that uses a 1-, 3-, and 6-month momentum score to select assets."
source = "https://engineeredportfolio.com/2018/05/02/accelerating-dual-momentum-investing/"
version = "1.1.0"
schedule = "@monthend"
benchmark = "VFINX"

[arguments.riskOn]
name = "Tickers"
description = "List of ETF, Mutual Fund, or Stock tickers to invest in"
typecode = "[]stock"
default = ["VFINX", "PRIDX"]

[arguments.riskOff]
name = "Out-of-Market Tickers"
description = "Ticker to use when model scores are all below 0"
typecode = "stock"
default = "VUSTX"

[suggested."Engineered Portfolio"]
riskOn = ["VFINX", "PRIDX"]
riskOff = ["VUSTX"]

[suggested."All ETF"]
riskOn = ["SPY", "SCZ"]
riskOff = ["TLT"]
```

### Top-level fields

The top-level fields identify the strategy:

- **name**: Human-readable name displayed in catalogs and reports.
- **shortcode**: Short unique identifier used in file paths, logs, and the `strategy:` prefix for composition.
- **description**: One-line summary of what the strategy does.
- **source**: URL to the original research or paper the strategy is based on. Optional but encouraged.
- **version**: Semantic version. Increment when the strategy logic changes.
- **schedule**: Default tradecron expression. The strategy can override this in Setup if needed.

### Benchmark

The benchmark defines what the strategy is compared against in performance reports. It's specified as a ticker symbol:

```toml
benchmark = "VFINX"
```

### Arguments

Arguments are the strategy's configurable parameters. Each argument has a name, description, type, and default value. When someone runs the strategy, they can override any argument.

```toml
[arguments.riskOn]
name = "Tickers"
description = "List of ETF, Mutual Fund, or Stock tickers to invest in"
typecode = "[]stock"
default = '[...]'
```

The key (`riskOn`) matches the strategy struct's field name (or `pvbt` struct tag). The engine populates the field automatically via reflection before calling Setup.

The `typecode` field tells the engine how to parse and validate the value. Supported types include:

| Typecode | Description | Go type |
|----------|-------------|---------|
| `stock` | A single instrument | `asset.Asset` |
| `[]stock` | A list of instruments | `universe.Universe` |
| `float` | A number | `float64` |
| `int` | An integer | `int` |
| `string` | Text | `string` |
| `bool` | True or false | `bool` |
| `duration` | Time duration | `time.Duration` |

For `stock` types, the default value is a ticker string. For `[]stock` types, it's an array of ticker strings. A `[]stock` argument matched to a `universe.Universe` field builds a `StaticUniverse` from the tickers and automatically registers it with the engine.

### Suggested configurations

Suggested configurations are named presets — known-good parameter combinations that users can select instead of configuring arguments individually.

```toml
[suggested."All ETF"]
riskOn = '[...]'
riskOff = '[...]'
```

Each suggested configuration overrides some or all of the strategy's arguments. Arguments not specified in a suggested configuration use their defaults.

Suggested configurations serve two purposes. For end users, they provide a curated starting point — "run the strategy the way the author intended." For the strategy author, they document the configurations they've tested and validated.

## Accessing configuration in Go

The engine populates strategy struct fields from TOML arguments automatically via reflection. Fields are matched to arguments by name, or by `pvbt` struct tag if the names differ.

### Field matching

```go
type ADM struct {
    riskOn       universe.Universe  // matches [arguments.riskOn] by name
    riskOff      universe.Universe  // matches [arguments.riskOff] by name
    lookback     float64            `pvbt:"lookbackMonths"` // matches by tag
}
```

### Type validation

The engine validates that each field's Go type is compatible with the argument's typecode (see the typecode table above). If a field's type doesn't match its argument's typecode, the engine panics at startup with a clear error message.

### Config metadata

The `Config` struct passed to Setup contains the strategy's metadata from the TOML (name, shortcode, description, version, schedule, benchmark). Strategy arguments are not accessed through Config -- they are already on the struct.

## The README

The README.md file is free-form Markdown. It's included automatically when the strategy is displayed in a catalog, documentation site, or report. Use it for:

- Detailed explanation of the strategy's logic and rationale
- Academic references or links to research papers
- Historical performance discussion
- Known limitations or market conditions where the strategy underperforms
- Changelog for version history

The README is for humans. The TOML is for the engine. Keep them complementary — the TOML describes the structure, the README tells the story.
