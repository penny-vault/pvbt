package cli

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/asset"
	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/universe"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	assetType    = reflect.TypeOf(asset.Asset{})
	universeType = reflect.TypeOf((*universe.Universe)(nil)).Elem()
	durationType = reflect.TypeOf(time.Duration(0))
)

// registerStrategyFlags inspects the strategy struct's exported fields
// and registers a cobra flag for each one. The flag name comes from
// the `pvbt` struct tag (falling back to the field name in kebab-case).
// The description comes from the `desc` tag. Default values come from
// the `default` tag or the field's current value.
func registerStrategyFlags(cmd *cobra.Command, strategy engine.Strategy) {
	registerStrategyFlagsWithOptions(cmd, strategy, false)
}

// registerStrategyFlagsForSweep registers strategy flags with int and
// float64 fields exposed as String flags so they can accept either a
// fixed value (e.g. "5") or a min:max:step range (e.g. "0:8:1"). Use
// this for subcommands that perform parameter sweeps; use
// registerStrategyFlags for run commands that take a single value.
func registerStrategyFlagsForSweep(cmd *cobra.Command, strategy engine.Strategy) {
	registerStrategyFlagsWithOptions(cmd, strategy, true)
}

func registerStrategyFlagsWithOptions(cmd *cobra.Command, strategy engine.Strategy, allowRanges bool) {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return
	}

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		// Skip fields marked test-only -- they must not be exposed as
		// CLI flags.
		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)

		desc := field.Tag.Get("desc")
		defaultStr := field.Tag.Get("default")
		fieldValue := val.Field(ii)

		switch {
		case field.Type == assetType:
			cmd.Flags().String(name, defaultStr, desc)

		case field.Type.Implements(universeType):
			cmd.Flags().String(name, defaultStr, desc)

		case field.Type == durationType:
			def := time.Duration(fieldValue.Int())

			if defaultStr != "" {
				if parsed, err := time.ParseDuration(defaultStr); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Duration(name, def, desc)

		case field.Type.Kind() == reflect.Float64:
			if allowRanges {
				def := strconv.FormatFloat(fieldValue.Float(), 'f', -1, 64)
				if defaultStr != "" {
					def = defaultStr
				}

				cmd.Flags().String(name, def, desc)

				continue
			}

			def := fieldValue.Float()

			if defaultStr != "" {
				if parsed, err := strconv.ParseFloat(defaultStr, 64); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Float64(name, def, desc)

		case field.Type.Kind() == reflect.String:
			def := fieldValue.String()
			if defaultStr != "" {
				def = defaultStr
			}

			cmd.Flags().String(name, def, desc)

		case field.Type.Kind() == reflect.Bool:
			def := fieldValue.Bool()

			if defaultStr != "" {
				if parsed, err := strconv.ParseBool(defaultStr); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Bool(name, def, desc)

		case field.Type.Kind() == reflect.Int:
			if allowRanges {
				def := strconv.Itoa(int(fieldValue.Int()))
				if defaultStr != "" {
					def = defaultStr
				}

				cmd.Flags().String(name, def, desc)

				continue
			}

			def := int(fieldValue.Int())

			if defaultStr != "" {
				if parsed, err := strconv.Atoi(defaultStr); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Int(name, def, desc)

		default:
			log.Warn().
				Str("field", field.Name).
				Str("type", field.Type.String()).
				Msg("strategy field type not supported for CLI flags")
		}
	}
}

// applyStrategyFlags reads flag values from the command's parsed flags
// and sets them on the strategy struct's fields via reflection. It
// returns the kebab-case names of the fields it actually wrote to, so
// the caller can hand them to engine.WithUserParams. Marking these
// fields prevents hydrateFields from later overwriting an explicit
// zero value (e.g. --sector-cap 0) with the field's struct-tag default.
func applyStrategyFlags(cmd *cobra.Command, strategy engine.Strategy) []string {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return nil
	}

	var applied []string

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)

		fieldValue := val.Field(ii)
		if !fieldValue.CanSet() {
			continue
		}

		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			continue
		}

		set := false

		switch {
		case field.Type == assetType:
			raw := strings.TrimSpace(flag.Value.String())
			if raw == "" {
				continue
			}

			fieldValue.Set(reflect.ValueOf(asset.Asset{Ticker: strings.ToUpper(raw)}))

			set = true

		case field.Type.Implements(universeType):
			raw := flag.Value.String()
			if raw == "" {
				continue
			}

			tickers := strings.Split(raw, ",")
			for idx := range tickers {
				tickers[idx] = strings.ToUpper(strings.TrimSpace(tickers[idx]))
			}

			fieldValue.Set(reflect.ValueOf(universe.NewStatic(tickers...)))

			set = true

		case field.Type == durationType:
			if parsed, err := time.ParseDuration(flag.Value.String()); err == nil {
				fieldValue.SetInt(int64(parsed))

				set = true
			}

		case field.Type.Kind() == reflect.Float64:
			if parsed, err := strconv.ParseFloat(flag.Value.String(), 64); err == nil {
				fieldValue.SetFloat(parsed)

				set = true
			}

		case field.Type.Kind() == reflect.String:
			fieldValue.SetString(flag.Value.String())

			set = true

		case field.Type.Kind() == reflect.Bool:
			if parsed, err := strconv.ParseBool(flag.Value.String()); err == nil {
				fieldValue.SetBool(parsed)

				set = true
			}

		case field.Type.Kind() == reflect.Int:
			if parsed, err := strconv.Atoi(flag.Value.String()); err == nil {
				fieldValue.SetInt(int64(parsed))

				set = true
			}

		default:
			log.Warn().
				Str("field", field.Name).
				Str("type", field.Type.String()).
				Msg("strategy field type not supported by applyStrategyFlags")
		}

		if set {
			applied = append(applied, name)
		}
	}

	return applied
}

// parseRangeFlag checks whether value contains min:max:step range syntax.
// If the value parses as three colon-separated floats, it returns a
// study.ParamSweep for the named field and true. Otherwise it returns
// the zero value and false.
func parseRangeFlag(field, value string) (study.ParamSweep, bool) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 {
		return study.ParamSweep{}, false
	}

	minVal, err1 := strconv.ParseFloat(parts[0], 64)
	maxVal, err2 := strconv.ParseFloat(parts[1], 64)
	stepVal, err3 := strconv.ParseFloat(parts[2], 64)

	if err1 != nil || err2 != nil || err3 != nil {
		return study.ParamSweep{}, false
	}

	return study.SweepRange(field, minVal, maxVal, stepVal), true
}

// strategyFlagFieldNames returns the kebab-case names of every strategy
// field that has a registered cobra flag and is not test-only. The list
// is intended for engine.WithUserParams in subcommands that apply CLI
// flags to per-run strategy instances (e.g. study stress-test): it lets
// hydrateFields know not to re-default these fields, so an explicit
// zero passed on the command line survives.
func strategyFlagFieldNames(cmd *cobra.Command, strategy engine.Strategy) []string {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return nil
	}

	var names []string

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)
		if cmd.Flags().Lookup(name) != nil {
			names = append(names, name)
		}
	}

	return names
}

// collectFixedParams walks the strategy's exported fields and returns a
// map of name -> string value for every flag the user explicitly set
// that is *not* part of a sweep. The returned map is intended to be
// passed as base parameters to a parameter optimizer so non-swept
// flags propagate to every backtest. Asset and universe fields are
// excluded because they require engine-side resolution that base
// params do not perform.
func collectFixedParams(cmd *cobra.Command, strategy engine.Strategy, sweeps []study.ParamSweep) map[string]string {
	swept := make(map[string]struct{}, len(sweeps))

	for _, sw := range sweeps {
		if !sw.IsPreset() {
			swept[sw.Field()] = struct{}{}
		}
	}

	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]string)

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		if field.Type == assetType || field.Type.Implements(universeType) {
			continue
		}

		name := engine.ParameterName(field)
		if _, isSwept := swept[name]; isSwept {
			continue
		}

		fl := cmd.Flags().Lookup(name)
		if fl == nil || !fl.Changed {
			continue
		}

		result[name] = fl.Value.String()
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// collectParamSweeps walks the strategy's exported fields, reads each
// corresponding flag's string value, and returns the subset that use
// range syntax (min:max:step).
func collectParamSweeps(cmd *cobra.Command, strategy engine.Strategy) []study.ParamSweep {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Pointer {
		val = val.Elem()
	}

	strategyType := val.Type()
	if strategyType.Kind() != reflect.Struct {
		return nil
	}

	var sweeps []study.ParamSweep

	for ii := 0; ii < strategyType.NumField(); ii++ {
		field := strategyType.Field(ii)
		if !field.IsExported() {
			continue
		}

		if engine.IsTestOnlyField(field) {
			continue
		}

		name := engine.ParameterName(field)

		fl := cmd.Flags().Lookup(name)
		if fl == nil {
			continue
		}

		if sweep, ok := parseRangeFlag(name, fl.Value.String()); ok {
			sweeps = append(sweeps, sweep)
		}
	}

	return sweeps
}
