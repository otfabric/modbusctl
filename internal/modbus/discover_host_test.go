package modbus

import (
	"slices"
	"testing"

	"github.com/otfabric/modbusctl/internal/config"
)

func TestForEachAssignableHost(t *testing.T) {
	t.Run("/30 skips network and broadcast", func(t *testing.T) {
		var got []string
		err := forEachAssignableHost("192.168.0.0/30", func(h string) bool {
			got = append(got, h)
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"192.168.0.1", "192.168.0.2"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("/31 yields both addresses", func(t *testing.T) {
		var got []string
		err := forEachAssignableHost("192.168.0.0/31", func(h string) bool {
			got = append(got, h)
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"192.168.0.0", "192.168.0.1"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v want %v", got, want)
		}
	})
	t.Run("stops when yield returns false", func(t *testing.T) {
		var got []string
		err := forEachAssignableHost("10.0.0.0/30", func(h string) bool {
			got = append(got, h)
			return false
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != "10.0.0.1" {
			t.Fatalf("got %v want single 10.0.0.1", got)
		}
	})
}

func TestEstimateDiscoverHostCount(t *testing.T) {
	n, err := EstimateDiscoverHostCount(config.DiscoverConfig{
		Subnets: []string{"192.168.0.0/30", "192.168.0.0/30"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deduped /30 hosts want 2 got %d", n)
	}
}
