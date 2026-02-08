package generator

import (
	"fmt"

	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
	"sigs.k8s.io/yaml"
)

// GNMICTarget represents a single gnmic target entry.
type GNMICTarget struct {
	Address       string            `json:"address" yaml:"address"`
	Labels        map[string]string `json:"labels" yaml:"labels"`
	Subscriptions []string          `json:"subscriptions" yaml:"subscriptions"`
}

// GNMICTargets is the top-level gnmic targets config.
type GNMICTargets struct {
	Targets map[string]GNMICTarget `json:"targets" yaml:"targets"`
}

// GenerateGNMICTargets converts NetBox devices to gnmic target YAML format.
func GenerateGNMICTargets(devices []netbox.Device) ([]byte, int, error) {
	targets := GNMICTargets{
		Targets: make(map[string]GNMICTarget),
	}

	count := 0
	for _, d := range devices {
		if !d.CustomFields.GNMIEnabled || d.PrimaryIP == "" {
			continue
		}

		port := d.CustomFields.GNMIPort
		if port == 0 {
			port = 6030
		}

		key := fmt.Sprintf("%s:%d", d.Name, port)
		address := fmt.Sprintf("%s:%d", d.PrimaryIP, port)

		subs := defaultSubscriptions(d)

		targets.Targets[key] = GNMICTarget{
			Address: address,
			Labels: map[string]string{
				"device":   d.Name,
				"site":     d.Site,
				"region":   d.Region,
				"vendor":   d.Manufacturer,
				"platform": d.Platform,
				"role":     d.Role,
				"tier":     d.MonitoringTier,
			},
			Subscriptions: subs,
		}
		count++
	}

	data, err := yaml.Marshal(targets)
	if err != nil {
		return nil, 0, fmt.Errorf("marshaling gnmic targets: %w", err)
	}

	return data, count, nil
}

func defaultSubscriptions(d netbox.Device) []string {
	subs := []string{"default-counters", "default-system"}
	if d.TelemetryProfile != "" {
		subs = append(subs, d.TelemetryProfile)
	} else {
		subs = append(subs, "default-bgp")
	}
	return subs
}
