// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/socks5test"
)

func wantInvalidInput(t *testing.T, what string, err error) {
	t.Helper()
	e, ok := errs.As(err)
	if !ok || e.Kind != errs.KindInvalidInput {
		t.Fatalf("%s: %v, want invalid input", what, err)
	}
}

// Invalid --proxy or --source-addr values are invalid input in every client path, before
// anything is dialled, also when a caller skips the CLI validation.
func TestDialConfigErrorsBeforeDial(t *testing.T) {
	for _, bad := range []config.DialConfig{{Proxy: "http://127.0.0.1:1"}, {SourceAddr: "eth0"}} {
		_, err := buildDeviceClientConfig("tcp://127.0.0.1:1", 100, false, config.SerialConfig{}, config.TLSConfig{}, bad)
		wantInvalidInput(t, "buildDeviceClientConfig", err)
		_, err = openDeviceLink("tcp://127.0.0.1:1", 100, false, config.SerialConfig{}, config.TLSConfig{}, bad)
		wantInvalidInput(t, "openDeviceLink", err)
		_, err = openRecordingLink("tcp://127.0.0.1:1", 100, false, config.SerialConfig{}, config.TLSConfig{}, bad, 1)
		wantInvalidInput(t, "openRecordingLink", err)
		_, err = CollectDiscover(context.Background(), config.DiscoverConfig{
			DialConfig: bad, Subnets: []string{"127.0.0.1/32"}, Port: 1, Parallel: 1,
		}, nil)
		wantInvalidInput(t, "CollectDiscover", err)
	}
}

func TestBuildDeviceClientConfigDialer(t *testing.T) {
	conf, err := buildDeviceClientConfig("tcp://127.0.0.1:1", 100, false, config.SerialConfig{}, config.TLSConfig{}, config.DialConfig{})
	if err != nil || conf.Dialer != nil {
		t.Fatalf("unset flags: dialer set %v, %v", conf.Dialer != nil, err)
	}
	conf, err = buildDeviceClientConfig("tcp://127.0.0.1:1", 100, false, config.SerialConfig{}, config.TLSConfig{}, config.DialConfig{SourceAddr: "127.0.0.1"})
	if err != nil || conf.Dialer == nil {
		t.Fatalf("--source-addr: dialer set %v, %v", conf.Dialer != nil, err)
	}
}

// CollectDiscover dials every host through the dialer of the flags.
func TestCollectDiscoverThroughProxy(t *testing.T) {
	d := mbtest.NewDevice().SetHolding(0, 1)
	addr := d.ServeTCP(t)
	px := socks5test.Start(t)
	px.Forward = func(string) string { return addr }
	out, err := CollectDiscover(context.Background(), config.DiscoverConfig{
		DialConfig: config.DialConfig{Proxy: px.URL()}, Subnets: []string{"192.0.2.1/32"}, Port: 502, Parallel: 1,
	}, nil)
	if err != nil || len(out.Devices) != 1 || out.Devices[0].IP != "192.0.2.1" {
		t.Fatalf("discover through proxy: %+v, %v", out, err)
	}
	if len(px.Requests()) != 1 {
		t.Fatalf("proxy saw %d requests", len(px.Requests()))
	}
}
