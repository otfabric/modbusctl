package config

import "testing"

func TestScanAlgorithmForExecution(t *testing.T) {
	t.Parallel()
	if got := ScanAlgorithmForExecution(nil); got != ScanAlgoSafe {
		t.Fatalf("nil: %q", got)
	}
	cfg := &ScanConfig{Algo: "  SMART "}
	if got := ScanAlgorithmForExecution(cfg); got != ScanAlgoSmart {
		t.Fatalf("trim lower: %q", got)
	}
	empty := &ScanConfig{Algo: ""}
	if got := ScanAlgorithmForExecution(empty); got != ScanAlgoSafe {
		t.Fatalf("empty algo: %q", got)
	}
	norm := &ScanConfig{Algo: "ignored", NormalizedAlgo: ScanAlgoDeep}
	if got := ScanAlgorithmForExecution(norm); got != ScanAlgoDeep {
		t.Fatalf("normalized wins: %q", got)
	}
}

func TestSunSpecModbusURL(t *testing.T) {
	t.Parallel()
	u := &SunSpecBaseConfig{URL: "  tcp://10.0.0.1:502  "}
	if got := SunSpecModbusURL(u); got != "tcp://10.0.0.1:502" {
		t.Fatalf("url: %q", got)
	}
	ip := &SunSpecBaseConfig{IP: "192.168.0.2", Port: 1502}
	if got := SunSpecModbusURL(ip); got != "tcp://192.168.0.2:1502" {
		t.Fatalf("ip:port: %q", got)
	}
}
