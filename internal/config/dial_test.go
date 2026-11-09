// SPDX-License-Identifier: MIT

package config

import (
	"strings"
	"testing"
)

func TestDialConfig_fromEnv(t *testing.T) {
	t.Setenv("MODBUSCTL_PROXY", "socks5h://127.0.0.1:1080")
	t.Setenv("MODBUSCTL_SOURCE_ADDR", "127.0.0.1")
	var cfg ReadConfig
	if err := LoadFromEnv(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Proxy != "socks5h://127.0.0.1:1080" || cfg.SourceAddr != "127.0.0.1" {
		t.Fatalf("env not loaded: %+v", cfg.DialConfig)
	}
	var disc DiscoverConfig
	if err := LoadFromEnv(&disc); err != nil {
		t.Fatal(err)
	}
	if !disc.IsSet() {
		t.Fatal("discover does not take MODBUSCTL_PROXY")
	}
}

func TestDialConfig_Dialer(t *testing.T) {
	if d, err := (DialConfig{}).Dialer(); d != nil || err != nil {
		t.Fatalf("unset: %v, %v; want nil, nil", d != nil, err)
	}
	if d, err := (DialConfig{Proxy: " socks5://127.0.0.1:1080 ", SourceAddr: " 127.0.0.1 "}).Dialer(); d == nil || err != nil {
		t.Fatalf("set: %v, %v", d != nil, err)
	}
	if _, err := (DialConfig{SourceAddr: "x"}).Dialer(); err == nil || !strings.Contains(err.Error(), "--source-addr") {
		t.Fatalf("invalid: %v", err)
	}
}

func TestValidateDialConfig_ipPortIsTCP(t *testing.T) {
	if err := ValidateDialConfig("", DialConfig{Proxy: "socks5://127.0.0.1:1080"}); err != nil {
		t.Fatalf("--ip/--port with --proxy: %v", err)
	}
	if err := ValidateDialConfig("rtu:///dev/x", DialConfig{}); err != nil {
		t.Fatalf("unset flags on rtu: %v", err)
	}
}
