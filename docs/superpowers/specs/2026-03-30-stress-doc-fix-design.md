# Fix stress/doc.go inaccuracies (#103)

## Problem

The package doc comment in `study/stress/doc.go` has two inaccuracies:

1. It lists "drawdown velocity" as a computed metric, but no such metric exists. The actual metrics computed in `analyze.go` are: maximum drawdown, total return, and worst single-day return.

2. The usage example calls `stress.DefaultScenarios()`, which does not exist. The correct API is `study.AllScenarios()` paired with `stress.New(scenarios)`.

## Changes

### Metric list (line 19)

Before:
```
maximum drawdown, drawdown velocity, total return, and worst single-day
return.
```

After:
```
maximum drawdown, total return, and worst single-day return.
```

### Usage example (lines 25-26)

Before:
```go
scenarios := stress.DefaultScenarios()
stressStudy := stress.New(scenarios)
```

After:
```go
scenarios := study.AllScenarios()
stressStudy := stress.New(scenarios)
```

## Scope

Only `study/stress/doc.go` is modified. No behavioral changes.
