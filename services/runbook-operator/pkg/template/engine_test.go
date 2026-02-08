package template

import (
	"strings"
	"testing"
)

func TestEngine_Render(t *testing.T) {
	tests := []struct {
		name     string
		template string
		params   map[string]interface{}
		want     string
		wantErr  bool
	}{
		{
			name:     "simple string parameter",
			template: "interface {{ .interface_name }}",
			params:   map[string]interface{}{"interface_name": "Ethernet1"},
			want:     "interface Ethernet1",
		},
		{
			name:     "multiple parameters",
			template: "device={{ .device }} interface={{ .interface }}",
			params:   map[string]interface{}{"device": "router-1", "interface": "Ethernet1"},
			want:     "device=router-1 interface=Ethernet1",
		},
		{
			name:     "nested map access",
			template: "target={{ .target.address }}:{{ .target.port }}",
			params: map[string]interface{}{
				"target": map[string]interface{}{"address": "10.0.0.1", "port": "6030"},
			},
			want: "target=10.0.0.1:6030",
		},
		{
			name:     "conditional true",
			template: "{{if .enabled}}active{{else}}inactive{{end}}",
			params:   map[string]interface{}{"enabled": true},
			want:     "active",
		},
		{
			name:     "conditional false",
			template: "{{if .enabled}}active{{else}}inactive{{end}}",
			params:   map[string]interface{}{"enabled": false},
			want:     "inactive",
		},
		{
			name:     "range over list",
			template: "{{range .interfaces}}[{{.}}]{{end}}",
			params:   map[string]interface{}{"interfaces": []string{"Eth1", "Eth2", "Eth3"}},
			want:     "[Eth1][Eth2][Eth3]",
		},
		{
			name:     "default function with empty value",
			template: `{{ default "fallback" .value }}`,
			params:   map[string]interface{}{"value": ""},
			want:     "fallback",
		},
		{
			name:     "default function with non-empty value",
			template: `{{ default "fallback" .value }}`,
			params:   map[string]interface{}{"value": "actual"},
			want:     "actual",
		},
		{
			name:     "default function with nil value",
			template: `{{ default "fallback" .missing }}`,
			params:   map[string]interface{}{},
			want:     "fallback",
		},
		{
			name:     "invalid template syntax",
			template: "{{ .unclosed",
			params:   map[string]interface{}{},
			wantErr:  true,
		},
		{
			name:     "empty template",
			template: "",
			params:   map[string]interface{}{},
			want:     "",
		},
	}

	engine := NewEngine()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := engine.Render(tc.template, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.want {
				t.Errorf("Render() = %q, want %q", result, tc.want)
			}
		})
	}
}

func TestEngine_Validate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		wantErr  bool
	}{
		{
			name:     "valid simple template",
			template: "Hello {{ .name }}",
			wantErr:  false,
		},
		{
			name:     "valid conditional",
			template: "{{if .enabled}}yes{{end}}",
			wantErr:  false,
		},
		{
			name:     "valid range",
			template: "{{range .items}}{{.}}{{end}}",
			wantErr:  false,
		},
		{
			name:     "valid with default function",
			template: `{{ default "val" .x }}`,
			wantErr:  false,
		},
		{
			name:     "invalid unclosed action",
			template: "{{ .unclosed",
			wantErr:  true,
		},
		{
			name:     "invalid unknown function",
			template: "{{ unknownFunc .x }}",
			wantErr:  true,
		},
		{
			name:     "empty string is valid",
			template: "",
			wantErr:  false,
		},
	}

	engine := NewEngine()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := engine.Validate(tc.template)
			if tc.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestEngine_RenderConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		params  map[string]interface{}
		wantKey string
		wantVal string
		wantErr bool
	}{
		{
			name:    "renders string value",
			config:  map[string]interface{}{"target": "{{ .device }}:6030"},
			params:  map[string]interface{}{"device": "10.0.0.1"},
			wantKey: "target",
			wantVal: "10.0.0.1:6030",
		},
		{
			name:    "passes through non-string values",
			config:  map[string]interface{}{"count": 42},
			params:  map[string]interface{}{},
			wantKey: "count",
			wantVal: "",
		},
		{
			name: "renders nested maps",
			config: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "{{ .val }}",
				},
			},
			params:  map[string]interface{}{"val": "rendered"},
			wantKey: "outer",
		},
		{
			name:    "invalid template in config",
			config:  map[string]interface{}{"bad": "{{ .unclosed"},
			params:  map[string]interface{}{},
			wantErr: true,
		},
	}

	engine := NewEngine()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := engine.RenderConfig(tc.config, tc.params)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantVal != "" {
				if got, ok := result[tc.wantKey].(string); !ok || got != tc.wantVal {
					t.Errorf("result[%q] = %v, want %q", tc.wantKey, result[tc.wantKey], tc.wantVal)
				}
			}
		})
	}
}

func TestEngine_Render_SecurityEdgeCases(t *testing.T) {
	engine := NewEngine()

	t.Run("HTML-like content is not escaped in text/template", func(t *testing.T) {
		result, err := engine.Render("{{ .val }}", map[string]interface{}{"val": "<script>alert('xss')</script>"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "<script>") {
			t.Error("text/template should not escape HTML")
		}
	})

	t.Run("large parameter value", func(t *testing.T) {
		largeVal := strings.Repeat("x", 10000)
		result, err := engine.Render("{{ .val }}", map[string]interface{}{"val": largeVal})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 10000 {
			t.Errorf("expected result length 10000, got %d", len(result))
		}
	})
}
