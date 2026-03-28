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
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return nil
	}
	return loadFromEnvStruct(v.Elem())
}

// MustLoadFromEnv calls [LoadFromEnv] and panics if any env value is invalid (startup safety).
func MustLoadFromEnv(cfg interface{}) {
	if err := LoadFromEnv(cfg); err != nil {
		panic(fmt.Sprintf("modbusctl: invalid environment: %v", err))
	}
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
		envValue := os.Getenv(tag)
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
