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

// Package registry discovers pvbt strategies published on GitHub.
//
// [Search] queries the GitHub Search API for repositories tagged with the
// pvbt-strategy topic and returns a slice of [Listing] structs. Results
// are cached locally for one hour; stale cache is used as a fallback on
// network errors.
//
//	listings, err := registry.Search(ctx, registry.SearchOptions{
//	    CacheDir: registry.DefaultCacheDir(),
//	})
//
// Each [Listing] contains the repository name, owner, description, clone
// URL, star count, and last-updated timestamp. Pass the CloneURL to
// [library.Install] to install a discovered strategy.
package registry
