package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
)

func newDescribeCmd(strategy engine.Strategy) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "describe",
		Short: "Display strategy metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := engine.DescribeStrategy(strategy)

			useJSON, err := cmd.Flags().GetBool("json")
			if err != nil {
				return err
			}

			if useJSON {
				data, marshalErr := json.MarshalIndent(info, "", "  ")
				if marshalErr != nil {
					return fmt.Errorf("marshal descriptor: %w", marshalErr)
				}

				fmt.Fprintln(cmd.OutOrStdout(), string(data))

				return nil
			}

			return renderDescribe(cmd, info)
		},
	}

	cmd.Flags().Bool("json", false, "Output as JSON")

	return cmd
}

func renderDescribe(cmd *cobra.Command, info engine.StrategyInfo) error {
	out := cmd.OutOrStdout()

	header := info.Name

	if info.ShortCode != "" {
		header += fmt.Sprintf(" (%s)", info.ShortCode)
	}

	if info.Version != "" {
		header += fmt.Sprintf(" v%s", info.Version)
	}

	fmt.Fprintln(out, header)

	if info.Description != "" {
		fmt.Fprintln(out, info.Description)
	}

	if info.Source != "" {
		fmt.Fprintf(out, "Source: %s\n", info.Source)
	}

	if info.Schedule != "" {
		fmt.Fprintf(out, "Schedule:  %s\n", info.Schedule)
	}

	if info.Benchmark != "" {
		fmt.Fprintf(out, "Benchmark: %s (suggested)\n", info.Benchmark)
	}

	if len(info.Parameters) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Parameters:")

		tw := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)

		for _, param := range info.Parameters {
			defaultStr := ""
			if param.Default != "" {
				defaultStr = fmt.Sprintf("default: %s", param.Default)
			}

			fmt.Fprintf(tw, "  %s\t%s\t%s\n", param.Name, param.Description, defaultStr)
		}

		tw.Flush()
	}

	if len(info.Suggestions) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Presets:")

		presetNames := make([]string, 0, len(info.Suggestions))
		for name := range info.Suggestions {
			presetNames = append(presetNames, name)
		}

		sort.Strings(presetNames)

		tw := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)

		for _, name := range presetNames {
			params := info.Suggestions[name]

			paramNames := make([]string, 0, len(params))
			for paramName := range params {
				paramNames = append(paramNames, paramName)
			}

			sort.Strings(paramNames)

			parts := make([]string, 0, len(paramNames))
			for _, paramName := range paramNames {
				parts = append(parts, fmt.Sprintf("%s=%s", paramName, params[paramName]))
			}

			fmt.Fprintf(tw, "  %s\t%s\n", name, strings.Join(parts, "  "))
		}

		tw.Flush()
	}

	return nil
}
