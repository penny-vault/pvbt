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

// Package optimize implements the parameter-optimization study type. It
// evaluates strategy parameter combinations across cross-validation splits,
// ranks them by out-of-sample performance, and produces a report with
// rankings, per-fold detail for the best combination, and an overfitting
// diagnostic comparing in-sample versus out-of-sample scores.
package optimize
