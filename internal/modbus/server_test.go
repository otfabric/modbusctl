package modbus

import (
	"testing"

	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/require"
)

func TestWriteRegistersToStore_FC1PackedBits(t *testing.T) {
	s := &MemoryStore{
		coils:     make(map[uint16][]byte),
		registers: make(map[uint16][]byte),
		discrete:  make(map[uint16][]byte),
	}
	// Three coils at 0..2: LSB-first in first byte => 0b101 = 5
	rec := types.CaptureRecord{StartAddress: 0, RegisterCount: 3, Data: []byte{5}}
	require.NoError(t, writeRegistersToStore(s, rec, 1))
	data, err := s.get(1, 0, 3)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 0, 1}, data)
}

func TestWriteRegistersToStore_FC2PackedBits(t *testing.T) {
	s := &MemoryStore{
		coils:     make(map[uint16][]byte),
		registers: make(map[uint16][]byte),
		discrete:  make(map[uint16][]byte),
	}
	rec := types.CaptureRecord{StartAddress: 10, RegisterCount: 3, Data: []byte{5}}
	require.NoError(t, writeRegistersToStore(s, rec, 2))
	data, err := s.get(2, 10, 3)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 0, 1}, data)
	_, err = s.get(1, 10, 1)
	require.Error(t, err)
}
