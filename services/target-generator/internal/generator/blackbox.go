package generator

import (
	"encoding/json"
	"fmt"

	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
)

// GenerateBlackboxTargets converts NetBox devices to Prometheus file_sd JSON for blackbox_exporter.
// Returns separate target lists per probe type (icmp, tcp_connect, http_2xx).
func GenerateBlackboxTargets(devices []netbox.Device) (map[string][]byte, int, error) {
	probeTargets := make(map[string][]PrometheusFileSDEntry)
	count := 0

	for _, d := range devices {
		if d.PrimaryIP == "" {
			continue
		}

		probes := d.CustomFields.BlackboxProbes
		if len(probes) == 0 {
			probes = []string{"icmp"}
		}

		for _, probe := range probes {
			target := targetForProbe(d, probe)
			if target == "" {
				continue
			}

			entry := PrometheusFileSDEntry{
				Targets: []string{target},
				Labels: map[string]string{
					"device":         d.Name,
					"site":           d.Site,
					"region":         d.Region,
					"__param_module": probe,
				},
			}
			probeTargets[probe] = append(probeTargets[probe], entry)
			count++
		}
	}

	result := make(map[string][]byte)
	for probe, entries := range probeTargets {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling blackbox targets for probe %s: %w", probe, err)
		}
		filename := fmt.Sprintf("blackbox-%s-targets.json", probe)
		result[filename] = data
	}

	return result, count, nil
}

func targetForProbe(d netbox.Device, probe string) string {
	switch probe {
	case "icmp":
		return d.PrimaryIP
	case "tcp_connect":
		return fmt.Sprintf("%s:22", d.PrimaryIP)
	case "http_2xx":
		return fmt.Sprintf("https://%s", d.PrimaryIP)
	default:
		return d.PrimaryIP
	}
}
