// SPDX-License-Identifier: MIT

package types_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterDecoders(t *testing.T) {
	seq := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	neg := []byte{0xFF, 0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	tests := []struct {
		format string
		size   int
		in     []byte
		want   float64
	}{
		{">h", 2, neg, -2},
		{"<h", 2, neg, -257},
		{">H", 2, neg, 65534},
		{"<H", 2, neg, 65279},
		{">i", 4, neg, -65537},
		{"<i", 4, neg, -257},
		{">I", 4, seq, 0x01020304},
		{"<I", 4, seq, 0x04030201},
		{">q", 8, neg, -0x0001000000000001},
		{"<q", 8, neg, -257},
		{">Q", 8, seq, 0x0102030405060708},
		{"<Q", 8, seq, 0x0807060504030201},
		{">f", 4, []byte{0x3F, 0xC0, 0x00, 0x00}, 1.5},
		{"<f", 4, []byte{0x00, 0x00, 0xC0, 0x3F}, 1.5},
		{">d", 8, []byte{0xC0, 0x04, 0, 0, 0, 0, 0, 0}, -2.5},
		{"<d", 8, []byte{0, 0, 0, 0, 0, 0, 0x04, 0xC0}, -2.5},
		{">B", 1, []byte{0xFE}, 254},
	}
	require.Len(t, types.RegisterDecoders, len(tests), "every decoder must be covered")
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			d, ok := types.RegisterDecoders[tt.format]
			require.True(t, ok)
			assert.Equal(t, tt.format, d.Format)
			assert.Equal(t, tt.size, d.Size)
			got, err := d.Decode(tt.in[:tt.size])
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRegisterDecoders_floatSpecials(t *testing.T) {
	got, err := types.RegisterDecoders[">f"].Decode([]byte{0x7F, 0xC0, 0x00, 0x00})
	require.NoError(t, err)
	assert.True(t, math.IsNaN(got))
	got, err = types.RegisterDecoders[">d"].Decode([]byte{0x7F, 0xF0, 0, 0, 0, 0, 0, 0})
	require.NoError(t, err)
	assert.True(t, math.IsInf(got, 1))
}

func TestDeviceProfile_json(t *testing.T) {
	var p types.DeviceProfile
	require.NoError(t, json.Unmarshal([]byte(`{"protocolData":{"registers":[
		{"controlledPropertyId":"power","valueScaleFactor":0.1,"start":100,"size":2,"format":">i"}]}}`), &p))
	assert.Equal(t, []types.RegisterDefinition{
		{ControlledPropertyId: "power", ValueScaleFactor: 0.1, Start: 100, Size: 2, Format: ">i"},
	}, p.ProtocolData.Registers)
}
