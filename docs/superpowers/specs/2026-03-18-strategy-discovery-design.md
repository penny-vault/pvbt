# Strategy Discovery via `pvbt-strategy` Topic

**Issue:** [#31 - CLI strategy discovery via pvbt-strategy topic](https://github.com/penny-vault/pvbt/issues/31)
**Date:** 2026-03-18
**Platform scope:** macOS and Linux only.

## Overview

Extend pvbt with the ability to discover community strategies published as Go modules tagged with the `pvbt-strategy` GitHub topic, browse them in a terminal UI grouped by category, install selected strategies locally, and run them via `pvbt <short-code>`.

## Commands

| Command | Description |
|---|---|
| `pvbt discover` | Launch TUI for browsing and installing strategies from GitHub |
| `pvbt discover --refresh` | Force refresh of cached discovery results |
| `pvbt list` | Text table of installed strategies (short-code, version, source repo) |
| `pvbt remove <short-code>` | Remove an installed strategy |
| `pvbt <short-code> [subcommand] [flags]` | Dispatch to an installed strategy binary |

## New Packages

### `registry/`

GitHub API client responsible for discovering strategies.

```go
type Listing struct {
    Name        string   // repo name
    Owner       string   // GitHub org/user
    Description string   // repo description
    Categories  []string // topics minus "pvbt-strategy"
    CloneURL    string
    Stars       int
    UpdatedAt   time.Time
}

func Search(ctx context.Context, opts SearchOptions) ([]Listing, error)
```

**Behavior:**

1. Create `~/.pvbt/cache/` with `os.MkdirAll` if it does not exist.
2. Check `~/.pvbt/cache/discover.json`. If present, under 1 hour old, and `ForceRefresh` is false, return cached results.
3. Otherwise, call GitHub Search API with `topic:pvbt-strategy` (paginated, up to 100 results per page, capped at 1000 results per GitHub API limit).
4. For each repo, strip `pvbt-strategy` from its topics. Remaining topics become categories (used as-is from GitHub, which are already lowercase and hyphenated).
5. Authentication: check `GITHUB_TOKEN` env var, then try `gh auth token` subprocess (with a 2-second timeout to avoid hanging if `gh` is misconfigured), then fall back to unauthenticated (60 req/hr).
6. Write results to a temp file, then rename to `discover.json` atomically (avoids corrupted cache on interrupted writes). Include a timestamp in the cached data.
7. Return the listing list.

Log cache hits at debug level, API calls at info level, and errors at error level via zerolog.

### `library/`

Local strategy management: download, build, index, and lookup.

```go
type InstalledStrategy struct {
    ShortCode   string    `json:"short_code"`
    RepoOwner   string    `json:"repo_owner"`
    RepoName    string    `json:"repo_name"`
    Version     string    `json:"version"`      // from Descriptor output
    BinPath     string    `json:"bin_path"`
    InstalledAt time.Time `json:"installed_at"`
}

func Install(ctx context.Context, cloneURL string) (*InstalledStrategy, error)
func List() ([]InstalledStrategy, error)
func Lookup(shortCode string) (*InstalledStrategy, error)
func Remove(shortCode string) error
```

**Install flow:**

1. Create `~/.pvbt/lib/<repo-name>/` with `os.MkdirAll` if it does not exist.
2. `git clone <cloneURL> ~/.pvbt/lib/<repo-name>/module/`.
3. `cd ~/.pvbt/lib/<repo-name>/module/ && go build -o ../bin/<repo-name> .`
4. Run the binary with the `describe` subcommand to extract `Descriptor` output as JSON. The binary calls `strategy.(Descriptor).Describe()` directly on the strategy struct -- no engine initialization required.
5. If the binary does not implement the `Descriptor` interface and the `describe` subcommand fails, the install fails with error: "strategy does not implement Descriptor interface -- cannot determine short-code." Clean up the partially installed directory.
6. If the short-code from `describe` collides with an already-installed strategy from a different repo, fail with error naming both repos. Clean up the partially installed directory. Short-code collision checking skips the strategy being replaced during re-install (same repo).
7. Write `index.json` in the strategy directory. Schema is a serialized `InstalledStrategy` struct (see JSON tags above).
8. Return the installed strategy info.

**Re-install behavior:** Installing a strategy that is already installed (same repo) replaces it. This is the update mechanism for v1 -- there is no separate update command.

**Lookup flow:**

Scan `~/.pvbt/lib/*/index.json` files to find the matching short-code, return the binary path.

**Toolchain requirements:** Both `git` and `go` must be on PATH. Checked at install time, not at browse time.

Log installs at info level, lookups at debug level, and errors at error level via zerolog.

## Local Storage Layout

```
~/.pvbt/
  cache/
    discover.json            # cached GitHub search results + timestamp
  lib/
    momentum-rotation/
      index.json             # serialized InstalledStrategy
      bin/momentum-rotation  # compiled binary
      module/                # git clone of strategy source
    adaptive-allocation/
      index.json
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

### `cli/remove.go` -- Remove Installed Strategy

Delegates to `library.Remove()`. Prints confirmation on success.

### Short-code Dispatch

The root cobra command handles unknown subcommands using cobra's `ValidArgsFunction` and the root command's `Args` handler. This ensures registered subcommands (`discover`, `list`, `remove`, `explore`) are routed normally by cobra. Only when no registered subcommand matches does the dispatch logic fire:

1. If `len(args) == 0`, call `cmd.Help()` and return (preserves default help behavior).
2. Call `library.Lookup(args[0])` to find the binary path.
3. If found, `syscall.Exec` the binary with the remaining args (replacing the pvbt process). Note: `syscall.Exec` is Unix-only; if Windows support is added later, this would need to change to `os/exec.Command` with signal forwarding.
4. If not found, return the standard "unknown command" error.

Example: `pvbt momentum-rotation backtest --start 2020-01-01` execs `~/.pvbt/lib/momentum-rotation/bin/momentum-rotation backtest --start 2020-01-01`.

### `describe` Subcommand

Add a `describe` subcommand to `cli.Run()` (alongside the existing `backtest`, `live`, and `snapshot` subcommands). When invoked, it calls `strategy.(Descriptor).Describe()` directly on the strategy struct without initializing the engine, emits the `StrategyDescription` as JSON to stdout, and exits. If the strategy does not implement `Descriptor`, it exits with a non-zero status and an error message to stderr.

The emitted JSON contains the fields from `StrategyDescription`: `ShortCode`, `Description`, `Source`, `Version`, `VersionDate`. Note that this deliberately excludes the richer `StrategyInfo` data (parameters, schedule, benchmark) which requires engine initialization. The library only needs short-code, description, and version at install time, so this tradeoff is acceptable.

This is how the library extracts short-code and metadata during install.

## Error Handling

**Build failures:** Report the error in the TUI inline next to the strategy. Do not abort the batch -- continue installing other selected strategies.

**Missing Descriptor:** Install fails with error: "strategy does not implement Descriptor interface -- cannot determine short-code." No fallback, no silent degradation. Partially installed files are cleaned up.

**Short-code collisions:** If two strategies declare the same short-code, the second install fails with a clear error naming both repos. User must remove one before installing the other.

**Network errors during discovery:**
- If GitHub API is unreachable and cache exists (even stale), use the stale cache with a warning: "showing cached results from X ago."
- If no cache exists, show the error in the TUI.

**Toolchain requirements:** If `git` or `go` is not on PATH, show an error at install time, not at browse time. Users should still be able to browse available strategies.

## Testing Strategy

**Registry:**
- Unit tests with a fake HTTP server returning canned GitHub API responses.
- Test cache hit/miss/expiry/staleness logic.
- Test auth token detection (env var, gh CLI, unauthenticated fallback).

**Library:**
- Unit tests for index read/write, lookup, short-code collision detection.
- Integration test that installs a test strategy module from a local path (avoids network dependency).
- Test re-install overwrites existing installation.
- Test cleanup on failed install (missing Descriptor, collision).

**TUI:**
- Unit tests for the bubbletea model: state transitions, key handling, selection logic.
- Feed canned registry and library data, assert on model state after key messages.

**Dispatch:**
- Test that unknown subcommands route through library lookup.
- Test that known subcommands (discover, list, remove, explore) are not intercepted.
