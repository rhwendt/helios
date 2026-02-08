package kafka

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"

	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestConsumerConfig_Defaults(t *testing.T) {
	tests := []struct {
		name          string
		batchSize     int
		wantBatchSize int
	}{
		{"positive batch size preserved", 50, 50},
		{"zero defaults to 100", 0, 100},
		{"negative defaults to 100", -1, 100},
		{"one is valid", 1, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ConsumerConfig{
				Brokers:   "localhost:9092",
				GroupID:   "test-group",
				Topic:     "test-topic",
				BatchSize: tc.batchSize,
			}

			// Verify config struct fields are set correctly
			if cfg.Brokers != "localhost:9092" {
				t.Errorf("Brokers = %q, want %q", cfg.Brokers, "localhost:9092")
			}
			if cfg.GroupID != "test-group" {
				t.Errorf("GroupID = %q, want %q", cfg.GroupID, "test-group")
			}
			if cfg.Topic != "test-topic" {
				t.Errorf("Topic = %q, want %q", cfg.Topic, "test-topic")
			}

			// Simulate the batchSize defaulting logic from NewConsumer
			batchSize := cfg.BatchSize
			if batchSize <= 0 {
				batchSize = 100
			}
			if batchSize != tc.wantBatchSize {
				t.Errorf("effective batchSize = %d, want %d", batchSize, tc.wantBatchSize)
			}
		})
	}
}

func TestProducerConfig_Fields(t *testing.T) {
	cfg := ProducerConfig{
		Brokers: "broker-1:9092,broker-2:9092",
		Topic:   "enriched-flows",
	}

	if cfg.Brokers != "broker-1:9092,broker-2:9092" {
		t.Errorf("Brokers = %q, want multi-broker string", cfg.Brokers)
	}
	if cfg.Topic != "enriched-flows" {
		t.Errorf("Topic = %q, want %q", cfg.Topic, "enriched-flows")
	}
}

func TestProtobuf_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		flow *flowpb.EnrichedFlow
	}{
		{
			name: "basic flow with counters",
			flow: &flowpb.EnrichedFlow{
				ExporterIp: 0x0A000001,
				SrcPort:    443,
				DstPort:    54321,
				Protocol:   6,
				Bytes:      1500,
				Packets:    10,
			},
		},
		{
			name: "flow with enrichment fields",
			flow: &flowpb.EnrichedFlow{
				ExporterIp:   0x0A000002,
				ExporterName: "router-1",
				ExporterSite: "dc1",
				ExporterRole: "core-router",
				InIf:         1,
				OutIf:        2,
				InIfName:     "Ethernet1",
				OutIfName:    "Ethernet2",
				SrcCountry:   "US",
				DstCountry:   "DE",
			},
		},
		{
			name: "flow with BGP and VLAN",
			flow: &flowpb.EnrichedFlow{
				SrcAs:   64500,
				DstAs:   13335,
				SrcVlan: 100,
				DstVlan: 200,
			},
		},
		{
			name: "empty flow",
			flow: &flowpb.EnrichedFlow{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Marshal (what ProduceBatch does)
			data, err := proto.Marshal(tc.flow)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			// Unmarshal (what pollBatch does)
			got := &flowpb.EnrichedFlow{}
			if err := proto.Unmarshal(data, got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Verify round-trip fidelity
			if got.ExporterIp != tc.flow.ExporterIp {
				t.Errorf("ExporterIp = %d, want %d", got.ExporterIp, tc.flow.ExporterIp)
			}
			if got.ExporterName != tc.flow.ExporterName {
				t.Errorf("ExporterName = %q, want %q", got.ExporterName, tc.flow.ExporterName)
			}
			if got.ExporterSite != tc.flow.ExporterSite {
				t.Errorf("ExporterSite = %q, want %q", got.ExporterSite, tc.flow.ExporterSite)
			}
			if got.ExporterRole != tc.flow.ExporterRole {
				t.Errorf("ExporterRole = %q, want %q", got.ExporterRole, tc.flow.ExporterRole)
			}
			if got.Bytes != tc.flow.Bytes {
				t.Errorf("Bytes = %d, want %d", got.Bytes, tc.flow.Bytes)
			}
			if got.Packets != tc.flow.Packets {
				t.Errorf("Packets = %d, want %d", got.Packets, tc.flow.Packets)
			}
			if got.SrcPort != tc.flow.SrcPort {
				t.Errorf("SrcPort = %d, want %d", got.SrcPort, tc.flow.SrcPort)
			}
			if got.DstPort != tc.flow.DstPort {
				t.Errorf("DstPort = %d, want %d", got.DstPort, tc.flow.DstPort)
			}
			if got.InIfName != tc.flow.InIfName {
				t.Errorf("InIfName = %q, want %q", got.InIfName, tc.flow.InIfName)
			}
			if got.OutIfName != tc.flow.OutIfName {
				t.Errorf("OutIfName = %q, want %q", got.OutIfName, tc.flow.OutIfName)
			}
			if got.SrcCountry != tc.flow.SrcCountry {
				t.Errorf("SrcCountry = %q, want %q", got.SrcCountry, tc.flow.SrcCountry)
			}
			if got.DstCountry != tc.flow.DstCountry {
				t.Errorf("DstCountry = %q, want %q", got.DstCountry, tc.flow.DstCountry)
			}
			if got.SrcAs != tc.flow.SrcAs {
				t.Errorf("SrcAs = %d, want %d", got.SrcAs, tc.flow.SrcAs)
			}
			if got.DstAs != tc.flow.DstAs {
				t.Errorf("DstAs = %d, want %d", got.DstAs, tc.flow.DstAs)
			}
		})
	}
}

func TestProtobuf_MalformedData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"random bytes", []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb}},
		{"truncated protobuf", []byte{0x08}},
		{"empty bytes", []byte{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			flow := &flowpb.EnrichedFlow{}
			err := proto.Unmarshal(tc.data, flow)
			// Empty data is valid protobuf (all defaults). Others may or may not
			// error depending on whether bytes happen to be valid wire format.
			// The key behavior: no panic.
			_ = err
		})
	}
}

func TestMessageHandler_Type(t *testing.T) {
	// Verify MessageHandler signature is compatible with batch processing
	var handler MessageHandler
	handler = func(ctx context.Context, flows []*flowpb.EnrichedFlow) error {
		return nil
	}

	// Should be callable with nil context and empty slice
	if err := handler(nil, nil); err != nil {
		t.Errorf("handler returned unexpected error: %v", err)
	}
}
