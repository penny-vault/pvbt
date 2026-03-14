# Selected Metric Column for Per-Timestep Portfolio Selection

## Problem

Two bugs exist in the current portfolio selection pipeline:

1. **Fallback assets cannot be returned.** Selectors like `MaxAboveZero` accept fallback assets as `[]asset.Asset`, but `df.Assets(fallbacks...)` returns nothing if those assets are not already in the input DataFrame.

2. **Per-timestep selection is lost.** `MaxAboveZero` collects all ever-selected assets into a flat `map[string]asset.Asset`, then returns them for all timesteps. If VOO is selected in January and SCZ in February, the result contains both assets for both months. `EqualWeight` then gives each 50% at every timestep, which is incorrect.

## Solution

`Select` adds a `Selected` metric column to the DataFrame instead of filtering assets. At each (asset, timestep), `Selected > 0` means the asset is chosen, `0` or NaN means it is not. Selector implementations that need fallback assets insert them into the DataFrame directly.

## Design

### Selected Constant

A constant defined in the `portfolio` package:

```go
const Selected data.Metric = "selected"
```

Selectors use this as the metric key when inserting the selection column. `EqualWeight` and `WeightedBySignal` look for this same key.

`Selected` is not added to the `data/metric.go` constants -- it is a portfolio-level concept, not a general-purpose metric.

### Selector Interface

The interface is unchanged:

```go
type Selector interface {
    Select(df *data.DataFrame) *data.DataFrame
}
```

`Select` mutates the input DataFrame in place via `df.Insert()` to add the `Selected` column for each asset at each timestep. It returns the same DataFrame pointer. The DataFrame is no longer filtered to fewer assets -- all original assets remain, plus any fallback assets the Selector inserts.

### MaxAboveZero

Constructor changes from:

```go
func MaxAboveZero(fallback []asset.Asset) Selector
```

to:

```go
func MaxAboveZero(metric data.Metric, fallback *data.DataFrame) Selector
```

- `metric` -- the column to evaluate (replaces the implicit `df.MetricList()[0]` convention).
- `fallback` -- a DataFrame containing fallback asset data for the same timestamps. When no asset has a positive value at a timestep, the fallback assets are inserted into the input DataFrame and marked `Selected=1.0`.

Behavior per timestep:

1. Find the asset with the highest positive value in `metric`.
2. Set `Selected=1.0` for that asset, `Selected=0.0` for all others.
3. If no asset qualifies, insert each fallback asset into the DataFrame (with its metric data) and set `Selected=1.0` for them at that timestep.

### EqualWeight

Signature changes to:

```go
func EqualWeight(df *data.DataFrame) (PortfolioPlan, error)
```

Behavior:

1. Look for the `Selected` column. If absent, return an error.
2. At each timestep, collect all assets where `Selected > 0`.
3. Assign equal weight (`1.0 / count`) among those assets.
4. Assets with `Selected == 0` or NaN are omitted from the `Allocation.Members` map.

Any non-zero `Selected` value means "in" -- the magnitude is ignored by `EqualWeight`.

### WeightedBySignal

Signature changes to:

```go
func WeightedBySignal(df *data.DataFrame, metric data.Metric) (PortfolioPlan, error)
```

Behavior:

1. Look for the `Selected` column. If absent, return an error.
2. At each timestep, collect all assets where `Selected > 0`.
3. Among those assets, read the `metric` values. Discard NaN and negative values.
4. Normalize remaining values to sum to 1.0.
5. If all values are zero, NaN, or negative among selected assets, fall back to equal weight among the selected assets for that timestep.

Any non-zero `Selected` value means "in" -- the magnitude is ignored.

### Caller Updates

The momentum-rotation example changes from:

```go
selected := portfolio.MaxAboveZero(s.RiskOff.Assets(e.CurrentDate())).Select(momentum)
plan := portfolio.EqualWeight(selected)
```

to:

```go
portfolio.MaxAboveZero(data.MetricClose, riskOffDF).Select(momentum)
plan, err := portfolio.EqualWeight(momentum)
```

- The caller builds a `riskOffDF` DataFrame containing fallback asset data instead of passing `[]asset.Asset`.
- `Select` mutates `momentum` in place. The caller passes the same DataFrame to `EqualWeight`.
- `EqualWeight` now returns an error that must be handled.

### Fractional Selection

The `Selected` column supports fractional values (e.g., 0.5) for future Selector implementations. Current selectors use binary 1.0/0.0. Both `EqualWeight` and `WeightedBySignal` treat any `Selected > 0` as "in" -- they do not use the magnitude for weighting.

## Out of Scope

- New Selector implementations beyond fixing `MaxAboveZero`.
- Changes to `DataFrame.Insert()` or other DataFrame internals.
- Changes to `PortfolioPlan`, `Allocation`, or `RebalanceTo`.
