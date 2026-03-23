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

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// candidatePaths returns the ordered list of config file paths to search when
// no explicit path is provided.
func candidatePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{"pvbt.toml"}
	}

	return []string{
		"pvbt.toml",
		filepath.Join(home, ".config", "pvbt", "config.toml"),
	}
}

// Load reads a TOML config file from configPath and returns a validated Config.
// If configPath is empty, Load searches ./pvbt.toml then
// ~/.config/pvbt/config.toml. If no file is found, a zero-value Config is
// returned without error. All errors are wrapped with context.
func Load(configPath string) (*Config, error) {
	vp := viper.New()
	vp.SetConfigType("toml")

	if configPath != "" {
		vp.SetConfigFile(configPath)
	} else {
		found := ""

		for _, candidate := range candidatePaths() {
			if _, statErr := os.Stat(candidate); statErr == nil {
				found = candidate
				break
			}
		}

		if found == "" {
			return &Config{}, nil
		}

		vp.SetConfigFile(found)
	}

	if err := vp.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("load config: read file: %w", err)
	}

	cfg := &Config{}
	if err := vp.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("load config: unmarshal: %w", err)
	}

	if err := cfg.ValidateAndApplyDefaults(); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}

// LoadFromCommand loads config from the --config flag on cmd, then applies any
// --risk-profile and --tax flag overrides that were explicitly set by the user.
// Re-validation is performed after overrides are applied.
func LoadFromCommand(cmd *cobra.Command) (*Config, error) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, fmt.Errorf("load config from command: get --config flag: %w", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		return nil, err
	}

	if cmd.Flags().Changed("risk-profile") {
		profile, flagErr := cmd.Flags().GetString("risk-profile")
		if flagErr != nil {
			return nil, fmt.Errorf("load config from command: get --risk-profile flag: %w", flagErr)
		}

		cfg.Risk.Profile = profile
	}

	if cmd.Flags().Changed("tax") {
		taxEnabled, flagErr := cmd.Flags().GetBool("tax")
		if flagErr != nil {
			return nil, fmt.Errorf("load config from command: get --tax flag: %w", flagErr)
		}

		cfg.Tax.Enabled = taxEnabled
	}

	if err := cfg.ValidateAndApplyDefaults(); err != nil {
		return nil, fmt.Errorf("load config from command: %w", err)
	}

	return cfg, nil
}

// ConfigFilePath returns the path of the config file that would be loaded for
// the given configPath argument. If configPath is non-empty it is returned as-is
// when the file exists. If configPath is empty the same search order as Load is
// used. An empty string is returned when no file is found.
func ConfigFilePath(configPath string) string {
	if configPath != "" {
		if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
			return ""
		}

		return configPath
	}

	for _, candidate := range candidatePaths() {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}
