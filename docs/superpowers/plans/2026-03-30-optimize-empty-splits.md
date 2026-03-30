# Fix: optimize.New panics on empty splits slice

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return a clear error from `Configurations()` when splits is empty instead of panicking.

**Architecture:** Add a length check at the top of `Optimizer.Configurations()`. Add a test that verifies the error.

**Tech Stack:** Go, Ginkgo/Gomega

---

### Task 1: Guard empty splits in Configurations and test it

**Files:**
- Modify: `study/optimize/optimize.go:81-104` (Configurations method)
- Test: `study/optimize/optimize_test.go`

- [ ] **Step 1: Write the failing test**

Add this test inside the existing `Describe("Configurations", ...)` block in `study/optimize/optimize_test.go`, after the existing `It` blocks:

```go
It("returns an error when splits is empty", func() {
    opt := optimize.New(nil)
    _, err := opt.Configurations(context.Background())
    Expect(err).To(MatchError(ContainSubstring("no splits")))
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd study/optimize && ginkgo run -v --focus "returns an error when splits is empty"`

Expected: FAIL -- panic with index out of range

- [ ] **Step 3: Add the guard to Configurations**

In `study/optimize/optimize.go`, add this at the top of `Configurations`, before the line that indexes `opt.splits[0]`:

```go
if len(opt.splits) == 0 {
    return nil, fmt.Errorf("optimizer: no splits configured")
}
```

Add `"fmt"` to the import block.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd study/optimize && ginkgo run -v --focus "returns an error when splits is empty"`

Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `cd study/optimize && ginkgo run -race`

Expected: all tests pass

- [ ] **Step 6: Run lint**

Run: `make lint`

Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add study/optimize/optimize.go study/optimize/optimize_test.go
git commit -m "fix: return error from Configurations when splits is empty (#106)"
```
