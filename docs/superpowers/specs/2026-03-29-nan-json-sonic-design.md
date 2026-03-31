# Design: Replace encoding/json with Sonic and Fix NaN JSON Encoding

**Issue:** #101
**Date:** 2026-03-29

## Problem

`encoding/json` returns `UnsupportedValueError` when encoding NaN or Inf float64
values. The stress and montecarlo report `Data()` methods encode floats that can
legitimately be NaN (empty slices, divide-by-zero in analyze functions), causing
report generation to fail. The optimize report works around this with a manual
`sanitizeFloat` function that replaces NaN/Inf with 0, but that conflates "no
data" with "zero."

## Solution

Replace `encoding/json` with [bytedance/sonic](https://github.com/bytedance/sonic)
across the entire codebase. Sonic is a drop-in replacement for `encoding/json`.

### Scope

1. **Add `github.com/bytedance/sonic` as a dependency.**

2. **Replace `encoding/json` with `sonic` in all 66 files.** Sonic's top-level
   API (`sonic.Marshal`, `sonic.Unmarshal`, `sonic.NewEncoder`, `sonic.NewDecoder`)
   mirrors `encoding/json`. Call sites change `json.X(...)` to `sonic.X(...)`.
   Files that use `json.RawMessage` retain a secondary `encoding/json` import
   for the type.

3. **Use a NaN-safe frozen config in the three report `Data()` methods.** Each
   report file declares a package-level variable:

   ```go
   var nanSafeAPI = sonic.Config{
       EscapeHTML:            true,
       SortMapKeys:           true,
       CompactMarshaler:      true,
       CopyString:            true,
       ValidateString:        true,
       EncodeNullForInfOrNan: true,
   }.Froze()
   ```

   The `Data()` methods use `nanSafeAPI.NewEncoder(writer).Encode(...)`.

4. **Remove manual sanitization from `optimize/report.go`:** delete the private
   `sanitizeFloat` function and the copy-and-sanitize logic in `Data()`. The
   method simplifies to a single encode call, same as stress and montecarlo.

### Testing

- The existing test suite provides regression coverage for the import swap.
- Add a test that verifies NaN and Inf float64 values encode as `null` via the
  NaN-safe config used by report `Data()` methods.

### Risks

- Sonic aims for stdlib compatibility but could have edge-case differences with
  custom marshalers, streaming decoders, or struct tag handling. The existing
  test suite mitigates this.
- New dependency added to the project.
