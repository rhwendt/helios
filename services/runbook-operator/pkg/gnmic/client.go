package gnmic

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// Client manages gNMI connections to network devices.
type Client struct {
	address    string
	username   string
	password   string
	tlsConfig  *tls.Config
	conn       *grpc.ClientConn
	gnmiClient gnmipb.GNMIClient
	log        *slog.Logger
	timeout    time.Duration
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithTLS configures TLS for the client.
func WithTLS(config *tls.Config) ClientOption {
	return func(c *Client) {
		c.tlsConfig = config
	}
}

// WithTimeout sets the default timeout for operations.
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = timeout
	}
}

// NewClient creates a new gNMI client.
func NewClient(address, username, password string, log *slog.Logger, opts ...ClientOption) *Client {
	c := &Client{
		address:  address,
		username: username,
		password: password,
		log:      log,
		timeout:  30 * time.Second,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes a gRPC connection to the device.
func (c *Client) Connect(ctx context.Context) error {
	if c.tlsConfig == nil {
		// TODO: Source TLS credentials from K8s Secrets via ESO
		return fmt.Errorf("TLS configuration is required for gNMI connections to %s", c.address)
	}
	transportCreds := credentials.NewTLS(c.tlsConfig)

	dialCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, c.address,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.address, err)
	}

	c.conn = conn
	c.gnmiClient = gnmipb.NewGNMIClient(conn)
	c.log.Info("connected to device", "address", c.address)
	return nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GNMIClient returns the underlying gNMI client.
func (c *Client) GNMIClient() gnmipb.GNMIClient {
	return c.gnmiClient
}
