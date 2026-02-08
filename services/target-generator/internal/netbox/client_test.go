package netbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestClient_ListMonitoredDevices(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantCount   int
		wantErr     bool
		wantName    string
		wantGNMI    bool
		wantSNMP    bool
	}{
		{
			name: "successful response with two devices",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Token test-token" {
					t.Error("missing or incorrect Authorization header")
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				resp := map[string]interface{}{
					"count": 2,
					"next":  nil,
					"results": []map[string]interface{}{
						{
							"id": 1, "name": "router-1", "primary_ip_address": "10.0.0.1",
							"site": "dc1", "role": "router", "manufacturer": "arista",
							"status": "active",
							"custom_fields": map[string]interface{}{
								"gnmi_enabled": true, "gnmi_port": 6030,
								"snmp_enabled": true, "snmp_module": "arista_sw",
							},
						},
						{
							"id": 2, "name": "switch-1", "primary_ip_address": "10.0.0.2",
							"site": "dc2", "role": "switch", "manufacturer": "cisco",
							"status": "active",
							"custom_fields": map[string]interface{}{
								"gnmi_enabled": false,
								"snmp_enabled": true, "snmp_module": "cisco_nxos",
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantCount: 2,
			wantName:  "router-1",
			wantGNMI:  true,
			wantSNMP:  true,
		},
		{
			name: "empty result set",
			handler: func(w http.ResponseWriter, r *http.Request) {
				resp := map[string]interface{}{
					"count": 0, "next": nil, "results": []interface{}{},
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			},
			wantCount: 0,
		},
		{
			name: "server error returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name: "invalid JSON returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("{invalid json"))
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			client := NewClient(server.URL, "test-token", testLogger())
			devices, err := client.ListMonitoredDevices(context.Background())

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(devices) != tc.wantCount {
				t.Fatalf("got %d devices, want %d", len(devices), tc.wantCount)
			}
			if tc.wantCount > 0 {
				if devices[0].Name != tc.wantName {
					t.Errorf("Name = %q, want %q", devices[0].Name, tc.wantName)
				}
				if devices[0].CustomFields.GNMIEnabled != tc.wantGNMI {
					t.Errorf("GNMIEnabled = %v, want %v", devices[0].CustomFields.GNMIEnabled, tc.wantGNMI)
				}
				if devices[0].CustomFields.SNMPEnabled != tc.wantSNMP {
					t.Errorf("SNMPEnabled = %v, want %v", devices[0].CustomFields.SNMPEnabled, tc.wantSNMP)
				}
			}
		})
	}
}

func TestClient_Pagination(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp map[string]interface{}

		if callCount == 1 {
			nextURL := "http://" + r.Host + "/api/dcim/devices/?offset=1&limit=1"
			resp = map[string]interface{}{
				"count": 2,
				"next":  nextURL,
				"results": []map[string]interface{}{
					{"id": 1, "name": "device-1", "primary_ip_address": "10.0.0.1", "custom_fields": map[string]interface{}{}},
				},
			}
		} else {
			resp = map[string]interface{}{
				"count": 2,
				"next":  nil,
				"results": []map[string]interface{}{
					{"id": 2, "name": "device-2", "primary_ip_address": "10.0.0.2", "custom_fields": map[string]interface{}{}},
				},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", testLogger())
	devices, err := client.ListMonitoredDevices(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestClient_AuthorizationHeader(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		resp := map[string]interface{}{"count": 0, "next": nil, "results": []interface{}{}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "my-secret-token", testLogger())
	client.ListMonitoredDevices(context.Background())

	if receivedAuth != "Token my-secret-token" {
		t.Errorf("Authorization header = %q, want %q", receivedAuth, "Token my-secret-token")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow server - context should cancel before response
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	client := NewClient(server.URL, "test-token", testLogger())
	_, err := client.ListMonitoredDevices(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
