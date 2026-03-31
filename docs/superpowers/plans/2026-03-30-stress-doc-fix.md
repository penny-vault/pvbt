# Fix stress/doc.go Inaccuracies Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct two inaccuracies in `study/stress/doc.go`: a reference to a non-existent function and a non-existent metric.

**Architecture:** Two edits to the package doc comment. No behavioral changes.

**Tech Stack:** Go doc comments

---

### Task 1: Fix doc comment inaccuracies

**Files:**
- Modify: `study/stress/doc.go:19` (metric list)
- Modify: `study/stress/doc.go:25` (usage example)

- [ ] **Step 1: Fix the metric list on line 19**

Remove "drawdown velocity" from the metric enumeration. Change:

```go
// maximum drawdown, drawdown velocity, total return, and worst single-day
// return.
```

To:

```go
// maximum drawdown, total return, and worst single-day return.
```

- [ ] **Step 2: Fix the usage example on line 25**

Replace the non-existent `stress.DefaultScenarios()` call with the correct API. Change:

```go
//	scenarios := stress.DefaultScenarios()
```

To:

```go
//	scenarios := study.AllScenarios()
```

- [ ] **Step 3: Verify the build passes**

Run: `go build ./study/stress/`
Expected: clean build, no errors

- [ ] **Step 4: Verify godoc renders correctly**

Run: `go doc ./study/stress/`
Expected: updated doc comment with correct metric list and usage example

- [ ] **Step 5: Commit**

```bash
git add study/stress/doc.go
git commit -m "fix: correct stress/doc.go metric list and usage example (#103)"
```
