package enricher

import (
	"log/slog"
	"net"
	"os"
	"testing"

	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// newPopulatedCache creates a NetBoxCache pre-loaded with test data.
func newPopulatedCache(devices map[string]DeviceMetadata) *NetBoxCache {
	return &NetBoxCache{
		devices: devices,
		logger:  newTestLogger(),
	}
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func TestEnrichFlow_NetBoxCacheLookup(t *testing.T) {
	tests := []struct {
		name          string
		exporterIP    uint32
		inIf          uint32
		outIf         uint32
		cache         map[string]DeviceMetadata
		wantName      string
		wantSite      string
		wantRole      string
		wantInIfName  string
		wantOutIfName string
	}{
		{
			name:       "enrich flow with device metadata from NetBox cache",
			exporterIP: ipToUint32(net.ParseIP("10.0.0.1")),
			inIf:       1,
			outIf:      2,
			cache: map[string]DeviceMetadata{
				"10.0.0.1": {
					Name:   "router-1",
					Site:   "dc1",
					Region: "us-east",
					Role:   "core-router",
					Interfaces: map[uint32]InterfaceMetadata{
						1: {Name: "Ethernet1", Speed: 10000},
						2: {Name: "Ethernet2", Speed: 25000},
					},
				},
			},
			wantName:      "router-1",
			wantSite:      "dc1",
			wantRole:      "core-router",
			wantInIfName:  "Ethernet1",
			wantOutIfName: "Ethernet2",
		},
		{
			name:       "enrich flow with device but no matching interfaces",
			exporterIP: ipToUint32(net.ParseIP("10.0.0.2")),
			inIf:       99,
			outIf:      100,
			cache: map[string]DeviceMetadata{
				"10.0.0.2": {
					Name:       "switch-1",
					Site:       "dc2",
					Region:     "eu-west",
					Role:       "access",
					Interfaces: map[uint32]InterfaceMetadata{},
				},
			},
			wantName:      "switch-1",
			wantSite:      "dc2",
			wantRole:      "access",
			wantInIfName:  "",
			wantOutIfName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := newPopulatedCache(tc.cache)
			e := New(cache, nil, newTestLogger())

			flow := &flowpb.EnrichedFlow{
				ExporterIp: tc.exporterIP,
				InIf:       tc.inIf,
				OutIf:      tc.outIf,
			}

			result := e.Enrich(flow)

			if result.ExporterName != tc.wantName {
				t.Errorf("ExporterName = %q, want %q", result.ExporterName, tc.wantName)
			}
			if result.ExporterSite != tc.wantSite {
				t.Errorf("ExporterSite = %q, want %q", result.ExporterSite, tc.wantSite)
			}
			if result.ExporterRole != tc.wantRole {
				t.Errorf("ExporterRole = %q, want %q", result.ExporterRole, tc.wantRole)
			}
			if result.InIfName != tc.wantInIfName {
				t.Errorf("InIfName = %q, want %q", result.InIfName, tc.wantInIfName)
			}
			if result.OutIfName != tc.wantOutIfName {
				t.Errorf("OutIfName = %q, want %q", result.OutIfName, tc.wantOutIfName)
			}
		})
	}
}

func TestEnrichFlow_CacheMissHandling(t *testing.T) {
	tests := []struct {
		name       string
		exporterIP uint32
		srcIP      []byte
		dstIP      []byte
	}{
		{
			name:       "unknown exporter IP passes through without enrichment",
			exporterIP: ipToUint32(net.ParseIP("192.168.99.99")),
			srcIP:      net.ParseIP("1.2.3.4").To4(),
			dstIP:      net.ParseIP("5.6.7.8").To4(),
		},
		{
			name:       "empty cache passes through",
			exporterIP: ipToUint32(net.ParseIP("10.0.0.1")),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := newPopulatedCache(map[string]DeviceMetadata{})
			e := New(cache, nil, newTestLogger())

			flow := &flowpb.EnrichedFlow{
				ExporterIp: tc.exporterIP,
				SrcIp:      tc.srcIP,
				DstIp:      tc.dstIP,
			}

			result := e.Enrich(flow)

			// Flow should pass through with original fields intact, no enrichment
			if result.ExporterName != "" {
				t.Errorf("expected empty ExporterName on cache miss, got %q", result.ExporterName)
			}
			if result.ExporterSite != "" {
				t.Errorf("expected empty ExporterSite on cache miss, got %q", result.ExporterSite)
			}
		})
	}
}

func TestEnrichFlow_GeoIPLookup(t *testing.T) {
	// GeoIP enrichment requires MaxMind DB files which aren't available in unit tests.
	// Test the nil GeoIP path and uint32ToIP conversion instead.

	t.Run("nil geoip reader skips enrichment", func(t *testing.T) {
		cache := newPopulatedCache(map[string]DeviceMetadata{})
		e := New(cache, nil, newTestLogger())

		flow := &flowpb.EnrichedFlow{
			SrcIp: net.ParseIP("8.8.8.8").To4(),
			DstIp: net.ParseIP("1.1.1.1").To4(),
		}

		result := e.Enrich(flow)

		if result.SrcCountry != "" {
			t.Errorf("expected empty SrcCountry with nil geoip, got %q", result.SrcCountry)
		}
		if result.DstCountry != "" {
			t.Errorf("expected empty DstCountry with nil geoip, got %q", result.DstCountry)
		}
	})
}

func TestUint32ToIP(t *testing.T) {
	tests := []struct {
		name string
		ip   uint32
		want string
	}{
		{"10.0.0.1", 0x0A000001, "10.0.0.1"},
		{"192.168.1.1", 0xC0A80101, "192.168.1.1"},
		{"255.255.255.255", 0xFFFFFFFF, "255.255.255.255"},
		{"0.0.0.0", 0x00000000, "0.0.0.0"},
		{"172.16.0.100", 0xAC100064, "172.16.0.100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := uint32ToIP(tc.ip)
			if result.String() != tc.want {
				t.Errorf("uint32ToIP(%#x) = %s, want %s", tc.ip, result.String(), tc.want)
			}
		})
	}
}

func TestNetBoxCache_LookupByIP(t *testing.T) {
	cache := newPopulatedCache(map[string]DeviceMetadata{
		"10.0.0.1": {Name: "router-1", Site: "dc1"},
		"10.0.0.2": {Name: "switch-1", Site: "dc2"},
	})

	t.Run("returns device for known IP", func(t *testing.T) {
		device, ok := cache.LookupByIP(net.ParseIP("10.0.0.1"))
		if !ok {
			t.Fatal("expected to find device")
		}
		if device.Name != "router-1" {
			t.Errorf("Name = %q, want router-1", device.Name)
		}
	})

	t.Run("returns false for unknown IP", func(t *testing.T) {
		_, ok := cache.LookupByIP(net.ParseIP("10.99.99.99"))
		if ok {
			t.Error("expected not to find device for unknown IP")
		}
	})

	t.Run("DeviceCount returns correct count", func(t *testing.T) {
		if cache.DeviceCount() != 2 {
			t.Errorf("DeviceCount() = %d, want 2", cache.DeviceCount())
		}
	})
}
