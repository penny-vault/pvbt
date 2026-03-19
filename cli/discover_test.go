package cli

import (
	tea "github.com/charmbracelet/bubbletea"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/penny-vault/pvbt/registry"
)

var _ = Describe("discoverModel", func() {
	var model discoverModel

	BeforeEach(func() {
		model = newDiscoverModel("", "", false)
	})

	It("starts in loading state", func() {
		Expect(model.state).To(Equal(stateLoading))
	})

	It("transitions to browsing on listings received", func() {
		listings := []registry.Listing{
			{Name: "strat1", Owner: "user1", Categories: []string{"cat1"}, Stars: 10},
			{Name: "strat2", Owner: "user2", Categories: []string{"cat2"}, Stars: 20},
		}

		updated, _ := model.Update(listingsMsg{listings: listings})
		model = updated.(discoverModel)

		Expect(model.state).To(Equal(stateBrowsing))
		Expect(model.items).To(HaveLen(2))
	})

	It("moves cursor with j/k keys", func() {
		model.state = stateBrowsing
		model.items = []discoverItem{
			{listing: registry.Listing{Name: "strat1"}},
			{listing: registry.Listing{Name: "strat2"}},
		}

		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = updated.(discoverModel)
		Expect(model.cursor).To(Equal(1))

		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model = updated.(discoverModel)
		Expect(model.cursor).To(Equal(0))
	})

	It("toggles selection with spacebar", func() {
		model.state = stateBrowsing
		model.items = []discoverItem{
			{listing: registry.Listing{Name: "strat1"}},
		}

		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
		model = updated.(discoverModel)
		Expect(model.items[0].selected).To(BeTrue())

		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
		model = updated.(discoverModel)
		Expect(model.items[0].selected).To(BeFalse())
	})

	It("does not allow selecting installed items", func() {
		model.state = stateBrowsing
		model.items = []discoverItem{
			{listing: registry.Listing{Name: "strat1"}, installed: true},
		}

		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
		model = updated.(discoverModel)
		Expect(model.items[0].selected).To(BeFalse())
	})
})
