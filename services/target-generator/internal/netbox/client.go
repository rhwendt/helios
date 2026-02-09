package netbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// Device represents a NetBox device with Helios-specific custom fields.
type Device struct {
	ID               int               `json:"id"`
	Name             string            `json:"name"`
	PrimaryIP        string            `json:"primary_ip_address"`
	Site             string            `json:"site"`
	Region           string            `json:"region"`
	Role             string            `json:"role"`
	Platform         string            `json:"platform"`
	Manufacturer     string            `json:"manufacturer"`
	Status           string            `json:"status"`
	CustomFields     DeviceCustomFields `json:"custom_fields"`
	Tags             []string          `json:"tags"`
	TelemetryProfile string            `json:"telemetry_profile"`
	MonitoringTier   string            `json:"monitoring_tier"`
}

// DeviceCustomFields holds Helios-specific custom fields from NetBox.
type DeviceCustomFields struct {
	GNMIEnabled    bool     `json:"gnmi_enabled"`
	GNMIPort       int      `json:"gnmi_port"`
	SNMPEnabled    bool     `json:"snmp_enabled"`
	SNMPModule     string   `json:"snmp_module"`
	BlackboxProbes []string `json:"blackbox_probes"`
}

// Client queries NetBox for device inventory with Helios monitoring enabled.
type Client struct {
	baseURL    string
	apiToken   string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a NetBox API client.
func NewClient(baseURL, apiToken string, logger *slog.Logger) *Client {
	return &Client{
		baseURL:  baseURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// paginatedResponse represents NetBox paginated API response.
type paginatedResponse struct {
	Count    int              `json:"count"`
	Next     *string          `json:"next"`
	Previous *string          `json:"previous"`
	Results  []json.RawMessage `json:"results"`
}

// ListMonitoredDevices returns all devices with helios_monitor=true custom field.
func (c *Client) ListMonitoredDevices(ctx context.Context) ([]Device, error) {
	var allDevices []Device
	nextURL := fmt.Sprintf("%s/api/dcim/devices/?cf_helios_monitor=true&status=active&limit=100", c.baseURL)

	for nextURL != "" {
		devices, next, err := c.fetchPage(ctx, nextURL)
		if err != nil {
			return nil, fmt.Errorf("fetching devices page: %w", err)
		}
		allDevices = append(allDevices, devices...)
		if next != nil {
			nextURL = *next
		} else {
			nextURL = ""
		}
	}

	c.logger.Info("fetched monitored devices from NetBox", "count", len(allDevices))
	return allDevices, nil
}

func (c *Client) fetchPage(ctx context.Context, rawURL string) ([]Device, *string, error) {
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var paginated paginatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&paginated); err != nil {
		return nil, nil, fmt.Errorf("decoding response: %w", err)
	}

	var devices []Device
	for _, raw := range paginated.Results {
		var d Device
		if err := json.Unmarshal(raw, &d); err != nil {
			c.logger.Warn("skipping device with unparseable data", "error", err)
			continue
		}
		devices = append(devices, d)
	}

	return devices, paginated.Next, nil
}
