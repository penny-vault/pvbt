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
across the entire codebase. Sonic is a drop-in replacement for `encoding/json`
with an `EncodeNullForInfOrNan` option that encodes NaN and Inf as JSON `null`.

### Scope

1. **Add `github.com/bytedance/sonic` as a dependency.**

2. **Create `internal/jsonutil` package** that wraps sonic with the right config:

   ```go
   package jsonutil

   import (
       "io"
       "github.com/bytedance/sonic"
   )

   var api = sonic.Config{
       EncodeNullForInfOrNan: true,
   }.Froze()

   func Marshal(val any) ([]byte, error)            { return api.Marshal(val) }
   func Unmarshal(buf []byte, val any) error         { return api.Unmarshal(buf, val) }
   func NewEncoder(writer io.Writer) sonic.Encoder   { return api.NewEncoder(writer) }
   func NewDecoder(reader io.Reader) sonic.Decoder   { return api.NewDecoder(reader) }
   ```

   This provides the same API as `encoding/json` with `EncodeNullForInfOrNan`
   enabled. All files import `internal/jsonutil` instead of `encoding/json`.

3. **Replace `encoding/json` imports in all 66 files.** Call sites change from
   `json.Marshal(...)` to `jsonutil.Marshal(...)` etc. No other code changes
   needed at call sites.

4. **Remove manual sanitization from `optimize/report.go`:** delete the private
   `sanitizeFloat` function and the copy-and-sanitize logic in `Data()`. The
   method simplifies to `jsonutil.NewEncoder(writer).Encode(or)`, same as stress
   and montecarlo already do.

5. **No changes needed to stress or montecarlo `Data()` methods** beyond the
   import swap -- sonic handles the NaN fields automatically.

### Testing

- Add a test that verifies NaN and Inf float64 values encode as `null` in JSON
  output. Place this alongside the report package or in a shared test helper.
- The existing test suite across all 66 files provides regression coverage for
  the import swap. Run `make test` to confirm no behavioral differences.

### Risks

- Sonic aims for stdlib compatibility but could have edge-case differences with
  custom marshalers, streaming decoders, or struct tag handling. The existing
  test suite mitigates this.
- New dependency added to the project.
