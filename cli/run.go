package cli

import (
	"fmt"
	"os"

	"github.com/penny-vault/pvbt/engine"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Run is the single entry point for strategy authors. It builds the
// cobra command tree, parses flags, and executes the appropriate
// subcommand.
func Run(strategy engine.Strategy) {
	rootCmd := &cobra.Command{
		Use:   strategy.Name(),
		Short: fmt.Sprintf("Run the %s strategy", strategy.Name()),
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

	rootCmd.AddCommand(newBacktestCmd(strategy))
	rootCmd.AddCommand(newLiveCmd(strategy))
	rootCmd.AddCommand(newSnapshotCmd(strategy))
	rootCmd.AddCommand(newDescribeCmd(strategy))
	rootCmd.AddCommand(newStudyCmd(strategy))

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
