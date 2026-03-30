# Replace encoding/json with Sonic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `encoding/json` with bytedance/sonic across the codebase so NaN/Inf floats encode as JSON `null` instead of causing errors.

**Architecture:** A thin `internal/jsonutil` package wraps sonic with `EncodeNullForInfOrNan: true`. All 66 files swap their `encoding/json` import for `jsonutil`. The optimize report's manual sanitization is removed since sonic handles NaN natively.

**Tech Stack:** Go, bytedance/sonic

**Spec:** `docs/superpowers/specs/2026-03-29-nan-json-sonic-design.md`

---

### File Map

- **Create:** `internal/jsonutil/jsonutil.go` -- thin wrapper exporting `Marshal`, `Unmarshal`, `MarshalIndent`, `NewEncoder`, `NewDecoder` backed by a frozen sonic config with `EncodeNullForInfOrNan: true`.
- **Create:** `internal/jsonutil/jsonutil_test.go` -- tests verifying NaN/Inf encode as `null`.
- **Modify:** `study/optimize/report.go` -- remove `sanitizeFloat` and the copy-and-sanitize logic in `Data()`.
- **Modify:** 66 files -- swap `encoding/json` import for `github.com/penny-vault/pvbt/internal/jsonutil`. Files that also use `json.RawMessage` retain a secondary `encoding/json` import for the type.

### Task 1: Add sonic dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go get github.com/bytedance/sonic@latest
```

- [ ] **Step 2: Verify it was added**

Run:
```bash
grep sonic go.mod
```

Expected: a `require` line for `github.com/bytedance/sonic`.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add bytedance/sonic JSON library"
```

### Task 2: Create internal/jsonutil package

**Files:**
- Create: `internal/jsonutil/jsonutil.go`

- [ ] **Step 1: Write the failing test**

Create `internal/jsonutil/jsonutil_test.go`:

```go
package jsonutil_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/penny-vault/pvbt/internal/jsonutil"
)

func TestMarshal_NaN_EncodesAsNull(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	data, err := jsonutil.Marshal(sample{Value: math.NaN()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(data)
	expected := `{"value":null}`
	if got != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestMarshal_PosInf_EncodesAsNull(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	data, err := jsonutil.Marshal(sample{Value: math.Inf(1)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(data)
	expected := `{"value":null}`
	if got != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestMarshal_NegInf_EncodesAsNull(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	data, err := jsonutil.Marshal(sample{Value: math.Inf(-1)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(data)
	expected := `{"value":null}`
	if got != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestMarshal_NormalFloat_PreservesValue(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	data, err := jsonutil.Marshal(sample{Value: 3.14})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(data)
	expected := `{"value":3.14}`
	if got != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}

func TestNewEncoder_NaN_EncodesAsNull(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	var buf bytes.Buffer
	err := jsonutil.NewEncoder(&buf).Encode(sample{Value: math.NaN()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	expected := "{\"value\":null}\n"
	if got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestUnmarshal_Null_DecodesAsZero(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	var result sample
	err := jsonutil.Unmarshal([]byte(`{"value":null}`), &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Value != 0 {
		t.Errorf("got %f, want 0", result.Value)
	}
}

func TestMarshalIndent_NaN_EncodesAsNull(t *testing.T) {
	type sample struct {
		Value float64 `json:"value"`
	}

	data, err := jsonutil.MarshalIndent(sample{Value: math.NaN()}, "", "  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := string(data)
	expected := "{\n  \"value\": null\n}"
	if got != expected {
		t.Errorf("got %s, want %s", got, expected)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./internal/jsonutil/...
```

Expected: compilation failure -- package does not exist yet.

- [ ] **Step 3: Write the implementation**

Create `internal/jsonutil/jsonutil.go`:

```go
// Package jsonutil provides JSON encoding and decoding backed by sonic with
// EncodeNullForInfOrNan enabled, so NaN and Inf float values encode as null.
package jsonutil

import (
	"io"

	"github.com/bytedance/sonic"
)

var api = sonic.Config{
	EscapeHTML:            true,
	SortMapKeys:           true,
	CompactMarshaler:      true,
	CopyString:            true,
	ValidateString:        true,
	EncodeNullForInfOrNan: true,
}.Froze()

// Marshal returns the JSON encoding of val.
func Marshal(val interface{}) ([]byte, error) {
	return api.Marshal(val)
}

// MarshalIndent is like Marshal but indents the output.
func MarshalIndent(val interface{}, prefix, indent string) ([]byte, error) {
	return api.MarshalIndent(val, prefix, indent)
}

// Unmarshal parses JSON-encoded data and stores the result in val.
func Unmarshal(data []byte, val interface{}) error {
	return api.Unmarshal(data, val)
}

// NewEncoder returns a new encoder that writes to writer.
func NewEncoder(writer io.Writer) sonic.Encoder {
	return api.NewEncoder(writer)
}

// NewDecoder returns a new decoder that reads from reader.
func NewDecoder(reader io.Reader) sonic.Decoder {
	return api.NewDecoder(reader)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./internal/jsonutil/...
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/jsonutil/
git commit -m "feat: add internal/jsonutil wrapping sonic with NaN-as-null encoding"
```

### Task 3: Simplify optimize report

**Files:**
- Modify: `study/optimize/report.go:17-119`

- [ ] **Step 1: Run existing optimize tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/optimize/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Replace the Data() method and remove sanitizeFloat**

Replace the entire `study/optimize/report.go` import block and everything from line 66 onward with:

Imports become:
```go
import (
	"io"
	"time"

	"github.com/penny-vault/pvbt/internal/jsonutil"
)
```

The `Data()` method simplifies to:
```go
func (or *optimizerReport) Data(writer io.Writer) error {
	return jsonutil.NewEncoder(writer).Encode(or)
}
```

Delete the `sanitizeFloat` function entirely (lines 112-119). Delete the `"math"` import.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/optimize/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add study/optimize/report.go
git commit -m "refactor: simplify optimize report Data() now that sonic handles NaN"
```

### Task 4: Swap imports in study/ packages

**Files:**
- Modify: `study/stress/report.go`
- Modify: `study/montecarlo/report.go`
- Modify: `study/report/vue_report_test.go`
- Modify: `study/optimize/analyze_test.go`
- Modify: `study/montecarlo/analyze_test.go`
- Modify: `study/stress/analyze_test.go`
- Modify: `study/integration_test.go` (retains `encoding/json` for `json.RawMessage`)

- [ ] **Step 1: Run study tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

For each file listed above, replace:
```go
"encoding/json"
```
with:
```go
"github.com/penny-vault/pvbt/internal/jsonutil"
```

And replace all `json.` call sites with `jsonutil.` (e.g., `json.NewEncoder` becomes `jsonutil.NewEncoder`, `json.Unmarshal` becomes `jsonutil.Unmarshal`).

**Exception:** `study/integration_test.go` uses `json.RawMessage`. This file keeps both imports:
```go
"encoding/json"
"github.com/penny-vault/pvbt/internal/jsonutil"
```
Only the function calls (`json.Unmarshal`, `json.NewDecoder`, etc.) change to `jsonutil.`; the `json.RawMessage` type references stay as-is.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./study/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add study/
git commit -m "refactor: swap encoding/json for jsonutil in study packages"
```

### Task 5: Swap imports in broker/alpaca/

**Files:**
- Modify: `broker/alpaca/client_test.go`
- Modify: `broker/alpaca/broker_test.go`
- Modify: `broker/alpaca/streamer.go`
- Modify: `broker/alpaca/streamer_test.go`
- Modify: `broker/alpaca/types.go` (retains `encoding/json` for `json.RawMessage`)

- [ ] **Step 1: Run alpaca tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/alpaca/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process as Task 4. Replace `"encoding/json"` with `"github.com/penny-vault/pvbt/internal/jsonutil"` and `json.` calls with `jsonutil.`.

**Exception:** `broker/alpaca/types.go` uses `json.RawMessage`. Keep both imports; only swap function calls.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/alpaca/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/alpaca/
git commit -m "refactor: swap encoding/json for jsonutil in broker/alpaca"
```

### Task 6: Swap imports in broker/etrade/

**Files:**
- Modify: `broker/etrade/auth.go`
- Modify: `broker/etrade/client.go`
- Modify: `broker/etrade/client_test.go`
- Modify: `broker/etrade/broker_test.go`
- Modify: `broker/etrade/streamer_test.go`

- [ ] **Step 1: Run etrade tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/etrade/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. No files in this package use `json.RawMessage`, so all imports are a clean swap.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/etrade/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/etrade/
git commit -m "refactor: swap encoding/json for jsonutil in broker/etrade"
```

### Task 7: Swap imports in broker/ibkr/

**Files:**
- Modify: `broker/ibkr/auth.go`
- Modify: `broker/ibkr/auth_test.go`
- Modify: `broker/ibkr/broker_test.go`
- Modify: `broker/ibkr/client.go` (retains `encoding/json` for `json.RawMessage`)
- Modify: `broker/ibkr/client_test.go`
- Modify: `broker/ibkr/streamer.go` (retains `encoding/json` for `json.RawMessage`)
- Modify: `broker/ibkr/streamer_test.go`

- [ ] **Step 1: Run ibkr tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ibkr/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. `broker/ibkr/client.go` and `broker/ibkr/streamer.go` use `json.RawMessage` -- keep both imports, only swap function calls.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/ibkr/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/ibkr/
git commit -m "refactor: swap encoding/json for jsonutil in broker/ibkr"
```

### Task 8: Swap imports in broker/schwab/

**Files:**
- Modify: `broker/schwab/auth.go`
- Modify: `broker/schwab/auth_test.go`
- Modify: `broker/schwab/broker_test.go`
- Modify: `broker/schwab/client.go`
- Modify: `broker/schwab/client_test.go`
- Modify: `broker/schwab/streamer.go` (retains `encoding/json` for `json.RawMessage`)
- Modify: `broker/schwab/streamer_test.go` (retains `encoding/json` for `json.RawMessage`)

- [ ] **Step 1: Run schwab tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/schwab/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. `broker/schwab/streamer.go` and `broker/schwab/streamer_test.go` use `json.RawMessage` -- keep both imports.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/schwab/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/schwab/
git commit -m "refactor: swap encoding/json for jsonutil in broker/schwab"
```

### Task 9: Swap imports in broker/tastytrade/

**Files:**
- Modify: `broker/tastytrade/broker_test.go`
- Modify: `broker/tastytrade/client.go`
- Modify: `broker/tastytrade/client_test.go`
- Modify: `broker/tastytrade/streamer.go`
- Modify: `broker/tastytrade/streamer_test.go`
- Modify: `broker/tastytrade/types.go` (retains `encoding/json` for `json.RawMessage`)

- [ ] **Step 1: Run tastytrade tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. `broker/tastytrade/types.go` uses `json.RawMessage` -- keep both imports.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tastytrade/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/tastytrade/
git commit -m "refactor: swap encoding/json for jsonutil in broker/tastytrade"
```

### Task 10: Swap imports in broker/tradier/

**Files:**
- Modify: `broker/tradier/auth.go`
- Modify: `broker/tradier/auth_test.go`
- Modify: `broker/tradier/broker_test.go`
- Modify: `broker/tradier/client.go`
- Modify: `broker/tradier/streamer.go`
- Modify: `broker/tradier/streamer_test.go`
- Modify: `broker/tradier/types.go` (retains `encoding/json` for `json.RawMessage`)
- Modify: `broker/tradier/types_test.go` (retains `encoding/json` for `json.RawMessage`)
- Modify: `broker/tradier/exports_test.go` (retains `encoding/json` for `json.RawMessage`)

- [ ] **Step 1: Run tradier tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tradier/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. `broker/tradier/types.go`, `broker/tradier/types_test.go`, and `broker/tradier/exports_test.go` use `json.RawMessage` -- keep both imports.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tradier/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/tradier/
git commit -m "refactor: swap encoding/json for jsonutil in broker/tradier"
```

### Task 11: Swap imports in broker/tradestation/

**Files:**
- Modify: `broker/tradestation/auth.go`
- Modify: `broker/tradestation/auth_test.go`
- Modify: `broker/tradestation/broker_test.go`
- Modify: `broker/tradestation/client.go`
- Modify: `broker/tradestation/client_test.go`
- Modify: `broker/tradestation/streamer.go`
- Modify: `broker/tradestation/streamer_test.go`

- [ ] **Step 1: Run tradestation tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tradestation/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. No files in this package use `json.RawMessage`.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/tradestation/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/tradestation/
git commit -m "refactor: swap encoding/json for jsonutil in broker/tradestation"
```

### Task 12: Swap imports in broker/webull/

**Files:**
- Modify: `broker/webull/auth.go`

- [ ] **Step 1: Run webull tests to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/webull/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap import in auth.go**

Same mechanical process. No `json.RawMessage` usage.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./broker/webull/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add broker/webull/
git commit -m "refactor: swap encoding/json for jsonutil in broker/webull"
```

### Task 13: Swap imports in remaining packages

**Files:**
- Modify: `library/library.go`
- Modify: `library/library_test.go`
- Modify: `library/registry.go`
- Modify: `library/registry_test.go`
- Modify: `cli/describe.go`
- Modify: `cli/describe_test.go`
- Modify: `engine/descriptor_test.go`
- Modify: `data/snapshot_recorder.go`
- Modify: `data/snapshot_provider.go`

- [ ] **Step 1: Run tests for all affected packages to establish baseline**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./library/... ./cli/... ./engine/... ./data/... -count=1
```

Expected: PASS.

- [ ] **Step 2: Swap imports in each file**

Same mechanical process. No files in these packages use `json.RawMessage`.

- [ ] **Step 3: Run tests to verify they still pass**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go test ./library/... ./cli/... ./engine/... ./data/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add library/ cli/ engine/ data/
git commit -m "refactor: swap encoding/json for jsonutil in library, cli, engine, data"
```

### Task 14: Full regression test and lint

- [ ] **Step 1: Run the full test suite**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make test
```

Expected: all tests PASS.

- [ ] **Step 2: Run linter**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make lint
```

Expected: no lint errors.

- [ ] **Step 3: Fix any issues found, then re-run both**

If any test failures or lint issues appear, fix them and re-run until clean.

- [ ] **Step 4: Verify no remaining encoding/json imports (except for RawMessage)**

Run:
```bash
cd /Users/jdf/Developer/penny-vault/pvbt && grep -r '"encoding/json"' --include='*.go' -l
```

Expected: only the 10 files that use `json.RawMessage` should still import `encoding/json`.

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: address lint and test issues from sonic migration"
```
