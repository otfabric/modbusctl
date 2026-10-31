// SPDX-License-Identifier: MIT

package capfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/go-modbus-capture/jsonl"
	"github.com/otfabric/go-modbus-capture/mbcsv"
	"github.com/otfabric/modbusctl/internal/capfile"
	"github.com/otfabric/modbusctl/internal/capfile/capfiletest"
	"github.com/otfabric/modbusctl/internal/replay"
	"github.com/otfabric/modbusctl/internal/types"
)

// FuzzLoadAndServe feeds arbitrary capture files, which the server loads from
// wherever the user got them, through the path `server static` and `server
// replay` take: load, build a device per target, answer a read.
func FuzzLoadAndServe(f *testing.F) {
	dir := f.TempDir()
	h := types.CaptureHeader{IP: "10.0.0.1", Port: 502, Unit: 1, Function: 3}
	rec := types.CaptureRecord{Iteration: 1, StartAddress: 0, RegisterCount: 2, Data: []byte{0, 1, 0, 2}}
	c := capfiletest.Capture(h, rec)
	p := capfiletest.Write(f, filepath.Join(dir, "seed.mbcap"), c)
	b, err := os.ReadFile(p)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	var js, cs bytes.Buffer
	jw, err := jsonl.NewWriter(&js, c.Header)
	if err != nil {
		f.Fatal(err)
	}
	if err := c.WriteTo(jw); err != nil {
		f.Fatal(err)
	}
	f.Add(js.Bytes())
	cw, err := mbcsv.NewWriter(&cs, c.Header)
	if err != nil {
		f.Fatal(err)
	}
	if err := c.WriteTo(cw); err != nil {
		f.Fatal(err)
	}
	f.Add(cs.Bytes())
	f.Add([]byte("MCAP"))
	f.Add([]byte(`{"format":"mbcap"}`))
	f.Add([]byte("t,stream,req,resp\n"))
	f.Fuzz(func(t *testing.T, data []byte) {
		path := filepath.Join(t.TempDir(), "in")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		l, err := capfile.Load(path)
		if err != nil {
			if l != nil {
				t.Fatal("Load returned a capture and an error")
			}
			return
		}
		end := l.End()
		for _, tg := range l.Targets {
			d, err := replay.Build(l.Capture, tg.ID, end, replay.Options{})
			if err != nil {
				continue
			}
			d.Values(end, 3, 0, 2)
			d.Read(end, []byte{3, 0, 0, 0, 2})
		}
		// The same bytes through the reader alone must not disagree on the
		// encoding: detection does not depend on the file name.
		if _, enc, err := capfile.OpenReader(bytes.NewReader(data)); err == nil && enc != l.Encoding {
			t.Fatalf("encoding %s, then %s", l.Encoding, enc)
		}
	})
}
