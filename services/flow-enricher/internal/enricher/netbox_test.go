package enricher

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// mockNetBoxDevicesResponse builds a NetBox paginated response for devices.
func mockNetBoxDevicesResponse(devices []json.RawMessage, nextURL *string) []byte {
	resp := netboxPaginatedResponse{
		Count:   len(devices),
		Next:    nextURL,
		Results: devices,
	}
	b, _ := json.Marshal(resp)
	return b
}

// mustMarshal marshals v to JSON, panicking on error.
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func TestFetchDevices_SinglePage(t *testing.T) {
	mux := http.NewServeMux()

	deviceJSON := mustMarshal(map[string]any{
		"id":   1,
		"name": "router-1",
		"primary_ip": map[string]any{
			"address": "10.0.0.1/32",
		},
		"site": map[string]any{
			"name": "dc1",
		},
		"region": map[string]any{
			"name": "us-east",
		},
		"role": map[string]any{
			"name": "core-router",
		},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "application/json" {
			http.Error(w, "bad accept header", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{deviceJSON}, nil))
	})

	ifaceJSON := mustMarshal(map[string]any{
		"id":    101,
		"name":  "Ethernet1",
		"speed": 10000,
		"custom_fields": map[string]any{
			"snmp_index": 1,
		},
		"label": "",
	})
	ifaceJSON2 := mustMarshal(map[string]any{
		"id":            102,
		"name":          "Ethernet2",
		"speed":         25000,
		"custom_fields": nil,
		"label":         "2",
	})

	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{ifaceJSON, ifaceJSON2}, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())

	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}

	dev, ok := devices["10.0.0.1"]
	if !ok {
		t.Fatal("expected device keyed by '10.0.0.1' (CIDR stripped)")
	}

	if dev.Name != "router-1" {
		t.Errorf("Name = %q, want %q", dev.Name, "router-1")
	}
	if dev.Site != "dc1" {
		t.Errorf("Site = %q, want %q", dev.Site, "dc1")
	}
	if dev.Region != "us-east" {
		t.Errorf("Region = %q, want %q", dev.Region, "us-east")
	}
	if dev.Role != "core-router" {
		t.Errorf("Role = %q, want %q", dev.Role, "core-router")
	}

	if len(dev.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces, got %d", len(dev.Interfaces))
	}
	if iface, ok := dev.Interfaces[1]; !ok {
		t.Error("expected interface with SNMP index 1")
	} else {
		if iface.Name != "Ethernet1" {
			t.Errorf("Interface[1].Name = %q, want %q", iface.Name, "Ethernet1")
		}
		if iface.Speed != 10000 {
			t.Errorf("Interface[1].Speed = %d, want 10000", iface.Speed)
		}
	}
	if iface, ok := dev.Interfaces[2]; !ok {
		t.Error("expected interface with SNMP index 2 (from label)")
	} else {
		if iface.Name != "Ethernet2" {
			t.Errorf("Interface[2].Name = %q, want %q", iface.Name, "Ethernet2")
		}
		if iface.Speed != 25000 {
			t.Errorf("Interface[2].Speed = %d, want 25000", iface.Speed)
		}
	}
}

func TestFetchDevices_Pagination(t *testing.T) {
	mux := http.NewServeMux()
	callCount := 0

	device1 := mustMarshal(map[string]any{
		"id":   1,
		"name": "router-1",
		"primary_ip": map[string]any{
			"address": "10.0.0.1/32",
		},
		"site":   map[string]any{"name": "dc1"},
		"region": map[string]any{"name": "us-east"},
		"role":   map[string]any{"name": "core"},
	})
	device2 := mustMarshal(map[string]any{
		"id":   2,
		"name": "router-2",
		"primary_ip": map[string]any{
			"address": "10.0.0.2/24",
		},
		"site":   map[string]any{"name": "dc2"},
		"region": map[string]any{"name": "eu-west"},
		"role":   map[string]any{"name": "edge"},
	})

	var srv *httptest.Server
	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			page2URL := fmt.Sprintf("%s/api/dcim/devices/?cf_helios_monitor=true&status=active&limit=100&offset=100", srv.URL)
			_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device1}, &page2URL))
		} else {
			_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device2}, nil))
		}
	})

	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	})

	srv = httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices (from 2 pages), got %d", len(devices))
	}
	if _, ok := devices["10.0.0.1"]; !ok {
		t.Error("missing device 10.0.0.1 from page 1")
	}
	if _, ok := devices["10.0.0.2"]; !ok {
		t.Error("missing device 10.0.0.2 from page 2")
	}
	if callCount != 2 {
		t.Errorf("expected 2 device page fetches, got %d", callCount)
	}
}

func TestFetchDevices_SkipsDeviceWithoutPrimaryIP(t *testing.T) {
	mux := http.NewServeMux()

	deviceNoPrimaryIP := mustMarshal(map[string]any{
		"id":         1,
		"name":       "no-ip-device",
		"primary_ip": nil,
		"site":       map[string]any{"name": "dc1"},
	})
	deviceWithIP := mustMarshal(map[string]any{
		"id":   2,
		"name": "has-ip-device",
		"primary_ip": map[string]any{
			"address": "10.0.0.5/32",
		},
		"site": map[string]any{"name": "dc1"},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{deviceNoPrimaryIP, deviceWithIP}, nil))
	})
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device (skip one without IP), got %d", len(devices))
	}
	if _, ok := devices["10.0.0.5"]; !ok {
		t.Error("expected device with IP 10.0.0.5")
	}
}

func TestFetchDevices_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"Not found."}`, http.StatusNotFound)
	}))
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	_, err := cache.fetchDevices(context.Background())
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "unexpected status 404") {
		t.Errorf("expected status 404 in error, got: %v", err)
	}
}

func TestFetchDevices_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	_, err := cache.fetchDevices(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decoding response") {
		t.Errorf("expected 'decoding response' in error, got: %v", err)
	}
}

func TestFetchDevices_MalformedDeviceJSON(t *testing.T) {
	mux := http.NewServeMux()

	// A device result that is not a valid JSON object for netboxDevice.
	badDevice := json.RawMessage(`"just a string"`)
	goodDevice := mustMarshal(map[string]any{
		"id":   2,
		"name": "good-device",
		"primary_ip": map[string]any{
			"address": "10.0.0.10/32",
		},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{badDevice, goodDevice}, nil))
	})
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v (should skip bad device, not fail)", err)
	}

	if len(devices) != 1 {
		t.Fatalf("expected 1 device (skipping malformed), got %d", len(devices))
	}
}

func TestFetchDevices_NilNestedFields(t *testing.T) {
	mux := http.NewServeMux()

	device := mustMarshal(map[string]any{
		"id":   1,
		"name": "minimal-device",
		"primary_ip": map[string]any{
			"address": "10.0.0.99",
		},
		"site":   nil,
		"region": nil,
		"role":   nil,
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device}, nil))
	})
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	dev, ok := devices["10.0.0.99"]
	if !ok {
		t.Fatal("expected device keyed by '10.0.0.99'")
	}
	if dev.Name != "minimal-device" {
		t.Errorf("Name = %q, want %q", dev.Name, "minimal-device")
	}
	if dev.Site != "" {
		t.Errorf("Site = %q, want empty (nil site)", dev.Site)
	}
	if dev.Region != "" {
		t.Errorf("Region = %q, want empty (nil region)", dev.Region)
	}
	if dev.Role != "" {
		t.Errorf("Role = %q, want empty (nil role)", dev.Role)
	}
}

func TestFetchDevices_InterfaceSNMPIndexFromLabel(t *testing.T) {
	mux := http.NewServeMux()

	device := mustMarshal(map[string]any{
		"id":   1,
		"name": "label-device",
		"primary_ip": map[string]any{
			"address": "10.1.1.1/32",
		},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device}, nil))
	})

	ifaceLabelOnly := mustMarshal(map[string]any{
		"id":            201,
		"name":          "GigabitEthernet0/0",
		"speed":         1000,
		"custom_fields": map[string]any{},
		"label":         "42",
	})
	ifaceNoIndex := mustMarshal(map[string]any{
		"id":            202,
		"name":          "Loopback0",
		"speed":         nil,
		"custom_fields": nil,
		"label":         "",
	})
	ifaceNonNumericLabel := mustMarshal(map[string]any{
		"id":            203,
		"name":          "Management0",
		"speed":         100,
		"custom_fields": nil,
		"label":         "mgmt",
	})

	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{ifaceLabelOnly, ifaceNoIndex, ifaceNonNumericLabel}, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	dev := devices["10.1.1.1"]
	// Only the interface with label "42" should be present; others should be skipped.
	if len(dev.Interfaces) != 1 {
		t.Fatalf("expected 1 interface (label-based), got %d", len(dev.Interfaces))
	}
	iface, ok := dev.Interfaces[42]
	if !ok {
		t.Fatal("expected interface with SNMP index 42")
	}
	if iface.Name != "GigabitEthernet0/0" {
		t.Errorf("Name = %q, want %q", iface.Name, "GigabitEthernet0/0")
	}
}

func TestFetchDevices_InterfaceAPIError(t *testing.T) {
	mux := http.NewServeMux()

	device := mustMarshal(map[string]any{
		"id":   1,
		"name": "iface-error-device",
		"primary_ip": map[string]any{
			"address": "10.2.2.2/32",
		},
		"site": map[string]any{"name": "dc1"},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device}, nil))
	})
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v (should handle interface errors gracefully)", err)
	}

	// Device should still be present but with empty interfaces.
	dev, ok := devices["10.2.2.2"]
	if !ok {
		t.Fatal("expected device even with interface fetch failure")
	}
	if dev.Name != "iface-error-device" {
		t.Errorf("Name = %q, want %q", dev.Name, "iface-error-device")
	}
	if len(dev.Interfaces) != 0 {
		t.Errorf("expected 0 interfaces on error, got %d", len(dev.Interfaces))
	}
}

func TestFetchDevices_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server — the context should cancel before completion.
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := cache.fetchDevices(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestFetchDevices_AuthorizationHeader(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	}))
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "my-secret-token", time.Minute, newTestLogger())
	_, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	expected := "Token my-secret-token"
	if receivedAuth != expected {
		t.Errorf("Authorization header = %q, want %q", receivedAuth, expected)
	}
}

func TestStripCIDR(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"10.0.0.1/32", "10.0.0.1"},
		{"10.0.0.1/24", "10.0.0.1"},
		{"192.168.1.1/16", "192.168.1.1"},
		{"10.0.0.1", "10.0.0.1"},
		{"2001:db8::1/128", "2001:db8::1"},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := stripCIDR(tc.input)
			if got != tc.want {
				t.Errorf("stripCIDR(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFetchDevices_InterfacePagination(t *testing.T) {
	mux := http.NewServeMux()

	device := mustMarshal(map[string]any{
		"id":   1,
		"name": "paginated-iface-device",
		"primary_ip": map[string]any{
			"address": "10.3.3.3/32",
		},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device}, nil))
	})

	ifaceCallCount := 0
	var srv *httptest.Server
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		ifaceCallCount++
		w.Header().Set("Content-Type", "application/json")
		if ifaceCallCount == 1 {
			iface1 := mustMarshal(map[string]any{
				"id":    301,
				"name":  "eth0",
				"speed": 10000,
				"custom_fields": map[string]any{
					"snmp_index": 1,
				},
			})
			page2URL := fmt.Sprintf("%s/api/dcim/interfaces/?device_id=1&limit=100&offset=100", srv.URL)
			_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{iface1}, &page2URL))
		} else {
			iface2 := mustMarshal(map[string]any{
				"id":    302,
				"name":  "eth1",
				"speed": 25000,
				"custom_fields": map[string]any{
					"snmp_index": 2,
				},
			})
			_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{iface2}, nil))
		}
	})

	srv = httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	dev := devices["10.3.3.3"]
	if len(dev.Interfaces) != 2 {
		t.Fatalf("expected 2 interfaces from 2 pages, got %d", len(dev.Interfaces))
	}
	if dev.Interfaces[1].Name != "eth0" {
		t.Errorf("Interface[1].Name = %q, want %q", dev.Interfaces[1].Name, "eth0")
	}
	if dev.Interfaces[2].Name != "eth1" {
		t.Errorf("Interface[2].Name = %q, want %q", dev.Interfaces[2].Name, "eth1")
	}
	if ifaceCallCount != 2 {
		t.Errorf("expected 2 interface page fetches, got %d", ifaceCallCount)
	}
}

func TestFetchDevices_RequestURLFormat(t *testing.T) {
	var receivedPath string
	var receivedQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/dcim/devices/") {
			receivedPath = r.URL.Path
			receivedQuery = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	}))
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	_, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if receivedPath != "/api/dcim/devices/" {
		t.Errorf("path = %q, want /api/dcim/devices/", receivedPath)
	}
	if !strings.Contains(receivedQuery, "cf_helios_monitor=true") {
		t.Errorf("query %q should contain cf_helios_monitor=true", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "status=active") {
		t.Errorf("query %q should contain status=active", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "limit=100") {
		t.Errorf("query %q should contain limit=100", receivedQuery)
	}
}

func TestFetchDevices_TrailingSlashInAPIURL(t *testing.T) {
	var receivedPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	}))
	defer srv.Close()

	// URL with trailing slash — should not produce double slash.
	cache := NewNetBoxCache(srv.URL+"/", "test-token", time.Minute, newTestLogger())
	_, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if strings.Contains(receivedPath, "//") {
		t.Errorf("path contains double slash: %q", receivedPath)
	}
}

func TestFetchDevices_IPWithoutCIDR(t *testing.T) {
	mux := http.NewServeMux()

	device := mustMarshal(map[string]any{
		"id":   1,
		"name": "bare-ip-device",
		"primary_ip": map[string]any{
			"address": "172.16.0.1",
		},
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse([]json.RawMessage{device}, nil))
	})
	mux.HandleFunc("/api/dcim/interfaces/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mockNetBoxDevicesResponse(nil, nil))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	cache := NewNetBoxCache(srv.URL, "test-token", time.Minute, newTestLogger())
	devices, err := cache.fetchDevices(context.Background())
	if err != nil {
		t.Fatalf("fetchDevices() error = %v", err)
	}

	if _, ok := devices["172.16.0.1"]; !ok {
		t.Error("expected device keyed by '172.16.0.1' (no CIDR to strip)")
	}
}
