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

// Package library manages installation, listing, and removal of pvbt
// strategy plugins. Each installed strategy is a standalone Go binary
// cloned from a Git repository, built locally, and indexed by short-code.
//
// # Installation
//
// [Install] clones a repository, builds the binary, validates it via
// the describe subcommand, checks for short-code collisions, and writes
// an index.json manifest:
//
//	strategy, err := library.Install(ctx, library.DefaultLibDir(), cloneURL)
//
// # Discovery
//
// [List] returns all installed strategies by scanning the library
// directory. [Lookup] finds a single strategy by short-code:
//
//	strategies, err := library.List(library.DefaultLibDir())
//	strategy, err := library.Lookup(library.DefaultLibDir(), "adm")
//
// # Removal
//
// [Remove] deletes a strategy's directory and binary:
//
//	library.Remove(library.DefaultLibDir(), "adm")
//
// The default library directory is ~/.pvbt/lib, returned by
// [DefaultLibDir]. The default cache directory is ~/.pvbt/cache,
// returned by [DefaultCacheDir].
package library
