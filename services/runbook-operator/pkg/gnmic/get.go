package gnmic

import (
	"context"
	"fmt"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// Get performs a single gNMI Get request.
func (c *Client) Get(ctx context.Context, paths []string) (*gnmipb.GetResponse, error) {
	if c.gnmiClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	var gnmiPaths []*gnmipb.Path
	for _, p := range paths {
		path, err := parsePath(p)
		if err != nil {
			return nil, fmt.Errorf("invalid path %q: %w", p, err)
		}
		gnmiPaths = append(gnmiPaths, path)
	}

	getReq := &gnmipb.GetRequest{
		Path:     gnmiPaths,
		Type:     gnmipb.GetRequest_ALL,
		Encoding: gnmipb.Encoding_JSON_IETF,
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.gnmiClient.Get(ctx, getReq)
	if err != nil {
		return nil, fmt.Errorf("gNMI Get failed: %w", err)
	}

	c.log.Info("gNMI Get completed", "paths", len(paths), "notifications", len(resp.Notification))
	return resp, nil
}

// Poll performs repeated Get requests until a condition is met or timeout expires.
func (c *Client) Poll(ctx context.Context, paths []string, interval time.Duration, retryUntil func(*gnmipb.GetResponse) bool) (*gnmipb.GetResponse, error) {
	deadline := time.After(c.timeout)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			return nil, fmt.Errorf("poll timeout exceeded")
		case <-ticker.C:
			resp, err := c.Get(ctx, paths)
			if err != nil {
				c.log.Warn("poll attempt failed", "error", err)
				continue
			}
			if retryUntil(resp) {
				return resp, nil
			}
		}
	}
}
