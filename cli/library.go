// Copyright 2021-2026
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/library"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// State constants for the library TUI.
const (
	libStateLoading       = "loading"
	libStateBrowsing      = "browsing"
	libStateDetail        = "detail"
	libStateConfirmUninst = "confirm_uninstall"
	libStateInstalling    = "installing"
	libStateDone          = "done"
)

// libraryItem represents a single strategy listing in the library TUI.
type libraryItem struct {
	listing   library.Listing
	selected  bool
	installed bool
	category  string
}

// libListingsMsg is sent when strategy listings have been fetched.
type libListingsMsg struct {
	listings []library.Listing
	err      error
}

// libInstalledMsg is sent when locally installed strategies are available.
type libInstalledMsg struct {
	installed []library.InstalledStrategy
	err       error
}

// libInstallResultMsg holds the result of installing a single strategy.
type libInstallResultMsg struct {
	repoName string
	err      error
}

// libBatchInstallDoneMsg is sent when all selected strategies have been installed.
type libBatchInstallDoneMsg struct {
	results []libInstallResultMsg
}

// libReadmeMsg is sent when a README has been fetched for a strategy.
type libReadmeMsg struct {
	key     string
	content string
	err     error
}

// libUninstallResultMsg is sent when an uninstall operation completes.
type libUninstallResultMsg struct {
	shortCode string
	err       error
}

// libraryModel is the bubbletea model for the library TUI.
type libraryModel struct {
	state            string
	items            []libraryItem
	categories       []string
	cursor           int
	width            int
	height           int
	filter           string
	filtering        bool
	forceRefresh     bool
	cacheDir         string
	libDir           string
	results          []libInstallResultMsg
	err              error
	detailIndex      int
	viewport         viewport.Model
	readmeCache      map[string]string
	uninstallTarget  string
	shortCodes       map[string]string
	pendingInstalled []library.InstalledStrategy
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

		case libStateDetail:
			return model.handleDetailKey(typedMsg)

		case libStateConfirmUninst:
			return model.handleConfirmUninstallKey(typedMsg)
		}

		return model, nil

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

		// Apply any installed data that arrived before listings.
		if model.pendingInstalled != nil {
			model.markInstalled(model.pendingInstalled)
			model.pendingInstalled = nil
		}

		return model, nil

	case libInstalledMsg:
		if typedMsg.err != nil {
			log.Warn().Err(typedMsg.err).Msg("failed to list installed strategies")
		}

		if len(model.items) == 0 {
			// Listings haven't arrived yet; stash for later.
			model.pendingInstalled = typedMsg.installed
		} else {
			model.markInstalled(typedMsg.installed)
		}

		return model, nil

	case libReadmeMsg:
		cacheKey := typedMsg.key
		if typedMsg.err != nil {
			model.readmeCache[cacheKey] = "README not available."
		} else {
			model.readmeCache[cacheKey] = typedMsg.content
		}

		rendered := model.renderMarkdown(model.readmeCache[cacheKey])
		model.viewport.SetContent(rendered)

		return model, nil

	case libBatchInstallDoneMsg:
		model.results = typedMsg.results
		model.state = libStateDone

		return model, tea.Quit

	case libUninstallResultMsg:
		if typedMsg.err != nil {
			model.err = typedMsg.err
		} else {
			// Clear installed flag for the uninstalled strategy.
			for idx := range model.items {
				repoName := model.items[idx].listing.Name
				if shortCode, ok := model.shortCodes[repoName]; ok && shortCode == typedMsg.shortCode {
					model.items[idx].installed = false
				}
			}
		}

		return model, nil
	}

	return model, nil
}

// handleListKey processes key events in browsing state.
func (model libraryModel) handleListKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyCtrlC, keyMsg.Type == tea.KeyEsc:
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q':
		return model, tea.Quit

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'j',
		keyMsg.Type == tea.KeyDown:
		maxPos := model.maxCursorPosition()
		if model.cursor < maxPos {
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

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'u':
		visible := model.visibleItems()
		if model.cursor < len(visible) {
			itemIndex := visible[model.cursor]
			if model.items[itemIndex].installed {
				repoName := model.items[itemIndex].listing.Name

				if shortCode, ok := model.shortCodes[repoName]; ok {
					model.uninstallTarget = shortCode
					model.state = libStateConfirmUninst
				}
			}
		}

		return model, nil

	case keyMsg.Type == tea.KeyEnter:
		if model.isOnInstallRow() {
			cmd := model.installSelected()
			if cmd != nil {
				model.state = libStateInstalling
			}

			return model, cmd
		}

		// Transition to detail view.
		visible := model.visibleItems()
		if model.cursor < len(visible) {
			itemIndex := visible[model.cursor]
			model.state = libStateDetail
			model.detailIndex = itemIndex
			model.viewport = viewport.New(model.width, model.height-4)

			cacheKey := model.items[itemIndex].listing.Owner + "/" + model.items[itemIndex].listing.Name
			if cached, ok := model.readmeCache[cacheKey]; ok {
				rendered := model.renderMarkdown(cached)
				model.viewport.SetContent(rendered)

				return model, nil
			}

			owner := model.items[itemIndex].listing.Owner
			repo := model.items[itemIndex].listing.Name

			return model, libFetchReadme(owner, repo)
		}

		return model, nil
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

// handleDetailKey processes key events in the detail view.
func (model libraryModel) handleDetailKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyCtrlC:
		return model, tea.Quit

	case keyMsg.Type == tea.KeyEsc:
		model.state = libStateBrowsing

		return model, nil

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'q':
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
	}

	// Pass remaining keys to viewport for scrolling.
	var cmd tea.Cmd

	model.viewport, cmd = model.viewport.Update(keyMsg)

	return model, cmd
}

// handleConfirmUninstallKey processes key events in the uninstall confirmation.
func (model libraryModel) handleConfirmUninstallKey(keyMsg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'y':
		shortCode := model.uninstallTarget
		libDir := model.libDir
		model.state = libStateBrowsing
		model.uninstallTarget = ""

		return model, func() tea.Msg {
			removeErr := library.Remove(libDir, shortCode)

			return libUninstallResultMsg{shortCode: shortCode, err: removeErr}
		}

	case keyMsg.Type == tea.KeyRunes && len(keyMsg.Runes) == 1 && keyMsg.Runes[0] == 'n',
		keyMsg.Type == tea.KeyEsc:
		model.state = libStateBrowsing
		model.uninstallTarget = ""

		return model, nil
	}

	return model, nil
}

// Style definitions for the library TUI.
var (
	libTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	libCategoryStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	libInstalledStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	libCursorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	libStarsStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	libSelectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	libFooterStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	libInstallRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	libMetaBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

// View renders the library TUI.
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

// listView renders the browsable strategy list.
func (model libraryModel) listView() string {
	var sb strings.Builder

	// Header.
	header := libTitleStyle.Render("pvbt library")
	if numSelected := model.selectedCount(); numSelected > 0 {
		header += fmt.Sprintf(" (%d selected)", numSelected)
	}

	sb.WriteString(header)
	sb.WriteString("\n")

	// Filter bar.
	if model.filtering {
		fmt.Fprintf(&sb, "/: %s_\n", model.filter)
	} else if model.filter != "" {
		fmt.Fprintf(&sb, "filter: %s (/ to edit, esc to clear)\n", model.filter)
	}

	sb.WriteString("\n")

	// Group visible items by category.
	visible := model.visibleItems()
	categoryItems := make(map[string][]int)

	for _, itemIdx := range visible {
		cat := model.items[itemIdx].category
		categoryItems[cat] = append(categoryItems[cat], itemIdx)
	}

	visiblePos := 0

	for _, cat := range model.categories {
		itemIndices, hasCategoryItems := categoryItems[cat]
		if !hasCategoryItems {
			continue
		}

		sb.WriteString(libCategoryStyle.Render(fmt.Sprintf("  %s", cat)))
		sb.WriteString("\n")

		for _, itemIdx := range itemIndices {
			item := model.items[itemIdx]

			// Cursor prefix.
			prefix := "  "
			if visiblePos == model.cursor {
				prefix = libCursorStyle.Render("> ")
			}

			// Checkbox.
			checkbox := "[ ]"
			if item.installed {
				checkbox = "[+]"
			} else if item.selected {
				checkbox = libSelectedStyle.Render("[x]")
			}

			// Name and stars.
			nameStr := fmt.Sprintf("%-30s", item.listing.Owner+"/"+item.listing.Name)
			stars := libStarsStyle.Render(fmt.Sprintf("*%d", item.listing.Stars))

			line := fmt.Sprintf("%s %s %s %s", prefix, checkbox, nameStr, stars)
			if item.installed {
				line = libInstalledStyle.Render(line)
			}

			sb.WriteString(line)
			sb.WriteString("\n")

			visiblePos++
		}
	}

	// Install row.
	if numSelected := model.selectedCount(); numSelected > 0 {
		sb.WriteString("\n")

		prefix := "  "
		if model.isOnInstallRow() {
			prefix = libCursorStyle.Render("> ")
		}

		sb.WriteString(prefix)
		sb.WriteString(libInstallRowStyle.Render(fmt.Sprintf("Install selected (%d)", numSelected)))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Footer.
	if model.state == libStateConfirmUninst {
		fmt.Fprintf(&sb, "  Uninstall %s? y/n", model.uninstallTarget)
	} else {
		sb.WriteString(libFooterStyle.Render("  j/k: move  space: select  enter: detail  i: install  u: uninstall  /: filter  esc/q: quit"))
	}

	sb.WriteString("\n")

	return sb.String()
}

// detailView renders the detail page for a single strategy.
func (model libraryModel) detailView() string {
	var sb strings.Builder

	item := model.items[model.detailIndex]

	// Status line.
	status := "not installed"
	if item.installed {
		status = "installed"
	} else if item.selected {
		status = "selected"
	}

	// Categories.
	cats := strings.Join(item.listing.Categories, ", ")
	if cats == "" {
		cats = "uncategorized"
	}

	// Build metadata content.
	metaContent := fmt.Sprintf(
		"%s  %s\n%s\nUpdated: %s  Categories: %s\nStatus: %s",
		libTitleStyle.Render(item.listing.Owner+"/"+item.listing.Name),
		libStarsStyle.Render(fmt.Sprintf("*%d", item.listing.Stars)),
		item.listing.Description,
		item.listing.UpdatedAt,
		cats,
		status,
	)

	boxWidth := max(model.width-4, 40)

	box := libMetaBoxStyle.Width(boxWidth).Render(metaContent)
	sb.WriteString(box)
	sb.WriteString("\n\n")

	// README viewport or loading indicator.
	cacheKey := item.listing.Owner + "/" + item.listing.Name
	if _, cached := model.readmeCache[cacheKey]; !cached {
		sb.WriteString("  Loading README...")
	} else {
		sb.WriteString(model.viewport.View())
	}

	sb.WriteString("\n")

	// Footer with navigation hints.
	sb.WriteString(libFooterStyle.Render("  esc: back  space: select  i: install  j/k: scroll"))
	sb.WriteString("\n")

	return sb.String()
}

// doneView renders the installation results or error.
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
			fmt.Fprintf(&sb, "  FAIL  %s: %v\n", result.repoName, result.err)
		} else {
			fmt.Fprintf(&sb, "  OK    %s\n", result.repoName)
		}
	}

	return sb.String()
}

// newLibraryCmd creates the cobra command for "pvbt library".
func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "Browse, install, and manage strategies from the pvbt registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			refresh, flagErr := cmd.Flags().GetBool("refresh")
			if flagErr != nil {
				return fmt.Errorf("reading refresh flag: %w", flagErr)
			}

			// Redirect zerolog to a buffer while the TUI runs.
			var logBuffer bytes.Buffer

			savedLogger := log.Logger
			log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: &logBuffer}).With().Timestamp().Logger()

			mdl := newLibraryModel("", "", refresh)
			program := tea.NewProgram(mdl, tea.WithAltScreen())

			finalModel, runErr := program.Run()

			// Restore logger and flush buffered logs.
			log.Logger = savedLogger

			if logBuffer.Len() > 0 {
				_, _ = os.Stderr.Write(logBuffer.Bytes())
			}

			if runErr != nil {
				return fmt.Errorf("running library TUI: %w", runErr)
			}

			// Print install/error results after the alt screen has cleared.
			if final, ok := finalModel.(libraryModel); ok && final.state == libStateDone {
				fmt.Fprint(cmd.OutOrStdout(), final.doneView())
			}

			return nil
		},
	}

	cmd.Flags().Bool("refresh", false, "Force refresh of strategy listings from GitHub")

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
			strategies, listErr := library.List(library.DefaultLibDir())
			if listErr != nil {
				return fmt.Errorf("list strategies: %w", listErr)
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

			if removeErr := library.Remove(library.DefaultLibDir(), shortCode); removeErr != nil {
				return removeErr
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Removed strategy %q\n", shortCode)

			return nil
		},
	}
}

// buildItems groups listings by their first category and sorts categories alphabetically.
func (model *libraryModel) buildItems(listings []library.Listing) {
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

// markInstalled marks items whose repo names match installed strategies and populates shortCodes.
func (model *libraryModel) markInstalled(installed []library.InstalledStrategy) {
	installedRepos := make(map[string]string, len(installed))

	for _, strategy := range installed {
		installedRepos[strategy.RepoName] = strategy.ShortCode
	}

	for idx := range model.items {
		repoName := model.items[idx].listing.Name
		if shortCode, ok := installedRepos[repoName]; ok {
			model.items[idx].installed = true
			model.items[idx].selected = false
			model.shortCodes[repoName] = shortCode
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

// selectedCount returns the number of selected (non-installed) items.
func (model libraryModel) selectedCount() int {
	count := 0

	for _, item := range model.items {
		if item.selected && !item.installed {
			count++
		}
	}

	return count
}

// maxCursorPosition returns the maximum valid cursor position, accounting for the install row.
func (model libraryModel) maxCursorPosition() int {
	visible := model.visibleItems()
	maxPos := len(visible) - 1

	if model.selectedCount() > 0 {
		maxPos++ // install row
	}

	return maxPos
}

// isOnInstallRow returns true if the cursor is on the install action row.
func (model libraryModel) isOnInstallRow() bool {
	if model.selectedCount() == 0 {
		return false
	}

	visible := model.visibleItems()

	return model.cursor == len(visible)
}

// renderMarkdown renders markdown content for display in the terminal.
func (model libraryModel) renderMarkdown(content string) string {
	termWidth := model.width
	if termWidth <= 0 {
		termWidth = 80
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(termWidth),
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

// installSelected creates a command that installs all selected strategies.
func (model libraryModel) installSelected() tea.Cmd {
	var toInstall []library.Listing

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

// libFetchListings returns a tea.Cmd that fetches strategy listings from the registry.
func libFetchListings(cacheDir string, forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		opts := library.SearchOptions{
			CacheDir:     cacheDir,
			BaseURL:      "https://api.github.com",
			ForceRefresh: forceRefresh,
		}

		listings, err := library.Search(ctx, opts)

		return libListingsMsg{listings: listings, err: err}
	}
}

// libFetchInstalled returns a tea.Cmd that lists locally installed strategies.
func libFetchInstalled(libDir string) tea.Cmd {
	return func() tea.Msg {
		installed, err := library.List(libDir)

		return libInstalledMsg{installed: installed, err: err}
	}
}

// libFetchReadme returns a tea.Cmd that fetches the README for a strategy.
func libFetchReadme(owner, repo string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		content, err := library.FetchREADME(ctx, owner, repo, library.ReadmeOptions{})
		cacheKey := owner + "/" + repo

		return libReadmeMsg{key: cacheKey, content: content, err: err}
	}
}
