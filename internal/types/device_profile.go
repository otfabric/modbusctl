package types

import (
	"encoding/binary"
	"math"
)

type Device struct {
	IP       string `json:"ip"`
	Port     uint16 `json:"port"`
	Unit     uint8  `json:"unit"`
	Function uint8  `json:"function"`
}

type RegisterDefinition struct {
	ControlledPropertyId string  `json:"controlledPropertyId"`
	ValueScaleFactor     float64 `json:"valueScaleFactor"`
	Start                uint16  `json:"start"`
	Size                 uint16  `json:"size"`
	Format               string  `json:"format"`
}

type DeviceProfile struct {
	ProtocolData struct {
		Registers []RegisterDefinition `json:"registers"`
	} `json:"protocolData"`
}

type RegisterDecoder struct {
	Format string
	Size   int
	Decode func([]byte) (float64, error)
}

var RegisterDecoders = map[string]RegisterDecoder{
	">h": {">h", 2, func(b []byte) (float64, error) { return float64(int16(binary.BigEndian.Uint16(b))), nil }},
	"<h": {"<h", 2, func(b []byte) (float64, error) { return float64(int16(binary.LittleEndian.Uint16(b))), nil }},
	">H": {">H", 2, func(b []byte) (float64, error) { return float64(binary.BigEndian.Uint16(b)), nil }},
	"<H": {"<H", 2, func(b []byte) (float64, error) { return float64(binary.LittleEndian.Uint16(b)), nil }},
	">i": {">i", 4, func(b []byte) (float64, error) { return float64(int32(binary.BigEndian.Uint32(b))), nil }},
	"<i": {"<i", 4, func(b []byte) (float64, error) { return float64(int32(binary.LittleEndian.Uint32(b))), nil }},
	">I": {">I", 4, func(b []byte) (float64, error) { return float64(binary.BigEndian.Uint32(b)), nil }},
	"<I": {"<I", 4, func(b []byte) (float64, error) { return float64(binary.LittleEndian.Uint32(b)), nil }},
	">q": {">q", 8, func(b []byte) (float64, error) { return float64(int64(binary.BigEndian.Uint64(b))), nil }},
	"<q": {"<q", 8, func(b []byte) (float64, error) { return float64(int64(binary.LittleEndian.Uint64(b))), nil }},
	">Q": {">Q", 8, func(b []byte) (float64, error) { return float64(binary.BigEndian.Uint64(b)), nil }},
	"<Q": {"<Q", 8, func(b []byte) (float64, error) { return float64(binary.LittleEndian.Uint64(b)), nil }},
	">f": {">f", 4, func(b []byte) (float64, error) { return float64(math.Float32frombits(binary.BigEndian.Uint32(b))), nil }},
	"<f": {"<f", 4, func(b []byte) (float64, error) {
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(b))), nil
	}},
	">d": {">d", 8, func(b []byte) (float64, error) { return math.Float64frombits(binary.BigEndian.Uint64(b)), nil }},
	"<d": {"<d", 8, func(b []byte) (float64, error) { return math.Float64frombits(binary.LittleEndian.Uint64(b)), nil }},
	">B": {">B", 1, func(b []byte) (float64, error) { return float64(b[0]), nil }},
}
