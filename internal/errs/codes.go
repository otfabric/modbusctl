package errs

// Stable machine-oriented codes for JSON error.code and scripts.
const (
	// An invocation cobra rejected: unknown command or flag, bad flag value, wrong arguments.
	CodeInvalidUsage = "invalid_usage"

	CodeInvalidFormat          = "invalid_format"
	CodeInvalidInput           = "invalid_input"
	CodeInvalidURL             = "invalid_url"
	CodeInvalidFlagCombination = "invalid_flag_combination"
	CodeInvalidUnitSelector    = "invalid_unit_selector"

	CodeTransportConnectFailed = "transport_connect_failed"
	CodeTransportTimeout       = "transport_timeout"
	CodeRequestTimeout         = "request_timeout"

	CodeModbusException = "modbus_exception"
	// SunSpec / diagnostics domain boundaries (cause retains transport, timeout, Modbus exception detail).
	CodeSunSpecDetectFailed       = "sunspec_detect_failed"
	CodeSunSpecModelHeadersFailed = "sunspec_model_headers_failed"
	CodeSunSpecSupportProbeFailed = "sunspec_support_probe_failed"
	CodeDiagnosticsFailed         = "diagnostics_failed"
	// Device identification (FC43) unavailable or invalid response — not a Modbus exception PDU.
	CodeDeviceIdentificationUnsupported = "device_identification_unsupported"
	// Modbus client construction failed (e.g. modbus.New) — not “device does not support FC43”.
	CodeModbusClientSetupFailed = "modbus_client_setup_failed"
	// Fingerprint stopped mid-probe after a transport/protocol error on SupportsFunction.
	CodeProbeInterrupted = "probe_interrupted"
	// Context was canceled (not invalid user input).
	CodeContextCanceled = "context_canceled"

	CodeJSONEncodeFailed       = "json_encode_failed"
	CodeTableRenderFailed      = "table_render_failed"
	CodeOutputFileWriteFailed  = "output_file_write_failed"
	CodeOutputFileCreateFailed = "output_file_create_failed"
	CodeCaptureLoadFailed      = "capture_load_failed"
	CodeInputFileOpenFailed    = "input_file_open_failed"
	CodeInputDecodeFailed      = "input_decode_failed"
	CodeCaptureWriteFailed     = "capture_write_failed"
	// A modbusctl 2.x capture file (.mcap), which modbusctl 3 does not read.
	CodeCaptureLegacyFormat = "capture_legacy_format"
	// A capture holds several targets or functions and the command needs one (--target, --function).
	CodeCaptureSelectionAmbiguous = "capture_selection_ambiguous"
	// The selected target or function has no data in the capture.
	CodeCaptureSelectionEmpty = "capture_selection_empty"
	// A capture holds a sample of a function code outside the Modbus read functions modbusctl supports.
	CodeCaptureUnsupportedFunction = "capture_unsupported_function"

	CodeInternalUnexpected = "internal_unexpected"
)
