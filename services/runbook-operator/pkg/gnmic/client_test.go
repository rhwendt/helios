package gnmic

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- Mock gNMI client ---

type mockGNMIClient struct {
	getFunc       func(ctx context.Context, in *gnmipb.GetRequest, opts ...grpc.CallOption) (*gnmipb.GetResponse, error)
	setFunc       func(ctx context.Context, in *gnmipb.SetRequest, opts ...grpc.CallOption) (*gnmipb.SetResponse, error)
	subscribeFunc func(ctx context.Context, opts ...grpc.CallOption) (gnmipb.GNMI_SubscribeClient, error)
	capFunc       func(ctx context.Context, in *gnmipb.CapabilityRequest, opts ...grpc.CallOption) (*gnmipb.CapabilityResponse, error)
}

func (m *mockGNMIClient) Get(ctx context.Context, in *gnmipb.GetRequest, opts ...grpc.CallOption) (*gnmipb.GetResponse, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, in, opts...)
	}
	return &gnmipb.GetResponse{}, nil
}

func (m *mockGNMIClient) Set(ctx context.Context, in *gnmipb.SetRequest, opts ...grpc.CallOption) (*gnmipb.SetResponse, error) {
	if m.setFunc != nil {
		return m.setFunc(ctx, in, opts...)
	}
	return &gnmipb.SetResponse{}, nil
}

func (m *mockGNMIClient) Subscribe(ctx context.Context, opts ...grpc.CallOption) (gnmipb.GNMI_SubscribeClient, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(ctx, opts...)
	}
	return nil, nil
}

func (m *mockGNMIClient) Capabilities(ctx context.Context, in *gnmipb.CapabilityRequest, opts ...grpc.CallOption) (*gnmipb.CapabilityResponse, error) {
	if m.capFunc != nil {
		return m.capFunc(ctx, in, opts...)
	}
	return &gnmipb.CapabilityResponse{}, nil
}

// --- Mock subscribe stream ---

type mockSubscribeStream struct {
	grpc.ClientStream
	responses []*gnmipb.SubscribeResponse
	idx       int
	sentReq   *gnmipb.SubscribeRequest
}

func (m *mockSubscribeStream) Send(req *gnmipb.SubscribeRequest) error {
	m.sentReq = req
	return nil
}

func (m *mockSubscribeStream) Recv() (*gnmipb.SubscribeResponse, error) {
	if m.idx >= len(m.responses) {
		return nil, io.EOF
	}
	resp := m.responses[m.idx]
	m.idx++
	return resp, nil
}

func (m *mockSubscribeStream) Header() (metadata.MD, error)  { return nil, nil }
func (m *mockSubscribeStream) Trailer() metadata.MD          { return nil }
func (m *mockSubscribeStream) CloseSend() error              { return nil }
func (m *mockSubscribeStream) Context() context.Context      { return context.Background() }
func (m *mockSubscribeStream) SendMsg(msg interface{}) error { return nil }
func (m *mockSubscribeStream) RecvMsg(msg interface{}) error { return nil }

// --- Tests ---

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())

	if c.address != "10.0.0.1:6030" {
		t.Errorf("address = %q, want %q", c.address, "10.0.0.1:6030")
	}
	if c.username != "admin" {
		t.Errorf("username = %q, want %q", c.username, "admin")
	}
	if c.password != "secret" {
		t.Errorf("password = %q, want %q", c.password, "secret")
	}
	if c.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", c.timeout, 30*time.Second)
	}
	if c.tlsConfig != nil {
		t.Error("tlsConfig should be nil by default")
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger(),
		WithTLS(tlsCfg),
		WithTimeout(5*time.Second),
	)

	if c.tlsConfig != tlsCfg {
		t.Error("WithTLS did not set tlsConfig")
	}
	if c.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", c.timeout, 5*time.Second)
	}
}

func TestClient_NotConnected(t *testing.T) {
	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())

	t.Run("Get fails when not connected", func(t *testing.T) {
		_, err := c.Get(context.Background(), []string{"/interfaces"})
		if err == nil {
			t.Fatal("expected error for unconnected client")
		}
		if !containsStr(err.Error(), "not connected") {
			t.Errorf("error = %q, want to contain 'not connected'", err.Error())
		}
	})

	t.Run("Set fails when not connected", func(t *testing.T) {
		_, err := c.Set(context.Background(), []SetRequest{
			{Operation: SetUpdate, Path: "/interfaces/interface", Value: "test"},
		})
		if err == nil {
			t.Fatal("expected error for unconnected client")
		}
		if !containsStr(err.Error(), "not connected") {
			t.Errorf("error = %q, want to contain 'not connected'", err.Error())
		}
	})

	t.Run("Subscribe fails when not connected", func(t *testing.T) {
		err := c.Subscribe(context.Background(), []string{"/interfaces"}, gnmipb.SubscriptionList_STREAM, nil)
		if err == nil {
			t.Fatal("expected error for unconnected client")
		}
		if !containsStr(err.Error(), "not connected") {
			t.Errorf("error = %q, want to contain 'not connected'", err.Error())
		}
	})
}

func TestClient_Close(t *testing.T) {
	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())

	// Close with nil connection should succeed
	if err := c.Close(); err != nil {
		t.Errorf("Close with nil conn: %v", err)
	}
}

func TestClient_Get(t *testing.T) {
	tests := []struct {
		name      string
		paths     []string
		mockResp  *gnmipb.GetResponse
		wantPaths int
		wantErr   bool
	}{
		{
			name:  "single path",
			paths: []string{"/interfaces/interface"},
			mockResp: &gnmipb.GetResponse{
				Notification: []*gnmipb.Notification{
					{
						Update: []*gnmipb.Update{
							{
								Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "interfaces"}}},
								Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"status":"up"}`)}},
							},
						},
					},
				},
			},
			wantPaths: 1,
		},
		{
			name:      "multiple paths",
			paths:     []string{"/interfaces", "/system/config"},
			mockResp:  &gnmipb.GetResponse{},
			wantPaths: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedReq *gnmipb.GetRequest
			mock := &mockGNMIClient{
				getFunc: func(ctx context.Context, in *gnmipb.GetRequest, opts ...grpc.CallOption) (*gnmipb.GetResponse, error) {
					capturedReq = in
					return tc.mockResp, nil
				},
			}

			c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())
			c.gnmiClient = mock

			resp, err := c.Get(context.Background(), tc.paths)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp == nil {
				t.Fatal("response should not be nil")
			}
			if len(capturedReq.Path) != tc.wantPaths {
				t.Errorf("request paths = %d, want %d", len(capturedReq.Path), tc.wantPaths)
			}
			if capturedReq.Encoding != gnmipb.Encoding_JSON_IETF {
				t.Errorf("encoding = %v, want JSON_IETF", capturedReq.Encoding)
			}
			if capturedReq.Type != gnmipb.GetRequest_ALL {
				t.Errorf("type = %v, want ALL", capturedReq.Type)
			}
		})
	}
}

func TestClient_Set(t *testing.T) {
	tests := []struct {
		name         string
		requests     []SetRequest
		wantUpdates  int
		wantReplaces int
		wantDeletes  int
		wantErr      bool
		errContains  string
	}{
		{
			name: "update operation",
			requests: []SetRequest{
				{Operation: SetUpdate, Path: "/interfaces/interface/config/enabled", Value: true},
			},
			wantUpdates: 1,
		},
		{
			name: "replace operation",
			requests: []SetRequest{
				{Operation: SetReplace, Path: "/interfaces/interface/config", Value: map[string]interface{}{"mtu": 9000}},
			},
			wantReplaces: 1,
		},
		{
			name: "delete operation",
			requests: []SetRequest{
				{Operation: SetDelete, Path: "/interfaces/interface/config/description"},
			},
			wantDeletes: 1,
		},
		{
			name: "mixed operations",
			requests: []SetRequest{
				{Operation: SetUpdate, Path: "/interfaces/interface/config/enabled", Value: true},
				{Operation: SetReplace, Path: "/interfaces/interface/config/mtu", Value: 9000},
				{Operation: SetDelete, Path: "/interfaces/interface/config/description"},
			},
			wantUpdates:  1,
			wantReplaces: 1,
			wantDeletes:  1,
		},
		{
			name: "unknown operation returns error",
			requests: []SetRequest{
				{Operation: "invalid", Path: "/test", Value: "val"},
			},
			wantErr:     true,
			errContains: "unknown operation",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedReq *gnmipb.SetRequest
			mock := &mockGNMIClient{
				setFunc: func(ctx context.Context, in *gnmipb.SetRequest, opts ...grpc.CallOption) (*gnmipb.SetResponse, error) {
					capturedReq = in
					return &gnmipb.SetResponse{}, nil
				},
			}

			c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())
			c.gnmiClient = mock

			_, err := c.Set(context.Background(), tc.requests)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.errContains != "" && !containsStr(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(capturedReq.Update) != tc.wantUpdates {
				t.Errorf("updates = %d, want %d", len(capturedReq.Update), tc.wantUpdates)
			}
			if len(capturedReq.Replace) != tc.wantReplaces {
				t.Errorf("replaces = %d, want %d", len(capturedReq.Replace), tc.wantReplaces)
			}
			if len(capturedReq.Delete) != tc.wantDeletes {
				t.Errorf("deletes = %d, want %d", len(capturedReq.Delete), tc.wantDeletes)
			}
		})
	}
}

func TestClient_Set_ValueEncoding(t *testing.T) {
	var capturedReq *gnmipb.SetRequest
	mock := &mockGNMIClient{
		setFunc: func(ctx context.Context, in *gnmipb.SetRequest, opts ...grpc.CallOption) (*gnmipb.SetResponse, error) {
			capturedReq = in
			return &gnmipb.SetResponse{}, nil
		},
	}

	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())
	c.gnmiClient = mock

	testVal := map[string]interface{}{"enabled": true, "mtu": 9000}
	_, err := c.Set(context.Background(), []SetRequest{
		{Operation: SetUpdate, Path: "/interfaces/interface/config", Value: testVal},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify value is JSON_IETF encoded
	update := capturedReq.Update[0]
	jsonVal := update.Val.GetJsonIetfVal()
	if jsonVal == nil {
		t.Fatal("expected JSON_IETF encoded value")
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(jsonVal, &decoded); err != nil {
		t.Fatalf("failed to unmarshal JSON_IETF value: %v", err)
	}
	if decoded["enabled"] != true {
		t.Errorf("decoded enabled = %v, want true", decoded["enabled"])
	}
}

func TestClient_Subscribe(t *testing.T) {
	syncResp := &gnmipb.SubscribeResponse{
		Response: &gnmipb.SubscribeResponse_SyncResponse{SyncResponse: true},
	}
	updateResp := &gnmipb.SubscribeResponse{
		Response: &gnmipb.SubscribeResponse_Update{
			Update: &gnmipb.Notification{
				Update: []*gnmipb.Update{
					{
						Path: &gnmipb.Path{Elem: []*gnmipb.PathElem{{Name: "interfaces"}}},
						Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"status":"up"}`)}},
					},
				},
			},
		},
	}

	stream := &mockSubscribeStream{
		responses: []*gnmipb.SubscribeResponse{updateResp, syncResp},
	}

	mock := &mockGNMIClient{
		subscribeFunc: func(ctx context.Context, opts ...grpc.CallOption) (gnmipb.GNMI_SubscribeClient, error) {
			return stream, nil
		},
	}

	c := NewClient("10.0.0.1:6030", "admin", "secret", testLogger())
	c.gnmiClient = mock

	var received []*gnmipb.SubscribeResponse
	handler := func(resp *gnmipb.SubscribeResponse) error {
		received = append(received, resp)
		return nil
	}

	err := c.Subscribe(context.Background(), []string{"/interfaces"}, gnmipb.SubscriptionList_STREAM, handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(received) != 2 {
		t.Fatalf("received %d responses, want 2", len(received))
	}

	// First response should be the update
	if received[0].GetUpdate() == nil {
		t.Error("first response should be an update")
	}
	// Second response should be sync
	if !received[1].GetSyncResponse() {
		t.Error("second response should be sync_response")
	}

	// Verify the subscribe request was sent with correct path and mode
	if stream.sentReq == nil {
		t.Fatal("subscribe request was not sent")
	}
	subList := stream.sentReq.GetSubscribe()
	if subList == nil {
		t.Fatal("subscribe request missing SubscriptionList")
	}
	if subList.Mode != gnmipb.SubscriptionList_STREAM {
		t.Errorf("mode = %v, want STREAM", subList.Mode)
	}
	if subList.Encoding != gnmipb.Encoding_JSON_IETF {
		t.Errorf("encoding = %v, want JSON_IETF", subList.Encoding)
	}
	if len(subList.Subscription) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(subList.Subscription))
	}
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantElem []string
	}{
		{"root path", "/", nil},
		{"empty path", "", nil},
		{"single element", "/interfaces", []string{"interfaces"}},
		{"multi-element path", "/interfaces/interface/config/enabled", []string{"interfaces", "interface", "config", "enabled"}},
		{"no leading slash", "interfaces/interface", []string{"interfaces", "interface"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parsePath(tc.path)
			if err != nil {
				t.Fatalf("parsePath error: %v", err)
			}
			if len(p.Elem) != len(tc.wantElem) {
				t.Fatalf("elem count = %d, want %d", len(p.Elem), len(tc.wantElem))
			}
			for i, want := range tc.wantElem {
				if p.Elem[i].Name != want {
					t.Errorf("elem[%d] = %q, want %q", i, p.Elem[i].Name, want)
				}
			}
		})
	}
}

func TestEncodeValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"boolean true", true},
		{"string value", "Ethernet1"},
		{"integer value", 9000},
		{"map value", map[string]interface{}{"enabled": true, "mtu": 1500}},
		{"nil value", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tv, err := encodeValue(tc.value)
			if err != nil {
				t.Fatalf("encodeValue error: %v", err)
			}
			if tv.GetJsonIetfVal() == nil {
				t.Fatal("expected JSON_IETF encoding")
			}

			// Verify it round-trips through JSON
			var decoded interface{}
			if err := json.Unmarshal(tv.GetJsonIetfVal(), &decoded); err != nil {
				t.Fatalf("failed to unmarshal encoded value: %v", err)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"empty", "", nil},
		{"root only", "/", nil},
		{"single", "/a", []string{"a"}},
		{"multi", "/a/b/c", []string{"a", "b", "c"}},
		{"trailing slash", "/a/b/", []string{"a", "b"}},
		{"double slash", "/a//b", []string{"a", "b"}},
		{"no leading slash", "a/b", []string{"a", "b"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitPath(tc.path)
			if len(got) != len(tc.want) {
				t.Fatalf("splitPath(%q) = %v (len %d), want %v (len %d)", tc.path, got, len(got), tc.want, len(tc.want))
			}
			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("splitPath(%q)[%d] = %q, want %q", tc.path, i, got[i], want)
				}
			}
		})
	}
}

func TestSetOperation_Constants(t *testing.T) {
	if SetUpdate != "update" {
		t.Errorf("SetUpdate = %q, want %q", SetUpdate, "update")
	}
	if SetReplace != "replace" {
		t.Errorf("SetReplace = %q, want %q", SetReplace, "replace")
	}
	if SetDelete != "delete" {
		t.Errorf("SetDelete = %q, want %q", SetDelete, "delete")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
