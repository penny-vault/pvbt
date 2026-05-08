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

package cli

import (
	"fmt"
	"strings"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/portfolio"
	"github.com/spf13/cobra"
)

// registerMarginFlags adds --margin-model, --max-leverage, and
// --gross-maintenance-leverage to the given command. The default model
// is Reg T (50% initial, 25% maintenance), matching real US brokerage
// accounts.
func registerMarginFlags(cmd *cobra.Command) {
	cmd.Flags().String("margin-model", "regt",
		"Margin model preset: 'regt' (50% initial / 25% maintenance) or 'cash' (no margin)")
	cmd.Flags().Float64("max-leverage", 0,
		"Override the entry-time gross leverage cap (LMV+SMV)/Equity. 0 uses the model preset or strategy value.")
	cmd.Flags().Float64("gross-maintenance-leverage", 0,
		"Override the gross leverage liquidation threshold. 0 uses the model preset or strategy value.")
}

// resolveMarginOptions reads the margin flags and returns the engine
// options that express the requested margin configuration. Per-knob
// flags (--max-leverage, --gross-maintenance-leverage) override the
// model preset.
func resolveMarginOptions(cmd *cobra.Command) ([]engine.Option, error) {
	model, err := cmd.Flags().GetString("margin-model")
	if err != nil {
		return nil, err
	}

	var opts []engine.Option

	switch strings.ToLower(strings.TrimSpace(model)) {
	case "regt", "":
		opts = append(opts, engine.WithMarginModel(portfolio.RegT{Initial: 0.5, Maintenance: 0.25}))
	case "cash":
		// Cash account: 1x entry cap, no leverage-driven liquidation
		// trigger. (Maintenance: 0 leaves the default 4.0 in place,
		// but a cash account cannot mathematically reach 4x gross
		// leverage so the trigger is dormant.)
		opts = append(opts, engine.WithMarginModel(portfolio.RegT{Initial: 1.0}))
	default:
		return nil, fmt.Errorf("unknown --margin-model %q (expected 'regt' or 'cash')", model)
	}

	maxLev, err := cmd.Flags().GetFloat64("max-leverage")
	if err != nil {
		return nil, err
	}

	if maxLev > 0 {
		opts = append(opts, engine.WithMaxLeverage(maxLev))
	}

	maintLev, err := cmd.Flags().GetFloat64("gross-maintenance-leverage")
	if err != nil {
		return nil, err
	}

	if maintLev > 0 {
		opts = append(opts, engine.WithGrossMaintenanceLeverage(maintLev))
	}

	return opts, nil
}
