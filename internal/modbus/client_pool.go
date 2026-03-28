package modbus

import (
	"github.com/otfabric/go-modbus"
)

// openModbusClientPool opens n TCP clients using the same validated config (for parallel per-unit work).
// cleanup closes all successfully opened clients. Config must already be valid for [modbus.ValidateConfig].
func openModbusClientPool(n int, conf modbus.Config) ([]*modbus.Client, func(), error) {
	if n < 1 {
		n = 1
	}
	if err := modbus.ValidateConfig(conf); err != nil {
		return nil, nil, ClientConfigInvalid(err)
	}
	clients := make([]*modbus.Client, 0, n)
	for i := 0; i < n; i++ {
		mc, err := modbus.New(conf)
		if err != nil {
			closeModbusClients(clients)
			return nil, nil, ClientSetupError(err)
		}
		if err := mc.Open(); err != nil {
			_ = mc.Close()
			closeModbusClients(clients)
			return nil, nil, TCPConnectionError(err)
		}
		clients = append(clients, mc)
	}
	cleanup := func() {
		closeModbusClients(clients)
	}
	return clients, cleanup, nil
}

func closeModbusClients(clients []*modbus.Client) {
	for _, c := range clients {
		if c != nil {
			_ = c.Close()
		}
	}
}
