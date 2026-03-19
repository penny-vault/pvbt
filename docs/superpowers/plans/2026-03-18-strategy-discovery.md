# Strategy Discovery Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable users to discover, install, and run community strategies published as Go modules tagged with the `pvbt-strategy` GitHub topic.

**Architecture:** Three layers -- `registry/` (GitHub API + cache), `library/` (download, build, index), and CLI commands (`discover`, `list`, `remove`, short-code dispatch). The discovery TUI is built with bubbletea following existing patterns in `cli/tui.go`.

**Tech Stack:** Go 1.25, cobra, bubbletea, lipgloss, zerolog, net/http (GitHub API), os/exec (git/go toolchain), encoding/json

**Spec:** `docs/superpowers/specs/2026-03-18-strategy-discovery-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `registry/registry.go` | GitHub API client, topic search, caching |
| `registry/registry_test.go` | Unit tests for registry |
| `library/library.go` | Install, list, lookup, remove strategies |
| `library/library_test.go` | Unit tests for library |
| `cli/discover.go` | Bubbletea TUI for browsing/installing strategies |
| `cli/discover_test.go` | TUI model unit tests |
| `cli/list.go` | `pvbt list` command |
| `cli/remove.go` | `pvbt remove` command |

### Modified Files

| File | Change |
|---|---|
| `cli/run.go` | Add `describe` subcommand |
| `cli/explore.go` | Add `discover`, `list`, `remove` subcommands + short-code dispatch via `RunE` |

---

## Task 1: `registry/` Package -- Types and Search

**Files:**
- Create: `registry/registry.go`
- Create: `registry/registry_test.go`

- [ ] **Step 1: Write failing test for Search with canned GitHub response**

Create the test file with a fake HTTP server that returns a canned GitHub search API response. Test that `Search` returns the expected `Listing` structs with categories extracted from topics.

```go
// registry/registry_test.go
package registry_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/registry"
)

var _ = Describe("Registry", func() {
	var (
		server   *httptest.Server
		cacheDir string
	)

	BeforeEach(func() {
		var err error
		cacheDir, err = os.MkdirTemp("", "pvbt-registry-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		os.RemoveAll(cacheDir)
	})

	Describe("Search", func() {
		It("parses GitHub search results into listings", func() {
			response := map[string]interface{}{
				"total_count": 2,
				"items": []map[string]interface{}{
					{
						"name":        "dual-momentum",
						"full_name":   "user1/dual-momentum",
						"description": "Classic dual momentum rotation",
						"clone_url":   "https://github.com/user1/dual-momentum.git",
						"owner":       map[string]interface{}{"login": "user1"},
						"topics":      []string{"pvbt-strategy", "tactical-asset-allocation"},
						"stargazers_count": 87,
						"updated_at":      "2026-03-01T00:00:00Z",
					},
					{
						"name":        "pairs-trading",
						"full_name":   "user2/pairs-trading",
						"description": "Statistical arbitrage pairs",
						"clone_url":   "https://github.com/user2/pairs-trading.git",
						"owner":       map[string]interface{}{"login": "user2"},
						"topics":      []string{"pvbt-strategy", "mean-reversion"},
						"stargazers_count": 23,
						"updated_at":      "2026-02-15T00:00:00Z",
					},
				},
			}

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.URL.Query().Get("q")).To(Equal("topic:pvbt-strategy"))
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))

			listings, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  server.URL,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(2))

			Expect(listings[0].Name).To(Equal("dual-momentum"))
			Expect(listings[0].Owner).To(Equal("user1"))
			Expect(listings[0].Categories).To(Equal([]string{"tactical-asset-allocation"}))
			Expect(listings[0].Stars).To(Equal(87))

			Expect(listings[1].Name).To(Equal("pairs-trading"))
			Expect(listings[1].Categories).To(Equal([]string{"mean-reversion"}))
		})

		It("returns cached results when cache is fresh", func() {
			cached := registry.CachedResults{
				Timestamp: time.Now(),
				Listings: []registry.Listing{
					{Name: "cached-strategy", Owner: "user1", Categories: []string{"test"}},
				},
			}
			cacheFile := filepath.Join(cacheDir, "discover.json")
			data, err := json.Marshal(cached)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(cacheFile, data, 0644)).To(Succeed())

			listings, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir: cacheDir,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("cached-strategy"))
		})

		It("ignores stale cache and fetches fresh", func() {
			cached := registry.CachedResults{
				Timestamp: time.Now().Add(-2 * time.Hour),
				Listings: []registry.Listing{
					{Name: "stale-strategy"},
				},
			}
			cacheFile := filepath.Join(cacheDir, "discover.json")
			data, err := json.Marshal(cached)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(cacheFile, data, 0644)).To(Succeed())

			response := map[string]interface{}{
				"total_count": 1,
				"items": []map[string]interface{}{
					{
						"name":        "fresh-strategy",
						"full_name":   "user1/fresh-strategy",
						"description": "Fresh from API",
						"clone_url":   "https://github.com/user1/fresh-strategy.git",
						"owner":       map[string]interface{}{"login": "user1"},
						"topics":      []string{"pvbt-strategy"},
						"stargazers_count": 10,
						"updated_at":      "2026-03-01T00:00:00Z",
					},
				},
			}

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))

			listings, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir:     cacheDir,
				BaseURL:      server.URL,
				ForceRefresh: false,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("fresh-strategy"))
		})

		It("forces refresh when ForceRefresh is true", func() {
			cached := registry.CachedResults{
				Timestamp: time.Now(),
				Listings: []registry.Listing{
					{Name: "cached-strategy"},
				},
			}
			cacheFile := filepath.Join(cacheDir, "discover.json")
			data, err := json.Marshal(cached)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(cacheFile, data, 0644)).To(Succeed())

			response := map[string]interface{}{
				"total_count": 1,
				"items": []map[string]interface{}{
					{
						"name":        "forced-strategy",
						"full_name":   "user1/forced-strategy",
						"description": "Forced fresh",
						"clone_url":   "https://github.com/user1/forced-strategy.git",
						"owner":       map[string]interface{}{"login": "user1"},
						"topics":      []string{"pvbt-strategy"},
						"stargazers_count": 5,
						"updated_at":      "2026-03-01T00:00:00Z",
					},
				},
			}

			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			}))

			listings, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir:     cacheDir,
				BaseURL:      server.URL,
				ForceRefresh: true,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("forced-strategy"))
		})

		It("falls back to stale cache on network error", func() {
			cached := registry.CachedResults{
				Timestamp: time.Now().Add(-2 * time.Hour),
				Listings: []registry.Listing{
					{Name: "stale-but-available"},
				},
			}
			cacheFile := filepath.Join(cacheDir, "discover.json")
			data, err := json.Marshal(cached)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(cacheFile, data, 0644)).To(Succeed())

			listings, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  "http://localhost:1", // unreachable
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(listings).To(HaveLen(1))
			Expect(listings[0].Name).To(Equal("stale-but-available"))
		})

		It("returns error on network error with no cache", func() {
			_, err := registry.Search(context.Background(), registry.SearchOptions{
				CacheDir: cacheDir,
				BaseURL:  "http://localhost:1",
			})

			Expect(err).To(HaveOccurred())
		})
	})
})
```

- [ ] **Step 2: Write test suite bootstrap**

```go
// registry/registry_suite_test.go
package registry_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Registry Suite")
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./registry/ -v`
Expected: Compilation failure -- package `registry` does not exist.

- [ ] **Step 4: Implement registry package**

```go
// registry/registry.go
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultBaseURL = "https://api.github.com"
	cacheTTL       = 1 * time.Hour
	ghAuthTimeout  = 2 * time.Second
)

// Listing represents a discovered strategy repository on GitHub.
type Listing struct {
	Name        string    `json:"name"`
	Owner       string    `json:"owner"`
	Description string    `json:"description"`
	Categories  []string  `json:"categories"`
	CloneURL    string    `json:"clone_url"`
	Stars       int       `json:"stars"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CachedResults stores search results with a timestamp for cache expiry.
type CachedResults struct {
	Timestamp time.Time `json:"timestamp"`
	Listings  []Listing `json:"listings"`
}

// SearchOptions configures the Search function.
type SearchOptions struct {
	CacheDir     string // directory for discover.json cache
	BaseURL      string // GitHub API base URL (for testing)
	ForceRefresh bool   // ignore cache TTL
}

// Search discovers strategies tagged with the pvbt-strategy topic on GitHub.
func Search(ctx context.Context, opts SearchOptions) ([]Listing, error) {
	if opts.BaseURL == "" {
		opts.BaseURL = defaultBaseURL
	}

	if err := os.MkdirAll(opts.CacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	cacheFile := filepath.Join(opts.CacheDir, "discover.json")

	// Check cache
	if !opts.ForceRefresh {
		cached, err := readCache(cacheFile)
		if err == nil && time.Since(cached.Timestamp) < cacheTTL {
			log.Debug().Str("cache_file", cacheFile).Msg("using cached discovery results")
			return cached.Listings, nil
		}
	}

	// Fetch from GitHub
	listings, err := fetchFromGitHub(ctx, opts)
	if err != nil {
		// Fall back to stale cache on network error
		cached, cacheErr := readCache(cacheFile)
		if cacheErr == nil {
			log.Warn().
				Err(err).
				Time("cached_at", cached.Timestamp).
				Msg("GitHub API unreachable, using stale cache")
			return cached.Listings, nil
		}

		return nil, fmt.Errorf("GitHub API search failed: %w", err)
	}

	// Write cache atomically
	if writeErr := writeCache(cacheFile, listings); writeErr != nil {
		log.Warn().Err(writeErr).Msg("failed to write discovery cache")
	}

	return listings, nil
}

func readCache(path string) (*CachedResults, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cached CachedResults
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	return &cached, nil
}

func writeCache(path string, listings []Listing) error {
	cached := CachedResults{
		Timestamp: time.Now(),
		Listings:  listings,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return err
	}

	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpFile, path)
}

func fetchFromGitHub(ctx context.Context, opts SearchOptions) ([]Listing, error) {
	token := resolveAuthToken()

	var allListings []Listing
	page := 1

	for {
		url := fmt.Sprintf("%s/search/repositories?q=topic:pvbt-strategy&per_page=100&page=%d", opts.BaseURL, page)

		log.Info().Str("url", url).Int("page", page).Msg("fetching strategies from GitHub")

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Accept", "application/vnd.github+json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var result struct {
			TotalCount int `json:"total_count"`
			Items      []struct {
				Name     string `json:"name"`
				FullName string `json:"full_name"`
				Owner    struct {
					Login string `json:"login"`
				} `json:"owner"`
				Description    string   `json:"description"`
				CloneURL       string   `json:"clone_url"`
				Topics         []string `json:"topics"`
				StargazersCount int     `json:"stargazers_count"`
				UpdatedAt      string   `json:"updated_at"`
			} `json:"items"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode GitHub response: %w", err)
		}

		resp.Body.Close()

		for _, item := range result.Items {
			categories := make([]string, 0, len(item.Topics))
			for _, topic := range item.Topics {
				if topic != "pvbt-strategy" {
					categories = append(categories, topic)
				}
			}

			updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)

			allListings = append(allListings, Listing{
				Name:        item.Name,
				Owner:       item.Owner.Login,
				Description: item.Description,
				Categories:  categories,
				CloneURL:    item.CloneURL,
				Stars:       item.StargazersCount,
				UpdatedAt:   updatedAt,
			})
		}

		// Stop if we got all results or hit 1000 (GitHub API cap)
		if len(allListings) >= result.TotalCount || len(allListings) >= 1000 || len(result.Items) < 100 {
			break
		}

		page++
	}

	return allListings, nil
}

func resolveAuthToken() string {
	// 1. Check GITHUB_TOKEN env var
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		log.Debug().Msg("using GITHUB_TOKEN for authentication")
		return token
	}

	// 2. Try gh auth token with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ghAuthTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err == nil {
		token := strings.TrimSpace(string(out))
		if token != "" {
			log.Debug().Msg("using gh CLI token for authentication")
			return token
		}
	}

	log.Debug().Msg("no authentication token found, using unauthenticated API access")
	return ""
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./registry/ -v`
Expected: All 6 tests pass.

- [ ] **Step 6: Commit**

```bash
git add registry/
git commit -m "feat: add registry package for GitHub strategy discovery"
```

---

## Task 2: `library/` Package -- Install, List, Lookup, Remove

**Files:**
- Create: `library/library.go`
- Create: `library/library_test.go`

- [ ] **Step 1: Write failing tests for library operations**

```go
// library/library_test.go
package library_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/penny-vault/pvbt/library"
)

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
		It("returns empty list when no strategies installed", func() {
			strategies, err := library.List(libDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(BeEmpty())
		})

		It("returns installed strategies from index files", func() {
			stratDir := filepath.Join(libDir, "test-strategy")
			Expect(os.MkdirAll(stratDir, 0755)).To(Succeed())

			installed := library.InstalledStrategy{
				ShortCode:   "test-strat",
				RepoOwner:   "user1",
				RepoName:    "test-strategy",
				Version:     "v1.0.0",
				BinPath:     filepath.Join(stratDir, "bin", "test-strategy"),
				InstalledAt: time.Now().Truncate(time.Second),
			}

			data, err := json.Marshal(installed)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(stratDir, "index.json"), data, 0644)).To(Succeed())

			strategies, err := library.List(libDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(HaveLen(1))
			Expect(strategies[0].ShortCode).To(Equal("test-strat"))
		})
	})

	Describe("Lookup", func() {
		It("finds strategy by short-code", func() {
			stratDir := filepath.Join(libDir, "test-strategy")
			Expect(os.MkdirAll(stratDir, 0755)).To(Succeed())

			installed := library.InstalledStrategy{
				ShortCode: "test-strat",
				RepoOwner: "user1",
				RepoName:  "test-strategy",
				BinPath:   filepath.Join(stratDir, "bin", "test-strategy"),
			}

			data, err := json.Marshal(installed)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(stratDir, "index.json"), data, 0644)).To(Succeed())

			found, err := library.Lookup(libDir, "test-strat")
			Expect(err).NotTo(HaveOccurred())
			Expect(found.RepoName).To(Equal("test-strategy"))
		})

		It("returns error for unknown short-code", func() {
			_, err := library.Lookup(libDir, "nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Remove", func() {
		It("removes an installed strategy", func() {
			stratDir := filepath.Join(libDir, "test-strategy")
			Expect(os.MkdirAll(stratDir, 0755)).To(Succeed())

			installed := library.InstalledStrategy{
				ShortCode: "test-strat",
				RepoOwner: "user1",
				RepoName:  "test-strategy",
				BinPath:   filepath.Join(stratDir, "bin", "test-strategy"),
			}

			data, err := json.Marshal(installed)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(stratDir, "index.json"), data, 0644)).To(Succeed())

			Expect(library.Remove(libDir, "test-strat")).To(Succeed())

			strategies, err := library.List(libDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(strategies).To(BeEmpty())
		})

		It("returns error for unknown short-code", func() {
			err := library.Remove(libDir, "nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("CheckCollision", func() {
		It("detects short-code collision from different repo", func() {
			stratDir := filepath.Join(libDir, "existing-strategy")
			Expect(os.MkdirAll(stratDir, 0755)).To(Succeed())

			installed := library.InstalledStrategy{
				ShortCode: "my-strat",
				RepoOwner: "user1",
				RepoName:  "existing-strategy",
			}

			data, err := json.Marshal(installed)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(stratDir, "index.json"), data, 0644)).To(Succeed())

			err = library.CheckCollision(libDir, "my-strat", "different-strategy")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("existing-strategy"))
		})

		It("allows same repo to re-install", func() {
			stratDir := filepath.Join(libDir, "existing-strategy")
			Expect(os.MkdirAll(stratDir, 0755)).To(Succeed())

			installed := library.InstalledStrategy{
				ShortCode: "my-strat",
				RepoOwner: "user1",
				RepoName:  "existing-strategy",
			}

			data, err := json.Marshal(installed)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(stratDir, "index.json"), data, 0644)).To(Succeed())

			err = library.CheckCollision(libDir, "my-strat", "existing-strategy")
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
```

- [ ] **Step 2: Write test suite bootstrap**

```go
// library/library_suite_test.go
package library_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLibrary(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Library Suite")
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./library/ -v`
Expected: Compilation failure -- package `library` does not exist.

- [ ] **Step 4: Implement library package**

```go
// library/library.go
package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog/log"
)

// InstalledStrategy records metadata for a locally installed strategy.
type InstalledStrategy struct {
	ShortCode   string    `json:"short_code"`
	RepoOwner   string    `json:"repo_owner"`
	RepoName    string    `json:"repo_name"`
	Version     string    `json:"version"`
	BinPath     string    `json:"bin_path"`
	InstalledAt time.Time `json:"installed_at"`
}

// Install clones a strategy repo, builds it, extracts its descriptor, and
// registers it in the local library.
func Install(ctx context.Context, libDir string, cloneURL string) (*InstalledStrategy, error) {
	// Extract repo name from clone URL
	repoName := repoNameFromURL(cloneURL)
	stratDir := filepath.Join(libDir, repoName)
	moduleDir := filepath.Join(stratDir, "module")
	binDir := filepath.Join(stratDir, "bin")
	binPath := filepath.Join(binDir, repoName)

	// Check toolchain requirements
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git is required but not found on PATH")
	}

	if _, err := exec.LookPath("go"); err != nil {
		return nil, fmt.Errorf("go is required but not found on PATH")
	}

	log.Info().Str("repo", repoName).Str("url", cloneURL).Msg("installing strategy")

	// Clean up existing module dir for re-install
	if err := os.RemoveAll(moduleDir); err != nil {
		return nil, fmt.Errorf("clean module directory: %w", err)
	}

	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("create bin directory: %w", err)
	}

	// Clone
	cloneCmd := exec.CommandContext(ctx, "git", "clone", cloneURL, moduleDir)
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		cleanup(stratDir)
		return nil, fmt.Errorf("git clone failed: %w", err)
	}

	// Build
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	buildCmd.Dir = moduleDir
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		cleanup(stratDir)
		return nil, fmt.Errorf("go build failed: %w", err)
	}

	// Run describe subcommand
	describeCmd := exec.CommandContext(ctx, binPath, "describe")
	describeOut, err := describeCmd.Output()
	if err != nil {
		cleanup(stratDir)
		return nil, fmt.Errorf("strategy does not implement Descriptor interface -- cannot determine short-code")
	}

	var desc engine.StrategyDescription
	if err := json.Unmarshal(describeOut, &desc); err != nil {
		cleanup(stratDir)
		return nil, fmt.Errorf("invalid describe output: %w", err)
	}

	if desc.ShortCode == "" {
		cleanup(stratDir)
		return nil, fmt.Errorf("strategy Descriptor returned empty short-code")
	}

	// Check for short-code collision (exempting self for re-install)
	if err := CheckCollision(libDir, desc.ShortCode, repoName); err != nil {
		cleanup(stratDir)
		return nil, err
	}

	// Write index
	installed := InstalledStrategy{
		ShortCode:   desc.ShortCode,
		RepoOwner:   ownerFromURL(cloneURL),
		RepoName:    repoName,
		Version:     desc.Version,
		BinPath:     binPath,
		InstalledAt: time.Now(),
	}

	indexData, err := json.MarshalIndent(installed, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal index: %w", err)
	}

	if err := os.WriteFile(filepath.Join(stratDir, "index.json"), indexData, 0644); err != nil {
		return nil, fmt.Errorf("write index: %w", err)
	}

	log.Info().Str("short_code", desc.ShortCode).Str("version", desc.Version).Msg("strategy installed")

	return &installed, nil
}

// List returns all locally installed strategies.
func List(libDir string) ([]InstalledStrategy, error) {
	entries, err := os.ReadDir(libDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var strategies []InstalledStrategy

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		indexPath := filepath.Join(libDir, entry.Name(), "index.json")

		data, err := os.ReadFile(indexPath)
		if err != nil {
			log.Debug().Str("dir", entry.Name()).Err(err).Msg("skipping directory without index.json")
			continue
		}

		var installed InstalledStrategy
		if err := json.Unmarshal(data, &installed); err != nil {
			log.Warn().Str("file", indexPath).Err(err).Msg("invalid index.json")
			continue
		}

		strategies = append(strategies, installed)
	}

	return strategies, nil
}

// Lookup finds an installed strategy by its short-code.
func Lookup(libDir string, shortCode string) (*InstalledStrategy, error) {
	strategies, err := List(libDir)
	if err != nil {
		return nil, err
	}

	for idx := range strategies {
		if strategies[idx].ShortCode == shortCode {
			log.Debug().Str("short_code", shortCode).Str("bin", strategies[idx].BinPath).Msg("strategy found")
			return &strategies[idx], nil
		}
	}

	return nil, fmt.Errorf("strategy %q not found", shortCode)
}

// Remove deletes a locally installed strategy by short-code.
func Remove(libDir string, shortCode string) error {
	strategies, err := List(libDir)
	if err != nil {
		return err
	}

	for _, strategy := range strategies {
		if strategy.ShortCode == shortCode {
			stratDir := filepath.Join(libDir, strategy.RepoName)
			if err := os.RemoveAll(stratDir); err != nil {
				return fmt.Errorf("remove strategy directory: %w", err)
			}

			log.Info().Str("short_code", shortCode).Msg("strategy removed")
			return nil
		}
	}

	return fmt.Errorf("strategy %q not found", shortCode)
}

// CheckCollision checks if a short-code is already used by a different repo.
// Returns nil if no collision or if the collision is with the same repo (re-install).
func CheckCollision(libDir string, shortCode string, repoName string) error {
	strategies, err := List(libDir)
	if err != nil {
		return err
	}

	for _, strategy := range strategies {
		if strategy.ShortCode == shortCode && strategy.RepoName != repoName {
			return fmt.Errorf(
				"short-code %q is already used by %s/%s; remove it first with: pvbt remove %s",
				shortCode, strategy.RepoOwner, strategy.RepoName, shortCode,
			)
		}
	}

	return nil
}

// DefaultLibDir returns the default library directory (~/.pvbt/lib).
func DefaultLibDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Err(err).Msg("cannot determine home directory")
	}

	return filepath.Join(home, ".pvbt", "lib")
}

// DefaultCacheDir returns the default cache directory (~/.pvbt/cache).
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Err(err).Msg("cannot determine home directory")
	}

	return filepath.Join(home, ".pvbt", "cache")
}

func repoNameFromURL(cloneURL string) string {
	// "https://github.com/user/repo.git" -> "repo"
	parts := strings.Split(cloneURL, "/")
	name := parts[len(parts)-1]
	return strings.TrimSuffix(name, ".git")
}

func ownerFromURL(cloneURL string) string {
	// "https://github.com/user/repo.git" -> "user"
	parts := strings.Split(cloneURL, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}

	return ""
}

func cleanup(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("failed to clean up after install failure")
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./library/ -v`
Expected: All 7 tests pass.

- [ ] **Step 6: Commit**

```bash
git add library/
git commit -m "feat: add library package for local strategy management"
```

---

## Task 3: `describe` Subcommand in `cli.Run()`

**Files:**
- Modify: `cli/run.go:17-48`
- Modify: `cli/cli_test.go` (add describe tests)

- [ ] **Step 1: Write failing test for describe subcommand**

Add to the existing cli test file. The test needs a strategy that implements Descriptor and one that does not.

```go
// Add to cli/cli_test.go (or cli/describe_test.go if preferred)

var _ = Describe("describe subcommand", func() {
	// Uses the existing descriptorStrategy and plainStrategy test types
	// from engine/descriptor_test.go patterns

	It("outputs StrategyDescription as JSON for a Descriptor strategy", func() {
		// Test that newDescribeCmd with a Descriptor strategy
		// emits valid JSON with shortcode, description, version
	})

	It("exits with error for a non-Descriptor strategy", func() {
		// Test that newDescribeCmd with a plain strategy returns error
	})
})
```

The actual test will instantiate the cobra command and capture its output.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "describe"`
Expected: FAIL -- `newDescribeCmd` not defined.

- [ ] **Step 3: Implement describe subcommand**

Add `newDescribeCmd` function and register it in `Run()`:

```go
// cli/describe.go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

func newDescribeCmd(strategy engine.Strategy) *cobra.Command {
	return &cobra.Command{
		Use:   "describe",
		Short: "Output strategy metadata as JSON",
		RunE: func(cmd *cobra.Command, args []string) error {
			descriptor, ok := strategy.(engine.Descriptor)
			if !ok {
				return fmt.Errorf("strategy does not implement Descriptor interface")
			}

			desc := descriptor.Describe()

			data, err := json.MarshalIndent(desc, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal descriptor: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}
```

Then modify `cli/run.go` to add the describe subcommand:

```go
// In Run(), after line 43 (rootCmd.AddCommand(newSnapshotCmd(strategy))):
rootCmd.AddCommand(newDescribeCmd(strategy))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "describe"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/describe.go cli/run.go cli/cli_test.go
git commit -m "feat: add describe subcommand to strategy CLI"
```

---

## Task 4: `pvbt list` Command

**Files:**
- Create: `cli/list.go`
- Modify: `cli/explore.go:19-48` (register list subcommand)

- [ ] **Step 1: Write failing test for list command**

Test that the command outputs a table of installed strategies.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "list"`
Expected: FAIL

- [ ] **Step 3: Implement list command**

```go
// cli/list.go
package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/penny-vault/pvbt/library"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			strategies, err := library.List(library.DefaultLibDir())
			if err != nil {
				return fmt.Errorf("list strategies: %w", err)
			}

			if len(strategies) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No strategies installed. Use 'pvbt discover' to find strategies.")
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
```

Register in `cli/explore.go` `RunPVBT()` after line 43:

```go
rootCmd.AddCommand(newListCmd())
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "list"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/list.go cli/explore.go
git commit -m "feat: add pvbt list command"
```

---

## Task 5: `pvbt remove` Command

**Files:**
- Create: `cli/remove.go`
- Modify: `cli/explore.go` (register remove subcommand)

- [ ] **Step 1: Write failing test for remove command**

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "remove"`

- [ ] **Step 3: Implement remove command**

```go
// cli/remove.go
package cli

import (
	"fmt"

	"github.com/penny-vault/pvbt/library"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
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

Register in `cli/explore.go` `RunPVBT()`:

```go
rootCmd.AddCommand(newRemoveCmd())
```

- [ ] **Step 4: Run test to verify it passes**

- [ ] **Step 5: Commit**

```bash
git add cli/remove.go cli/explore.go
git commit -m "feat: add pvbt remove command"
```

---

## Task 6: Short-code Dispatch

**Files:**
- Modify: `cli/explore.go:19-48` (add RunE to root command)

- [ ] **Step 1: Write failing test for dispatch**

Test that an unknown subcommand matching an installed strategy's short-code would trigger dispatch. Test that known subcommands like "explore", "list", "discover" are not intercepted. Test that `pvbt` with no args shows help.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "dispatch"`

- [ ] **Step 3: Implement dispatch**

Rather than adding `RunE` + `ArbitraryArgs` to the root command (which interferes with cobra's subcommand routing and help behavior), intercept at the `Execute` error path. After `rootCmd.Execute()` returns an error, check if `os.Args[1]` matches an installed strategy short-code:

```go
// Replace the Execute block in RunPVBT():

if err := rootCmd.Execute(); err != nil {
    // Check if the first arg is an installed strategy short-code.
    // This fires only when cobra found no matching subcommand.
    if len(os.Args) > 1 {
        installed, lookupErr := library.Lookup(library.DefaultLibDir(), os.Args[1])
        if lookupErr == nil {
            argv := append([]string{installed.BinPath}, os.Args[2:]...)
            if execErr := syscall.Exec(installed.BinPath, argv, os.Environ()); execErr != nil {
                fmt.Fprintf(os.Stderr, "exec %s: %v\n", installed.BinPath, execErr)
            }
        }
    }

    os.Exit(1)
}
```

This approach:
- Does not modify cobra's argument parsing or help behavior
- Only fires when cobra itself rejects the command
- Registered subcommands (explore, discover, list, remove) work normally
- `pvbt --help` and `pvbt` with no args work normally
- `syscall.Exec` requires importing `"syscall"`

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "dispatch"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cli/explore.go
git commit -m "feat: add short-code dispatch for installed strategies"
```

---

## Task 7: Discovery TUI -- Model and State Machine

**Files:**
- Create: `cli/discover.go`
- Create: `cli/discover_test.go`

- [ ] **Step 1: Write failing tests for TUI model states and transitions**

Test the bubbletea model:
- Initial state is `loading`
- After receiving listings message, transitions to `browsing`
- Arrow keys / j,k move cursor
- Spacebar toggles selection
- `q` sends quit
- Enter with selections transitions to `installing`

```go
// cli/discover_test.go
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
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "discoverModel"`
Expected: FAIL -- types not defined.

- [ ] **Step 3: Implement the TUI model and state machine**

```go
// cli/discover.go
package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/penny-vault/pvbt/library"
	"github.com/penny-vault/pvbt/registry"
	"github.com/spf13/cobra"
)

const (
	stateLoading    = "loading"
	stateBrowsing   = "browsing"
	stateInstalling = "installing"
	stateDone       = "done"
)

type discoverItem struct {
	listing   registry.Listing
	selected  bool
	installed bool
	category  string
}

type listingsMsg struct {
	listings []registry.Listing
	err      error
}

type installedMsg struct {
	installed []library.InstalledStrategy
}

type installResultMsg struct {
	repoName string
	err      error
}

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

func newDiscoverModel(cacheDir string, libDir string, forceRefresh bool) discoverModel {
	if cacheDir == "" {
		cacheDir = library.DefaultCacheDir()
	}

	if libDir == "" {
		libDir = library.DefaultLibDir()
	}

	return discoverModel{
		state:        stateLoading,
		forceRefresh: forceRefresh,
		cacheDir:     cacheDir,
		libDir:       libDir,
	}
}

func (m discoverModel) Init() tea.Cmd {
	return tea.Batch(
		fetchListings(m.cacheDir, m.forceRefresh),
		fetchInstalled(m.libDir),
	)
}

func fetchListings(cacheDir string, forceRefresh bool) tea.Cmd {
	return func() tea.Msg {
		listings, err := registry.Search(context.Background(), registry.SearchOptions{
			CacheDir:     cacheDir,
			ForceRefresh: forceRefresh,
		})
		return listingsMsg{listings: listings, err: err}
	}
}

func fetchInstalled(libDir string) tea.Cmd {
	return func() tea.Msg {
		installed, _ := library.List(libDir)
		return installedMsg{installed: installed}
	}
}

func (m discoverModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case listingsMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.buildItems(msg.listings, nil)
		m.state = stateBrowsing
		return m, nil
	case installedMsg:
		m.markInstalled(msg.installed)
		return m, nil
	case batchInstallDoneMsg:
		m.results = msg.results
		m.state = stateDone
		return m, tea.Quit
	}

	return m, nil
}

func (m *discoverModel) buildItems(listings []registry.Listing, installed []library.InstalledStrategy) {
	categorySet := make(map[string]bool)
	m.items = make([]discoverItem, 0, len(listings))

	for _, listing := range listings {
		category := "uncategorized"
		if len(listing.Categories) > 0 {
			category = listing.Categories[0]
		}

		categorySet[category] = true

		m.items = append(m.items, discoverItem{
			listing:  listing,
			category: category,
		})
	}

	m.categories = make([]string, 0, len(categorySet))
	for cat := range categorySet {
		m.categories = append(m.categories, cat)
	}

	sort.Strings(m.categories)
}

func (m *discoverModel) markInstalled(installed []library.InstalledStrategy) {
	shortCodes := make(map[string]bool)
	for _, inst := range installed {
		shortCodes[inst.RepoName] = true
	}

	for idx := range m.items {
		if shortCodes[m.items[idx].listing.Name] {
			m.items[idx].installed = true
		}
	}
}

func (m discoverModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.handleFilterKey(msg)
	}

	switch {
	case msg.Type == tea.KeyCtrlC || (msg.Type == tea.KeyRunes && string(msg.Runes) == "q"):
		return m, tea.Quit

	case msg.Type == tea.KeyRunes && string(msg.Runes) == "j",
		msg.Type == tea.KeyDown:
		if m.cursor < len(m.visibleItems())-1 {
			m.cursor++
		}

	case msg.Type == tea.KeyRunes && string(msg.Runes) == "k",
		msg.Type == tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}

	case msg.Type == tea.KeySpace:
		visible := m.visibleItems()
		if m.cursor < len(visible) && !visible[m.cursor].installed {
			// Find the item in m.items and toggle
			for idx := range m.items {
				if m.items[idx].listing.Name == visible[m.cursor].listing.Name {
					m.items[idx].selected = !m.items[idx].selected
					break
				}
			}
		}

	case msg.Type == tea.KeyRunes && string(msg.Runes) == "/":
		m.filtering = true
		m.filter = ""

	case msg.Type == tea.KeyEnter:
		return m, m.installSelected()
	}

	return m, nil
}

func (m discoverModel) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.filtering = false
		m.filter = ""
		m.cursor = 0
	case tea.KeyEnter:
		m.filtering = false
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.cursor = 0
		}
	case tea.KeyRunes:
		m.filter += string(msg.Runes)
		m.cursor = 0
	}

	return m, nil
}

func (m discoverModel) visibleItems() []discoverItem {
	if m.filter == "" {
		return m.items
	}

	var filtered []discoverItem

	for _, item := range m.items {
		if strings.Contains(strings.ToLower(item.listing.Name), strings.ToLower(m.filter)) ||
			strings.Contains(strings.ToLower(item.listing.Description), strings.ToLower(m.filter)) {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

func (m discoverModel) installSelected() tea.Cmd {
	var selected []discoverItem

	for _, item := range m.items {
		if item.selected {
			selected = append(selected, item)
		}
	}

	if len(selected) == 0 {
		return nil
	}

	m.state = stateInstalling

	// Return a command that installs each strategy sequentially,
	// sending an installResultMsg for each, then installDoneMsg.
	return func() tea.Msg {
		var results []installResultMsg

		for _, item := range selected {
			_, err := library.Install(context.Background(), m.libDir, item.listing.CloneURL)
			results = append(results, installResultMsg{repoName: item.listing.Name, err: err})
		}

		return batchInstallDoneMsg{results: results}
	}
}

// batchInstallDoneMsg carries all install results at once.
type batchInstallDoneMsg struct {
	results []installResultMsg
}

func (m discoverModel) View() string {
	switch m.state {
	case stateLoading:
		return "Loading strategies from GitHub...\n"
	case stateInstalling:
		return m.viewInstalling()
	case stateDone:
		return m.viewDone()
	default:
		return m.viewBrowsing()
	}
}

func (m discoverModel) viewBrowsing() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %s\n", m.err)
	}

	var builder strings.Builder

	// Header
	headerStyle := lipgloss.NewStyle().Bold(true)
	builder.WriteString(headerStyle.Render("Strategy Discovery"))
	builder.WriteString("                          [q] quit  [/] filter  [enter] install\n\n")

	if m.filtering {
		builder.WriteString(fmt.Sprintf("  Filter: %s_\n\n", m.filter))
	}

	visible := m.visibleItems()

	// Group by category
	byCategory := make(map[string][]int)
	for idx, item := range visible {
		byCategory[item.category] = append(byCategory[item.category], idx)
	}

	categories := make([]string, 0, len(byCategory))
	for cat := range byCategory {
		categories = append(categories, cat)
	}

	sort.Strings(categories)

	cursorIdx := 0

	for _, cat := range categories {
		catStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
		builder.WriteString(fmt.Sprintf("  %s\n", catStyle.Render(cat)))

		for _, idx := range byCategory[cat] {
			item := visible[idx]
			prefix := "  "

			if cursorIdx == m.cursor {
				prefix = "> "
			}

			checkbox := "[ ]"
			if item.selected {
				checkbox = "[x]"
			}

			if item.installed {
				checkbox = " * "
			}

			builder.WriteString(fmt.Sprintf("%s  %s %-24s %-40s * %d\n",
				prefix, checkbox, item.listing.Name,
				truncate(item.listing.Description, 40),
				item.listing.Stars))

			cursorIdx++
		}

		builder.WriteString("\n")
	}

	return builder.String()
}

func (m discoverModel) viewInstalling() string {
	var builder strings.Builder
	builder.WriteString("Installing strategies...\n\n")

	for _, result := range m.results {
		if result.err != nil {
			builder.WriteString(fmt.Sprintf("  x %s: %s\n", result.repoName, result.err))
		} else {
			builder.WriteString(fmt.Sprintf("  + %s: installed\n", result.repoName))
		}
	}

	return builder.String()
}

func (m discoverModel) viewDone() string {
	var builder strings.Builder
	builder.WriteString("Installation complete.\n\n")

	for _, result := range m.results {
		if result.err != nil {
			builder.WriteString(fmt.Sprintf("  x %s: %s\n", result.repoName, result.err))
		} else {
			builder.WriteString(fmt.Sprintf("  + %s: installed\n", result.repoName))
		}
	}

	return builder.String()
}

func truncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	return text[:maxLen-3] + "..."
}

func newDiscoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Browse and install community strategies",
		RunE: func(cmd *cobra.Command, args []string) error {
			forceRefresh, _ := cmd.Flags().GetBool("refresh")

			model := newDiscoverModel("", "", forceRefresh)
			program := tea.NewProgram(model, tea.WithAltScreen())
			_, err := program.Run()

			return err
		},
	}

	cmd.Flags().Bool("refresh", false, "Force refresh of cached discovery results")

	return cmd
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./cli/ -v -run "discoverModel"`
Expected: PASS

- [ ] **Step 5: Register discover command**

Add in `cli/explore.go` `RunPVBT()`:

```go
rootCmd.AddCommand(newDiscoverCmd())
```

- [ ] **Step 6: Commit**

```bash
git add cli/discover.go cli/discover_test.go cli/explore.go
git commit -m "feat: add pvbt discover TUI for strategy browsing and installation"
```

---

## Task 8: Wire Everything Together and Integration Test

**Files:**
- Modify: `cli/explore.go` (ensure all commands registered + dispatch)

- [ ] **Step 1: Verify all commands are registered in RunPVBT()**

Confirm `cli/explore.go` `RunPVBT()` has:
```go
rootCmd.AddCommand(newExploreCmd())
rootCmd.AddCommand(newDiscoverCmd())
rootCmd.AddCommand(newListCmd())
rootCmd.AddCommand(newRemoveCmd())
```
Plus the `RunE` dispatch from Task 6.

- [ ] **Step 2: Run all tests**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && go test ./... -v`
Expected: All tests pass across all packages.

- [ ] **Step 3: Run linter**

Run: `cd /Users/jdf/Developer/penny-vault/pvbt && golangci-lint run ./...`
Expected: No errors.

- [ ] **Step 4: Fix any lint issues**

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat: wire strategy discovery commands into pvbt CLI"
```

---

## Task 9: Manual Smoke Test Checklist

These are manual verification steps (not automated):

- [ ] **Step 1:** Build pvbt: `go build -o pvbt .`
- [ ] **Step 2:** Run `./pvbt --help` -- should show discover, list, remove, explore
- [ ] **Step 3:** Run `./pvbt list` -- should show "No strategies installed"
- [ ] **Step 4:** Run `./pvbt discover` -- should show the TUI (may be empty if no repos have the topic yet)
- [ ] **Step 5:** Verify `./pvbt discover --refresh` forces a fresh fetch
- [ ] **Step 6:** Build the example momentum-rotation strategy and verify `describe` subcommand works:
  ```bash
  cd examples/momentum-rotation && go build -o momentum-rotation .
  ./momentum-rotation describe
  ```
  Expected: JSON output with shortcode, description, version
