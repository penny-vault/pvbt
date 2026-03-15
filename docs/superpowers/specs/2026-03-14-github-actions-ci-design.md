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

## Workflow-Level Settings

- **Permissions:** `contents: read` (principle of least privilege)
- **Concurrency:** Cancel in-progress runs for the same PR branch to save compute

## Jobs

All jobs run on `ubuntu-latest` with the latest stable Go. The `actions/setup-go@v5` action handles Go installation and module caching automatically. All steps use `actions/checkout@v4`.

### 1. `tidy` -- Module Tidiness

Verifies that `go.mod` and `go.sum` are up to date.

Steps:
1. Checkout code (`actions/checkout@v4`)
2. Set up Go (`actions/setup-go@v5`, version `stable`)
3. Run `go mod tidy`
4. Run `git diff --exit-code go.mod go.sum` -- fails if tidy produced changes

### 2. `lint` -- Static Analysis

Runs golangci-lint, which already includes govet and gofmt as part of its standard linter set.

Steps:
1. Checkout code (`actions/checkout@v4`)
2. Set up Go (`actions/setup-go@v5`, version `stable`)
3. Run `golangci-lint` via `golangci/golangci-lint-action@v7`

Note: No separate `go vet` or `go fmt` steps are needed -- golangci-lint's standard linters include `govet`, and the `.golangci.yml` enables the `gofmt` formatter. In CI, golangci-lint detects formatting issues (fails the build) rather than silently rewriting files, which is the correct behavior.

### 3. `test` -- Build and Test

Compiles the binary and runs the full test suite with race detection.

Steps:
1. Checkout code (`actions/checkout@v4`)
2. Set up Go (`actions/setup-go@v5`, version `stable`)
3. Run `make build`
4. Install Ginkgo CLI via `go install github.com/onsi/ginkgo/v2/ginkgo` (uses version from `go.mod`)
5. Run `make test` (`ginkgo run -race ./...`)

## Key Decisions

- **Go version `stable`:** Auto-updates to latest stable release. No multi-version matrix needed.
- **`golangci/golangci-lint-action@v7`:** Handles installation, caching, and version pinning for golangci-lint. More reliable than manual installation.
- **Module caching via `setup-go` v5:** Automatically caches `~/go/pkg/mod` keyed on `go.sum`. No separate `actions/cache` step needed.
- **Parallel jobs:** All three jobs run concurrently. Each pays its own Go setup cost, but module caching mitigates this. Benefit is seeing all failure categories at once.
- **No artifacts:** CI is pass/fail only. No binary upload or release automation.
- **Existing Makefile:** CI reuses the `build` and `test` targets already defined in the Makefile, keeping CI config thin and behavior consistent with local development.
- **Ginkgo version from `go.mod`:** Installing without `@latest` ensures CI uses the same Ginkgo version as local development, preventing version drift.
- **Permissions hardening:** Workflow requests only `contents: read`, following the principle of least privilege.
- **Concurrency control:** Cancels in-progress runs on the same PR branch to avoid wasting Actions minutes on superseded pushes.

## Workflow File

`.github/workflows/ci.yml`
