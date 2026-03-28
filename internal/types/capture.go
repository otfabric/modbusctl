package types

type CaptureHeader struct {
	IP        string
	Port      uint16
	Unit      uint8
	Function  uint8
	StartTime int64
}

type CaptureRecord struct {
	Iteration         uint32
	RequestTimestamp  int64
	ResponseTimestamp int64
	StartAddress      uint16
	RegisterCount     uint16
	Data              []byte
}

type CaptureHeaderJson struct {
	IP        string `json:"ip"`
	Port      uint16 `json:"port"`
	Unit      uint8  `json:"unit"`
	Function  uint8  `json:"function"`
	StartTime string `json:"start_time"`
}

type CaptureRecordJson struct {
	Iteration         uint32 `json:"iteration"`
	RequestTimestamp  string `json:"request_timestamp"`
	ResponseTimestamp string `json:"response_timestamp"`
	StartAddress      uint16 `json:"start_address"`
	RegisterCount     uint16 `json:"register_count"`
	Data              string `json:"raw_data"`
}

type AddressBlock struct {
	StartAddress  uint16 `json:"start_address"`
	RegisterCount uint16 `json:"register_count"`
}

type IterationDetail struct {
	BlockCount     int
	TotalRegisters int
	FirstRequestTS int64
	LastResponseTS int64
	MinAddr        uint16
	MaxAddr        uint16
}

type DiscoverJson struct {
	IP        string `json:"ip"`
	Port      uint16 `json:"port"`
	Mac       string `json:"mac"`
	Interface string `json:"interface"`
}
