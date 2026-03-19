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

package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog/log"
)

// InstalledStrategy represents a strategy that has been installed locally.
type InstalledStrategy struct {
	ShortCode   string    `json:"short_code"`
	RepoOwner   string    `json:"repo_owner"`
	RepoName    string    `json:"repo_name"`
	Version     string    `json:"version"`
	BinPath     string    `json:"bin_path"`
	InstalledAt time.Time `json:"installed_at"`
}

// Install clones a strategy repository, builds it, extracts its short-code
// via the "describe" subcommand, checks for collisions, and writes an
// index.json. On failure it cleans up the cloned directory.
func Install(ctx context.Context, libDir string, cloneURL string) (*InstalledStrategy, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found on PATH: %w", err)
	}

	if _, err := exec.LookPath("go"); err != nil {
		return nil, fmt.Errorf("go not found on PATH: %w", err)
	}

	repoName := repoNameFromURL(cloneURL)
	owner := ownerFromURL(cloneURL)
	destDir := filepath.Join(libDir, repoName)

	if err := os.MkdirAll(libDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating library directory: %w", err)
	}

	// Clone the repository.
	gitClone := exec.CommandContext(ctx, "git", "clone", "--depth=1", cloneURL, destDir)
	if output, err := gitClone.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %s: %w", string(output), err)
	}

	// Build the binary.
	binPath := filepath.Join(destDir, repoName)
	goBuild := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	goBuild.Dir = destDir

	if output, err := goBuild.CombinedOutput(); err != nil {
		cleanup(destDir)
		return nil, fmt.Errorf("go build failed: %s: %w", string(output), err)
	}

	// Run <binary> describe to get strategy metadata.
	describeCmd := exec.CommandContext(ctx, binPath, "describe")

	describeOutput, err := describeCmd.Output()
	if err != nil {
		cleanup(destDir)
		return nil, fmt.Errorf("running describe command: %w", err)
	}

	var description engine.StrategyInfo
	if err := json.Unmarshal(describeOutput, &description); err != nil {
		cleanup(destDir)
		return nil, fmt.Errorf("parsing describe output: %w", err)
	}

	shortCode := description.ShortCode
	if shortCode == "" {
		cleanup(destDir)
		return nil, errors.New("strategy did not provide a short-code in describe output")
	}

	// Check for collision with existing strategies.
	if err := CheckCollision(libDir, shortCode, repoName); err != nil {
		cleanup(destDir)
		return nil, err
	}

	strategy := &InstalledStrategy{
		ShortCode:   shortCode,
		RepoOwner:   owner,
		RepoName:    repoName,
		Version:     description.Version,
		BinPath:     binPath,
		InstalledAt: time.Now().UTC(),
	}

	// Write index.json.
	indexData, err := json.MarshalIndent(strategy, "", "  ")
	if err != nil {
		cleanup(destDir)
		return nil, fmt.Errorf("marshaling index.json: %w", err)
	}

	indexPath := filepath.Join(destDir, "index.json")
	if err := os.WriteFile(indexPath, indexData, 0o644); err != nil {
		cleanup(destDir)
		return nil, fmt.Errorf("writing index.json: %w", err)
	}

	log.Info().
		Str("shortCode", shortCode).
		Str("repo", repoName).
		Str("version", description.Version).
		Msg("strategy installed")

	return strategy, nil
}

// List returns all installed strategies by scanning libDir/*/index.json.
// Returns nil, nil if the directory does not exist.
func List(libDir string) ([]InstalledStrategy, error) {
	if _, err := os.Stat(libDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(libDir)
	if err != nil {
		return nil, fmt.Errorf("reading library directory: %w", err)
	}

	var strategies []InstalledStrategy

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		indexPath := filepath.Join(libDir, entry.Name(), "index.json")

		data, readErr := os.ReadFile(indexPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}

			return nil, fmt.Errorf("reading %s: %w", indexPath, readErr)
		}

		var strategy InstalledStrategy
		if unmarshalErr := json.Unmarshal(data, &strategy); unmarshalErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", indexPath, unmarshalErr)
		}

		strategies = append(strategies, strategy)
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
			return &strategies[idx], nil
		}
	}

	return nil, fmt.Errorf("strategy with short-code %q not found", shortCode)
}

// Remove removes an installed strategy identified by short-code.
func Remove(libDir string, shortCode string) error {
	strategy, err := Lookup(libDir, shortCode)
	if err != nil {
		return err
	}

	strategyDir := filepath.Join(libDir, strategy.RepoName)

	if err := os.RemoveAll(strategyDir); err != nil {
		return fmt.Errorf("removing strategy directory %s: %w", strategyDir, err)
	}

	log.Info().
		Str("shortCode", shortCode).
		Str("repo", strategy.RepoName).
		Msg("strategy removed")

	return nil
}

// CheckCollision checks whether the given short-code is already in use by a
// different repository. Returns nil if there is no collision or if the same
// repo is re-installing.
func CheckCollision(libDir string, shortCode string, repoName string) error {
	existing, err := Lookup(libDir, shortCode)
	if err != nil {
		// Not found means no collision.
		return nil
	}

	if existing.RepoName != repoName {
		return fmt.Errorf(
			"short-code %q is already installed from repo %q; cannot install from %q",
			shortCode, existing.RepoName, repoName,
		)
	}

	return nil
}

// DefaultLibDir returns the default library directory path.
func DefaultLibDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warn().Err(err).Msg("could not determine home directory")
		return filepath.Join(".", ".pvbt", "lib")
	}

	return filepath.Join(homeDir, ".pvbt", "lib")
}

// DefaultCacheDir returns the default cache directory path.
func DefaultCacheDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Warn().Err(err).Msg("could not determine home directory")
		return filepath.Join(".", ".pvbt", "cache")
	}

	return filepath.Join(homeDir, ".pvbt", "cache")
}

// repoNameFromURL extracts the repository name from a clone URL.
// Handles both HTTPS and SSH-style URLs.
func repoNameFromURL(cloneURL string) string {
	// Remove trailing .git suffix.
	trimmed := strings.TrimSuffix(cloneURL, ".git")

	// Try parsing as a standard URL first.
	if parsed, parseErr := url.Parse(trimmed); parseErr == nil && parsed.Host != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Handle SSH-style URLs like git@github.com:owner/repo.
	if colonIdx := strings.LastIndex(trimmed, ":"); colonIdx != -1 {
		pathPart := trimmed[colonIdx+1:]

		parts := strings.Split(pathPart, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: use the last path component.
	return filepath.Base(trimmed)
}

// ownerFromURL extracts the owner (org or user) from a clone URL.
func ownerFromURL(cloneURL string) string {
	trimmed := strings.TrimSuffix(cloneURL, ".git")

	// Try parsing as a standard URL first.
	if parsed, parseErr := url.Parse(trimmed); parseErr == nil && parsed.Host != "" {
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	// Handle SSH-style URLs like git@github.com:owner/repo.
	if colonIdx := strings.LastIndex(trimmed, ":"); colonIdx != -1 {
		pathPart := trimmed[colonIdx+1:]

		parts := strings.Split(pathPart, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}

	return ""
}

// cleanup removes a directory and logs a warning if removal fails.
func cleanup(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Warn().Err(err).Str("dir", dir).Msg("failed to clean up directory")
	}
}
