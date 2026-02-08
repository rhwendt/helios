package gnmic

import (
	"context"
	"fmt"
	"io"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// SubscribeHandler is called for each subscription response.
type SubscribeHandler func(*gnmipb.SubscribeResponse) error

// Subscribe creates a streaming gNMI subscription for validation.
func (c *Client) Subscribe(ctx context.Context, paths []string, mode gnmipb.SubscriptionList_Mode, handler SubscribeHandler) error {
	if c.gnmiClient == nil {
		return fmt.Errorf("client not connected")
	}

	var subscriptions []*gnmipb.Subscription
	for _, p := range paths {
		path, err := parsePath(p)
		if err != nil {
			return fmt.Errorf("invalid path %q: %w", p, err)
		}
		subscriptions = append(subscriptions, &gnmipb.Subscription{
			Path: path,
			Mode: gnmipb.SubscriptionMode_TARGET_DEFINED,
		})
	}

	subReq := &gnmipb.SubscribeRequest{
		Request: &gnmipb.SubscribeRequest_Subscribe{
			Subscribe: &gnmipb.SubscriptionList{
				Subscription: subscriptions,
				Mode:         mode,
				Encoding:     gnmipb.Encoding_JSON_IETF,
			},
		},
	}

	stream, err := c.gnmiClient.Subscribe(ctx)
	if err != nil {
		return fmt.Errorf("failed to create subscribe stream: %w", err)
	}

	if err := stream.Send(subReq); err != nil {
		return fmt.Errorf("failed to send subscribe request: %w", err)
	}

	c.log.Info("gNMI Subscribe started", "paths", len(paths), "mode", mode.String())

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("subscribe stream error: %w", err)
		}
		if err := handler(resp); err != nil {
			return err
		}
	}
}
