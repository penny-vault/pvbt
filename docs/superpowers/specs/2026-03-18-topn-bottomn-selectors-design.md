# TopN and BottomN Selectors

## Summary

Add two new `Selector` implementations to the portfolio package: `TopN` selects the N assets with the highest values in a metric column at each timestep, and `BottomN` selects the N assets with the lowest values.

## Design

### Struct

A single unexported struct backs both constructors:

```go
type topBottomN struct {
    count     int
    metric    data.Metric
    ascending bool
}
```

- `count`: number of assets to select per timestep.
- `metric`: the DataFrame metric column to rank by.
- `ascending`: `false` for TopN (highest first), `true` for BottomN (lowest first).

### Constructors

```go
func TopN(n int, metric data.Metric) Selector
func BottomN(n int, metric data.Metric) Selector
```

Panics if `n < 1`.

### Select Logic

For each timestep:

1. Collect all `(asset, value)` pairs for the configured metric.
2. Discard entries where the value is NaN.
3. Sort by value: descending for TopN, ascending for BottomN.
4. Mark the first `min(n, len(valid))` assets as `Selected = 1.0`.
5. Mark remaining assets as `Selected = 0.0`.

Ties are broken by iteration order (no stable sort guarantee, consistent with MaxAboveZero).

Assets whose metric value is NaN (including assets with no data for the metric column) receive `Selected = 0.0`. Selected values are binary (1.0 or 0.0), not fractional.

Insert errors on the `Selected` column are logged as warnings, consistent with MaxAboveZero.

### No Fallback

Unlike MaxAboveZero, these selectors do not support a fallback DataFrame. If fewer than N assets have valid values at a timestep, whatever is available is selected.

## Files

- `portfolio/top_bottom_n.go` -- struct, constructors, Select method
- `portfolio/top_bottom_n_test.go` -- Ginkgo tests

## Test Cases

- TopN selects the N assets with the highest metric values
- BottomN selects the N assets with the lowest metric values
- Fewer than N valid assets: selects all available
- NaN values are excluded from ranking
- Single asset DataFrame
- All NaN values: no assets selected
- N larger than asset count: all assets selected
- Leadership changes across timesteps
- Zero timesteps: returns DataFrame with Selected column, no panic
- Mixed positive and negative values ranked correctly
- n < 1 panics
- Tied values: correct count selected even when ties exist
- Asset with NaN at some timesteps but valid at others
