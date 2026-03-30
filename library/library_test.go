package library_test

import (
	"github.com/bytedance/sonic"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/library"
)

func writeIndex(dir string, strategy library.InstalledStrategy) {
	data, err := sonic.Marshal(strategy)
	Expect(err).NotTo(HaveOccurred())
	err = os.MkdirAll(dir, 0o755)
	Expect(err).NotTo(HaveOccurred())
	err = os.WriteFile(filepath.Join(dir, "index.json"), data, 0o644)
	Expect(err).NotTo(HaveOccurred())
}

var _ = Describe("Library", func() {
	var libDir string

	BeforeEach(func() {
		var err error
		libDir, err = os.MkdirTemp("", "pvbt-library-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(libDir)
	})

	Describe("List", func() {
		It("returns empty when no strategies are installed", func() {
			strategies, err := library.List(libDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(BeEmpty())
		})

		It("returns empty when the library directory does not exist", func() {
			nonExistentDir := filepath.Join(libDir, "does-not-exist")
			strategies, err := library.List(nonExistentDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(BeNil())
		})

		It("returns installed strategies from index.json files", func() {
			strategy := library.InstalledStrategy{
				ShortCode:   "momentum",
				RepoOwner:   "penny-vault",
				RepoName:    "momentum-strategy",
				Version:     "v1.0.0",
				BinPath:     filepath.Join(libDir, "momentum-strategy", "momentum-strategy"),
				InstalledAt: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			}
			writeIndex(filepath.Join(libDir, "momentum-strategy"), strategy)

			strategies, err := library.List(libDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(HaveLen(1))
			Expect(strategies[0].ShortCode).To(Equal("momentum"))
			Expect(strategies[0].RepoName).To(Equal("momentum-strategy"))
		})
	})

	Describe("Lookup", func() {
		It("finds a strategy by short-code", func() {
			strategy := library.InstalledStrategy{
				ShortCode:   "daa",
				RepoOwner:   "penny-vault",
				RepoName:    "daa-strategy",
				Version:     "v2.1.0",
				BinPath:     filepath.Join(libDir, "daa-strategy", "daa-strategy"),
				InstalledAt: time.Date(2026, 2, 20, 14, 30, 0, 0, time.UTC),
			}
			writeIndex(filepath.Join(libDir, "daa-strategy"), strategy)

			found, err := library.Lookup(libDir, "daa")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).NotTo(BeNil())
			Expect(found.ShortCode).To(Equal("daa"))
			Expect(found.RepoName).To(Equal("daa-strategy"))
		})

		It("returns error for unknown short-code", func() {
			_, err := library.Lookup(libDir, "nonexistent")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent"))
		})
	})

	Describe("Remove", func() {
		It("removes an installed strategy", func() {
			strategyDir := filepath.Join(libDir, "remove-me")
			strategy := library.InstalledStrategy{
				ShortCode:   "removable",
				RepoOwner:   "penny-vault",
				RepoName:    "remove-me",
				Version:     "v1.0.0",
				BinPath:     filepath.Join(strategyDir, "remove-me"),
				InstalledAt: time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC),
			}
			writeIndex(strategyDir, strategy)

			err := library.Remove(libDir, "removable")
			Expect(err).NotTo(HaveOccurred())

			_, statErr := os.Stat(strategyDir)
			Expect(os.IsNotExist(statErr)).To(BeTrue())
		})

		It("returns error for unknown short-code", func() {
			err := library.Remove(libDir, "ghost")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ghost"))
		})
	})

	Describe("CheckCollision", func() {
		BeforeEach(func() {
			strategy := library.InstalledStrategy{
				ShortCode:   "collider",
				RepoOwner:   "penny-vault",
				RepoName:    "collider-strategy",
				Version:     "v1.0.0",
				BinPath:     filepath.Join(libDir, "collider-strategy", "collider-strategy"),
				InstalledAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
			}
			writeIndex(filepath.Join(libDir, "collider-strategy"), strategy)
		})

		It("detects collision from a different repo", func() {
			err := library.CheckCollision(libDir, "collider", "other-repo")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("collider"))
		})

		It("allows the same repo to re-install", func() {
			err := library.CheckCollision(libDir, "collider", "collider-strategy")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
