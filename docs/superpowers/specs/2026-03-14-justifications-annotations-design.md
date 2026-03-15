# Justifications and Annotations Design

## Status

Draft

## Problem

The legacy system populated `pie.Justifications` with momentum scores and
intermediate values ("B", "CF", "T", "Bond Fraction") that were useful for
debugging and UI display of why a particular trade was made. The new engine
has no mechanism for strategies to record why they made decisions.

## Decision

Add two complementary features:

- **Annotations:** Step-level key-value entries that capture intermediate
  computations (momentum scores, signal values, DataFrames) explaining the
  strategy's reasoning on a given date.
- **Justifications:** Transaction-level strings explaining why a specific
  trade was made.

## Annotation

### Struct

```go
// Annotation is a single key-value entry recorded by a strategy to explain
// its reasoning at a point in time.
type Annotation struct {
    Timestamp int64
    Key       string
    Value     string
}
```

`Timestamp` is Unix seconds. `Key` is a short identifier (e.g., `"SPY-Momentum"`,
`"bond_fraction"`). `Value` is always a string.

### Account storage

The Account struct gains a new field:

```go
annotations []Annotation
```

Annotations are append-only, like the transaction log.

### Portfolio interface additions

```go
// Annotate records a key-value annotation for the given timestamp.
// Call this during Compute to capture intermediate computations
// that explain why the strategy made its decisions. Multiple calls
// accumulate entries.
Annotate(timestamp int64, key, value string)

// Annotations returns the full annotation log in the order entries
// were recorded.
Annotations() []Annotation
```

### Annotator interface (data package)

To avoid a circular dependency between the `data` and `portfolio` packages,
the `data` package defines a minimal interface:

```go
// Annotator receives key-value annotations. Portfolio satisfies this
// interface, allowing DataFrame.Annotate to push entries without
// depending on the portfolio package.
type Annotator interface {
    Annotate(timestamp int64, key, value string)
}
```

### DataFrame.Annotate

```go
// Annotate pushes every non-NaN cell in the DataFrame as a key-value
// annotation to the destination. Keys are formatted as "TICKER-Metric".
// Values are the float formatted as a string. Returns the DataFrame
// for chaining. If the DataFrame has an error, this is a no-op.
func (df *DataFrame) Annotate(dest Annotator) *DataFrame
```

For each row (time), asset, and metric in the DataFrame, if the value is
not NaN, call:

```go
dest.Annotate(t.Unix(), ticker+"-"+string(metric), formattedValue)
```

This allows a strategy to annotate with a single call:

```go
momentumDF.Annotate(portfolio)
```

## Justification

### Transaction field

Add a `Justification` field to `Transaction`:

```go
type Transaction struct {
    // ... existing fields ...

    // Justification is an optional explanation of why this trade was made.
    // Set automatically from the Allocation's Justification field during
    // RebalanceTo, or from the WithJustification OrderModifier during Order.
    Justification string
}
```

### Allocation field

Add a `Justification` field to `Allocation`:

```go
type Allocation struct {
    Date          time.Time
    Members       map[asset.Asset]float64
    Justification string
}
```

When `RebalanceTo` processes an Allocation with a non-empty `Justification`,
it copies the string onto every `Transaction` (buy, sell, fee) generated from
that Allocation.

### OrderModifier

Add `WithJustification` as an `OrderModifier` for the `Order` path:

```go
// WithJustification attaches an explanation to the resulting transaction.
func WithJustification(reason string) OrderModifier
```

This sets the `Justification` field on the Transaction produced by the Order.

## SQLite Serialization

### Schema changes

Bump `schemaVersion` from `"2"` to `"3"`.

Add a new `annotations` table:

```sql
CREATE TABLE annotations (
    timestamp INTEGER NOT NULL,
    key       TEXT NOT NULL,
    value     TEXT NOT NULL
);
CREATE INDEX idx_annotations_timestamp ON annotations(timestamp);
```

Add a `justification` column to the `transactions` table:

```sql
CREATE TABLE transactions (
    date          TEXT NOT NULL,
    type          TEXT NOT NULL,
    ticker        TEXT,
    figi          TEXT,
    quantity      REAL,
    price         REAL,
    amount        REAL,
    qualified     INTEGER,
    justification TEXT
);
```

### Write methods

Add `writeAnnotations` following the same pattern as `writeMetrics`:
prepare a statement, loop over `a.annotations`, insert each row.

Update `writeTransactions` to include the `justification` column.

### Read methods

Add `readAnnotations` following the same pattern as `readMetrics`:
query, scan, append to `a.annotations`.

Update `readTransactions` to scan the `justification` column and
populate `Transaction.Justification`.

## Files Changed

### Source

- `portfolio/annotation.go` -- new file: `Annotation` struct
- `data/annotator.go` -- new file: `Annotator` interface
- `data/data_frame.go` -- add `Annotate(dest Annotator) *DataFrame` method
- `portfolio/portfolio.go` -- add `Annotate` and `Annotations` to `Portfolio` interface
- `portfolio/account.go` -- add `annotations` field, implement `Annotate` and `Annotations`
- `portfolio/transaction.go` -- add `Justification` field to `Transaction`
- `portfolio/allocation.go` -- add `Justification` field to `Allocation`
- `portfolio/account.go` -- update `RebalanceTo` to copy Allocation.Justification onto Transactions
- `portfolio/account.go` -- update `Order`/`submitAndRecord` to support `WithJustification`
- `portfolio/order_modifier.go` -- add `WithJustification` modifier (or wherever OrderModifier is defined)
- `portfolio/sqlite.go` -- bump schema version, add annotations table, add justification column, add write/read methods

### Tests

- `portfolio/annotation_test.go` -- test `Annotate`, `Annotations`
- `data/data_frame_test.go` -- test `DataFrame.Annotate`
- `portfolio/rebalance_test.go` -- test justification propagation from Allocation to Transaction
- `portfolio/order_test.go` -- test `WithJustification` modifier
- `portfolio/sqlite_test.go` -- test round-trip of annotations and justifications
