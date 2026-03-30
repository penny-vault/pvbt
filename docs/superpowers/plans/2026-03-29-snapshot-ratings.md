# Snapshot Ratings Capture Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire `PVDataProvider` as `RatingProvider` in the snapshot command so rated universes and their downstream metric fetches are captured.

**Architecture:** `PVDataProvider` already implements `RatingProvider`. The `SnapshotRecorder` already has full recording/replaying plumbing for ratings. The only gap is the one-line wiring in `cli/snapshot.go`. The empty `metrics` table is a downstream consequence -- once ratings work, `Fetch` calls for rated assets will populate metrics automatically.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Ginkgo/Gomega

---

### Task 1: Wire RatingProvider in snapshot command

**Files:**
- Modify: `cli/snapshot.go:111-116`

- [ ] **Step 1: Run existing tests to establish baseline**

Run: `make test`
Expected: All tests pass.

- [ ] **Step 2: Modify snapshot command to pass RatingProvider**

In `cli/snapshot.go`, change the `SnapshotRecorderConfig` at line 111-116 from:

```go
recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
    BatchProvider: provider,
    AssetProvider: provider,
    // IndexProvider and RatingProvider are nil unless PVDataProvider
    // implements them in the future or the strategy registers its own.
})
```

to:

```go
recorder, err := data.NewSnapshotRecorder(outputPath, data.SnapshotRecorderConfig{
    BatchProvider:  provider,
    AssetProvider:  provider,
    RatingProvider: provider,
})
```

- [ ] **Step 3: Run tests to verify nothing broke**

Run: `make test`
Expected: All tests pass. No behavior change for strategies that don't use rated universes.

- [ ] **Step 4: Run lint**

Run: `make lint`
Expected: Clean.

- [ ] **Step 5: Commit**

```bash
git add cli/snapshot.go
git commit -m "fix: wire RatingProvider in snapshot command (#128)

PVDataProvider implements RatingProvider but was not passed to the
SnapshotRecorderConfig. Strategies using RatedUniverse got an empty
universe, which also caused downstream metric fetches to return no
data."
```
