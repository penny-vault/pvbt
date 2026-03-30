# Replace encoding/json with Sonic Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `encoding/json` with bytedance/sonic across the codebase so NaN/Inf floats encode as JSON `null` instead of causing errors.

**Architecture:** Import `github.com/bytedance/sonic` directly in place of `encoding/json`. For the three report files that encounter NaN/Inf, use a package-level frozen config with `EncodeNullForInfOrNan: true`. Everything else uses sonic's top-level functions as a drop-in replacement.

**Tech Stack:** Go, bytedance/sonic

**Spec:** `docs/superpowers/specs/2026-03-29-nan-json-sonic-design.md`

---

### Task 1: Add sonic dependency

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/bytedance/sonic@latest
```

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add bytedance/sonic JSON library"
```

### Task 2: Fix report files with NaN-safe config and remove sanitizeFloat

**Files:**
- Modify: `study/optimize/report.go`
- Modify: `study/stress/report.go`
- Modify: `study/montecarlo/report.go`

This is the core bug fix. Each report file gets:

1. Import swap: `"encoding/json"` -> `"github.com/bytedance/sonic"`
2. A package-level frozen config:

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

3. `Data()` uses `nanSafeAPI.NewEncoder(writer).Encode(...)`.

For `optimize/report.go` specifically: delete `sanitizeFloat` and the entire copy-and-sanitize block in `Data()`. Remove `"math"` import.

- [ ] **Step 1: Run baseline tests**

```bash
go test ./study/optimize/... ./study/stress/... ./study/montecarlo/... -count=1
```

- [ ] **Step 2: Update all three report files**
- [ ] **Step 3: Run tests, verify pass**
- [ ] **Step 4: Commit**

```bash
git add study/optimize/report.go study/stress/report.go study/montecarlo/report.go
git commit -m "fix: use sonic with EncodeNullForInfOrNan in report Data() methods

Replaces encoding/json with a NaN-safe sonic config so NaN/Inf float
values encode as JSON null. Removes manual sanitizeFloat from optimize."
```

### Task 3: Swap encoding/json for sonic across the rest of the codebase

**Files:** All 63 remaining files that import `encoding/json`.

Mechanical find-and-replace:
- `"encoding/json"` -> `"github.com/bytedance/sonic"`
- `json.Marshal` -> `sonic.Marshal`
- `json.MarshalIndent` -> `sonic.MarshalIndent`
- `json.Unmarshal` -> `sonic.Unmarshal`
- `json.NewEncoder` -> `sonic.NewEncoder`
- `json.NewDecoder` -> `sonic.NewDecoder`

**Exception:** 10 files use `json.RawMessage`. These keep a secondary `encoding/json` import for the type only; function calls still become `sonic.X`.

- [ ] **Step 1: Run full baseline**

```bash
make test
```

- [ ] **Step 2: Perform the import swap across all remaining files**
- [ ] **Step 3: Run full test suite, verify pass**

```bash
make test
```

- [ ] **Step 4: Run linter**

```bash
make lint
```

- [ ] **Step 5: Verify only RawMessage files retain encoding/json**

```bash
grep -r '"encoding/json"' --include='*.go' -l
```

Expected: only the 10 files that use `json.RawMessage`.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: replace encoding/json with sonic across the codebase"
```
