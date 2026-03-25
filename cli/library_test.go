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
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/library"
)

var _ = Describe("libraryModel", func() {
	var model libraryModel

	sampleListings := []library.Listing{
		{Name: "momentum", Owner: "alice", Description: "A momentum strategy", Categories: []string{"trend"}, CloneURL: "https://github.com/alice/momentum.git", Stars: 10},
		{Name: "value-pick", Owner: "bob", Description: "A value strategy", Categories: []string{"fundamental"}, CloneURL: "https://github.com/bob/value-pick.git", Stars: 5},
		{Name: "mean-revert", Owner: "charlie", Description: "Mean reversion", Categories: []string{"trend"}, CloneURL: "https://github.com/charlie/mean-revert.git", Stars: 8},
	}

	sampleInstalled := []library.InstalledStrategy{
		{ShortCode: "mom", RepoOwner: "alice", RepoName: "momentum", Version: "v1.0.0", BinPath: "/usr/local/bin/momentum"},
	}

	BeforeEach(func() {
		model = newLibraryModel("/tmp/cache", "/tmp/lib", false)
	})

	// ---- Initial state ----

	Describe("initial state", func() {
		It("starts in loading state", func() {
			Expect(model.state).To(Equal(libStateLoading))
		})

		It("transitions to browsing on listings received", func() {
			updated, _ := model.Update(libListingsMsg{listings: sampleListings})
			model = updated.(libraryModel)

			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(model.items).To(HaveLen(3))
		})

		It("sorts categories alphabetically", func() {
			updated, _ := model.Update(libListingsMsg{listings: sampleListings})
			model = updated.(libraryModel)

			Expect(model.categories).To(Equal([]string{"fundamental", "trend"}))
		})

		It("marks installed strategies and populates shortcode map", func() {
			updated, _ := model.Update(libListingsMsg{listings: sampleListings})
			model = updated.(libraryModel)

			updated, _ = model.Update(libInstalledMsg{installed: sampleInstalled})
			model = updated.(libraryModel)

			Expect(model.items[0].installed).To(BeTrue())
			Expect(model.items[1].installed).To(BeFalse())
			Expect(model.shortCodes["momentum"]).To(Equal("mom"))
		})
	})

	// ---- List view navigation ----

	Describe("list view navigation", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: library.Listing{Name: "strat1", Description: "first"}},
				{listing: library.Listing{Name: "strat2", Description: "second"}},
				{listing: library.Listing{Name: "strat3", Description: "third"}},
			}
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

		It("moves cursor with arrow keys", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(1))

			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(0))
		})

		It("does not move cursor below the last item", func() {
			model.cursor = 2

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(2))
		})

		It("does not move cursor above zero", func() {
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

		It("allows cursor to move to install row when items are selected", func() {
			model.items[0].selected = true
			// With 3 items and 1 selected, maxCursorPosition = 3 (indices 0,1,2 + install row)
			model.cursor = 2

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			model = updated.(libraryModel)
			Expect(model.cursor).To(Equal(3))
			Expect(model.isOnInstallRow()).To(BeTrue())
		})

		It("space is a no-op on the install row", func() {
			model.items[0].selected = true
			model.cursor = 3 // install row

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			// Items remain unchanged.
			Expect(model.items[0].selected).To(BeTrue())
			Expect(model.items[1].selected).To(BeFalse())
		})

		It("i key triggers install when items are selected", func() {
			model.items[1].selected = true

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateInstalling))
			Expect(cmd).NotTo(BeNil())
		})

		It("i key does nothing when no items are selected", func() {
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(cmd).To(BeNil())
		})
	})

	// ---- Search filtering ----

	Describe("search filtering", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: library.Listing{Name: "momentum", Description: "A momentum strategy"}},
				{listing: library.Listing{Name: "value-pick", Description: "A value strategy"}},
				{listing: library.Listing{Name: "mean-revert", Description: "Mean reversion momentum"}},
			}
		})

		It("/ enters filter mode", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeTrue())
			Expect(model.filter).To(Equal(""))
		})

		It("filters by name", func() {
			model.filtering = true
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("value")})
			model = updated.(libraryModel)

			visible := model.visibleItems()
			Expect(visible).To(HaveLen(1))
			Expect(model.items[visible[0]].listing.Name).To(Equal("value-pick"))
		})

		It("filters by description", func() {
			model.filtering = true
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("reversion")})
			model = updated.(libraryModel)

			visible := model.visibleItems()
			Expect(visible).To(HaveLen(1))
			Expect(model.items[visible[0]].listing.Name).To(Equal("mean-revert"))
		})

		It("esc clears filter", func() {
			model.filtering = true
			model.filter = "momentum"
			model.cursor = 1

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeFalse())
			Expect(model.filter).To(Equal(""))
			Expect(model.cursor).To(Equal(0))
		})

		It("enter locks filter", func() {
			model.filtering = true
			model.filter = "value"

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)
			Expect(model.filtering).To(BeFalse())
			Expect(model.filter).To(Equal("value"))
		})
	})

	// ---- Detail view ----

	Describe("detail view", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.width = 80
			model.height = 24
			model.items = []libraryItem{
				{listing: library.Listing{Name: "momentum", Owner: "alice", Description: "A momentum strategy"}},
				{listing: library.Listing{Name: "value-pick", Owner: "bob", Description: "A value strategy"}},
			}
		})

		It("enter transitions to detail view", func() {
			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateDetail))
			Expect(model.detailIndex).To(Equal(0))
			Expect(cmd).NotTo(BeNil()) // fetches README
		})

		It("esc returns to browsing from detail", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("q returns to browsing instead of quitting", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(cmd).To(BeNil())
		})

		It("space toggles selection in detail view", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeTrue())

			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeFalse())
		})

		It("cannot select installed item from detail view", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.items[0].installed = true
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
			model = updated.(libraryModel)
			Expect(model.items[0].selected).To(BeFalse())
		})

		It("i triggers install from detail view", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.items[0].selected = true
			model.viewport = viewport.New(80, 20)

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateInstalling))
			Expect(cmd).NotTo(BeNil())
		})

		It("README message populates cache", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(libReadmeMsg{key: "alice/momentum", content: "# Momentum\nGreat strategy"})
			model = updated.(libraryModel)
			Expect(model.readmeCache["alice/momentum"]).To(Equal("# Momentum\nGreat strategy"))
		})

		It("cached README skips fetch", func() {
			model.readmeCache["alice/momentum"] = "# Cached"

			updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateDetail))
			Expect(cmd).To(BeNil())
		})

		It("failed README shows error message", func() {
			model.state = libStateDetail
			model.detailIndex = 0
			model.viewport = viewport.New(80, 20)

			updated, _ := model.Update(libReadmeMsg{key: "alice/momentum", err: fmt.Errorf("network error")})
			model = updated.(libraryModel)
			Expect(model.readmeCache["alice/momentum"]).To(Equal("README not available."))
		})
	})

	// ---- Uninstall flow ----

	Describe("uninstall flow", func() {
		BeforeEach(func() {
			model.state = libStateBrowsing
			model.items = []libraryItem{
				{listing: library.Listing{Name: "momentum", Owner: "alice"}, installed: true},
				{listing: library.Listing{Name: "value-pick", Owner: "bob"}, installed: false},
			}
			model.shortCodes["momentum"] = "mom"
		})

		It("u enters confirmation for installed strategy", func() {
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateConfirmUninst))
			Expect(model.uninstallTarget).To(Equal("mom"))
		})

		It("u is ignored for non-installed strategy", func() {
			model.cursor = 1

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
		})

		It("n cancels uninstall confirmation", func() {
			model.state = libStateConfirmUninst
			model.uninstallTarget = "mom"

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(model.uninstallTarget).To(Equal(""))
		})

		It("esc cancels uninstall confirmation", func() {
			model.state = libStateConfirmUninst
			model.uninstallTarget = "mom"

			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
			model = updated.(libraryModel)
			Expect(model.state).To(Equal(libStateBrowsing))
			Expect(model.uninstallTarget).To(Equal(""))
		})

		It("successful uninstall clears installed flag", func() {
			updated, _ := model.Update(libUninstallResultMsg{shortCode: "mom"})
			model = updated.(libraryModel)
			Expect(model.items[0].installed).To(BeFalse())
		})
	})
})
