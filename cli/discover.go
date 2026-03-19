package cli

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/library"
	"github.com/penny-vault/pvbt/registry"
	"github.com/spf13/cobra"
)

// State constants for the discover TUI.
const (
	stateLoading    = "loading"
	stateBrowsing   = "browsing"
	stateInstalling = "installing"
	stateDone       = "done"
)

// discoverItem represents a single strategy listing in the TUI.
type discoverItem struct {
	listing   registry.Listing
	selected  bool
	installed bool
	category  string
}

// listingsMsg is sent when strategy listings have been fetched from the registry.
type listingsMsg struct {
	listings []registry.Listing
	err      error
}

// installedMsg is sent when the list of locally installed strategies is available.
type installedMsg struct {
	installed []library.InstalledStrategy
}

// installResultMsg holds the result of installing a single strategy.
type installResultMsg struct {
	repoName string
	err      error
}

// batchInstallDoneMsg is sent when all selected strategies have been installed.
type batchInstallDoneMsg struct {
	results []installResultMsg
}

// discoverModel is the bubbletea model for the discover TUI.
type discoverModel struct {
	state        string
	items        []discoverItem
	categories   []string
	cursor       int
	width        int
	height       int
	filter       string
	filtering    bool
	forceRefresh bool
	cacheDir     string
	libDir       string
	results      []installResultMsg
	err          error
}

// newDiscoverModel creates a new discoverModel with sensible defaults.
func newDiscoverModel(cacheDir, libDir string, forceRefresh bool) discoverModel {
	if cacheDir == "" {
		cacheDir = library.DefaultCacheDir()
	}

	if libDir == "" {
		libDir = library.DefaultLibDir()
	}

	return discoverModel{
		state:        stateLoading,
		cacheDir:     cacheDir,
		libDir:       libDir,
		forceRefresh: forceRefresh,
	}
}

// Init returns the initial commands to fetch listings and installed strategies.
func (model discoverModel) Init() tea.Cmd {
	return tea.Batch(
		fetchListings(model.cacheDir, model.forceRefresh),
		fetchInstalled(model.libDir),
	)
}

// Update handles messages and returns the updated model.
func (model discoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typedMsg := msg.(type) {
	case tea.KeyMsg:
		if model.filtering {
			return model.handleFilterKey(typedMsg)
		}

		return model.handleKey(typedMsg)

	case tea.WindowSizeMsg:
		model.width = typedMsg.Width
		model.height = typedMsg.Height

		return model, nil

	case listingsMsg:
		if typedMsg.err != nil {
			model.err = typedMsg.err
			model.state = stateDone

			return model, tea.Quit
		}

		model.buildItems(typedMsg.listings)
		model.state = stateBrowsing

		return model, nil

	case installedMsg:
		model.markInstalled(typedMsg.installed)

		return model, nil

	case batchInstallDoneMsg:
		model.results = typedMsg.results
		model.state = stateDone

		return model, tea.Quit
	}

	return model, nil
}

// handleKey processes key events in browsing mode.
func (model discoverModel) handleKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyCtrlC:
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q':
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'j',
		keyMsg.Type == tea.KeyDown:
		visible := model.visibleItems()
		if model.cursor < len(visible)-1 {
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

	case keyMsg.Type == tea.KeyEnter:
		return model, model.installSelected()
	}

	return model, nil
}

// handleFilterKey processes key events while in filter mode.
func (model discoverModel) handleFilterKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

// visibleItems returns the indices of items that match the current filter.
func (model discoverModel) visibleItems() []int {
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

// installSelected creates a command that installs all selected strategies.
func (model discoverModel) installSelected() tea.Cmd {
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
		var results []installResultMsg

		ctx := context.Background()

		for _, listing := range toInstall {
			_, installErr := library.Install(ctx, libDir, listing.CloneURL)
			results = append(results, installResultMsg{
				repoName: listing.Name,
				err:      installErr,
			})
		}

		return batchInstallDoneMsg{results: results}
	}
}

// buildItems groups listings by their first category.
func (model *discoverModel) buildItems(listings []registry.Listing) {
	model.items = make([]discoverItem, 0, len(listings))
	categorySet := make(map[string]bool)

	for _, listing := range listings {
		categoryName := "uncategorized"
		if len(listing.Categories) > 0 {
			categoryName = listing.Categories[0]
		}

		categorySet[categoryName] = true

		model.items = append(model.items, discoverItem{
			listing:  listing,
			category: categoryName,
		})
	}

	model.categories = make([]string, 0, len(categorySet))
	for cat := range categorySet {
		model.categories = append(model.categories, cat)
	}
}

// markInstalled marks items whose repo names match installed strategies.
func (model *discoverModel) markInstalled(installed []library.InstalledStrategy) {
	installedRepos := make(map[string]bool, len(installed))

	for _, strategy := range installed {
		installedRepos[strategy.RepoName] = true
	}

	for idx := range model.items {
		if installedRepos[model.items[idx].listing.Name] {
			model.items[idx].installed = true
			model.items[idx].selected = false
		}
	}
}

// View renders the TUI.
func (model discoverModel) View() string {
	switch model.state {
	case stateLoading:
		return "Loading strategies from GitHub...\n"

	case stateBrowsing:
		return model.browsingView()

	case stateInstalling:
		return "Installing selected strategies...\n"

	case stateDone:
		return model.doneView()
	}

	return ""
}

// browsingView renders the strategy list with checkboxes and cursor.
func (model discoverModel) browsingView() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	installedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	starsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	var builder strings.Builder

	builder.WriteString(titleStyle.Render("pvbt discover"))
	builder.WriteString("\n")

	if model.filtering {
		fmt.Fprintf(&builder, "Filter: %s_\n", model.filter)
	} else if model.filter != "" {
		fmt.Fprintf(&builder, "Filter: %s (press / to edit)\n", model.filter)
	}

	builder.WriteString("\n")

	visible := model.visibleItems()

	// Group visible items by category for display.
	categoryItems := make(map[string][]int)

	for _, itemIdx := range visible {
		cat := model.items[itemIdx].category
		categoryItems[cat] = append(categoryItems[cat], itemIdx)
	}

	visiblePosition := 0

	for _, cat := range model.categories {
		itemIndices, hasCategoryItems := categoryItems[cat]
		if !hasCategoryItems {
			continue
		}

		builder.WriteString(titleStyle.Render(fmt.Sprintf("  %s", cat)))
		builder.WriteString("\n")

		for _, itemIdx := range itemIndices {
			item := model.items[itemIdx]

			prefix := "  "
			if visiblePosition == model.cursor {
				prefix = cursorStyle.Render("> ")
			}

			checkbox := "[ ]"
			if item.installed {
				checkbox = installedStyle.Render("[*]")
			} else if item.selected {
				checkbox = "[x]"
			}

			stars := starsStyle.Render(fmt.Sprintf("(%d)", item.listing.Stars))

			line := fmt.Sprintf("%s %s %s/%s %s", prefix, checkbox, item.listing.Owner, item.listing.Name, stars)

			if item.installed {
				line = installedStyle.Render(line)
			}

			builder.WriteString(line)
			builder.WriteString("\n")

			visiblePosition++
		}
	}

	builder.WriteString("\n")
	builder.WriteString("  j/k: move  space: select  enter: install  /: filter  q: quit\n")

	return builder.String()
}

// doneView renders the installation results summary.
func (model discoverModel) doneView() string {
	if model.err != nil {
		return fmt.Sprintf("Error: %v\n", model.err)
	}

	if len(model.results) == 0 {
		return "No strategies were installed.\n"
	}

	var builder strings.Builder

	builder.WriteString("Installation results:\n\n")

	for _, result := range model.results {
		if result.err != nil {
			fmt.Fprintf(&builder, "  FAIL  %s: %v\n", result.repoName, result.err)
		} else {
			fmt.Fprintf(&builder, "  OK    %s\n", result.repoName)
		}
	}

	return builder.String()
}

// fetchListings returns a tea.Cmd that fetches strategy listings from the registry.
func fetchListings(cacheDir string, forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		opts := registry.SearchOptions{
			CacheDir:     cacheDir,
			BaseURL:      "https://api.github.com",
			ForceRefresh: forceRefresh,
		}

		listings, err := registry.Search(ctx, opts)

		return listingsMsg{listings: listings, err: err}
	}
}

// fetchInstalled returns a tea.Cmd that lists locally installed strategies.
func fetchInstalled(libDir string) tea.Cmd {
	return func() tea.Msg {
		installed, err := library.List(libDir)
		if err != nil {
			// Return empty list on error; the user will see items as not installed.
			return installedMsg{installed: nil}
		}

		return installedMsg{installed: installed}
	}
}

// newDiscoverCmd creates the cobra command for "pvbt discover".
func newDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Browse and install strategies from the pvbt registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, err := cmd.Flags().GetBool("refresh")
			if err != nil {
				return fmt.Errorf("reading refresh flag: %w", err)
			}

			model := newDiscoverModel("", "", refresh)
			program := tea.NewProgram(model, tea.WithAltScreen())

			if _, err := program.Run(); err != nil {
				return fmt.Errorf("running discover TUI: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().Bool("refresh", false, "Force refresh of strategy listings from GitHub")

	return cmd
}
