package library

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
