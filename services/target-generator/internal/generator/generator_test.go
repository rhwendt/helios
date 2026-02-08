package generator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
)

func sampleDevices() []netbox.Device {
	return []netbox.Device{
		{
			ID:             1,
			Name:           "router-1",
			PrimaryIP:      "10.0.0.1",
			Site:           "dc1",
			Region:         "us-east",
			Role:           "router",
			Manufacturer:   "arista",
			Platform:       "eos",
			Status:         "active",
			MonitoringTier: "premium",
			CustomFields: netbox.DeviceCustomFields{
				GNMIEnabled:    true,
				GNMIPort:       6030,
				SNMPEnabled:    true,
				SNMPModule:     "arista_sw",
				BlackboxProbes: []string{"icmp", "tcp_connect"},
			},
		},
		{
			ID:             2,
			Name:           "switch-1",
			PrimaryIP:      "10.0.0.2",
			Site:           "dc2",
			Region:         "eu-west",
			Role:           "switch",
			Manufacturer:   "cisco",
			Platform:       "nxos",
			Status:         "active",
			MonitoringTier: "standard",
			CustomFields: netbox.DeviceCustomFields{
				GNMIEnabled:    false,
				SNMPEnabled:    true,
				SNMPModule:     "cisco_nxos",
				BlackboxProbes: []string{"icmp"},
			},
		},
		{
			ID:           3,
			Name:         "no-ip-device",
			PrimaryIP:    "",
			Site:         "dc1",
			Role:         "firewall",
			Manufacturer: "paloalto",
			CustomFields: netbox.DeviceCustomFields{
				GNMIEnabled: true,
				SNMPEnabled: true,
			},
		},
	}
}

func TestGenerateGNMICTargets(t *testing.T) {
	tests := []struct {
		name          string
		devices       []netbox.Device
		wantCount     int
		wantContains  []string
		wantExcludes  []string
	}{
		{
			name:         "only gNMI-enabled devices with IPs",
			devices:      sampleDevices(),
			wantCount:    1,
			wantContains: []string{"router-1"},
			wantExcludes: []string{"switch-1", "no-ip-device"},
		},
		{
			name:      "empty device list",
			devices:   []netbox.Device{},
			wantCount: 0,
		},
		{
			name: "custom gNMI port",
			devices: []netbox.Device{
				{
					Name: "custom-port", PrimaryIP: "10.0.0.5",
					CustomFields: netbox.DeviceCustomFields{GNMIEnabled: true, GNMIPort: 57400},
				},
			},
			wantCount:    1,
			wantContains: []string{"57400"},
		},
		{
			name: "default port when zero",
			devices: []netbox.Device{
				{
					Name: "default-port", PrimaryIP: "10.0.0.6",
					CustomFields: netbox.DeviceCustomFields{GNMIEnabled: true, GNMIPort: 0},
				},
			},
			wantCount:    1,
			wantContains: []string{"6030"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, count, err := GenerateGNMICTargets(tc.devices)
			if err != nil {
				t.Fatalf("GenerateGNMICTargets error: %v", err)
			}
			if count != tc.wantCount {
				t.Errorf("count = %d, want %d", count, tc.wantCount)
			}
			yamlStr := string(data)
			for _, s := range tc.wantContains {
				if !strings.Contains(yamlStr, s) {
					t.Errorf("output should contain %q", s)
				}
			}
			for _, s := range tc.wantExcludes {
				if strings.Contains(yamlStr, s) {
					t.Errorf("output should NOT contain %q", s)
				}
			}
		})
	}
}

func TestGenerateSNMPTargets(t *testing.T) {
	tests := []struct {
		name      string
		devices   []netbox.Device
		wantCount int
	}{
		{
			name:      "SNMP-enabled devices with IPs",
			devices:   sampleDevices(),
			wantCount: 2, // router-1 and switch-1 both have SNMP enabled
		},
		{
			name:      "empty list",
			devices:   []netbox.Device{},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, count, err := GenerateSNMPTargets(tc.devices)
			if err != nil {
				t.Fatalf("GenerateSNMPTargets error: %v", err)
			}
			if count != tc.wantCount {
				t.Errorf("count = %d, want %d", count, tc.wantCount)
			}
			if count > 0 {
				var entries []PrometheusFileSDEntry
				if err := json.Unmarshal(data, &entries); err != nil {
					t.Fatalf("invalid JSON: %v", err)
				}
				for _, entry := range entries {
					if entry.Labels["__param_module"] == "" {
						t.Error("SNMP target missing __param_module label")
					}
				}
			}
		})
	}
}

func TestGenerateSNMPTargets_LabelTaxonomy(t *testing.T) {
	devices := sampleDevices()
	data, _, err := GenerateSNMPTargets(devices)
	if err != nil {
		t.Fatalf("GenerateSNMPTargets error: %v", err)
	}

	var entries []PrometheusFileSDEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	requiredLabels := []string{"device", "site", "region", "vendor", "platform", "role", "tier"}
	for i, entry := range entries {
		for _, label := range requiredLabels {
			if _, ok := entry.Labels[label]; !ok {
				t.Errorf("entry[%d] missing required label %q", i, label)
			}
		}
	}
}

func TestGenerateBlackboxTargets(t *testing.T) {
	tests := []struct {
		name      string
		devices   []netbox.Device
		wantCount int
		wantProbes []string
	}{
		{
			name:       "devices with multiple probes",
			devices:    sampleDevices(),
			wantCount:  3, // router-1: icmp+tcp_connect, switch-1: icmp
			wantProbes: []string{"icmp", "tcp_connect"},
		},
		{
			name: "device with default icmp probe",
			devices: []netbox.Device{
				{
					Name: "basic-device", PrimaryIP: "10.0.0.10",
					CustomFields: netbox.DeviceCustomFields{BlackboxProbes: nil},
				},
			},
			wantCount:  1,
			wantProbes: []string{"icmp"},
		},
		{
			name:      "no devices",
			devices:   []netbox.Device{},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, count, err := GenerateBlackboxTargets(tc.devices)
			if err != nil {
				t.Fatalf("GenerateBlackboxTargets error: %v", err)
			}
			if count != tc.wantCount {
				t.Errorf("count = %d, want %d", count, tc.wantCount)
			}
			for _, probe := range tc.wantProbes {
				found := false
				for filename := range result {
					if strings.Contains(filename, probe) {
						found = true
						break
					}
				}
				if !found && count > 0 {
					t.Errorf("expected probe file for %q", probe)
				}
			}
		})
	}
}

func TestBuildLabels(t *testing.T) {
	d := netbox.Device{
		Name:           "test-device",
		Site:           "dc1",
		Region:         "us-east",
		Manufacturer:   "arista",
		Platform:       "eos",
		Role:           "router",
		MonitoringTier: "premium",
	}

	labels := BuildLabels(d)

	expected := map[string]string{
		"device":   "test-device",
		"site":     "dc1",
		"region":   "us-east",
		"vendor":   "arista",
		"platform": "eos",
		"role":     "router",
		"tier":     "premium",
	}

	for key, want := range expected {
		if got := labels[key]; got != want {
			t.Errorf("labels[%q] = %q, want %q", key, got, want)
		}
	}

	// Verify all LabelTaxonomy keys are present
	for _, key := range LabelTaxonomy {
		if _, ok := labels[key]; !ok {
			t.Errorf("LabelTaxonomy key %q missing from BuildLabels output", key)
		}
	}
}

func TestDefaultSNMPModule(t *testing.T) {
	tests := []struct {
		manufacturer string
		platform     string
		want         string
	}{
		{"arista", "eos", "arista_eos"},
		{"cisco", "iosxe", "cisco_iosxe"},
		{"cisco", "nxos", "cisco_nxos"},
		{"cisco", "ios", "cisco_ios"},
		{"juniper", "junos", "juniper_junos"},
		{"paloalto", "panos", "paloalto_panos"},
		{"unknown", "unknown", "if_mib"},
		{"", "", "if_mib"},
	}

	for _, tc := range tests {
		t.Run(tc.manufacturer+"/"+tc.platform, func(t *testing.T) {
			got := defaultSNMPModule(tc.manufacturer, tc.platform)
			if got != tc.want {
				t.Errorf("defaultSNMPModule(%q, %q) = %q, want %q", tc.manufacturer, tc.platform, got, tc.want)
			}
		})
	}
}
