// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"errors"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
)

// Unusable TLS files on a tcp+tls:// URL are invalid input in every client path,
// reported before anything is dialled, also when a caller skips the CLI validation.
func TestTLSConfigErrorsBeforeDial(t *testing.T) {
	const url = "tcp+tls://127.0.0.1:1"
	bad := config.TLSConfig{TLSCert: "/no/such/cert.pem"}
	ctx := context.Background()

	wantInvalid := func(what string, err error) {
		t.Helper()
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeInvalidInput {
			t.Errorf("%s: %v, want an invalid_input error", what, err)
		}
	}

	_, err := openDeviceLink(url, 100, false, config.SerialConfig{}, bad)
	wantInvalid("openDeviceLink", err)
	_, err = openRecordingLink(url, 100, false, config.SerialConfig{}, bad, 1)
	wantInvalid("openRecordingLink", err)

	var fp config.FingerprintConfig
	fp.URL, fp.UnitID, fp.TLSConfig = url, "1", bad
	_, err = CollectFingerprint(ctx, fp)
	wantInvalid("CollectFingerprint", err)

	var id config.IdentifyConfig
	id.URL, id.UnitID, id.TLSConfig = url, "1", bad
	_, err = CollectDeviceIdentification(ctx, id)
	wantInvalid("CollectDeviceIdentification", err)

	var rs config.ReportServerIDConfig
	rs.URL, rs.UnitID, rs.TLSConfig = url, "1", bad
	_, err = CollectReportServerID(ctx, rs)
	wantInvalid("CollectReportServerID", err)

	ss := &config.SunSpecBaseConfig{URL: url, TLSConfig: bad}
	_, _, err = openSunSpecModbusClient(url, ss)
	wantInvalid("openSunSpecModbusClient", err)
}
