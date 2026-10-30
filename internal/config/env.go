package config

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// LoadFromEnv populates tagged fields from the process environment.
// Malformed numeric or boolean values produce errors (joined when multiple vars fail).
func LoadFromEnv(cfg interface{}) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	return loadFromEnvStruct(v.Elem())
}

// envErr is the first error of [DeferLoadFromEnv], reported by [EnvError].
var envErr error

// DeferLoadFromEnv calls [LoadFromEnv] and keeps the first error for [EnvError]. Commands
// are wired in package init, before anything can render a fatal error, so that a malformed
// variable must not end the process here.
func DeferLoadFromEnv(cfg interface{}) {
	if err := LoadFromEnv(cfg); err != nil && envErr == nil {
		envErr = err
	}
}

// EnvError returns the first malformed environment variable [DeferLoadFromEnv] met, or nil.
func EnvError() error {
	return envErr
}

// legacyEnv maps a canonical environment variable to its older spelling, which stays
// accepted (ADR-0006 of otfabric/engineering).
var legacyEnv = map[string]string{"MODBUSCTL_FORMAT": "MODBUSCTL_OUTPUT_FORMAT"}

// Getenv returns the value of the environment variable name. The older spelling is read
// when name is empty; the canonical name wins when both are set.
func Getenv(name string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	if old, ok := legacyEnv[name]; ok {
		return os.Getenv(old)
	}
	return ""
}

func loadFromEnvStruct(v reflect.Value) error {
	t := v.Type()
	var errs []error
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		sf := t.Field(i)
		if sf.Anonymous && field.Kind() == reflect.Struct {
			if err := loadFromEnvStruct(field); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if !field.CanSet() {
			continue
		}
		tag := sf.Tag.Get("env")
		if tag == "" {
			continue
		}
		envValue := Getenv(tag)
		if envValue == "" {
			continue
		}
		switch field.Kind() {
		case reflect.String:
			field.SetString(envValue)
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint:
			val, err := strconv.ParseUint(envValue, 10, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", tag, envValue, err))
				continue
			}
			if field.OverflowUint(val) {
				errs = append(errs, fmt.Errorf("%s=%q: value overflows %s", tag, envValue, field.Kind()))
				continue
			}
			field.SetUint(val)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val, err := strconv.ParseInt(envValue, 10, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", tag, envValue, err))
				continue
			}
			if field.OverflowInt(val) {
				errs = append(errs, fmt.Errorf("%s=%q: value overflows %s", tag, envValue, field.Kind()))
				continue
			}
			field.SetInt(val)
		case reflect.Bool:
			val, err := strconv.ParseBool(envValue)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s=%q: %w", tag, envValue, err))
				continue
			}
			field.SetBool(val)
		case reflect.Slice:
			if field.Type().Elem().Kind() != reflect.String {
				continue
			}
			parts := strings.Split(envValue, ",")
			for j := range parts {
				parts[j] = strings.TrimSpace(parts[j])
			}
			field.Set(reflect.ValueOf(parts))
		default:
			// Tagged env on unsupported kind — skip (same as RegisterFlags / struct design).
		}
	}
	return errors.Join(errs...)
}
