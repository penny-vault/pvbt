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

// Package report defines the types and interfaces for structured backtest
// reports. Domain types (Header, EquityCurve, Risk, etc.) implement the
// Section interface, allowing renderers to type-assert for styled output.
//
// Report builders live in subpackages (e.g., summary.Build) and renderers
// in renderer subpackages (e.g., renderer/terminal.Render).
package report
