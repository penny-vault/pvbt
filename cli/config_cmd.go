package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/penny-vault/pvbt/config"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Display resolved middleware configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfig(cmd)
		},
	}

	cmd.Flags().String("risk-profile", "", "Risk profile (conservative, moderate, aggressive, none)")
	cmd.Flags().Bool("tax", false, "Enable tax optimization")

	return cmd
}

func runConfig(cmd *cobra.Command) error {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		return fmt.Errorf("config: get --config flag: %w", err)
	}

	filePath := config.ConfigFilePath(configPath)

	cfg, err := config.LoadFromCommand(cmd)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// If no config file was found and no flags were set, nothing to show.
	if filePath == "" && !cfg.HasMiddleware() {
		fmt.Fprintln(os.Stdout, "No configuration found.")
		return nil
	}

	if filePath != "" {
		fmt.Fprintf(os.Stdout, "Config file: %s\n", filePath)
	}

	printRiskSection(os.Stdout, cfg)
	printTaxSection(os.Stdout, cfg)

	return nil
}

func printRiskSection(out *os.File, cfg *config.Config) {
	rc := cfg.Risk

	// Determine if anything risk-related is configured.
	hasProfile := rc.Profile != ""
	hasOverrides := rc.MaxPositionSize != nil ||
		rc.MaxPositionCount != nil ||
		rc.DrawdownCircuitBreaker != nil ||
		rc.VolatilityScalerLookback != nil ||
		rc.GrossExposureLimit != nil ||
		rc.NetExposureLimit != nil

	if !hasProfile && !hasOverrides {
		return
	}

	fmt.Fprintln(out, "\nRisk:")

	if hasProfile {
		fmt.Fprintf(out, "  profile: %s\n", rc.Profile)
	}

	resolved := cfg.ResolveProfile()
	baseline := config.ProfileBaseline(rc.Profile)

	printFloat64Field(out, "max_position_size", resolved.MaxPositionSize, baseline.MaxPositionSize, rc.MaxPositionSize, hasProfile)
	printIntField(out, "max_position_count", resolved.MaxPositionCount, baseline.MaxPositionCount, rc.MaxPositionCount, hasProfile)
	printFloat64Field(out, "drawdown_circuit_breaker", resolved.DrawdownCircuitBreaker, baseline.DrawdownCircuitBreaker, rc.DrawdownCircuitBreaker, hasProfile)
	printIntField(out, "volatility_scaler_lookback", resolved.VolatilityScalerLookback, baseline.VolatilityScalerLookback, rc.VolatilityScalerLookback, hasProfile)
	printFloat64Field(out, "gross_exposure_limit", resolved.GrossExposureLimit, baseline.GrossExposureLimit, rc.GrossExposureLimit, hasProfile)
	printFloat64Field(out, "net_exposure_limit", resolved.NetExposureLimit, baseline.NetExposureLimit, rc.NetExposureLimit, hasProfile)
}

// printFloat64Field prints a single float64 risk field with an annotation when
// a profile is active.
func printFloat64Field(out *os.File, name string, resolved, baselineVal, overrideVal *float64, hasProfile bool) {
	if resolved == nil {
		return
	}

	if !hasProfile {
		fmt.Fprintf(out, "  %s: %g\n", name, *resolved)
		return
	}

	annotation := annotationFloat64(baselineVal, overrideVal)
	fmt.Fprintf(out, "  %s: %g (%s)\n", name, *resolved, annotation)
}

// printIntField prints a single int risk field with an annotation when a
// profile is active.
func printIntField(out *os.File, name string, resolved, baselineVal, overrideVal *int, hasProfile bool) {
	if resolved == nil {
		return
	}

	if !hasProfile {
		fmt.Fprintf(out, "  %s: %d\n", name, *resolved)
		return
	}

	annotation := annotationInt(baselineVal, overrideVal)
	fmt.Fprintf(out, "  %s: %d (%s)\n", name, *resolved, annotation)
}

// annotationFloat64 returns the annotation string for a float64 field given
// its baseline value and any explicit override.
//
// Rules:
//   - baseline is nil (field not part of profile): "added"
//   - override is set (value differs from profile default): "override"
//   - value comes from profile: "profile default"
func annotationFloat64(baselineVal, overrideVal *float64) string {
	if baselineVal == nil {
		return "added"
	}

	if overrideVal != nil {
		return "override"
	}

	return "profile default"
}

// annotationInt returns the annotation string for an int field.
func annotationInt(baselineVal, overrideVal *int) string {
	if baselineVal == nil {
		return "added"
	}

	if overrideVal != nil {
		return "override"
	}

	return "profile default"
}

func printTaxSection(out *os.File, cfg *config.Config) {
	tc := cfg.Tax

	if !tc.Enabled && tc.LossThreshold == 0 && !tc.GainOffsetOnly && len(tc.Substitutes) == 0 {
		return
	}

	fmt.Fprintln(out, "\nTax:")
	fmt.Fprintf(out, "  enabled: %v\n", tc.Enabled)

	if tc.Enabled || tc.LossThreshold != 0 {
		fmt.Fprintf(out, "  loss_threshold: %g\n", tc.LossThreshold)
	}

	fmt.Fprintf(out, "  gain_offset_only: %v\n", tc.GainOffsetOnly)

	if len(tc.Substitutes) > 0 {
		fmt.Fprintln(out, "  substitutes:")

		keys := make([]string, 0, len(tc.Substitutes))
		for k := range tc.Substitutes {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		for _, k := range keys {
			fmt.Fprintf(out, "    %s -> %s\n", k, tc.Substitutes[k])
		}
	}
}
