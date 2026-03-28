package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestCheckIdentifyConfig(t *testing.T) {
	validCfg := config.IdentifyConfig{
		UnitClientConfig: config.UnitClientConfig{IP: "192.168.1.1", Port: 502, UnitID: "1", Timeout: 1000, Parallel: 10},
	}
	assert.NoError(t, CheckIdentifyConfig(validCfg))
	ipv6 := validCfg
	ipv6.IP = "::1"
	assert.NoError(t, CheckIdentifyConfig(ipv6))
	ipv6.IP = "2001:db8::1"
	ipv6.Port = 1502
	assert.NoError(t, CheckIdentifyConfig(ipv6))
	validCfg.UnitID = "all"
	assert.NoError(t, CheckIdentifyConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.IP = "invalid"
	assert.Error(t, CheckIdentifyConfig(invalidCfg))
	invalidCfg = validCfg
	invalidCfg.UnitID = "999"
	assert.Error(t, CheckIdentifyConfig(invalidCfg))
	invalidCfg.UnitID = "abc"
	assert.Error(t, CheckIdentifyConfig(invalidCfg))
	invalidCfg.UnitID = "1,,3"
	assert.Error(t, CheckIdentifyConfig(invalidCfg))
	invalidCfg = validCfg
	invalidCfg.Timeout = 0
	assert.NoError(t, CheckIdentifyConfig(invalidCfg))
	invalidCfg.Timeout = 100
	assert.Error(t, CheckIdentifyConfig(invalidCfg))
}

func TestCheckFingerprintConfig(t *testing.T) {
	validCfg := config.FingerprintConfig{
		IP: "10.0.0.1", Port: 502, UnitID: "1", Timeout: 2000, Interval: 0,
	}
	assert.NoError(t, CheckFingerprintConfig(validCfg))
	validCfg.UnitID = "1-10"
	assert.NoError(t, CheckFingerprintConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.UnitID = ""
	assert.Error(t, CheckFingerprintConfig(invalidCfg))
	invalidCfg = validCfg
	invalidCfg.IP = "invalid"
	assert.Error(t, CheckFingerprintConfig(invalidCfg))
	invalidCfg = validCfg
	invalidCfg.Timeout = 100
	assert.Error(t, CheckFingerprintConfig(invalidCfg))
}

func TestCheckReadConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "valid_output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	validCfg := config.ReadConfig{
		DeviceConfig:  config.DeviceConfig{IP: "192.168.1.1", Port: 502, Unit: 1},
		Function:      3,
		StartAddress:  0,
		RegisterCount: 10,
		OutputFile:    tmpFile.Name(),
	}
	assert.NoError(t, CheckReadConfig(validCfg))
	ipv6 := validCfg
	ipv6.IP = "::1"
	assert.NoError(t, CheckReadConfig(ipv6))

	invalidCfg := validCfg
	invalidCfg.IP = "invalid"
	assert.Error(t, CheckReadConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.Function = 99
	assert.Error(t, CheckReadConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.RegisterCount = 0
	assert.Error(t, CheckReadConfig(invalidCfg))

	// Last two registers: must not reject due to uint16 wrap on start+count.
	tail := validCfg
	tail.StartAddress = 65534
	tail.RegisterCount = 2
	assert.NoError(t, CheckReadConfig(tail))

	invalidCfg = validCfg
	invalidCfg.Unit = 0
	assert.Error(t, CheckReadConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.Timeout = 100
	assert.Error(t, CheckReadConfig(invalidCfg))

	// start + count must not wrap uint16 or exceed address space
	overflow := validCfg
	overflow.StartAddress = 65520
	overflow.RegisterCount = 20
	assert.Error(t, CheckReadConfig(overflow))
}

func TestCheckScanConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "scan_output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	validCfg := config.ScanConfig{
		DeviceConfig: config.DeviceConfig{IP: "10.0.0.1", Port: 502, Unit: 2},
		Function:     3,
		StartAddress: 0,
		EndAddress:   100,
		Delay:        500,
		OutputFile:   tmpFile.Name(),
	}
	assert.NoError(t, CheckScanConfig(&validCfg))

	invalidCfg := validCfg
	invalidCfg.EndAddress = 0
	assert.Error(t, CheckScanConfig(&invalidCfg))

	invalidCfg = validCfg
	invalidCfg.Delay = 60001
	assert.Error(t, CheckScanConfig(&invalidCfg))

	// Algo: valid values (empty defaults to safe in executor; safe/smart/deep accepted)
	assert.NoError(t, CheckScanConfig(&validCfg)) // Algo empty
	assert.Equal(t, config.ScanAlgoSafe, validCfg.NormalizedAlgo)
	validSafe := validCfg
	validSafe.Algo = "safe"
	assert.NoError(t, CheckScanConfig(&validSafe))
	assert.Equal(t, config.ScanAlgoSafe, validSafe.NormalizedAlgo)
	validSmart := validCfg
	validSmart.Algo = "smart"
	assert.NoError(t, CheckScanConfig(&validSmart))
	assert.Equal(t, config.ScanAlgoSmart, validSmart.NormalizedAlgo)
	validDeep := validCfg
	validDeep.Algo = "deep"
	assert.NoError(t, CheckScanConfig(&validDeep))
	assert.Equal(t, config.ScanAlgoDeep, validDeep.NormalizedAlgo)

	validStepped := validCfg
	validStepped.Algo = "stepped"
	validStepped.Step = 1000
	assert.NoError(t, CheckScanConfig(&validStepped))
	validLinear := validCfg
	validLinear.Algo = "linear"
	assert.NoError(t, CheckScanConfig(&validLinear))
	validBoundary := validCfg
	validBoundary.Algo = "boundary"
	validBoundary.SeedStart = 50
	validBoundary.SeedCount = 10
	assert.NoError(t, CheckScanConfig(&validBoundary))

	// Boundary: requires seed-count 1..125
	invalidCfg = validCfg
	invalidCfg.Algo = "boundary"
	invalidCfg.SeedCount = 0
	assert.Error(t, CheckScanConfig(&invalidCfg))
	invalidCfg.SeedCount = 126
	assert.Error(t, CheckScanConfig(&invalidCfg))

	// Boundary: full seed must lie inside [StartAddress, EndAddress]
	invalidCfg = validCfg
	invalidCfg.Algo = "boundary"
	invalidCfg.SeedStart = 0
	invalidCfg.SeedCount = 10
	invalidCfg.StartAddress = 5 // seed start 0 < 5
	assert.Error(t, CheckScanConfig(&invalidCfg))
	invalidCfg = validCfg
	invalidCfg.Algo = "boundary"
	invalidCfg.SeedStart = 95
	invalidCfg.SeedCount = 10 // seed end 104 > EndAddress 100
	assert.Error(t, CheckScanConfig(&invalidCfg))

	// Algo: invalid value
	invalidCfg = validCfg
	invalidCfg.Algo = "invalid"
	assert.Error(t, CheckScanConfig(&invalidCfg))

	// Stepped: step must be 1..65535
	invalidCfg = validCfg
	invalidCfg.Algo = "stepped"
	invalidCfg.Step = 0
	assert.Error(t, CheckScanConfig(&invalidCfg))

	invalidCfg = validCfg
	invalidCfg.Timeout = 50
	assert.Error(t, CheckScanConfig(&invalidCfg))

	sun := config.ScanConfig{
		DeviceConfig: config.DeviceConfig{IP: "10.0.0.1", Port: 502, Unit: 2},
		Function:     3,
		Algo:         "sunspec",
		OutputFile:   tmpFile.Name(),
	}
	assert.NoError(t, CheckScanConfig(&sun))
	assert.Equal(t, config.ScanAlgoSunspec, sun.NormalizedAlgo)
	badBases := sun
	badBases.SunSpecBases = "not-a-number"
	assert.Error(t, CheckScanConfig(&badBases))
}

func TestCheckRecordConfig(t *testing.T) {
	inputTmp, err := os.CreateTemp("", "input_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(inputTmp.Name()) }()

	_, err = inputTmp.WriteString(`[{"start_address":0,"register_count":10}]`)
	assert.NoError(t, err)
	err = inputTmp.Close()
	assert.NoError(t, err)

	outputTmp, err := os.CreateTemp("", "record_output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(outputTmp.Name()) }()

	validCfg := config.RecordConfig{
		DeviceConfig: config.DeviceConfig{IP: "127.0.0.1", Port: 502, Unit: 1},
		Function:     4,
		Interval:     5,
		Duration:     30,
		BlocksFile:   inputTmp.Name(),
		OutputFile:   outputTmp.Name(),
	}
	assert.NoError(t, CheckRecordConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.Duration = 2
	assert.Error(t, CheckRecordConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.BlocksFile = ""
	assert.Error(t, CheckRecordConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.Timeout = 150
	assert.Error(t, CheckRecordConfig(invalidCfg))
}

func TestCheckConvertConfig(t *testing.T) {
	inputTmp, err := os.CreateTemp("", "input_*.mcap")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(inputTmp.Name()) }()

	outputTmp, err := os.CreateTemp("", "output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(outputTmp.Name()) }()

	validCfg := config.ConvertConfig{
		InputFile:  inputTmp.Name(),
		FormatType: "json",
		OutputFile: outputTmp.Name(),
	}
	assert.NoError(t, CheckConvertConfig(&validCfg))

	upper := validCfg
	upper.FormatType = "CSV"
	assert.NoError(t, CheckConvertConfig(&upper))
	assert.Equal(t, "csv", upper.FormatType)

	invalidCfg := validCfg
	invalidCfg.FormatType = "unsupported"
	assert.Error(t, CheckConvertConfig(&invalidCfg))
}

func TestCheckExtractConfig(t *testing.T) {
	inputTmp, err := os.CreateTemp("", "valid_input_*.mcap")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(inputTmp.Name()) }()

	outputTmp, err := os.CreateTemp("", "extracted_output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(outputTmp.Name()) }()

	validCfg := config.ExtractConfig{
		InputFile:  inputTmp.Name(),
		OutputFile: outputTmp.Name(),
	}
	assert.NoError(t, CheckExtractConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.InputFile = ""
	assert.Error(t, CheckExtractConfig(invalidCfg))
}

func TestCheckDeviceProfileDecodeConfig(t *testing.T) {
	inputTmp, err := os.CreateTemp("", "valid_input_*.mcap")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(inputTmp.Name()) }()

	profileTmp, err := os.CreateTemp("", "profile_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(profileTmp.Name()) }()

	_, err = profileTmp.WriteString(`{
    "protocolData": {
        "registers": [
            {
                "controlledPropertyId": "breaker.contactwear",
                "valueScaleFactor": 0.001538,
                "start": 20,
                "size": 1,
                "format": ">H"
            },
            {
                "controlledPropertyId": "breaker.totaloperations",
                "valueScaleFactor": 1,
                "start": 21,
                "size": 1,
                "format": ">H"
            }
        ]
    }
}`)
	assert.NoError(t, err)
	err = profileTmp.Close()
	assert.NoError(t, err)

	outputTmp, err := os.CreateTemp("", "decoded_output_*.json")
	assert.NoError(t, err)
	defer func() { _ = os.Remove(outputTmp.Name()) }()

	validCfg := config.DeviceProfileDecodeConfig{
		InputFile:     inputTmp.Name(),
		DeviceProfile: profileTmp.Name(),
		OutputFile:    outputTmp.Name(),
	}
	assert.NoError(t, CheckDeviceProfileDecodeConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.DeviceProfile = ""
	assert.Error(t, CheckDeviceProfileDecodeConfig(invalidCfg))
}

func TestValidateModbusAddress(t *testing.T) {
	// Exactly one of url or ip must be set
	assert.NoError(t, config.ValidateModbusAddress("tcp://10.0.0.1:502", ""))
	assert.NoError(t, config.ValidateModbusAddress("", "10.0.0.1"))
	assert.Error(t, config.ValidateModbusAddress("tcp://10.0.0.1:502", "10.0.0.1")) // both set
	assert.Error(t, config.ValidateModbusAddress("", ""))                           // neither set
	// Unsupported scheme
	assert.Error(t, config.ValidateModbusAddress("http://10.0.0.1:502", ""))
	// Supported schemes
	assert.NoError(t, config.ValidateModbusAddress("tcp+tls://10.0.0.1:502", ""))
	assert.NoError(t, config.ValidateModbusAddress("rtuovertcp://10.0.0.1:502", ""))
}

func TestModbusURL(t *testing.T) {
	assert.Equal(t, "tcp://10.0.0.1:502", config.ModbusURL("tcp://10.0.0.1:502", "", 0))
	assert.Equal(t, "tcp://10.0.0.1:502", config.ModbusURL("", "10.0.0.1", 502))
	assert.Equal(t, "tcp://10.0.0.1:502", config.ModbusURL("", "10.0.0.1", 0)) // defaults to 502
	assert.Equal(t, "", config.ModbusURL("", "", 0))
}

func TestCheckSunSpecDetectConfig(t *testing.T) {
	validCfg := config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			IP: "192.168.1.10", Port: 502, Unit: 1, Regtype: "holding",
		},
	}
	assert.NoError(t, CheckSunSpecDetectConfig(validCfg))

	// URL instead of IP
	urlCfg := config.SunSpecDetectConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			URL: "tcp://192.168.1.10:502", Unit: 1, Regtype: "holding",
		},
	}
	assert.NoError(t, CheckSunSpecDetectConfig(urlCfg))

	// Both URL and IP: error
	bothCfg := validCfg
	bothCfg.URL = "tcp://10.0.0.1:502"
	assert.Error(t, CheckSunSpecDetectConfig(bothCfg))

	// Unit 0 and 255 allowed (full Modbus range 0-255, e.g. SunSpec over gateway)
	unit0Cfg := validCfg
	unit0Cfg.Unit = 0
	assert.NoError(t, CheckSunSpecDetectConfig(unit0Cfg))
	unit255Cfg := validCfg
	unit255Cfg.Unit = 255
	assert.NoError(t, CheckSunSpecDetectConfig(unit255Cfg))

	// Invalid regtype
	invalidCfg := validCfg
	invalidCfg.Regtype = "foo"
	assert.Error(t, CheckSunSpecDetectConfig(invalidCfg))

	// Invalid bases
	invalidCfg = validCfg
	invalidCfg.Bases = "abc"
	assert.Error(t, CheckSunSpecDetectConfig(invalidCfg))

	shortTimeout := validCfg
	shortTimeout.Timeout = 100
	assert.Error(t, CheckSunSpecDetectConfig(shortTimeout))
}

func TestCheckSunSpecModelsConfig(t *testing.T) {
	validCfg := config.SunSpecModelsConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			IP: "192.168.1.10", Port: 502, Unit: 1, Regtype: "holding",
		},
	}
	assert.NoError(t, CheckSunSpecModelsConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.MaxModels = -1
	assert.Error(t, CheckSunSpecModelsConfig(invalidCfg))
}

func TestCheckSunSpecMapConfig(t *testing.T) {
	validCfg := config.SunSpecMapConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			IP: "192.168.1.10", Port: 502, Unit: 1, Regtype: "holding",
		},
	}
	assert.NoError(t, CheckSunSpecMapConfig(validCfg))
}

func TestCheckSunSpecProbeConfig(t *testing.T) {
	validCfg := config.SunSpecProbeConfig{
		SunSpecBaseConfig: config.SunSpecBaseConfig{
			IP: "192.168.1.10", Port: 502, Unit: 1, Regtype: "input",
		},
	}
	assert.NoError(t, CheckSunSpecProbeConfig(validCfg))

	invalidCfg := validCfg
	invalidCfg.Port = 0
	assert.Error(t, CheckSunSpecProbeConfig(invalidCfg))
}

func TestCheckDiagnosticConfig(t *testing.T) {
	validCfg := config.DiagnosticConfig{
		IP: "192.168.1.10", Port: 502, UnitID: 1, Timeout: 2000,
		SubFunction: "returnquerydata",
	}
	assert.NoError(t, CheckDiagnosticConfig(validCfg))

	// URL variant
	urlCfg := config.DiagnosticConfig{
		URL: "tcp://192.168.1.10:502", UnitID: 1, Timeout: 2000,
		SubFunction: "returnbusmessagecount",
	}
	assert.NoError(t, CheckDiagnosticConfig(urlCfg))

	// Invalid sub-function
	invalidCfg := validCfg
	invalidCfg.SubFunction = "bogus"
	assert.Error(t, CheckDiagnosticConfig(invalidCfg))

	// Invalid hex data
	invalidCfg = validCfg
	invalidCfg.Data = "ZZZ"
	assert.Error(t, CheckDiagnosticConfig(invalidCfg))

	// Timeout 0 = default budget
	invalidCfg = validCfg
	invalidCfg.Timeout = 0
	assert.NoError(t, CheckDiagnosticConfig(invalidCfg))
	invalidCfg.Timeout = 100
	assert.Error(t, CheckDiagnosticConfig(invalidCfg))

	invalidCfg = validCfg
	invalidCfg.SubFunction = "returnbusmessagecount"
	invalidCfg.Data = "01020304"
	assert.Error(t, CheckDiagnosticConfig(invalidCfg))
}

func TestCheckDiscoverConfig_parallelBounds(t *testing.T) {
	valid := config.DiscoverConfig{
		Subnets:  []string{"192.168.1.0/24"},
		Port:     502,
		Parallel: 1,
	}
	assert.NoError(t, CheckDiscoverConfig(valid))

	valid64 := valid
	valid64.Parallel = 64
	assert.NoError(t, CheckDiscoverConfig(valid64))

	tooLow := valid
	tooLow.Parallel = 0
	assert.Error(t, CheckDiscoverConfig(tooLow))

	tooHigh := valid
	tooHigh.Parallel = 65
	assert.Error(t, CheckDiscoverConfig(tooHigh))
}

func TestCheckDiscoverConfig_hostCap(t *testing.T) {
	huge := config.DiscoverConfig{
		Subnets:  []string{"10.0.0.0/15"},
		Port:     502,
		Parallel: 1,
	}
	err := CheckDiscoverConfig(huge)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "force-large-scan")

	withForce := huge
	withForce.ForceLargeScan = true
	assert.NoError(t, CheckDiscoverConfig(withForce))
}

func TestValidateFile_outputTrailingSlash(t *testing.T) {
	d := t.TempDir()
	assert.NoError(t, validateFile(d+string(filepath.Separator), false))
}
