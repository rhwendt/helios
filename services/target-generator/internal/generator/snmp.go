package generator

import (
	"encoding/json"
	"fmt"

	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
)

// PrometheusFileSDEntry represents a Prometheus file_sd target group.
type PrometheusFileSDEntry struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// GenerateSNMPTargets converts NetBox devices to Prometheus file_sd JSON for snmp_exporter.
func GenerateSNMPTargets(devices []netbox.Device) ([]byte, int, error) {
	var entries []PrometheusFileSDEntry
	count := 0

	for _, d := range devices {
		if !d.CustomFields.SNMPEnabled || d.PrimaryIP == "" {
			continue
		}

		module := d.CustomFields.SNMPModule
		if module == "" {
			module = defaultSNMPModule(d.Manufacturer, d.Platform)
		}

		labels := map[string]string{
			"device":         d.Name,
			"site":           d.Site,
			"region":         d.Region,
			"vendor":         d.Manufacturer,
			"platform":       d.Platform,
			"role":           d.Role,
			"tier":           d.MonitoringTier,
			"__param_module": module,
		}

		entries = append(entries, PrometheusFileSDEntry{
			Targets: []string{d.PrimaryIP},
			Labels:  labels,
		})
		count++
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling SNMP targets: %w", err)
	}

	return data, count, nil
}

func defaultSNMPModule(manufacturer, platform string) string {
	switch manufacturer {
	case "arista":
		return "arista_eos"
	case "cisco":
		if platform == "iosxe" {
			return "cisco_iosxe"
		}
		if platform == "nxos" {
			return "cisco_nxos"
		}
		return "cisco_ios"
	case "juniper":
		return "juniper_junos"
	case "paloalto":
		return "paloalto_panos"
	default:
		return "if_mib"
	}
}
