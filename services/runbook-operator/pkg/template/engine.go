package template

import (
	"bytes"
	"fmt"
	"text/template"
)

// Engine renders Go templates for parameter substitution in runbook steps.
type Engine struct {
	funcMap template.FuncMap
}

// NewEngine creates a new template engine.
func NewEngine() *Engine {
	return &Engine{
		funcMap: template.FuncMap{
			"default": func(def, val interface{}) interface{} {
				if val == nil || val == "" {
					return def
				}
				return val
			},
		},
	}
}

// Render processes a template string with the given parameters.
func (e *Engine) Render(tmplStr string, params map[string]interface{}) (string, error) {
	tmpl, err := template.New("runbook").Funcs(e.funcMap).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// Validate checks if a template string is valid without executing it.
func (e *Engine) Validate(tmplStr string) error {
	_, err := template.New("validate").Funcs(e.funcMap).Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}
	return nil
}

// RenderConfig processes a map of config values, rendering any string values as templates.
func (e *Engine) RenderConfig(config map[string]interface{}, params map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for key, val := range config {
		switch v := val.(type) {
		case string:
			rendered, err := e.Render(v, params)
			if err != nil {
				return nil, fmt.Errorf("failed to render config key %q: %w", key, err)
			}
			result[key] = rendered
		case map[string]interface{}:
			nested, err := e.RenderConfig(v, params)
			if err != nil {
				return nil, err
			}
			result[key] = nested
		default:
			result[key] = val
		}
	}
	return result, nil
}
