# Library TUI Redesign

## Overview

Redesign the strategy discovery and management TUI. Consolidate `pvbt discover`, `pvbt list`, and `pvbt remove` under a single `pvbt library` command with a polished BubbleTea interface that supports browsing, searching, installing, and uninstalling strategies.

## Problems addressed

1. Zerolog output bleeds into the TUI, corrupting the display.
2. No way to search strategies (only a basic filter mode).
3. No way to view strategy descriptions or README content.
4. Minimal styling makes the interface hard to scan.
5. Strategy management is split across three separate commands.

## Command structure

- `pvbt library` -- launches the interactive TUI.
- `pvbt library list` -- non-interactive list of installed strategies (replaces `pvbt list`).
- `pvbt library remove <short-code>` -- non-interactive remove (replaces `pvbt remove`).
- `pvbt library --refresh` -- force-refresh strategy listings from GitHub.

The old `pvbt discover`, `pvbt list`, and `pvbt remove` commands are deleted.

## TUI design

### List view (main screen)

- **Header:** styled "pvbt library" title with count of selected strategies.
- **Body:** strategies grouped by category with styled category headers. Each row shows a selection checkbox, owner/name, stars, and short description (truncated to fit terminal width). Installed strategies appear in dim text with a checkmark instead of a checkbox.
- **Footer:** keybinding hints. An "Install selected (N)" row appears at the bottom of the list as a selectable item.

### Keybindings (list view)

| Key | Action |
|-----|--------|
| `j/k`, arrows | Move cursor |
| `space` | Toggle selection |
| `enter` | Open detail view for highlighted item |
| `/` | Enter search mode (filters by name/description, escape clears) |
| `i` | Install all selected strategies |
| `u` | Uninstall highlighted strategy (installed only, with confirmation) |
| `q`, ctrl-c | Quit |

### Detail view (full screen, replaces list)

- **Metadata section:** owner/name, stars, last updated, install status, categories. Displayed in a lipgloss-bordered box.
- **README section:** fetched from GitHub API, rendered with glamour (charmbracelet/glamour), displayed in a scrollable bubbles/viewport.
- `space` toggles selection from this view.
- `j/k` or arrows scroll the README.
- `escape` or `q` returns to the list view.

### Installing state

Shows progress as each strategy installs.

### Done state

Shows results summary, then exits.

## Zerolog handling

Before launching BubbleTea, redirect the global zerolog logger to a `bytes.Buffer`. After the TUI exits, flush the buffer contents to stderr so warnings and errors from registry calls remain visible.

## README fetching

Add `FetchREADME(ctx context.Context, owner, repo string) (string, error)` to the `registry` package. It calls `GET /repos/{owner}/{repo}/readme` with `Accept: application/vnd.github.raw+json` to get plain text. Uses the same auth token resolution as the search API. Results are cached in memory on the model (`map[string]string` keyed by `owner/repo`) so repeated views don't re-fetch.

## Styling

Use lipgloss throughout:

- Bold colored header.
- Category headers with a distinct color.
- Dimmed text for installed items.
- Highlighted/reverse style for the cursor row.
- Styled footer with keybinding hints.
- Detail view: bordered box for metadata, glamour-rendered markdown in a viewport for the README.

## Dependencies

- `github.com/charmbracelet/glamour` -- new direct dependency for markdown rendering.
- `github.com/charmbracelet/bubbles` -- promote from indirect to direct for viewport and textinput components.
- `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss` -- already direct dependencies.

## Files changed

- **Delete:** `cli/discover.go`, `cli/discover_test.go`, `cli/list.go`, `cli/remove.go`.
- **Create:** `cli/library.go` (TUI model, views, keybindings), `cli/library_test.go` (Ginkgo tests).
- **Modify:** `cli/explore.go` (replace `newDiscoverCmd`/`newListCmd`/`newRemoveCmd` with `newLibraryCmd`).
- **Modify:** `registry/registry.go` (add `FetchREADME` function).

## Testing

Ginkgo/Gomega style, matching existing conventions:

- List view: navigation, search filtering, selection toggling, cursor bounds.
- Detail view: enter/escape transitions, selection from detail view, README display.
- Installed items: can't be selected, can be uninstalled.
- "Install selected" bottom row behavior.
- README fetch: mocked to test detail view content.
- Existing registry and library tests unchanged.
