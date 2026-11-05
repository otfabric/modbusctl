// SPDX-License-Identifier: MIT

package analysis

import (
	"bytes"
	"os"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/fsutil"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportDeviceProfileDecode_registerPastTheAddressSpace: without the profile
// check in front, a register at 65535 of size 2 read registers 65535 and 0.
func TestExportDeviceProfileDecode_registerPastTheAddressSpace(t *testing.T) {
	capture := writeCapture(t,
		types.CaptureRecord{Iteration: 1, StartAddress: 65535, RegisterCount: 1, Data: []byte{0x12, 0x34}},
		types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 1, Data: []byte{0x56, 0x78}})
	profile := writeBytes(t, "wrap.json", []byte(`{"protocolData":{"registers":[
		{"controlledPropertyId":"top\u001b[31m","valueScaleFactor":1,"start":65535,"size":2,"format":">I"}]}}`))
	var buf bytes.Buffer
	require.NoError(t, ExportDeviceProfileDecode(&buf, capture, profile, config.CaptureSelection{}))
	assert.Contains(t, buf.String(), `Missing data for top\x1b[31m at 65535 (size 2)`)
	assert.NotContains(t, buf.String(), "\x1b")
}

func TestExportDeviceProfileDecode_profileTooLarge(t *testing.T) {
	capture := writeCapture(t, types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 1, Data: []byte{0, 1}})
	big := writeBytes(t, "big.json", nil)
	require.NoError(t, os.Truncate(big, fsutil.MaxJSONInput+1))
	err := ExportDeviceProfileDecode(&bytes.Buffer{}, capture, big, config.CaptureSelection{})
	require.ErrorIs(t, err, fsutil.ErrInputTooLarge)
}
