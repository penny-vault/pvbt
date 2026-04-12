package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime/pprof"

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
	if err := run(strategy); err != nil {
		os.Exit(1)
	}
}

// run is the testable half of Run. It is factored out so the deferred
// profile cleanup always fires before Run's os.Exit: defer runs on
// normal return, on panic, and when run hands control back to Run.
func run(strategy engine.Strategy) error {
	rootCmd, cleanup := newRootCmd(strategy)
	defer cleanup()

	return rootCmd.Execute()
}

// newRootCmd builds the root cobra command for a strategy binary. It
// returns the command together with a cleanup function that must be
// deferred by the caller; the cleanup function stops and closes the
// CPU profile when one was started by the --cpu-profile flag. Cleanup
// cannot live in PersistentPostRunE because cobra does not invoke that
// hook when a subcommand's RunE returns a non-nil error, which would
// leak the profile file descriptor and lose the captured samples.
//
// newRootCmd is extracted from Run so tests can exercise the command
// tree in-process without calling os.Exit. The --cpu-profile persistent
// flag is wired here so every subcommand (backtest, live, snapshot,
// describe, study, config) inherits it and no strategy has to carry
// runtime/pprof boilerplate in its own main.
func newRootCmd(strategy engine.Strategy) (*cobra.Command, func()) {
	var cpuProfileFile *os.File

	cleanup := func() {
		if cpuProfileFile == nil {
			return
		}

		pprof.StopCPUProfile()

		if err := cpuProfileFile.Close(); err != nil {
			// This is the single place in the codebase where degrading
			// from error to warning is acceptable: the command has
			// already finished running, the defer has no return value,
			// and the alternative is either leaking the file or
			// double-reporting an error after the command exited.
			log.Warn().Err(err).Msg("cli: close cpu profile")
		}

		cpuProfileFile = nil
	}

	rootCmd := &cobra.Command{
		Use:   strategy.Name(),
		Short: fmt.Sprintf("Run the %s strategy", strategy.Name()),
	}

	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("config", "", "Path to config file (default: ./pvbt.toml or ~/.config/pvbt/config.toml)")
	rootCmd.PersistentFlags().String("cpu-profile", "", "Write a Go CPU profile to the given path for the duration of the command")

	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		log.Fatal().Err(err).Msg("failed to bind log-level flag")
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		level, err := zerolog.ParseLevel(viper.GetString("log-level"))
		if err != nil {
			level = zerolog.InfoLevel
		}

		zerolog.SetGlobalLevel(level)

		log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().Timestamp().Logger()

		profilePath, err := cmd.Flags().GetString("cpu-profile")
		if err != nil {
			return fmt.Errorf("cli: read cpu-profile flag: %w", err)
		}

		if profilePath == "" {
			return nil
		}

		file, err := os.Create(profilePath)
		if err != nil {
			return fmt.Errorf("cli: create cpu profile %q: %w", profilePath, err)
		}

		if err := pprof.StartCPUProfile(file); err != nil {
			if closeErr := file.Close(); closeErr != nil {
				return fmt.Errorf("cli: start cpu profile: %w", errors.Join(err, closeErr))
			}

			return fmt.Errorf("cli: start cpu profile: %w", err)
		}

		cpuProfileFile = file

		return nil
	}

	rootCmd.AddCommand(newBacktestCmd(strategy))
	rootCmd.AddCommand(newLiveCmd(strategy))
	rootCmd.AddCommand(newSnapshotCmd(strategy))
	rootCmd.AddCommand(newDescribeCmd(strategy))
	rootCmd.AddCommand(newStudyCmd(strategy))
	rootCmd.AddCommand(newConfigCmd())

	return rootCmd, cleanup
}
