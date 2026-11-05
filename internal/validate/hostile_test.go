// SPDX-License-Identifier: MIT

package validate

import (
	"strings"
	"testing"
	"time"

	"github.com/otfabric/modbusctl/internal/config"
)

// TestCheckDiscoverConfig_hugeSubnetsAreRefusedQuickly: the scope check enumerated
// every host of the subnets into a map before comparing with the cap, so
// 0.0.0.0/0 or an IPv6 /64 ran for hours and exhausted memory. It stops above
// the cap now.
func TestCheckDiscoverConfig_hugeSubnetsAreRefusedQuickly(t *testing.T) {
	for _, subnet := range []string{"0.0.0.0/0", "10.0.0.0/8", "fd00::/64", "::/0"} {
		done := make(chan error, 1)
		go func() {
			done <- CheckDiscoverConfig(config.DiscoverConfig{Subnets: []string{"192.168.1.0/24", subnet}, Port: 502, Parallel: 4})
		}()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "more than 65536 unique hosts") {
				t.Fatalf("%s: %v", subnet, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: the scope check did not finish", subnet)
		}
	}
	if err := CheckDiscoverConfig(config.DiscoverConfig{Subnets: []string{"10.0.0.0/16"}, Port: 502, Parallel: 4}); err != nil {
		t.Fatalf("a /16 is within the cap: %v", err)
	}
}
