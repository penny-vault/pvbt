package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/data"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RunPVBT is the entry point for the standalone pvbt tool.
func RunPVBT() {
	rootCmd := &cobra.Command{
		Use:   "pvbt",
		Short: "Penny Vault backtesting tools",
	}

	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal().Err(err).Msg("failed to bind log-level flag")
	}

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		level, err := zerolog.ParseLevel(viper.GetString("log-level"))
		if err != nil {
			level = zerolog.InfoLevel
		}

		zerolog.SetGlobalLevel(level)

		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().Timestamp().Logger()
	}

	rootCmd.AddCommand(newExploreCmd())
	rootCmd.AddCommand(newListCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newExploreCmd() *cobra.Command {
	now := time.Now()
	oneYearAgo := now.AddDate(-1, 0, 0)

	cmd := &cobra.Command{
		Use:   "explore <tickers> <metrics> [flags]",
		Short: "Query and visualize data from the PVDataProvider",
		Long: `Explore fetches data for the given tickers and metrics from the
PVDataProvider database and displays it as a table or graph.

  explore AAPL,MSFT AdjClose,Volume
  explore AAPL AdjClose --graph
  explore --list-metrics`,
		Args: func(cmd *cobra.Command, args []string) error {
			listMetrics, err := cmd.Flags().GetBool("list-metrics")
			if err != nil {
				return fmt.Errorf("reading list-metrics flag: %w", err)
			}

			if listMetrics {
				return nil
			}

			if len(args) < 2 {
				return fmt.Errorf("requires 2 arguments: <tickers> <metrics>")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			listMetrics, err := cmd.Flags().GetBool("list-metrics")
			if err != nil {
				return fmt.Errorf("reading list-metrics flag: %w", err)
			}

			if listMetrics {
				return runListMetrics()
			}

			return runExplore(cmd, args[0], args[1])
		},
	}

	cmd.Flags().String("start", oneYearAgo.Format("2006-01-02"), "Start date (YYYY-MM-DD)")
	cmd.Flags().String("end", now.Format("2006-01-02"), "End date (YYYY-MM-DD)")
	cmd.Flags().Bool("graph", false, "Show TUI graph instead of table")
	cmd.Flags().Bool("list-metrics", false, "List all available metric names and exit")

	return cmd
}

func runListMetrics() error {
	names := data.AllMetricNames()
	for _, name := range names {
		fmt.Println(name)
	}

	return nil
}

func runExplore(cmd *cobra.Command, tickersArg, metricsArg string) error {
	ctx := context.Background()

	tickers := strings.Split(tickersArg, ",")
	metricNames := strings.Split(metricsArg, ",")

	// resolve metrics
	metrics := make([]data.Metric, 0, len(metricNames))
	for _, name := range metricNames {
		name = strings.TrimSpace(name)

		m, ok := data.MetricByName(name)
		if !ok {
			return fmt.Errorf("unknown metric %q (use --list-metrics to see available names)", name)
		}

		metrics = append(metrics, m)
	}

	// parse dates
	startStr, err := cmd.Flags().GetString("start")
	if err != nil {
		return err
	}

	start, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return fmt.Errorf("invalid start date: %w", err)
	}

	endStr, err := cmd.Flags().GetString("end")
	if err != nil {
		return err
	}

	end, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return fmt.Errorf("invalid end date: %w", err)
	}

	// create provider
	provider, err := data.NewPVDataProvider(nil)
	if err != nil {
		return fmt.Errorf("create data provider: %w", err)
	}
	defer provider.Close()

	// resolve tickers to assets
	assets := make([]asset.Asset, 0, len(tickers))
	for _, ticker := range tickers {
		ticker = strings.TrimSpace(ticker)

		a, err := provider.LookupAsset(ctx, ticker)
		if err != nil {
			return fmt.Errorf("lookup ticker %q: %w", ticker, err)
		}

		assets = append(assets, a)
	}

	// fetch data
	req := data.DataRequest{
		Assets:  assets,
		Metrics: metrics,
		Start:   start,
		End:     end,
	}

	log.Info().
		Strs("tickers", tickers).
		Int("metrics", len(metrics)).
		Time("start", start).
		Time("end", end).
		Msg("fetching data")

	df, err := provider.Fetch(ctx, req)
	if err != nil {
		return fmt.Errorf("fetch data: %w", err)
	}

	if df.Len() == 0 {
		fmt.Println("No data returned.")
		return nil
	}

	showGraph, err := cmd.Flags().GetBool("graph")
	if err != nil {
		return err
	}

	if showGraph {
		return runExploreGraph(df)
	}

	fmt.Print(df.Table())
	fmt.Printf("\n%d rows\n", df.Len())

	return nil
}
