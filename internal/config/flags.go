package config

import (
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
)

// RegisterFlags binds cobra flags to tagged fields on cfg (pointer to struct).
// Numeric and bool fields use reflect.Value.Addr().Interface().(*T) with the matching
// pflag *Var function — no unsafe.Pointer casts. Supported kinds: string, bool, []string,
// uint8–uint64, int, int8–int64, and uint.
func RegisterFlags(cmd *cobra.Command, cfg interface{}) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		flagName := field.Tag.Get("flag")
		desc := field.Tag.Get("desc")
		envVar := field.Tag.Get("env")

		if field.Anonymous {
			RegisterFlags(cmd, v.Field(i).Addr().Interface())
			continue
		}

		if flagName == "" {
			continue
		}

		if envVar != "" {
			desc = fmt.Sprintf("%s [env: %s]", desc, envVar)
		}

		fv := v.Field(i)
		switch field.Type.Kind() {
		case reflect.String:
			cmd.Flags().StringVar(fv.Addr().Interface().(*string), flagName, fv.String(), desc)
		case reflect.Uint8:
			cmd.Flags().Uint8Var(fv.Addr().Interface().(*uint8), flagName, uint8(fv.Uint()), desc)
		case reflect.Uint16:
			cmd.Flags().Uint16Var(fv.Addr().Interface().(*uint16), flagName, uint16(fv.Uint()), desc)
		case reflect.Uint32:
			cmd.Flags().Uint32Var(fv.Addr().Interface().(*uint32), flagName, uint32(fv.Uint()), desc)
		case reflect.Uint64:
			cmd.Flags().Uint64Var(fv.Addr().Interface().(*uint64), flagName, fv.Uint(), desc)
		case reflect.Uint:
			cmd.Flags().UintVar(fv.Addr().Interface().(*uint), flagName, uint(fv.Uint()), desc)
		case reflect.Int:
			cmd.Flags().IntVar(fv.Addr().Interface().(*int), flagName, int(fv.Int()), desc)
		case reflect.Int8:
			cmd.Flags().Int8Var(fv.Addr().Interface().(*int8), flagName, int8(fv.Int()), desc)
		case reflect.Int16:
			cmd.Flags().Int16Var(fv.Addr().Interface().(*int16), flagName, int16(fv.Int()), desc)
		case reflect.Int32:
			cmd.Flags().Int32Var(fv.Addr().Interface().(*int32), flagName, int32(fv.Int()), desc)
		case reflect.Int64:
			cmd.Flags().Int64Var(fv.Addr().Interface().(*int64), flagName, fv.Int(), desc)
		case reflect.Bool:
			cmd.Flags().BoolVar(fv.Addr().Interface().(*bool), flagName, fv.Bool(), desc)
		case reflect.Slice:
			if field.Type.Elem().Kind() != reflect.String {
				panic(fmt.Sprintf("config.RegisterFlags: field %s: flag %q only supports []string slices", field.Name, flagName))
			}
			var cur []string
			if !fv.IsNil() {
				cur = fv.Interface().([]string)
			}
			cmd.Flags().StringSliceVar(fv.Addr().Interface().(*[]string), flagName, cur, desc)
		default:
			panic(fmt.Sprintf("config.RegisterFlags: field %s (%s) has flag %q but unsupported reflect.Kind %s — add a case or remove the flag tag",
				field.Name, field.Type.String(), flagName, field.Type.Kind()))
		}
	}
}
