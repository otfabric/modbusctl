package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/mdlayher/arp"
	mb "github.com/otfabric/modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/types"
)

func PerformDiscoveryScan(cfg config.DiscoverConfig) error {
	var ips []string
	for _, subnet := range cfg.Subnets {
		subnetIPs, err := getIPsFromCIDR(subnet)
		if err != nil {
			return fmt.Errorf("invalid subnet %s: %w", subnet, err)
		}
		ips = append(ips, subnetIPs...)
	}

	if cfg.Parallel < 1 {
		cfg.Parallel = 1
	}

	var (
		wg    sync.WaitGroup
		sem   = make(chan struct{}, cfg.Parallel)
		mutex sync.Mutex
	)

	var results []struct {
		ip  string
		mac string
	}

	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			addr := net.JoinHostPort(ip, fmt.Sprintf("%d", cfg.Port))
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				modbusURL := config.ModbusURL("", ip, cfg.Port)
				client, cleanup, err := connect(modbusURL)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer cleanup()
				_, err = client.ReadRawBytes(context.Background(), 1, 0, 2, mb.HoldingRegister)
				if !isValidModbusResponse(err) {
					_ = conn.Close()
					return
				}

				mac := ""
				if cfg.ResolveMAC {
					mac, _ = resolveMACAddress(ip, cfg.NetworkInterface)
				}
				mutex.Lock()
				results = append(results, struct {
					ip  string
					mac string
				}{
					ip:  ip,
					mac: mac,
				})
				mutex.Unlock()
				_ = conn.Close()
			}
		}(ip)
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		ip1 := net.ParseIP(results[i].ip).To4()
		ip2 := net.ParseIP(results[j].ip).To4()
		if ip1 == nil || ip2 == nil {
			return results[i].ip < results[j].ip
		}
		for k := range 4 {
			if ip1[k] != ip2[k] {
				return ip1[k] < ip2[k]
			}
		}
		return false
	})

	for _, r := range results {
		if cfg.ResolveMAC && r.mac != "" {
			fmt.Printf("✅ Modbus device found at %s:%d (MAC: %s)\n", r.ip, cfg.Port, r.mac)
		} else {
			fmt.Printf("✅ Modbus device found at %s:%d\n", r.ip, cfg.Port)
		}
	}

	if cfg.OutputFile != "" {
		jsonResults := []types.DiscoverJson{}
		for _, r := range results {
			jsonResults = append(jsonResults, types.DiscoverJson{
				IP:        r.ip,
				Port:      cfg.Port,
				Mac:       r.mac,
				Interface: cfg.NetworkInterface,
			})
		}
		data, err := json.MarshalIndent(jsonResults, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal results to JSON: %w", err)
		}
		if err := os.WriteFile(cfg.OutputFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
	}

	return nil
}

func getIPsFromCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incIP(ip) {
		ips = append(ips, ip.String())
	}

	if len(ips) > 2 {
		return ips[1 : len(ips)-1], nil
	}
	return ips, nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

// resolveMACAddress tries to resolve the MAC address for the given IP using ARP.
func resolveMACAddress(ip string, ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", err
	}

	client, err := arp.Dial(iface)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()

	dstIP, err := netip.ParseAddr(ip)
	if err != nil {
		return "", err
	}
	mac, err := client.Resolve(dstIP)
	if err != nil {
		return "", err
	}

	return mac.String(), nil
}

// isValidModbusResponse returns true if err is nil or a Modbus exception (device responded).
func isValidModbusResponse(err error) bool {
	if err == nil {
		return true
	}
	var excErr *mb.ExceptionError
	return errors.As(err, &excErr)
}
