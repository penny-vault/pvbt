# GitHub Actions CI Design

## Overview

Add continuous integration to pvbt via GitHub Actions. The workflow runs three parallel jobs on every push to `main` and every pull request targeting `main`: module tidiness check, linting, and build+test.

## Trigger

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

## Jobs

All jobs run on `ubuntu-latest` with the latest stable Go. The `setup-go` v5 action handles Go installation and module caching automatically.

### 1. `tidy` -- Module Tidiness

Verifies that `go.mod` and `go.sum` are up to date.

Steps:
1. Checkout code
2. Set up Go (`stable`)
3. Run `go mod tidy`
4. Run `git diff --exit-code go.mod go.sum` -- fails if tidy produced changes

### 2. `lint` -- Static Analysis

Runs the full lint suite using the existing Makefile target.

Steps:
1. Checkout code
2. Set up Go (`stable`)
3. Run `golangci-lint` via the official `golangci/golangci-lint-action` (pinned version)
4. Run `go vet ./...`

Note: `go fmt` is already covered by the golangci-lint configuration. The `golangci-lint-action` handles its own caching separate from Go module caching.

### 3. `test` -- Build and Test

Compiles the binary and runs the full test suite with race detection.

Steps:
1. Checkout code
2. Set up Go (`stable`)
3. Run `make build`
4. Install Ginkgo CLI via `go install github.com/onsi/ginkgo/v2/ginkgo@latest`
5. Run `make test` (`ginkgo run -race ./...`)

## Key Decisions

- **Go version `stable`:** Auto-updates to latest stable release. No multi-version matrix needed.
- **`golangci/golangci-lint-action`:** Handles installation, caching, and version pinning for golangci-lint. More reliable than manual installation.
- **Module caching via `setup-go` v5:** Automatically caches `~/go/pkg/mod` keyed on `go.sum`. No separate `actions/cache` step needed.
- **Parallel jobs:** All three jobs run concurrently. Each pays its own Go setup cost, but module caching mitigates this. Benefit is seeing all failure categories at once.
- **No artifacts:** CI is pass/fail only. No binary upload or release automation.
- **Existing Makefile:** CI reuses the `build`, `test`, and `lint` targets already defined in the Makefile where possible, keeping CI config thin and behavior consistent with local development.

## Workflow File

`.github/workflows/ci.yml`
