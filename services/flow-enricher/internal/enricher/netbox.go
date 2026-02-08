package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DeviceMetadata holds enrichment data for a network device.
type DeviceMetadata struct {
	Name       string
	Site       string
	Region     string
	Role       string
	Interfaces map[uint32]InterfaceMetadata // keyed by SNMP index
}

// InterfaceMetadata holds enrichment data for a device interface.
type InterfaceMetadata struct {
	Name  string
	Speed uint64
}

// NetBoxCache provides device metadata lookup by IP address.
type NetBoxCache struct {
	mu      sync.RWMutex
	devices map[string]DeviceMetadata // keyed by management IP

	apiURL   string
	apiToken string
	interval time.Duration
	logger   *slog.Logger
}

// NewNetBoxCache creates a new NetBox cache with the given configuration.
func NewNetBoxCache(apiURL, apiToken string, refreshInterval time.Duration, logger *slog.Logger) *NetBoxCache {
	return &NetBoxCache{
		devices:  make(map[string]DeviceMetadata),
		apiURL:   apiURL,
		apiToken: apiToken,
		interval: refreshInterval,
		logger:   logger,
	}
}

// Start begins periodic cache refresh. It blocks until the context is cancelled.
func (c *NetBoxCache) Start(ctx context.Context) error {
	// Initial load
	if err := c.refresh(ctx); err != nil {
		c.logger.Error("initial NetBox cache refresh failed", "error", err)
	}

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				c.logger.Error("NetBox cache refresh failed", "error", err)
			}
		}
	}
}

// LookupByIP returns device metadata for the given IP address.
func (c *NetBoxCache) LookupByIP(ip net.IP) (DeviceMetadata, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	meta, ok := c.devices[ip.String()]
	return meta, ok
}

// DeviceCount returns the number of devices in the cache.
func (c *NetBoxCache) DeviceCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.devices)
}

// refresh fetches all devices from NetBox and rebuilds the cache.
func (c *NetBoxCache) refresh(ctx context.Context) error {
	c.logger.Info("refreshing NetBox device cache")
	start := time.Now()

	devices, err := c.fetchDevices(ctx)
	if err != nil {
		return fmt.Errorf("fetching devices from NetBox: %w", err)
	}

	c.mu.Lock()
	c.devices = devices
	c.mu.Unlock()

	c.logger.Info("NetBox cache refreshed",
		"devices", len(devices),
		"duration", time.Since(start),
	)
	return nil
}

// netboxPaginatedResponse represents a paginated response from the NetBox API.
type netboxPaginatedResponse struct {
	Count    int               `json:"count"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
	Results  []json.RawMessage `json:"results"`
}

// netboxDevice represents the relevant fields from a NetBox device API response.
type netboxDevice struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	PrimaryIP *struct {
		Address string `json:"address"`
	} `json:"primary_ip"`
	Site *struct {
		Name string `json:"name"`
	} `json:"site"`
	Region *struct {
		Name string `json:"name"`
	} `json:"region"`
	Role *struct {
		Name string `json:"name"`
	} `json:"role"`
}

// netboxInterface represents the relevant fields from a NetBox interface API response.
type netboxInterface struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Speed        *int   `json:"speed"` // in kbps from NetBox
	CustomFields *struct {
		SNMPIndex *int `json:"snmp_index"`
	} `json:"custom_fields"`
	Label string `json:"label"`
}

// httpClient returns an *http.Client with a reasonable timeout.
func (c *NetBoxCache) httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// fetchDevices queries the NetBox API for all devices with helios_monitor=true.
// Returns a map keyed by management IP.
func (c *NetBoxCache) fetchDevices(ctx context.Context) (map[string]DeviceMetadata, error) {
	client := c.httpClient()
	devices := make(map[string]DeviceMetadata)

	// Fetch all monitored devices with pagination.
	nextURL := fmt.Sprintf("%s/api/dcim/devices/?cf_helios_monitor=true&status=active&limit=100", strings.TrimRight(c.apiURL, "/"))

	for nextURL != "" {
		rawDevices, next, err := c.fetchPage(ctx, client, nextURL)
		if err != nil {
			return nil, fmt.Errorf("fetching devices page: %w", err)
		}

		for _, raw := range rawDevices {
			var d netboxDevice
			if err := json.Unmarshal(raw, &d); err != nil {
				c.logger.Warn("skipping device with unparseable data", "error", err)
				continue
			}

			if d.PrimaryIP == nil || d.PrimaryIP.Address == "" {
				c.logger.Warn("skipping device without primary IP", "device", d.Name, "id", d.ID)
				continue
			}

			// Strip CIDR notation (e.g. "10.0.0.1/32" -> "10.0.0.1").
			mgmtIP := stripCIDR(d.PrimaryIP.Address)

			meta := DeviceMetadata{
				Name:       d.Name,
				Interfaces: make(map[uint32]InterfaceMetadata),
			}
			if d.Site != nil {
				meta.Site = d.Site.Name
			}
			if d.Region != nil {
				meta.Region = d.Region.Name
			}
			if d.Role != nil {
				meta.Role = d.Role.Name
			}

			// Fetch interfaces for this device.
			ifaces, err := c.fetchInterfaces(ctx, client, d.ID)
			if err != nil {
				c.logger.Warn("failed to fetch interfaces for device", "device", d.Name, "id", d.ID, "error", err)
				// Continue with empty interfaces rather than failing the entire refresh.
			} else {
				meta.Interfaces = ifaces
			}

			devices[mgmtIP] = meta
		}

		if next != nil {
			nextURL = *next
		} else {
			nextURL = ""
		}
	}

	return devices, nil
}

// fetchInterfaces retrieves all interfaces for a given device ID from NetBox.
func (c *NetBoxCache) fetchInterfaces(ctx context.Context, client *http.Client, deviceID int) (map[uint32]InterfaceMetadata, error) {
	interfaces := make(map[uint32]InterfaceMetadata)

	nextURL := fmt.Sprintf("%s/api/dcim/interfaces/?device_id=%d&limit=100", strings.TrimRight(c.apiURL, "/"), deviceID)

	for nextURL != "" {
		rawIfaces, next, err := c.fetchPage(ctx, client, nextURL)
		if err != nil {
			return nil, fmt.Errorf("fetching interfaces page: %w", err)
		}

		for _, raw := range rawIfaces {
			var iface netboxInterface
			if err := json.Unmarshal(raw, &iface); err != nil {
				c.logger.Warn("skipping interface with unparseable data", "error", err)
				continue
			}

			// Determine SNMP index: prefer custom_fields.snmp_index, fall back to parsing label.
			snmpIndex := uint32(0)
			if iface.CustomFields != nil && iface.CustomFields.SNMPIndex != nil {
				snmpIndex = uint32(*iface.CustomFields.SNMPIndex)
			} else if iface.Label != "" {
				// Try to parse label as SNMP index.
				var parsed int
				if _, err := fmt.Sscanf(iface.Label, "%d", &parsed); err == nil && parsed > 0 {
					snmpIndex = uint32(parsed)
				}
			}

			if snmpIndex == 0 {
				continue // No usable SNMP index — skip.
			}

			speed := uint64(0)
			if iface.Speed != nil {
				speed = uint64(*iface.Speed)
			}

			interfaces[snmpIndex] = InterfaceMetadata{
				Name:  iface.Name,
				Speed: speed,
			}
		}

		if next != nil {
			nextURL = *next
		} else {
			nextURL = ""
		}
	}

	return interfaces, nil
}

// fetchPage fetches a single page from the NetBox paginated API.
func (c *NetBoxCache) fetchPage(ctx context.Context, client *http.Client, rawURL string) ([]json.RawMessage, *string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var paginated netboxPaginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&paginated); err != nil {
		return nil, nil, fmt.Errorf("decoding response: %w", err)
	}

	return paginated.Results, paginated.Next, nil
}

// stripCIDR removes CIDR notation from an IP address string.
// e.g. "10.0.0.1/32" → "10.0.0.1", "10.0.0.1" → "10.0.0.1"
func stripCIDR(addr string) string {
	if idx := strings.IndexByte(addr, '/'); idx != -1 {
		return addr[:idx]
	}
	return addr
}
