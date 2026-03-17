package cli

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/penny-vault/pvbt/universe"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var universeType = reflect.TypeOf((*universe.Universe)(nil)).Elem()

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

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = toKebabCase(field.Name)
		}

		desc := field.Tag.Get("desc")
		defaultStr := field.Tag.Get("default")
		fieldValue := val.Field(ii)

		// Handle universe.Universe interface fields as comma-separated ticker strings.
		if field.Type.Implements(universeType) {
			def := defaultStr
			cmd.Flags().String(name, def, desc)

			if f := cmd.Flags().Lookup(name); f != nil {
				if err := viper.BindPFlag(name, f); err != nil {
					log.Fatal().Err(err).Str("flag", name).Msg("failed to bind strategy flag")
				}
			}

			continue
		}

		switch field.Type.Kind() {
		case reflect.Float64:
			def := fieldValue.Float()

			if defaultStr != "" {
				if parsed, err := strconv.ParseFloat(defaultStr, 64); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Float64(name, def, desc)
		case reflect.String:
			def := fieldValue.String()
			if defaultStr != "" {
				def = defaultStr
			}

			cmd.Flags().String(name, def, desc)
		case reflect.Bool:
			def := fieldValue.Bool()

			if defaultStr != "" {
				if parsed, err := strconv.ParseBool(defaultStr); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Bool(name, def, desc)
		case reflect.Int:
			def := int(fieldValue.Int())

			if defaultStr != "" {
				if parsed, err := strconv.Atoi(defaultStr); err == nil {
					def = parsed
				}
			}

			cmd.Flags().Int(name, def, desc)
		default:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				def := time.Duration(fieldValue.Int())

				if defaultStr != "" {
					if parsed, err := time.ParseDuration(defaultStr); err == nil {
						def = parsed
					}
				}

				cmd.Flags().Duration(name, def, desc)
			}
		}

		if f := cmd.Flags().Lookup(name); f != nil {
			if err := viper.BindPFlag(name, f); err != nil {
				log.Fatal().Err(err).Str("flag", name).Msg("failed to bind strategy flag")
			}
		}
	}
}

// applyStrategyFlags reads flag values from viper and sets them on the
// strategy struct's fields via reflection.
func applyStrategyFlags(strategy engine.Strategy) {
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

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = toKebabCase(field.Name)
		}

		flagValue := val.Field(ii)
		if !flagValue.CanSet() {
			continue
		}

		// Universe fields are stored as comma-separated tickers. Create a
		// StaticUniverse without a data source; the engine's hydrateFields
		// will re-wire it with the proper data source later.
		if field.Type.Implements(universeType) {
			raw := viper.GetString(name)
			if raw == "" {
				continue
			}

			tickers := strings.Split(raw, ",")
			for idx := range tickers {
				tickers[idx] = strings.TrimSpace(tickers[idx])
			}

			flagValue.Set(reflect.ValueOf(universe.NewStatic(tickers...)))

			continue
		}

		switch field.Type.Kind() {
		case reflect.Float64:
			flagValue.SetFloat(viper.GetFloat64(name))
		case reflect.String:
			flagValue.SetString(viper.GetString(name))
		case reflect.Bool:
			flagValue.SetBool(viper.GetBool(name))
		case reflect.Int:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				flagValue.SetInt(int64(viper.GetDuration(name)))
			} else {
				flagValue.SetInt(int64(viper.GetInt(name)))
			}
		}
	}
}

// toKebabCase converts a PascalCase or camelCase name to kebab-case.
func toKebabCase(s string) string {
	var result strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('-')
		}

		result.WriteRune(r)
	}

	return strings.ToLower(result.String())
}
