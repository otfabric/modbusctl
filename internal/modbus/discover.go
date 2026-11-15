// SPDX-License-Identifier: MIT

package modbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/mdlayher/arp"
	mb "github.com/otfabric/go-modbus"
	"github.com/otfabric/modbusctl/internal/config"
	"github.com/otfabric/modbusctl/internal/errs"
	"github.com/otfabric/modbusctl/internal/fsutil"
	"github.com/otfabric/modbusctl/internal/types"
)

// CollectDiscover probes subnets and returns structured output. Debug lines go to stderr when non-nil.
// Optional JSON file side effect: when cfg.OutputFile is set, writes the legacy []DiscoverJson array (same as before).
func CollectDiscover(ctx context.Context, cfg config.DiscoverConfig, stderr io.Writer) (*types.DiscoverOutput, error) {
	if stderr == nil {
		stderr = io.Discard
	}
	if cfg.Parallel < 1 {
		cfg.Parallel = 1
	}
	dial, err := cfg.Dialer()
	if err != nil {
		return nil, errs.InvalidInput(errs.CodeInvalidInput, err.Error(), err)
	}

	jobs := make(chan string, int(cfg.Parallel)*4)
	go func() {
		defer close(jobs)
		seen := make(map[string]struct{}, 1024)
		for _, subnet := range cfg.Subnets {
			_ = forEachAssignableHost(subnet, func(ip string) bool {
				if _, ok := seen[ip]; ok {
					return true
				}
				seen[ip] = struct{}{}
				select {
				case <-ctx.Done():
					return false
				case jobs <- ip:
					return true
				}
			})
		}
	}()

	var (
		wg    sync.WaitGroup
		mutex sync.Mutex
		raw   []struct {
			ip  string
			mac string
		}
	)

	par := int(cfg.Parallel)
	for range par {
		wg.Go(func() {
			for ip := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				modbusURL := config.ModbusURL("", ip, cfg.Port)
				client, cleanup, err := connectDiscovery(modbusURL, cfg.Debug, dial)
				if err != nil {
					if cfg.Debug {
						_, _ = fmt.Fprintf(stderr, "DEBUG [discover] %s: connect/setup: %v\n", ip, err)
					}
					continue
				}
				func() {
					defer cleanup()
					_, err = client.ReadRegisterBytes(ctx, 1, 0, 2, mb.HoldingRegister)
					if !isValidModbusResponse(err) {
						if cfg.Debug && err != nil {
							_, _ = fmt.Fprintf(stderr, "DEBUG [discover] %s: holding read: %v\n", ip, err)
						}
						return
					}
					mac := ""
					if cfg.ResolveMAC {
						mac, _ = resolveMACAddress(ctx, ip, cfg.NetworkInterface)
					}
					mutex.Lock()
					raw = append(raw, struct {
						ip  string
						mac string
					}{ip: ip, mac: mac})
					mutex.Unlock()
				}()
			}
		})
	}
	wg.Wait()

	sort.Slice(raw, func(i, j int) bool {
		ip1 := net.ParseIP(raw[i].ip).To4()
		ip2 := net.ParseIP(raw[j].ip).To4()
		if ip1 == nil || ip2 == nil {
			return raw[i].ip < raw[j].ip
		}
		for k := range 4 {
			if ip1[k] != ip2[k] {
				return ip1[k] < ip2[k]
			}
		}
		return false
	})

	devices := make([]types.DiscoverJson, 0, len(raw))
	for _, r := range raw {
		devices = append(devices, types.DiscoverJson{
			IP:        r.ip,
			Port:      cfg.Port,
			Mac:       r.mac,
			Interface: cfg.NetworkInterface,
		})
	}

	out := &types.DiscoverOutput{
		Port:      cfg.Port,
		Interface: cfg.NetworkInterface,
		Subnets:   append([]string(nil), cfg.Subnets...),
		Devices:   devices,
	}

	if cfg.OutputFile != "" {
		data, err := json.MarshalIndent(devices, "", "  ")
		if err != nil {
			return nil, errs.Output(errs.CodeJSONEncodeFailed, err)
		}
		if err := fsutil.WriteOutput(cfg.OutputFile, data); err != nil {
			return nil, errs.Output(errs.CodeOutputFileWriteFailed, err)
		}
	}

	return out, nil
}

// EstimateDiscoverHostCount returns how many unique assignable IPs would be probed for cfg (same rules as [CollectDiscover]).
// It enumerates every host: for a subnet of unknown size use [DiscoverHostCountAtMost].
func EstimateDiscoverHostCount(cfg config.DiscoverConfig) (int, error) {
	return DiscoverHostCountAtMost(cfg, math.MaxInt-1)
}

// DiscoverHostCountAtMost is [EstimateDiscoverHostCount] that stops counting above limit: it returns limit+1 for a
// larger scope, without enumerating (and remembering) the hosts of a /8 or an IPv6 /64.
func DiscoverHostCountAtMost(cfg config.DiscoverConfig, limit int) (int, error) {
	seen := make(map[string]struct{}, 1024)
	n := 0
	for _, subnet := range cfg.Subnets {
		err := forEachAssignableHost(subnet, func(ip string) bool {
			if _, ok := seen[ip]; ok {
				return true
			}
			seen[ip] = struct{}{}
			n++
			return n <= limit
		})
		if err != nil {
			return 0, err
		}
		if n > limit {
			return n, nil
		}
	}
	return n, nil
}

// forEachAssignableHost calls yield for each host address to probe in cidr, using the same rules as legacy getIPsFromCIDR
// (omit network and broadcast when the CIDR has more than two addresses). Stops early if yield returns false.
func forEachAssignableHost(cidr string, yield func(host string) bool) error {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}
	first := append(net.IP(nil), ip.Mask(ipnet.Mask)...)
	second := append(net.IP(nil), first...)
	incIP(second)
	if !ipnet.Contains(second) {
		_ = yield(first.String())
		return nil
	}
	third := append(net.IP(nil), second...)
	incIP(third)
	if !ipnet.Contains(third) {
		if !yield(first.String()) {
			return nil
		}
		_ = yield(second.String())
		return nil
	}
	cur := append(net.IP(nil), second...)
	for ipnet.Contains(cur) {
		next := append(net.IP(nil), cur...)
		incIP(next)
		if !ipnet.Contains(next) {
			break
		}
		if !yield(cur.String()) {
			return nil
		}
		cur = next
	}
	return nil
}

func incIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

// dialARP opens the ARP client of an interface on a raw socket. Tests replace it with a
// client on a connection of their own.
var dialARP = arp.Dial

// resolveMACAddress tries to resolve the MAC address for the given IP using ARP. It waits at
// most discoveryMACTimeout for the reply, and returns ctx's error as soon as ctx ends.
func resolveMACAddress(ctx context.Context, ip string, ifaceName string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", err
	}

	client, err := dialARP(iface)
	if err != nil {
		return "", err
	}
	defer func() { _ = client.Close() }()
	// Closing the socket ends a read that waits for the reply.
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	if err := client.SetDeadline(time.Now().Add(discoveryMACTimeout)); err != nil {
		return "", err
	}

	dstIP, err := netip.ParseAddr(ip)
	if err != nil {
		return "", err
	}
	mac, err := client.Resolve(dstIP)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return "", cerr
		}
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
