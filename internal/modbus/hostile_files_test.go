// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/fsutil"
	"github.com/otfabric/modbusctl/internal/mbtest"
	"github.com/otfabric/modbusctl/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecord_outputNamingTheBlocksFile: --output naming the blocks file replaced the
// plan with the capture.
func TestRecord_outputNamingTheBlocksFile(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	blocks := coreBlocksFile(t, types.AddressBlock{StartAddress: 0, RegisterCount: 1})
	before, err := os.ReadFile(blocks)
	require.NoError(t, err)
	_, err = RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, Duration: 10,
		BlocksFile: blocks, OutputFile: blocks,
	}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInvalidFlagCombination)
	after, err := os.ReadFile(blocks)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Zero(t, dev.RequestCount(0), "nothing was sent")
}

// TestRecord_blocksFileTooLarge: a blocks file above fsutil.MaxJSONInput is refused
// without reading it whole.
func TestRecord_blocksFileTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blocks.json")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	require.NoError(t, os.Truncate(path, fsutil.MaxJSONInput+1))
	_, err := RecordToCapture(context.Background(), config.RecordConfig{
		DeviceConfig: config.DeviceConfig{URL: coreClosedURL(t)}, Timeout: coreShortTimeoutMS, Function: 3, Duration: 10,
		BlocksFile: path, OutputFile: coreCapturePath(t),
	}, nil)
	coreRequireKind(t, err, errs.KindInvalidInput, errs.CodeInputFileOpenFailed)
	require.ErrorIs(t, err, fsutil.ErrInputTooLarge)
}

// TestCaptures_ownerOnlyAndNeverThroughSymlinks: captures were created 0644 (before
// umask) and written through a symbolic link at the output path.
func TestCaptures_ownerOnlyAndNeverThroughSymlinks(t *testing.T) {
	dev := coreScanDevice(coreRange{0, 3})
	dir := t.TempDir()
	out := filepath.Join(dir, "read.mbcap")
	_, err := CollectRead(context.Background(), config.ReadConfig{DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: out}, nil)
	require.NoError(t, err)
	st, err := os.Stat(out)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(fsutil.OutputMode), st.Mode().Perm())
	}

	victim := filepath.Join(dir, "victim")
	require.NoError(t, os.WriteFile(victim, []byte("keep"), 0o600))
	link := filepath.Join(dir, "link.mbcap")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	_, err = CollectRead(context.Background(), config.ReadConfig{DeviceConfig: coreTCP(t, dev), Timeout: coreTimeoutMS, Function: 3, RegisterCount: 1, OutputFile: link}, nil)
	coreRequireKind(t, err, errs.KindOutput, errs.CodeOutputFileCreateFailed)
	require.ErrorIs(t, err, fsutil.ErrOutputLink)
	b, _ := os.ReadFile(victim)
	assert.Equal(t, "keep", string(b))
}

// TestDiscover_outputFileIsOwnerOnly: the discovery result was written 0644.
func TestDiscover_outputFileIsOwnerOnly(t *testing.T) {
	dev := mbtest.NewDevice().SetHolding(0, 1)
	port := collPort(t, dev.ServeTCP(t))
	path := filepath.Join(t.TempDir(), "devices.json")
	_, err := CollectDiscover(context.Background(), config.DiscoverConfig{Subnets: []string{collLoopback}, Port: port, Parallel: 1, OutputFile: path}, nil)
	require.NoError(t, err)
	st, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(fsutil.OutputMode), st.Mode().Perm())
	}
}
