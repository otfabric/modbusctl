package modbus

import (
	"encoding/binary"
	"strings"

	"github.com/otfabric/go-modbus/sunspec"
	"github.com/otfabric/modbusctl/internal/config"
)

type sunspecPhase int

const (
	sunspecDetectBase sunspecPhase = iota
	sunspecWalkModels
	sunspecDone
)

const sunspecDefaultMaxModels = 256

type sunspecStrategy struct {
	cfg scanSettings

	phase sunspecPhase

	// Phase 1: base detection
	bases     []uint16
	baseIndex int
	baseAddr  uint16

	// Phase 2: model-chain walk
	currentAddr uint16
	modelCount  int
	maxModels   int
	maxSpan     uint16

	// Body reading sub-phase (within walkModels)
	readingBody   bool
	bodyAddr      uint16
	bodyRemaining uint16
}

func newSunSpecStrategy(cfg scanSettings) *sunspecStrategy {
	return &sunspecStrategy{cfg: cfg}
}

func (s *sunspecStrategy) Name() string { return "sunspec" }

func (s *sunspecStrategy) Init(cfg scanSettings) {
	s.cfg = cfg

	var bases []uint16
	if strings.TrimSpace(cfg.SunSpecBases) != "" {
		var err error
		bases, err = config.ParseSunSpecBases(cfg.SunSpecBases)
		if err != nil {
			// Scan config with algo sunspec must pass [validate.CheckScanConfig] first.
			panic("modbusctl: invalid SunSpecBases after validation: " + err.Error())
		}
	}
	if len(bases) == 0 {
		// Copy default list so we don't mutate the library slice.
		bases = make([]uint16, len(sunspec.DefaultBaseAddresses))
		copy(bases, sunspec.DefaultBaseAddresses)
	}
	s.bases = bases

	// Max models: default 256 if not set.
	s.maxModels = cfg.SunSpecMaxModels
	if s.maxModels <= 0 {
		s.maxModels = sunspecDefaultMaxModels
	}
	s.maxSpan = cfg.SunSpecMaxSpan

	// If a known base is provided, skip detection.
	if cfg.SunSpecBase > 0 {
		s.phase = sunspecWalkModels
		s.baseAddr = cfg.SunSpecBase
		s.currentAddr = cfg.SunSpecBase + 2
		return
	}

	if len(s.bases) == 0 {
		s.phase = sunspecDone
		return
	}

	s.phase = sunspecDetectBase
	s.baseIndex = 0
}

func (s *sunspecStrategy) Next() (ScanTask, bool) {
	switch s.phase {
	case sunspecDetectBase:
		if s.baseIndex >= len(s.bases) {
			s.phase = sunspecDone
			return ScanTask{}, false
		}
		return ScanTask{Start: s.bases[s.baseIndex], Count: 2}, true

	case sunspecWalkModels:
		if s.readingBody {
			count := s.bodyRemaining
			if count > 125 {
				count = 125
			}
			return ScanTask{Start: s.bodyAddr, Count: count}, true
		}
		if s.modelCount >= s.maxModels {
			s.phase = sunspecDone
			return ScanTask{}, false
		}
		if s.maxSpan > 0 && s.currentAddr-s.baseAddr > s.maxSpan {
			s.phase = sunspecDone
			return ScanTask{}, false
		}
		return ScanTask{Start: s.currentAddr, Count: 2}, true

	default:
		return ScanTask{}, false
	}
}

func (s *sunspecStrategy) OnResult(task ScanTask, result ScanResult) {
	switch s.phase {
	case sunspecDetectBase:
		if result.Success && len(result.Data) >= 4 {
			r0 := binary.BigEndian.Uint16(result.Data[0:2])
			r1 := binary.BigEndian.Uint16(result.Data[2:4])
			if r0 == sunspec.MarkerReg0 && r1 == sunspec.MarkerReg1 {
				s.baseAddr = task.Start
				s.currentAddr = task.Start + 2
				s.phase = sunspecWalkModels
				return
			}
		}
		s.baseIndex++
		if s.baseIndex >= len(s.bases) {
			s.phase = sunspecDone
		}

	case sunspecWalkModels:
		if s.readingBody {
			if !result.Success {
				s.phase = sunspecDone
				return
			}
			s.bodyAddr += task.Count
			s.bodyRemaining -= task.Count
			if s.bodyRemaining == 0 {
				s.readingBody = false
			}
			return
		}

		if !result.Success || len(result.Data) < 4 {
			s.phase = sunspecDone
			return
		}
		id := binary.BigEndian.Uint16(result.Data[0:2])
		length := binary.BigEndian.Uint16(result.Data[2:4])

		if id == sunspec.EndModelID && length == sunspec.EndModelLength {
			// End model found — executor already wrote the successful read.
			s.phase = sunspecDone
			return
		}

		if length == 0 {
			// Malformed: length 0 with non-end model ID.
			s.phase = sunspecDone
			return
		}

		// Address overflow check.
		next := uint32(s.currentAddr) + 2 + uint32(length)
		if next > 65535 {
			s.phase = sunspecDone
			return
		}

		// Set up body reading, then advance to next header.
		s.readingBody = true
		s.bodyAddr = s.currentAddr + 2
		s.bodyRemaining = length
		s.currentAddr = uint16(next)
		s.modelCount++
	case sunspecDone:
		// Terminal phase; executor should not deliver further results.
	}
}

func (s *sunspecStrategy) Done() bool {
	return s.phase == sunspecDone
}
