# Library TUI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Consolidate `pvbt discover`, `pvbt list`, and `pvbt remove` into a single `pvbt library` command with a polished TUI that supports browsing, searching, detail views with README rendering, installing, and uninstalling strategies.

**Architecture:** BubbleTea TUI with two views (list and detail). The list view shows strategies grouped by category with search filtering. The detail view fetches and renders the repo README via glamour. A zerolog buffer redirect prevents log corruption of the TUI. The cobra command tree is restructured with `library` as the parent command and `list`/`remove` as subcommands.

**Tech Stack:** Go, BubbleTea, Lipgloss, Bubbles (viewport), Glamour, Ginkgo/Gomega, Cobra

**Spec:** `docs/superpowers/specs/2026-03-23-library-tui-redesign-design.md`

---

## File Structure

| File | Responsibility |
|------|---------------|
| `registry/readme.go` (create) | `FetchREADME` function and `ReadmeOptions` struct |
| `registry/readme_test.go` (create) | Tests for `FetchREADME` |
| `cli/library.go` (create) | TUI model, views, keybindings, cobra command |
| `cli/library_test.go` (create) | Ginkgo tests for TUI model |
| `cli/explore.go` (modify) | Replace old command registrations with `newLibraryCmd` |
| `cli/discover.go` (delete) | Replaced by `cli/library.go` |
| `cli/discover_test.go` (delete) | Replaced by `cli/library_test.go` |
| `cli/list.go` (delete) | Moved into `newLibraryCmd` as subcommand |
| `cli/remove.go` (delete) | Moved into `newLibraryCmd` as subcommand |

---

### Task 1: Add FetchREADME to registry package

**Files:**
- Create: `registry/readme.go`
- Create: `registry/readme_test.go`

- [ ] **Step 1: Write the failing test for FetchREADME**

Create `registry/readme_test.go`:

```go
package registry_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/registry"
)

var _ = Describe("FetchREADME", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("fetches raw README content from GitHub API", func() {
		readmeContent := "# My Strategy\n\nThis is a test strategy."
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			Expect(req.URL.Path).To(Equal("/repos/alice/momentum/readme"))
			Expect(req.Header.Get("Accept")).To(Equal("application/vnd.github.raw+json"))
			fmt.Fprint(writer, readmeContent)
		}))
		defer server.Close()

		opts := registry.ReadmeOptions{BaseURL: server.URL}
		content, err := registry.FetchREADME(ctx, "alice", "momentum", opts)
		Expect(err).NotTo(HaveOccurred())
		Expect(content).To(Equal(readmeContent))
	})

	It("returns error on non-200 response", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
			writer.WriteHeader(http.StatusNotFound)
			fmt.Fprint(writer, `{"message": "Not Found"}`)
		}))
		defer server.Close()

		opts := registry.ReadmeOptions{BaseURL: server.URL}
		_, err := registry.FetchREADME(ctx, "alice", "missing", opts)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("404"))
	})

	It("returns error on network failure", func() {
		opts := registry.ReadmeOptions{BaseURL: "http://127.0.0.1:1"}
		_, err := registry.FetchREADME(ctx, "alice", "repo", opts)
		Expect(err).To(HaveOccurred())
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./registry/`
Expected: FAIL -- `FetchREADME` and `ReadmeOptions` not defined

- [ ] **Step 3: Write minimal implementation**

Create `registry/readme.go`:

```go
package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// ReadmeOptions configures the FetchREADME function.
type ReadmeOptions struct {
	BaseURL string
}

// FetchREADME fetches the raw README content for a GitHub repository.
// It calls GET /repos/{owner}/{repo}/readme with Accept: application/vnd.github.raw+json.
func FetchREADME(ctx context.Context, owner, repo string, opts ReadmeOptions) (string, error) {
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	requestURL := fmt.Sprintf("%s/repos/%s/%s/readme", baseURL, owner, repo)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating readme request: %w", err)
	}

	request.Header.Set("Accept", "application/vnd.github.raw+json")

	authToken := resolveAuthToken()
	if authToken != "" {
		request.Header.Set("Authorization", "Bearer "+authToken)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("fetching readme: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("reading readme response: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d for readme: %s", response.StatusCode, string(body))
	}

	return string(body), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./registry/`
Expected: PASS (all registry tests including new FetchREADME tests)

- [ ] **Step 5: Commit**

```bash
git add registry/readme.go registry/readme_test.go
git commit -m "feat: add FetchREADME to registry package"
```

---

### Task 2: Add glamour dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add glamour dependency**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go get github.com/charmbracelet/glamour
```

- [ ] **Step 2: Verify build still works**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add charmbracelet/glamour for markdown rendering"
```

---

### Task 3: Create library TUI model and types

This task creates the core model, types, message types, and constructor -- enough to compile but with no view or update logic yet.

**Files:**
- Create: `cli/library.go`
- Create: `cli/library_test.go`

- [ ] **Step 1: Write the failing test for model construction and initial state**

Create `cli/library_test.go`:

```go
package cli

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/library"
	"github.com/penny-vault/pvbt/registry"
)

var _ = Describe("libraryModel", func() {
	var model libraryModel

	BeforeEach(func() {
		model = newLibraryModel("", "", false)
	})

	It("starts in loading state", func() {
		Expect(model.state).To(Equal(libStateLoading))
	})

	It("transitions to browsing on listings received", func() {
		listings := []registry.Listing{
			{Name: "strat1", Owner: "user1", Categories: []string{"cat1"}, Stars: 10},
			{Name: "strat2", Owner: "user2", Categories: []string{"cat2"}, Stars: 20},
		}

		updated, _ := model.Update(libListingsMsg{listings: listings})
		model = updated.(libraryModel)

		Expect(model.state).To(Equal(libStateBrowsing))
		Expect(model.items).To(HaveLen(2))
		Expect(model.categories).To(ConsistOf("cat1", "cat2"))
	})

	It("sorts categories alphabetically", func() {
		listings := []registry.Listing{
			{Name: "s1", Owner: "o1", Categories: []string{"zebra"}},
			{Name: "s2", Owner: "o2", Categories: []string{"alpha"}},
			{Name: "s3", Owner: "o3", Categories: []string{"middle"}},
		}

		updated, _ := model.Update(libListingsMsg{listings: listings})
		model = updated.(libraryModel)

		Expect(model.categories).To(Equal([]string{"alpha", "middle", "zebra"}))
	})

	It("marks installed strategies and populates shortcode map", func() {
		model.state = libStateBrowsing
		model.items = []libraryItem{
			{listing: registry.Listing{Name: "strat1"}},
			{listing: registry.Listing{Name: "strat2"}},
		}

		installed := []library.InstalledStrategy{
			{RepoName: "strat1", ShortCode: "s1"},
		}

		updated, _ := model.Update(libInstalledMsg{installed: installed})
		model = updated.(libraryModel)

		Expect(model.items[0].installed).To(BeTrue())
		Expect(model.items[1].installed).To(BeFalse())
		Expect(model.shortCodes).To(HaveKeyWithValue("strat1", "s1"))
	})
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: FAIL -- types not defined

- [ ] **Step 3: Write the model, types, and constructor**

Create `cli/library.go`:

```go
package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/library"
	"github.com/penny-vault/pvbt/registry"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// TUI state constants.
const (
	libStateLoading      = "loading"
	libStateBrowsing     = "browsing"
	libStateDetail       = "detail"
	libStateConfirmUninst = "confirm_uninstall"
	libStateInstalling   = "installing"
	libStateDone         = "done"
)

// libraryItem represents a single strategy in the TUI.
type libraryItem struct {
	listing   registry.Listing
	selected  bool
	installed bool
	category  string
}

// Message types.

type libListingsMsg struct {
	listings []registry.Listing
	err      error
}

type libInstalledMsg struct {
	installed []library.InstalledStrategy
	err       error
}

type libInstallResultMsg struct {
	repoName string
	err      error
}

type libBatchInstallDoneMsg struct {
	results []libInstallResultMsg
}

type libReadmeMsg struct {
	owner   string
	repo    string
	content string
	err     error
}

type libUninstallResultMsg struct {
	repoName string
	err      error
}

// libraryModel is the bubbletea model for the library TUI.
type libraryModel struct {
	state        string
	items        []libraryItem
	categories   []string
	cursor       int
	width        int
	height       int
	filter       string
	filtering    bool
	forceRefresh bool
	cacheDir     string
	libDir       string
	results      []libInstallResultMsg
	err          error

	// Detail view state
	detailIndex int
	viewport    viewport.Model
	readmeCache map[string]string

	// Uninstall confirmation state
	uninstallTarget string

	// Installed strategy short-code map (repo name -> short-code)
	shortCodes map[string]string
}

// newLibraryModel creates a new libraryModel with sensible defaults.
func newLibraryModel(cacheDir, libDir string, forceRefresh bool) libraryModel {
	if cacheDir == "" {
		cacheDir = library.DefaultCacheDir()
	}

	if libDir == "" {
		libDir = library.DefaultLibDir()
	}

	return libraryModel{
		state:        libStateLoading,
		cacheDir:     cacheDir,
		libDir:       libDir,
		forceRefresh: forceRefresh,
		readmeCache:  make(map[string]string),
		shortCodes:   make(map[string]string),
	}
}

// Init returns the initial commands to fetch listings and installed strategies.
func (model libraryModel) Init() tea.Cmd {
	return tea.Batch(
		libFetchListings(model.cacheDir, model.forceRefresh),
		libFetchInstalled(model.libDir),
	)
}

// buildItems groups listings by their first category, sorted alphabetically.
func (model *libraryModel) buildItems(listings []registry.Listing) {
	model.items = make([]libraryItem, 0, len(listings))
	categorySet := make(map[string]bool)

	for _, listing := range listings {
		categoryName := "uncategorized"
		if len(listing.Categories) > 0 {
			categoryName = listing.Categories[0]
		}

		categorySet[categoryName] = true

		model.items = append(model.items, libraryItem{
			listing:  listing,
			category: categoryName,
		})
	}

	model.categories = make([]string, 0, len(categorySet))
	for cat := range categorySet {
		model.categories = append(model.categories, cat)
	}

	sort.Strings(model.categories)
}

// markInstalled marks items whose repo names match installed strategies
// and populates the shortCodes map.
func (model *libraryModel) markInstalled(installed []library.InstalledStrategy) {
	for _, strategy := range installed {
		model.shortCodes[strategy.RepoName] = strategy.ShortCode
	}

	for idx := range model.items {
		if _, isInstalled := model.shortCodes[model.items[idx].listing.Name]; isInstalled {
			model.items[idx].installed = true
			model.items[idx].selected = false
		}
	}
}

// visibleItems returns the indices of items that match the current filter.
func (model libraryModel) visibleItems() []int {
	if model.filter == "" {
		indices := make([]int, len(model.items))
		for idx := range model.items {
			indices[idx] = idx
		}

		return indices
	}

	lowerFilter := strings.ToLower(model.filter)

	var indices []int

	for idx, item := range model.items {
		nameMatch := strings.Contains(strings.ToLower(item.listing.Name), lowerFilter)
		descMatch := strings.Contains(strings.ToLower(item.listing.Description), lowerFilter)

		if nameMatch || descMatch {
			indices = append(indices, idx)
		}
	}

	return indices
}

// selectedCount returns the number of selected (non-installed) strategies.
func (model libraryModel) selectedCount() int {
	count := 0
	for _, item := range model.items {
		if item.selected && !item.installed {
			count++
		}
	}

	return count
}

// Tea commands for fetching data.

func libFetchListings(cacheDir string, forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		opts := registry.SearchOptions{
			CacheDir:     cacheDir,
			BaseURL:      "https://api.github.com",
			ForceRefresh: forceRefresh,
		}

		listings, err := registry.Search(ctx, opts)

		return libListingsMsg{listings: listings, err: err}
	}
}

func libFetchInstalled(libDir string) tea.Cmd {
	return func() tea.Msg {
		installed, err := library.List(libDir)

		return libInstalledMsg{installed: installed, err: err}
	}
}
```

Note: The `Update` and `View` methods, plus the cobra command, will be added in subsequent tasks. For now the model needs a stub `Update` and `View` to satisfy the `tea.Model` interface -- add these minimal stubs:

```go
// Update handles messages and returns the updated model.
func (model libraryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMsg := msg.(type) {
	case tea.WindowSizeMsg:
		model.width = typedMsg.Width
		model.height = typedMsg.Height

		return model, nil

	case libListingsMsg:
		if typedMsg.err != nil {
			model.err = typedMsg.err
			model.state = libStateDone

			return model, tea.Quit
		}

		model.buildItems(typedMsg.listings)
		model.state = libStateBrowsing

		return model, nil

	case libInstalledMsg:
		if typedMsg.err != nil {
			log.Warn().Err(typedMsg.err).Msg("failed to list installed strategies")
		}

		model.markInstalled(typedMsg.installed)

		return model, nil
	}

	return model, nil
}

// View renders the TUI.
func (model libraryModel) View() string {
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/library.go cli/library_test.go
git commit -m "feat: add library TUI model, types, and constructor"
```

---

### Task 4: List view navigation and selection

Add key handling for list view: j/k movement, space to select, `/` to filter, `q`/ctrl-c to quit.

**Files:**
- Modify: `cli/library.go`
- Modify: `cli/library_test.go`

- [ ] **Step 1: Write failing tests for list view navigation**

Append to `cli/library_test.go` inside the outer `Describe`:

```go
	Describe("list view navigation", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: registry.Listing{Name: "strat1", Owner: "o1"}, category: "cat1"},
				{listing: registry.Listing{Name: "strat2", Owner: "o2"}, category: "cat1"},
				{listing: registry.Listing{Name: "strat3", Owner: "o3"}, category: "cat2"},
			}
			model.categories = []string{"cat1", "cat2"}
		})

		It("moves cursor down with j", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(1))
		})

		It("moves cursor up with k", func() {
			model.cursor = 2
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(1))
		})

		It("does not move cursor below last item", func() {
			model.cursor = 2
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(2))
		})

		It("does not move cursor above first item", func() {
			model.cursor = 0
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(0))
		})

		It("toggles selection with space", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeTrue())

			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeFalse())
		})

		It("does not allow selecting installed items", func() {
			model.items[0].installed = true
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeFalse())
		})

		It("accounts for install row in cursor bounds when items are selected", func() {
			model.items[0].selected = true
			// With 3 items + 1 install row = 4 positions (indices 0-3)
			model.cursor = 3
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(3))
		})

		It("moves cursor down with arrow key", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(1))
		})

		It("moves cursor up with arrow key", func() {
			model.cursor = 1
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(0))
		})

		It("triggers install with i key when items are selected", func() {
			model.items[0].selected = true
			_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			Expect(cmd).NotTo(BeNil())
		})

		It("returns nil command for i key with no selection", func() {
			_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			Expect(cmd).To(BeNil())
		})

		It("space does nothing on install row", func() {
			model.items[0].selected = true
			model.cursor = 3 // install row position
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			// No item selection changed
			selectedCount := 0
			for _, item := range model.items {
				if item.selected {
					selectedCount++
				}
			}
			Expect(selectedCount).To(Equal(1))
		})
	})

	Describe("search filtering", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: registry.Listing{Name: "momentum", Description: "A momentum strategy"}},
				{listing: registry.Listing{Name: "mean-revert", Description: "Mean reversion"}},
				{listing: registry.Listing{Name: "volatility", Description: "Vol targeting"}},
			}
			model.categories = []string{"uncategorized"}
		})

		It("enters filter mode with /", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeTrue())
		})

		It("filters items by name", func() {
			model.filtering = true
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
			model = updated.(libraryModel)

			visible := model.visibleItems()
			Expect(visible).To(HaveLen(2)) // momentum and mean-revert
		})

		It("filters items by description", func() {
			model.filtering = true
			model.filter = "vol"
			visible := model.visibleItems()
			Expect(visible).To(HaveLen(1))
		})

		It("clears filter on escape", func() {
			model.filtering = true
			model.filter = "mom"
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeFalse())
			Expect(model.filter).To(Equal(""))
		})

		It("locks in filter on enter", func() {
			model.filtering = true
			model.filter = "mom"
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeFalse())
			Expect(model.filter).To(Equal("mom"))
		})
	})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: FAIL -- key handling not implemented

- [ ] **Step 3: Implement list view key handling**

Update `Update` method in `cli/library.go` to dispatch to `handleListKey` and `handleFilterKey` when state is `libStateBrowsing`:

```go
// Update handles messages and returns the updated model.
func (model libraryModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMsg := msg.(type) {
	case tea.KeyMsg:
		switch model.state {
		case libStateBrowsing:
			if model.filtering {
				return model.handleFilterKey(typedMsg)
			}

			return model.handleListKey(typedMsg)
		}

	case tea.WindowSizeMsg:
		model.width = typedMsg.Width
		model.height = typedMsg.Height

		return model, nil

	case libListingsMsg:
		if typedMsg.err != nil {
			model.err = typedMsg.err
			model.state = libStateDone

			return model, tea.Quit
		}

		model.buildItems(typedMsg.listings)
		model.state = libStateBrowsing

		return model, nil

	case libInstalledMsg:
		if typedMsg.err != nil {
			log.Warn().Err(typedMsg.err).Msg("failed to list installed strategies")
		}

		model.markInstalled(typedMsg.installed)

		return model, nil
	}

	return model, nil
}

// maxCursorPosition returns the maximum valid cursor position,
// accounting for the install row when items are selected.
func (model libraryModel) maxCursorPosition() int {
	visible := model.visibleItems()
	maxPos := len(visible) - 1

	if model.selectedCount() > 0 {
		maxPos++ // install row
	}

	return maxPos
}

// isOnInstallRow returns true if the cursor is on the install row.
func (model libraryModel) isOnInstallRow() bool {
	visible := model.visibleItems()

	return model.selectedCount() > 0 && model.cursor == len(visible)
}

// handleListKey processes key events in list/browsing mode.
func (model libraryModel) handleListKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyCtrlC:
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q':
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'j',
		keyMsg.Type == tea.KeyDown:
		if model.cursor < model.maxCursorPosition() {
			model.cursor++
		}

		return model, nil

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'k',
		keyMsg.Type == tea.KeyUp:
		if model.cursor > 0 {
			model.cursor--
		}

		return model, nil

	case keyMsg.Type == tea.KeySpace:
		if model.isOnInstallRow() {
			return model, nil
		}

		visible := model.visibleItems()
		if model.cursor < len(visible) {
			itemIndex := visible[model.cursor]
			if !model.items[itemIndex].installed {
				model.items[itemIndex].selected = !model.items[itemIndex].selected
			}
		}

		return model, nil

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == '/':
		model.filtering = true
		model.filter = ""

		return model, nil

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'i':
		cmd := model.installSelected()
		if cmd != nil {
			model.state = libStateInstalling
		}

		return model, cmd

	case keyMsg.Type == tea.KeyEnter:
		if model.isOnInstallRow() {
			model.state = libStateInstalling

			return model, model.installSelected()
		}

		// Transition to detail view
		visible := model.visibleItems()
		if model.cursor >= len(visible) {
			return model, nil
		}

		itemIndex := visible[model.cursor]
		model.state = libStateDetail
		model.detailIndex = itemIndex
		model.viewport = viewport.New(model.width, model.height-8)

		item := model.items[itemIndex]
		cacheKey := item.listing.Owner + "/" + item.listing.Name

		if cached, hasCached := model.readmeCache[cacheKey]; hasCached {
			rendered := model.renderMarkdown(cached)
			model.viewport.SetContent(rendered)

			return model, nil
		}

		model.viewport.SetContent("Loading README...")

		owner := item.listing.Owner
		repo := item.listing.Name

		return model, func() tea.Msg {
			ctx := context.Background()
			content, fetchErr := registry.FetchREADME(ctx, owner, repo, registry.ReadmeOptions{})

			return libReadmeMsg{owner: owner, repo: repo, content: content, err: fetchErr}
		}
	}

	return model, nil
}

// handleFilterKey processes key events while in filter mode.
func (model libraryModel) handleFilterKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch keyMsg.Type {
	case tea.KeyEsc:
		model.filtering = false
		model.filter = ""
		model.cursor = 0

		return model, nil

	case tea.KeyEnter:
		model.filtering = false

		return model, nil

	case tea.KeyBackspace:
		if len(model.filter) > 0 {
			model.filter = model.filter[:len(model.filter)-1]
			model.cursor = 0
		}

		return model, nil

	case tea.KeyRunes:
		model.filter += string(keyMsg.Runes)
		model.cursor = 0

		return model, nil
	}

	return model, nil
}

// installSelected creates a command that installs all selected strategies.
func (model libraryModel) installSelected() tea.Cmd {
	var toInstall []registry.Listing

	for _, item := range model.items {
		if item.selected && !item.installed {
			toInstall = append(toInstall, item.listing)
		}
	}

	if len(toInstall) == 0 {
		return nil
	}

	libDir := model.libDir

	return func() tea.Msg {
		var results []libInstallResultMsg

		ctx := context.Background()

		for _, listing := range toInstall {
			_, installErr := library.Install(ctx, libDir, listing.CloneURL)
			results = append(results, libInstallResultMsg{
				repoName: listing.Name,
				err:      installErr,
			})
		}

		return libBatchInstallDoneMsg{results: results}
	}
}

```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/library.go cli/library_test.go
git commit -m "feat: add list view navigation, selection, and search filtering"
```

---

### Task 5: Detail view with README rendering

Add the detail view: enter from list, fetch README, render with glamour in a viewport, escape to return.

**Files:**
- Modify: `cli/library.go`
- Modify: `cli/library_test.go`

- [ ] **Step 1: Write failing tests for detail view**

Append to `cli/library_test.go`:

```go
	Describe("detail view", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: registry.Listing{Name: "strat1", Owner: "user1", Description: "A strategy", Stars: 42, UpdatedAt: "2026-01-15", Categories: []string{"momentum"}}},
				{listing: registry.Listing{Name: "strat2", Owner: "user2"}},
			}
			model.categories = []string{"momentum"}
			model.width = 80
			model.height = 24
		})

		It("transitions to detail state on enter", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateDetail))
			Expect(model.detailIndex).To(Equal(0))
		})

		It("returns to browsing on escape from detail view", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("returns to browsing on q from detail view", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("toggles selection from detail view with space", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeTrue())
		})

		It("does not select installed items from detail view", func() {
			model.items[0].installed = true
			model.state = libStateDetail
			model.detailIndex = 0
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeFalse())
		})

		It("triggers install from detail view with i key", func() {
			model.items[0].selected = true
			model.state = libStateDetail
			model.detailIndex = 0
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			resultModel := updated.(libraryModel)
			Expect(cmd).NotTo(BeNil())
			Expect(resultModel.state).To(Equal(libStateInstalling))
		})

		It("populates viewport with README content on message", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(libReadmeMsg{
				owner:   "user1",
				repo:    "strat1",
				content: "# README\n\nHello world.",
			})
			model = updated.(libraryModel)

			Expect(model.readmeCache).To(HaveKey("user1/strat1"))
		})

		It("caches README and does not re-fetch", func() {
			model.readmeCache["user1/strat1"] = "# Cached"
			model.state = libStateBrowsing
			model.cursor = 0

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)

			// Should transition to detail without issuing a fetch command
			// (the README is already cached)
			Expect(model.state).To(Equal(libStateDetail))
			// cmd should be nil since README is cached
			Expect(cmd).To(BeNil())
		})

		It("shows error message when README fetch fails", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(libReadmeMsg{
				owner: "user1",
				repo:  "strat1",
				err:   fmt.Errorf("network timeout"),
			})
			model = updated.(libraryModel)

			Expect(model.readmeCache).To(HaveKey("user1/strat1"))
			Expect(model.readmeCache["user1/strat1"]).To(ContainSubstring("README not available"))
		})
	})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: FAIL

- [ ] **Step 3: Implement detail view logic**

Update `cli/library.go`:

1. Add the `renderMarkdown` helper (the detail view enter logic is already inline in `handleListKey` from Task 4):

```go
// renderMarkdown renders markdown content using glamour.
func (model libraryModel) renderMarkdown(content string) string {
	renderWidth := model.width - 4
	if renderWidth < 40 {
		renderWidth = 40
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(renderWidth),
	)
	if err != nil {
		return content
	}

	rendered, err := renderer.Render(content)
	if err != nil {
		return content
	}

	return rendered
}
```

2. Add detail view key handling to the `Update` method's `tea.KeyMsg` switch:

```go
		case libStateDetail:
			return model.handleDetailKey(typedMsg)
```

3. Add `libReadmeMsg` handling to `Update`:

```go
	case libReadmeMsg:
		cacheKey := typedMsg.owner + "/" + typedMsg.repo
		if typedMsg.err != nil {
			model.readmeCache[cacheKey] = "README not available."
			model.viewport.SetContent("README not available.")
		} else {
			model.readmeCache[cacheKey] = typedMsg.content
			rendered := model.renderMarkdown(typedMsg.content)
			model.viewport.SetContent(rendered)
		}

		return model, nil
```

4. Add `libBatchInstallDoneMsg` handling to `Update`:

```go
	case libBatchInstallDoneMsg:
		model.results = typedMsg.results
		model.state = libStateDone

		return model, tea.Quit
```

5. Add `handleDetailKey`:

```go
// handleDetailKey processes key events in the detail view.
func (model libraryModel) handleDetailKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyCtrlC:
		return model, tea.Quit

	case keyMsg.Type == tea.KeyEsc,
		keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q':
		model.state = libStateBrowsing

		return model, nil

	case keyMsg.Type == tea.KeySpace:
		if !model.items[model.detailIndex].installed {
			model.items[model.detailIndex].selected = !model.items[model.detailIndex].selected
		}

		return model, nil

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'i':
		cmd := model.installSelected()
		if cmd != nil {
			model.state = libStateInstalling
		}

		return model, cmd

	default:
		// Pass through to viewport for scrolling
		var cmd tea.Cmd
		model.viewport, cmd = model.viewport.Update(keyMsg)

		return model, cmd
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/library.go cli/library_test.go
git commit -m "feat: add detail view with README fetching and glamour rendering"
```

---

### Task 6: Uninstall flow

Add inline y/n confirmation for uninstalling installed strategies.

**Files:**
- Modify: `cli/library.go`
- Modify: `cli/library_test.go`

- [ ] **Step 1: Write failing tests for uninstall**

Append to `cli/library_test.go`:

```go
	Describe("uninstall flow", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: registry.Listing{Name: "strat1", Owner: "o1"}, installed: true, category: "cat1"},
				{listing: registry.Listing{Name: "strat2", Owner: "o2"}, category: "cat1"},
			}
			model.categories = []string{"cat1"}
			model.shortCodes = map[string]string{"strat1": "s1"}
		})

		It("enters confirmation state on u for installed strategy", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateConfirmUninst))
			Expect(model.uninstallTarget).To(Equal("strat1"))
		})

		It("ignores u for non-installed strategy", func() {
			model.cursor = 1
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("cancels uninstall on n", func() {
			model.state = libStateConfirmUninst
			model.uninstallTarget = "strat1"
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(model.uninstallTarget).To(Equal(""))
		})

		It("cancels uninstall on escape", func() {
			model.state = libStateConfirmUninst
			model.uninstallTarget = "strat1"
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("clears installed flag on successful uninstall", func() {
			model.state = libStateBrowsing
			updated, _ := model.Update(libUninstallResultMsg{repoName: "strat1"})
			model = updated.(libraryModel)
			Expect(model.items[0].installed).To(BeFalse())
		})
	})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: FAIL

- [ ] **Step 3: Implement uninstall flow**

Add to `handleListKey` in `cli/library.go`, in the switch statement:

```go
	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'u':
		visible := model.visibleItems()
		if model.cursor < len(visible) {
			itemIndex := visible[model.cursor]
			if model.items[itemIndex].installed {
				model.state = libStateConfirmUninst
				model.uninstallTarget = model.items[itemIndex].listing.Name
			}
		}

		return model, nil
```

Add confirmation key handling to the `Update` switch:

```go
		case libStateConfirmUninst:
			return model.handleConfirmUninstallKey(typedMsg)
```

Add the confirmation handler:

```go
// handleConfirmUninstallKey processes y/n/escape in uninstall confirmation.
func (model libraryModel) handleConfirmUninstallKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'y':
		repoName := model.uninstallTarget
		shortCode := model.shortCodes[repoName]
		libDir := model.libDir
		model.state = libStateBrowsing
		model.uninstallTarget = ""

		return model, func() tea.Msg {
			err := library.Remove(libDir, shortCode)

			return libUninstallResultMsg{repoName: repoName, err: err}
		}

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'n',
		keyMsg.Type == tea.KeyEsc:
		model.state = libStateBrowsing
		model.uninstallTarget = ""

		return model, nil
	}

	return model, nil
}
```

Add `libUninstallResultMsg` handling to `Update`:

```go
	case libUninstallResultMsg:
		if typedMsg.err != nil {
			model.err = typedMsg.err

			return model, nil
		}

		for idx := range model.items {
			if model.items[idx].listing.Name == typedMsg.repoName {
				model.items[idx].installed = false
			}
		}

		delete(model.shortCodes, typedMsg.repoName)

		return model, nil
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/library.go cli/library_test.go
git commit -m "feat: add uninstall flow with inline y/n confirmation"
```

---

### Task 7: View rendering with lipgloss styling

Implement all `View()` methods: loading, browsing (list), detail, installing, done.

**Files:**
- Modify: `cli/library.go`

- [ ] **Step 1: Implement the View methods**

Replace the stub `View()` in `cli/library.go`:

```go
// Styles used across views.
var (
	libTitleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	libCategoryStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	libInstalledStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	libCursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	libStarsStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	libSelectedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	libFooterStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	libInstallRow     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	libMetaBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

// View renders the TUI.
func (model libraryModel) View() string {
	switch model.state {
	case libStateLoading:
		return "\n  Loading strategies from GitHub...\n"

	case libStateBrowsing, libStateConfirmUninst:
		return model.listView()

	case libStateDetail:
		return model.detailView()

	case libStateInstalling:
		return "\n  Installing selected strategies...\n"

	case libStateDone:
		return model.doneView()
	}

	return ""
}

// listView renders the strategy list with categories, checkboxes, and cursor.
func (model libraryModel) listView() string {
	var sb strings.Builder

	// Header
	selectedN := model.selectedCount()
	header := "pvbt library"
	if selectedN > 0 {
		header += fmt.Sprintf(" (%d selected)", selectedN)
	}

	sb.WriteString("\n  ")
	sb.WriteString(libTitleStyle.Render(header))
	sb.WriteString("\n")

	// Filter bar
	if model.filtering {
		sb.WriteString(fmt.Sprintf("  /: %s_\n", model.filter))
	} else if model.filter != "" {
		sb.WriteString(fmt.Sprintf("  filter: %s (/ to edit, esc to clear)\n", model.filter))
	}

	sb.WriteString("\n")

	// Group visible items by category
	visible := model.visibleItems()
	categoryItems := make(map[string][]int)

	for _, itemIdx := range visible {
		cat := model.items[itemIdx].category
		categoryItems[cat] = append(categoryItems[cat], itemIdx)
	}

	visiblePos := 0
	descMaxWidth := model.width - 42
	if descMaxWidth < 10 {
		descMaxWidth = 10
	}

	for _, cat := range model.categories {
		itemIndices, hasCatItems := categoryItems[cat]
		if !hasCatItems {
			continue
		}

		sb.WriteString("  ")
		sb.WriteString(libCategoryStyle.Render(cat))
		sb.WriteString("\n")

		for _, itemIdx := range itemIndices {
			item := model.items[itemIdx]

			// Cursor
			prefix := "    "
			if visiblePos == model.cursor {
				prefix = "  " + libCursorStyle.Render("> ")
			}

			// Checkbox
			checkbox := "[ ]"
			if item.installed {
				checkbox = libInstalledStyle.Render("[+]")
			} else if item.selected {
				checkbox = libSelectedStyle.Render("[x]")
			}

			// Stars
			stars := libStarsStyle.Render(fmt.Sprintf("*%d", item.listing.Stars))

			// Name
			name := fmt.Sprintf("%s/%s", item.listing.Owner, item.listing.Name)

			// Description (truncated)
			desc := item.listing.Description
			if len(desc) > descMaxWidth {
				desc = desc[:descMaxWidth-3] + "..."
			}

			line := fmt.Sprintf("%s%s %-30s %4s  %s", prefix, checkbox, name, stars, desc)

			if item.installed {
				line = libInstalledStyle.Render(line)
			}

			sb.WriteString(line)
			sb.WriteString("\n")

			visiblePos++
		}

		sb.WriteString("\n")
	}

	// Install row (only when items are selected)
	if selectedN > 0 {
		prefix := "    "
		if model.isOnInstallRow() {
			prefix = "  " + libCursorStyle.Render("> ")
		}

		sb.WriteString(prefix)
		sb.WriteString(libInstallRow.Render(fmt.Sprintf("Install selected (%d)", selectedN)))
		sb.WriteString("\n\n")
	}

	// Footer
	footer := "j/k: move  space: select  enter: details  /: search  i: install  u: uninstall  q: quit"
	if model.state == libStateConfirmUninst {
		footer = fmt.Sprintf("Uninstall %s? y/n", model.uninstallTarget)
	}

	sb.WriteString("  ")
	sb.WriteString(libFooterStyle.Render(footer))
	sb.WriteString("\n")

	return sb.String()
}

// detailView renders the strategy detail with metadata and README.
func (model libraryModel) detailView() string {
	if model.detailIndex >= len(model.items) {
		return ""
	}

	item := model.items[model.detailIndex]
	var sb strings.Builder

	// Back hint
	sb.WriteString("\n  ")
	sb.WriteString(libFooterStyle.Render("esc: back  space: select  i: install"))
	sb.WriteString("\n\n")

	// Metadata box
	status := "not installed"
	if item.installed {
		status = "installed"
	} else if item.selected {
		status = "selected for install"
	}

	cats := strings.Join(item.listing.Categories, ", ")
	if cats == "" {
		cats = "uncategorized"
	}

	meta := fmt.Sprintf(
		"%s  %s\n%s  %s  %s\n%s: %s",
		libTitleStyle.Render(item.listing.Owner+"/"+item.listing.Name),
		libStarsStyle.Render(fmt.Sprintf("*%d", item.listing.Stars)),
		item.listing.Description,
		libFooterStyle.Render("updated "+item.listing.UpdatedAt),
		libFooterStyle.Render("("+cats+")"),
		"Status",
		status,
	)

	boxWidth := model.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}

	sb.WriteString(libMetaBoxStyle.Width(boxWidth).Render(meta))
	sb.WriteString("\n\n")

	// README viewport
	sb.WriteString(model.viewport.View())
	sb.WriteString("\n")

	return sb.String()
}

// doneView renders the installation results summary.
func (model libraryModel) doneView() string {
	if model.err != nil {
		return fmt.Sprintf("\n  Error: %v\n", model.err)
	}

	if len(model.results) == 0 {
		return "\n  No strategies were installed.\n"
	}

	var sb strings.Builder

	sb.WriteString("\n  Installation results:\n\n")

	for _, result := range model.results {
		if result.err != nil {
			sb.WriteString(fmt.Sprintf("  FAIL  %s: %v\n", result.repoName, result.err))
		} else {
			sb.WriteString(fmt.Sprintf("  OK    %s\n", result.repoName))
		}
	}

	return sb.String()
}
```

- [ ] **Step 2: Verify the build compiles**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go build ./...`
Expected: BUILD SUCCESS

- [ ] **Step 3: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && ginkgo run -race ./cli/`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add cli/library.go
git commit -m "feat: add styled list and detail view rendering"
```

---

### Task 8: Zerolog buffer redirect and cobra command

Wire up the cobra command tree, zerolog buffer redirect, and delete old commands.

**Files:**
- Modify: `cli/library.go`
- Modify: `cli/explore.go`
- Delete: `cli/discover.go`, `cli/discover_test.go`, `cli/list.go`, `cli/remove.go`

- [ ] **Step 1: Add cobra command to cli/library.go**

Append to `cli/library.go`:

```go
// newLibraryCmd creates the cobra command for "pvbt library".
func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Browse, install, and manage strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, err := cmd.Flags().GetBool("refresh")
			if err != nil {
				return fmt.Errorf("reading refresh flag: %w", err)
			}

			// Redirect zerolog to a buffer during TUI execution.
			var logBuffer bytes.Buffer
			originalLogger := log.Logger
			log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: &logBuffer}).
				With().Timestamp().Logger()

			model := newLibraryModel("", "", refresh)
			program := tea.NewProgram(model, tea.WithAltScreen())

			_, runErr := program.Run()

			// Restore logger and flush buffered logs to stderr.
			log.Logger = originalLogger

			if logBuffer.Len() > 0 {
				os.Stderr.Write(logBuffer.Bytes())
			}

			if runErr != nil {
				return fmt.Errorf("running library TUI: %w", runErr)
			}

			return nil
		},
	}

	cmd.Flags().Bool("refresh", false, "Force refresh of strategy listings from GitHub")

	// Subcommands
	cmd.AddCommand(newLibraryListCmd())
	cmd.AddCommand(newLibraryRemoveCmd())

	return cmd
}

// newLibraryListCmd creates the "pvbt library list" subcommand.
func newLibraryListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			strategies, err := library.List(library.DefaultLibDir())
			if err != nil {
				return fmt.Errorf("listing strategies: %w", err)
			}

			if len(strategies) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No strategies installed. Use 'pvbt library' to find strategies.")

				return nil
			}

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(writer, "SHORT-CODE\tVERSION\tREPO")

			for _, strategy := range strategies {
				fmt.Fprintf(writer, "%s\t%s\t%s/%s\n",
					strategy.ShortCode, strategy.Version,
					strategy.RepoOwner, strategy.RepoName)
			}

			return writer.Flush()
		},
	}
}

// newLibraryRemoveCmd creates the "pvbt library remove" subcommand.
func newLibraryRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <short-code>",
		Short: "Remove an installed strategy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			shortCode := args[0]

			if err := library.Remove(library.DefaultLibDir(), shortCode); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed strategy %q\n", shortCode)

			return nil
		},
	}
}
```

Add `"text/tabwriter"` to the imports in `cli/library.go`.

- [ ] **Step 2: Update cli/explore.go to use newLibraryCmd**

In `cli/explore.go`, replace lines 45-48:

```go
	rootCmd.AddCommand(newExploreCmd())
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newRemoveCmd())
	rootCmd.AddCommand(newDiscoverCmd())
```

With:

```go
	rootCmd.AddCommand(newExploreCmd())
	rootCmd.AddCommand(newLibraryCmd())
```

- [ ] **Step 3: Delete old command files**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt
rm cli/discover.go cli/discover_test.go cli/list.go cli/remove.go
```

- [ ] **Step 4: Verify build compiles and tests pass**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && go build ./... && ginkgo run -race ./cli/ ./registry/
```
Expected: BUILD SUCCESS, all tests PASS

- [ ] **Step 5: Run lint**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make lint
```
Expected: PASS (fix any issues before committing)

- [ ] **Step 6: Commit**

```bash
git rm cli/discover.go cli/discover_test.go cli/list.go cli/remove.go
git add cli/library.go cli/explore.go
git commit -m "feat: consolidate discover/list/remove into pvbt library command

Replace pvbt discover, pvbt list, and pvbt remove with pvbt library.
Redirect zerolog to a buffer during TUI execution to prevent log
corruption. This is a breaking CLI change."
```

---

### Task 9: Final integration check and changelog

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run full test suite**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make test
```
Expected: PASS

- [ ] **Step 2: Run lint**

```bash
cd /Users/jdf/Developer/penny-vault/pvbt && make lint
```
Expected: PASS

- [ ] **Step 3: Update changelog**

Add under the `[Unreleased]` section in `CHANGELOG.md`:

```markdown
### Changed
- The `pvbt discover`, `pvbt list`, and `pvbt remove` commands are consolidated into `pvbt library`, with `list` and `remove` as subcommands.

### Added
- The library TUI shows strategy descriptions and GitHub README content rendered with styled markdown.
- Strategies in the library TUI can be uninstalled with `u` and confirmed inline.
- The library TUI supports searching strategies by name or description with `/`.
```

- [ ] **Step 4: Commit changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: update changelog for library TUI redesign"
```
