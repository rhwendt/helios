package gnmic

import (
	"context"
	"encoding/json"
	"fmt"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// SetOperation defines a gNMI Set operation type.
type SetOperation string

const (
	SetUpdate  SetOperation = "update"
	SetReplace SetOperation = "replace"
	SetDelete  SetOperation = "delete"
)

// SetRequest represents a gNMI Set request.
type SetRequest struct {
	Operation SetOperation
	Path      string
	Value     interface{}
}

// Set performs a gNMI Set operation with update, replace, or delete.
func (c *Client) Set(ctx context.Context, requests []SetRequest) (*gnmipb.SetResponse, error) {
	if c.gnmiClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	setReq := &gnmipb.SetRequest{}

	for _, req := range requests {
		path, err := parsePath(req.Path)
		if err != nil {
			return nil, fmt.Errorf("invalid path %q: %w", req.Path, err)
		}

		switch req.Operation {
		case SetUpdate:
			typedVal, err := encodeValue(req.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to encode value: %w", err)
			}
			setReq.Update = append(setReq.Update, &gnmipb.Update{
				Path: path,
				Val:  typedVal,
			})
		case SetReplace:
			typedVal, err := encodeValue(req.Value)
			if err != nil {
				return nil, fmt.Errorf("failed to encode value: %w", err)
			}
			setReq.Replace = append(setReq.Replace, &gnmipb.Update{
				Path: path,
				Val:  typedVal,
			})
		case SetDelete:
			setReq.Delete = append(setReq.Delete, path)
		default:
			return nil, fmt.Errorf("unknown operation: %s", req.Operation)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.gnmiClient.Set(ctx, setReq)
	if err != nil {
		return nil, fmt.Errorf("gNMI Set failed: %w", err)
	}

	c.log.Info("gNMI Set completed", "updates", len(setReq.Update), "replaces", len(setReq.Replace), "deletes", len(setReq.Delete))
	return resp, nil
}

func encodeValue(value interface{}) (*gnmipb.TypedValue, error) {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &gnmipb.TypedValue{
		Value: &gnmipb.TypedValue_JsonIetfVal{
			JsonIetfVal: jsonBytes,
		},
	}, nil
}

func parsePath(pathStr string) (*gnmipb.Path, error) {
	path := &gnmipb.Path{}
	if pathStr == "" || pathStr == "/" {
		return path, nil
	}

	// Simple path parsing: split by '/'
	elements := splitPath(pathStr)
	for _, elem := range elements {
		if elem != "" {
			path.Elem = append(path.Elem, &gnmipb.PathElem{Name: elem})
		}
	}
	return path, nil
}

func splitPath(path string) []string {
	var result []string
	current := ""
	for _, ch := range path {
		if ch == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
