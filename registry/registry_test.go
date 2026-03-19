package registry_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/registry"
)

// cannedSearchResponse builds a GitHub Search API JSON response with the given repos.
func cannedSearchResponse(repos []map[string]interface{}) string {
	response := map[string]interface{}{
		"total_count":        len(repos),
		"incomplete_results": false,
		"items":              repos,
	}
	data, err := json.Marshal(response)
	Expect(err).NotTo(HaveOccurred())
	return string(data)
}

func makeRepo(name, owner, description, cloneURL string, stars int, topics []string, updatedAt string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"description": description,
		"clone_url":   cloneURL,
		"stargazers_count": stars,
		"updated_at":  updatedAt,
		"topics":      topics,
		"owner": map[string]interface{}{
			"login": owner,
		},
	}
}

var _ = Describe("Search", func() {
	var (
		cacheDir string
		ctx      context.Context
	)

	BeforeEach(func() {
		var err error
		cacheDir, err = os.MkdirTemp("", "registry-test-*")
		Expect(err).NotTo(HaveOccurred())
		ctx = context.Background()
	})

	AfterEach(func() {
		os.RemoveAll(cacheDir)
	})

	Context("parsing GitHub search results", func() {
		It("parses repos into Listing structs with categories extracted from topics", func() {
			repos := []map[string]interface{}{
				makeRepo(
					"momentum-strategy", "alice", "A momentum strategy",
					"https://github.com/alice/momentum-strategy.git", 42,
					[]string{"pvbt-strategy", "momentum", "equity"},
					"2026-01-15T10:30:00Z",
				),
				makeRepo(
					"mean-reversion", "bob", "Mean reversion approach",
					"https://github.com/bob/mean-reversion.git", 7,
					[]string{"pvbt-strategy", "mean-reversion"},
					"2025-12-01T08:00:00Z",
				),
			}

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, cannedSearchResponse(repos))
			}))
			defer server.Close()

			opts := registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  server.URL,
			}

			listings, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(2))

			Expect(listings[0].Name).To(Equal("momentum-strategy"))
			Expect(listings[0].Owner).To(Equal("alice"))
			Expect(listings[0].Description).To(Equal("A momentum strategy"))
			Expect(listings[0].CloneURL).To(Equal("https://github.com/alice/momentum-strategy.git"))
			Expect(listings[0].Stars).To(Equal(42))
			Expect(listings[0].UpdatedAt).To(Equal("2026-01-15T10:30:00Z"))
			// pvbt-strategy should be stripped from categories
			Expect(listings[0].Categories).To(ConsistOf("momentum", "equity"))

			Expect(listings[1].Name).To(Equal("mean-reversion"))
			Expect(listings[1].Owner).To(Equal("bob"))
			// pvbt-strategy stripped, only mean-reversion remains
			Expect(listings[1].Categories).To(ConsistOf("mean-reversion"))
		})
	})

	Context("caching behavior", func() {
		var (
			server      *httptest.Server
			requestCount atomic.Int32
			repos       []map[string]interface{}
		)

		BeforeEach(func() {
			repos = []map[string]interface{}{
				makeRepo("test-strat", "owner", "desc",
					"https://github.com/owner/test-strat.git", 1,
					[]string{"pvbt-strategy"}, "2026-01-01T00:00:00Z"),
			}
			requestCount.Store(0)
			server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				requestCount.Add(1)
				writer.Header().Set("Content-Type", "application/json")
				fmt.Fprint(writer, cannedSearchResponse(repos))
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("returns cached results without hitting server when cache is fresh", func() {
			opts := registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  server.URL,
			}

			// First call populates cache
			firstResult, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(firstResult).To(HaveLen(1))
			Expect(requestCount.Load()).To(Equal(int32(1)))

			// Second call should use cache
			secondResult, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(secondResult).To(HaveLen(1))
			Expect(requestCount.Load()).To(Equal(int32(1))) // no additional request
		})

		It("fetches fresh data when cache is stale", func() {
			// Write a cache file with an old timestamp
			staleCache := registry.CachedResults{
				Timestamp: time.Now().Add(-2 * time.Hour),
				Listings: []registry.Listing{
					{Name: "old-strat", Owner: "old-owner"},
				},
			}
			cacheData, err := json.Marshal(staleCache)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(cacheDir, "discover.json"), cacheData, 0o644)
			Expect(err).NotTo(HaveOccurred())

			opts := registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  server.URL,
			}

			listings, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(requestCount.Load()).To(Equal(int32(1))) // server was hit
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("test-strat")) // got fresh data
		})

		It("ignores fresh cache when ForceRefresh is set", func() {
			opts := registry.SearchOptions{
				CacheDir:     cacheDir,
				BaseURL:      server.URL,
				ForceRefresh: true,
			}

			// First call populates cache
			_, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(requestCount.Load()).To(Equal(int32(1)))

			// Second call with ForceRefresh should hit server again
			listings, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(requestCount.Load()).To(Equal(int32(2)))
			Expect(listings).To(HaveLen(1))
		})
	})

	Context("network error handling", func() {
		It("falls back to stale cache on network error", func() {
			// Write a stale cache
			staleCache := registry.CachedResults{
				Timestamp: time.Now().Add(-2 * time.Hour),
				Listings: []registry.Listing{
					{Name: "cached-strat", Owner: "cached-owner", Description: "from cache"},
				},
			}
			cacheData, err := json.Marshal(staleCache)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(filepath.Join(cacheDir, "discover.json"), cacheData, 0o644)
			Expect(err).NotTo(HaveOccurred())

			opts := registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  "http://127.0.0.1:1", // unreachable
			}

			listings, err := registry.Search(ctx, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("cached-strat"))
		})

		It("returns error on network error with no cache", func() {
			opts := registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  "http://127.0.0.1:1", // unreachable
			}

			_, err := registry.Search(ctx, opts)
			Expect(err).To(HaveOccurred())
		})
	})
})
