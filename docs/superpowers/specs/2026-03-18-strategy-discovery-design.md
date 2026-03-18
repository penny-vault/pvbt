# Strategy Discovery via `pvbt-strategy` Topic

**Issue:** [#31 - CLI strategy discovery via pvbt-strategy topic](https://github.com/penny-vault/pvbt/issues/31)
**Date:** 2026-03-18

## Overview

Extend pvbt with the ability to discover community strategies published as Go modules tagged with the `pvbt-strategy` GitHub topic, browse them in a terminal UI grouped by category, install selected strategies locally, and run them via `pvbt <short-code>`.

## Commands

| Command | Description |
|---|---|
| `pvbt discover` | Launch TUI for browsing and installing strategies from GitHub |
| `pvbt discover --refresh` | Force refresh of cached discovery results |
| `pvbt list` | Text table of installed strategies (short-code, version, source repo) |
| `pvbt <short-code> [subcommand] [flags]` | Dispatch to an installed strategy binary |

## New Packages

### `registry/`

GitHub API client responsible for discovering strategies.

```go
type Strategy struct {
    Name        string   // repo name
    Owner       string   // GitHub org/user
    Description string   // repo description
    Categories  []string // topics minus "pvbt-strategy"
    CloneURL    string
    Stars       int
    UpdatedAt   time.Time
}

func Search(ctx context.Context, opts SearchOptions) ([]Strategy, error)
```

**Behavior:**

1. Check `~/.pvbt/cache/discover.json`. If present, under 1 hour old, and `ForceRefresh` is false, return cached results.
2. Otherwise, call GitHub Search API with `topic:pvbt-strategy` (paginated, up to 100 results per page).
3. For each repo, strip `pvbt-strategy` from its topics. Remaining topics become categories.
4. Authentication: check `GITHUB_TOKEN` env var, then try `gh auth token` subprocess, then fall back to unauthenticated (60 req/hr).
5. Write results and timestamp to cache file.
6. Return the strategy list.

### `library/`

Local strategy management: download, build, index, and lookup.

```go
type InstalledStrategy struct {
    ShortCode   string
    RepoOwner   string
    RepoName    string
    Version     string    // go module version or commit hash
    BinPath     string    // path to compiled binary
    InstalledAt time.Time
}

func Install(ctx context.Context, cloneURL string) (*InstalledStrategy, error)
func List() ([]InstalledStrategy, error)
func Lookup(shortCode string) (*InstalledStrategy, error)
func Remove(shortCode string) error
```

**Install flow:**

1. `go mod download` the module into `~/.pvbt/lib/<repo-name>/module/`.
2. Build the binary into `~/.pvbt/lib/<repo-name>/bin/`.
3. Run the binary with `--describe` to extract `Descriptor` output (short-code, description, version) as JSON.
4. If the binary does not respond to `--describe`, the install fails with error: "strategy does not implement Descriptor interface -- cannot determine short-code."
5. Write an `index.json` in the strategy directory recording the `InstalledStrategy` metadata.
6. Return the installed strategy info.

**Lookup flow:**

Scan `~/.pvbt/lib/*/index.json` files to find the matching short-code, return the binary path.

## Local Storage Layout

```
~/.pvbt/
  cache/
    discover.json            # cached GitHub search results + timestamp
  lib/
    momentum-rotation/
      bin/momentum-rotation  # compiled binary
      module/                # Go module source
    adaptive-allocation/
      bin/adaptive-allocation
      module/
```

## CLI Changes

### `cli/discover.go` -- Discovery TUI

Built with bubbletea, following existing patterns in `cli/tui.go` and `cli/explore_graph.go`.

**Layout:**

```
Strategy Discovery                          [q] quit  [/] filter  [enter] install

  tactical-asset-allocation
    [ ] adaptive-allocation    Risk-parity with momentum overlay     * 42
    [x] dual-momentum          Classic dual momentum rotation        * 87

  mean-reversion
    [ ] pairs-trading          Statistical arbitrage pairs           * 23

  uncategorized
    [ ] my-custom-strat        No category topics                    * 5

  installed
    momentum-rotation          v0.3.1                                * 120
```

**Interaction:**

- Arrow keys / j,k to navigate
- Spacebar to toggle selection
- `/` to filter/search by name
- `Enter` to install all selected strategies (shows progress inline)
- `q` to quit
- Categories are collapsible headers
- Strategies without non-`pvbt-strategy` topics go under "uncategorized"
- Already-installed strategies shown in a separate "installed" section with version info, not selectable

**Model states:** `loading` (fetching from registry), `browsing` (main view), `installing` (download/build progress), `done`.

### `cli/list.go` -- Installed Strategy List

Simple text table output of installed strategies: short-code, version, source repo.

### Short-code Dispatch

The root cobra command intercepts unknown subcommands:

1. Add a `RunE` on the root command that catches unrecognized args.
2. Call `library.Lookup(args[0])` to find the binary path.
3. If found, `syscall.Exec` the binary with the remaining args (replacing the pvbt process).
4. If not found, fall through to the normal "unknown command" error.

Example: `pvbt momentum-rotation backtest --start 2020-01-01` execs `~/.pvbt/lib/momentum-rotation/bin/momentum-rotation backtest --start 2020-01-01`.

### `--describe` Flag

Add a `--describe` flag to `cli.Run()` so that any strategy built with pvbt emits its `Descriptor` output as JSON and exits. This is how the library extracts short-code and metadata during install.

## Error Handling

**Build failures:** Report the error in the TUI inline next to the strategy. Do not abort the batch -- continue installing other selected strategies.

**Missing Descriptor:** Install fails with error: "strategy does not implement Descriptor interface -- cannot determine short-code." No fallback, no silent degradation.

**Short-code collisions:** If two strategies declare the same short-code, the second install fails with a clear error naming both repos. User must remove one before installing the other.

**Network errors during discovery:**
- If GitHub API is unreachable and cache exists (even stale), use the stale cache with a warning: "showing cached results from X ago."
- If no cache exists, show the error in the TUI.

**Go toolchain requirement:** If `go` is not on PATH, show an error at install time, not at browse time. Users should still be able to browse available strategies.

## Testing Strategy

**Registry:**
- Unit tests with a fake HTTP server returning canned GitHub API responses.
- Test cache hit/miss/expiry/staleness logic.
- Test auth token detection (env var, gh CLI, unauthenticated fallback).

**Library:**
- Unit tests for index read/write, lookup, short-code collision detection.
- Integration test that installs a test strategy module from a local path (avoids network dependency).

**TUI:**
- Unit tests for the bubbletea model: state transitions, key handling, selection logic.
- Feed canned registry and library data, assert on model state after key messages.

**Dispatch:**
- Test that unknown subcommands route through library lookup.
- Test that known subcommands (discover, list, backtest) are not intercepted.
