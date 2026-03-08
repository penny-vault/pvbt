package cli

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/penny-vault/pvbt/engine"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// registerStrategyFlags inspects the strategy struct's exported fields
// and registers a cobra flag for each one. The flag name comes from
// the `pvbt` struct tag (falling back to the field name in kebab-case).
// The description comes from the `desc` tag. Default values come from
// the `default` tag or the field's current value.
func registerStrategyFlags(cmd *cobra.Command, strategy engine.Strategy) {
	v := reflect.ValueOf(strategy)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = toKebabCase(field.Name)
		}

		desc := field.Tag.Get("desc")
		defaultStr := field.Tag.Get("default")
		fv := v.Field(i)

		switch field.Type.Kind() {
		case reflect.Float64:
			def := fv.Float()
			if defaultStr != "" {
				if parsed, err := strconv.ParseFloat(defaultStr, 64); err == nil {
					def = parsed
				}
			}
			cmd.Flags().Float64(name, def, desc)
		case reflect.String:
			def := fv.String()
			if defaultStr != "" {
				def = defaultStr
			}
			cmd.Flags().String(name, def, desc)
		case reflect.Bool:
			def := fv.Bool()
			if defaultStr != "" {
				if parsed, err := strconv.ParseBool(defaultStr); err == nil {
					def = parsed
				}
			}
			cmd.Flags().Bool(name, def, desc)
		case reflect.Int:
			def := int(fv.Int())
			if defaultStr != "" {
				if parsed, err := strconv.Atoi(defaultStr); err == nil {
					def = parsed
				}
			}
			cmd.Flags().Int(name, def, desc)
		default:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				def := time.Duration(fv.Int())
				if defaultStr != "" {
					if parsed, err := time.ParseDuration(defaultStr); err == nil {
						def = parsed
					}
				}
				cmd.Flags().Duration(name, def, desc)
			}
		}

		if f := cmd.Flags().Lookup(name); f != nil {
			viper.BindPFlag(name, f)
		}
	}
}

// applyStrategyFlags reads flag values from viper and sets them on the
// strategy struct's fields via reflection.
func applyStrategyFlags(strategy engine.Strategy) {
	v := reflect.ValueOf(strategy)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	if t.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		name := field.Tag.Get("pvbt")
		if name == "" {
			name = toKebabCase(field.Name)
		}

		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}

		switch field.Type.Kind() {
		case reflect.Float64:
			fv.SetFloat(viper.GetFloat64(name))
		case reflect.String:
			fv.SetString(viper.GetString(name))
		case reflect.Bool:
			fv.SetBool(viper.GetBool(name))
		case reflect.Int:
			if field.Type == reflect.TypeOf(time.Duration(0)) {
				fv.SetInt(int64(viper.GetDuration(name)))
			} else {
				fv.SetInt(int64(viper.GetInt(name)))
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
