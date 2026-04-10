package cli

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/study"
	"github.com/penny-vault/pvbt/universe"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	universeType = reflect.TypeOf((*universe.Universe)(nil)).Elem()
	durationType = reflect.TypeOf(time.Duration(0))
)

// registerStrategyFlags inspects the strategy struct's exported fields
// and registers a cobra flag for each one. The flag name comes from
// the `pvbt` struct tag (falling back to the field name in kebab-case).
// The description comes from the `desc` tag. Default values come from
// the `default` tag or the field's current value.
func registerStrategyFlags(cmd *cobra.Command, strategy engine.Strategy) {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Ptr {
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

		name := engine.ParameterName(field)

		desc := field.Tag.Get("desc")
		defaultStr := field.Tag.Get("default")
		fieldValue := val.Field(ii)

		switch {
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
// and sets them on the strategy struct's fields via reflection.
func applyStrategyFlags(cmd *cobra.Command, strategy engine.Strategy) {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Ptr {
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

		name := engine.ParameterName(field)

		fieldValue := val.Field(ii)
		if !fieldValue.CanSet() {
			continue
		}

		flag := cmd.Flags().Lookup(name)
		if flag == nil {
			continue
		}

		switch {
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

		case field.Type == durationType:
			if parsed, err := time.ParseDuration(flag.Value.String()); err == nil {
				fieldValue.SetInt(int64(parsed))
			}

		case field.Type.Kind() == reflect.Float64:
			if parsed, err := strconv.ParseFloat(flag.Value.String(), 64); err == nil {
				fieldValue.SetFloat(parsed)
			}

		case field.Type.Kind() == reflect.String:
			fieldValue.SetString(flag.Value.String())

		case field.Type.Kind() == reflect.Bool:
			if parsed, err := strconv.ParseBool(flag.Value.String()); err == nil {
				fieldValue.SetBool(parsed)
			}

		case field.Type.Kind() == reflect.Int:
			if parsed, err := strconv.Atoi(flag.Value.String()); err == nil {
				fieldValue.SetInt(int64(parsed))
			}

		default:
			log.Warn().
				Str("field", field.Name).
				Str("type", field.Type.String()).
				Msg("strategy field type not supported by applyStrategyFlags")
		}
	}
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

// collectParamSweeps walks the strategy's exported fields, reads each
// corresponding flag's string value, and returns the subset that use
// range syntax (min:max:step).
func collectParamSweeps(cmd *cobra.Command, strategy engine.Strategy) []study.ParamSweep {
	val := reflect.ValueOf(strategy)
	if val.Kind() == reflect.Ptr {
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
