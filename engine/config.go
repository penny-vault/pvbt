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

package engine

import "github.com/penny-vault/pvbt/asset"

// Config holds strategy metadata parsed from the TOML file. Strategy
// arguments are not accessed through Config -- the engine populates
// them directly on the strategy struct via reflection before calling
// Setup.
type Config struct {
	Name        string
	Shortcode   string
	Description string
	Source      string
	Version     string
	Schedule    string
	Benchmark   asset.Asset
}
