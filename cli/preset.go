package cli

import (
	"fmt"
	"sort"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

// applyPreset looks up the named preset from the strategy's suggestions
// and sets the corresponding flag values. Flags explicitly provided by
// the user override preset values because cobra tracks whether a flag was
// changed.
func applyPreset(cmd *cobra.Command, strategy engine.Strategy) error {
	presetFlag := cmd.Flags().Lookup("preset")
	if presetFlag == nil || !presetFlag.Changed {
		return nil
	}

	presetName := presetFlag.Value.String()

	// Build the suggestions map from struct tags.
	params := engine.StrategyParameters(strategy)
	suggestions := make(map[string]map[string]string)

	for _, param := range params {
		for sugName, sugVal := range param.Suggestions {
			if suggestions[sugName] == nil {
				suggestions[sugName] = make(map[string]string)
			}

			suggestions[sugName][param.Name] = sugVal
		}
	}

	preset, ok := suggestions[presetName]
	if !ok {
		available := make([]string, 0, len(suggestions))
		for name := range suggestions {
			available = append(available, name)
		}

		sort.Strings(available)

		return fmt.Errorf("unknown preset %q (available: %v)", presetName, available)
	}

	// Set flag values for preset parameters, but only if the user did not
	// explicitly provide the flag.
	for paramName, value := range preset {
		flag := cmd.Flags().Lookup(paramName)
		if flag == nil {
			continue
		}

		if flag.Changed {
			// User explicitly set this flag -- don't override.
			continue
		}

		if err := flag.Value.Set(value); err != nil {
			return fmt.Errorf("preset %q: setting %s=%s: %w", presetName, paramName, value, err)
		}
	}

	return nil
}
