package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	cacheFreshDuration = 1 * time.Hour
	maxResults         = 1000
	perPage            = 100
)

// Listing represents a strategy repository discovered from GitHub.
type Listing struct {
	Name        string   `json:"name"`
	Owner       string   `json:"owner"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	CloneURL    string   `json:"clone_url"`
	Stars       int      `json:"stars"`
	UpdatedAt   string   `json:"updated_at"`
}

// CachedResults holds cached search results with a timestamp.
type CachedResults struct {
	Timestamp time.Time `json:"timestamp"`
	Listings  []Listing `json:"listings"`
}

// SearchOptions configures the Search function.
type SearchOptions struct {
	CacheDir     string
	BaseURL      string
	ForceRefresh bool
}

// githubSearchResponse represents a GitHub Search API response.
type githubSearchResponse struct {
	TotalCount int              `json:"total_count"`
	Items      []githubRepoItem `json:"items"`
}

// githubRepoItem represents a single repository in GitHub search results.
type githubRepoItem struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CloneURL    string          `json:"clone_url"`
	Stars       int             `json:"stargazers_count"`
	UpdatedAt   string          `json:"updated_at"`
	Topics      []string        `json:"topics"`
	Owner       githubOwnerItem `json:"owner"`
}

// githubOwnerItem represents the owner object in a GitHub search result.
type githubOwnerItem struct {
	Login string `json:"login"`
}

// Search discovers pvbt-strategy repositories from GitHub.
func Search(ctx context.Context, opts SearchOptions) ([]Listing, error) {
	if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	cachePath := filepath.Join(opts.CacheDir, "discover.json")

	// Check cache unless ForceRefresh is set
	if !opts.ForceRefresh {
		cached, cacheErr := loadCache(cachePath)
		if cacheErr == nil && time.Since(cached.Timestamp) < cacheFreshDuration {
			log.Debug().Str("cache_path", cachePath).Msg("returning fresh cached results")
			return cached.Listings, nil
		}
	}

	// Fetch from GitHub
	listings, fetchErr := fetchFromGitHub(ctx, opts.BaseURL)
	if fetchErr != nil {
		log.Warn().Err(fetchErr).Msg("failed to fetch from GitHub API")

		// Fall back to stale cache if available
		cached, cacheErr := loadCache(cachePath)
		if cacheErr == nil {
			log.Debug().Msg("falling back to stale cache after network error")
			return cached.Listings, nil
		}

		return nil, fmt.Errorf("fetching strategies from GitHub: %w (no cached results available)", fetchErr)
	}

	// Write cache atomically
	if writeErr := writeCache(cachePath, listings); writeErr != nil {
		log.Warn().Err(writeErr).Msg("failed to write cache file")
	}

	return listings, nil
}

// loadCache reads and parses the cache file at the given path.
func loadCache(cachePath string) (*CachedResults, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("reading cache file: %w", err)
	}

	var cached CachedResults
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, fmt.Errorf("parsing cache file: %w", err)
	}

	return &cached, nil
}

// writeCache writes the listings to the cache file atomically using a temp file and rename.
func writeCache(cachePath string, listings []Listing) error {
	cached := CachedResults{
		Timestamp: time.Now(),
		Listings:  listings,
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return fmt.Errorf("marshaling cache: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Dir(cachePath), "discover-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp cache file: %w", err)
	}

	tempPath := tempFile.Name()

	if _, writeErr := tempFile.Write(data); writeErr != nil {
		tempFile.Close()
		os.Remove(tempPath)

		return fmt.Errorf("writing temp cache file: %w", writeErr)
	}

	if closeErr := tempFile.Close(); closeErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("closing temp cache file: %w", closeErr)
	}

	if renameErr := os.Rename(tempPath, cachePath); renameErr != nil {
		os.Remove(tempPath)
		return fmt.Errorf("renaming temp cache file: %w", renameErr)
	}

	return nil
}

// resolveAuthToken attempts to find a GitHub token for API authentication.
// It checks GITHUB_TOKEN env var first, then falls back to `gh auth token`.
func resolveAuthToken() string {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	output, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// fetchFromGitHub retrieves strategy repositories from the GitHub Search API with pagination.
func fetchFromGitHub(ctx context.Context, baseURL string) ([]Listing, error) {
	authToken := resolveAuthToken()

	var allListings []Listing

	for page := 1; ; page++ {
		requestURL := fmt.Sprintf("%s/search/repositories?q=topic:pvbt-strategy&per_page=%d&page=%d", baseURL, perPage, page)
		log.Info().Str("url", requestURL).Int("page", page).Msg("fetching from GitHub Search API")

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		request.Header.Set("Accept", "application/vnd.github+json")

		if authToken != "" {
			request.Header.Set("Authorization", "Bearer "+authToken)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("executing request for page %d: %w", page, err)
		}

		body, readErr := io.ReadAll(response.Body)
		response.Body.Close() // close explicitly in pagination loop, not with defer

		if readErr != nil {
			return nil, fmt.Errorf("reading response body for page %d: %w", page, readErr)
		}

		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("GitHub API returned status %d: %s", response.StatusCode, string(body))
		}

		var searchResult githubSearchResponse
		if unmarshalErr := json.Unmarshal(body, &searchResult); unmarshalErr != nil {
			return nil, fmt.Errorf("parsing GitHub response for page %d: %w", page, unmarshalErr)
		}

		for _, item := range searchResult.Items {
			listing := convertItemToListing(item)
			allListings = append(allListings, listing)
		}

		// Stop if we've fetched all results or hit the max
		totalFetched := len(allListings)
		if totalFetched >= searchResult.TotalCount || totalFetched >= maxResults || len(searchResult.Items) < perPage {
			break
		}
	}

	return allListings, nil
}

// convertItemToListing converts a GitHub repo item to a Listing, stripping pvbt-strategy from topics.
func convertItemToListing(item githubRepoItem) Listing {
	var categories []string

	for _, topic := range item.Topics {
		if topic != "pvbt-strategy" {
			categories = append(categories, topic)
		}
	}

	return Listing{
		Name:        item.Name,
		Owner:       item.Owner.Login,
		Description: item.Description,
		Categories:  categories,
		CloneURL:    item.CloneURL,
		Stars:       item.Stars,
		UpdatedAt:   item.UpdatedAt,
	}
}
