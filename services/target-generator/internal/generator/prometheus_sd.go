package generator

import (
	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
)

// LabelTaxonomy defines the standard Helios label set applied to all targets.
var LabelTaxonomy = []string{
	"device",
	"site",
	"region",
	"vendor",
	"platform",
	"role",
	"tier",
}

// BuildLabels constructs the standard Helios label set from a NetBox device.
func BuildLabels(d netbox.Device) map[string]string {
	return map[string]string{
		"device":   d.Name,
		"site":     d.Site,
		"region":   d.Region,
		"vendor":   d.Manufacturer,
		"platform": d.Platform,
		"role":     d.Role,
		"tier":     d.MonitoringTier,
	}
}
