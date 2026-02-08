package kafka

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"google.golang.org/protobuf/proto"

	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

// Producer writes enriched flow protobuf messages to a Kafka topic.
type Producer struct {
	producer *kafka.Producer
	topic    string
	logger   *slog.Logger
}

// ProducerConfig holds configuration for the Kafka producer.
type ProducerConfig struct {
	Brokers string
	Topic   string
}

// NewProducer creates a new Kafka producer.
func NewProducer(cfg ProducerConfig, logger *slog.Logger) (*Producer, error) {
	p, err := kafka.NewProducer(&kafka.ConfigMap{
		"bootstrap.servers":   cfg.Brokers,
		"linger.ms":           10,
		"batch.num.messages":  1000,
		"compression.type":    "lz4",
		"acks":                "all",
		"retries":             3,
		"retry.backoff.ms":    100,
		"delivery.timeout.ms": 30000,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Kafka producer: %w", err)
	}

	return &Producer{
		producer: p,
		topic:    cfg.Topic,
		logger:   logger,
	}, nil
}

// ProduceBatch sends a batch of enriched flows to Kafka.
func (p *Producer) ProduceBatch(ctx context.Context, flows []*flowpb.EnrichedFlow) error {
	deliveryChan := make(chan kafka.Event, len(flows))

	for _, flow := range flows {
		data, err := proto.Marshal(flow)
		if err != nil {
			p.logger.Warn("failed to marshal enriched flow", "error", err)
			continue
		}

		err = p.producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &p.topic,
				Partition: kafka.PartitionAny,
			},
			Value: data,
		}, deliveryChan)
		if err != nil {
			p.logger.Error("failed to produce message", "error", err)
		}
	}

	// Wait for delivery confirmations
	var errs int
	for i := 0; i < len(flows); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-deliveryChan:
			m := e.(*kafka.Message)
			if m.TopicPartition.Error != nil {
				errs++
				p.logger.Warn("delivery failed", "error", m.TopicPartition.Error)
			}
		}
	}

	if errs > 0 {
		return fmt.Errorf("failed to deliver %d/%d messages", errs, len(flows))
	}
	return nil
}

// Flush waits for all outstanding messages to be delivered.
func (p *Producer) Flush(timeoutMs int) {
	p.producer.Flush(timeoutMs)
}

// Close shuts down the producer.
func (p *Producer) Close() {
	p.producer.Flush(5000)
	p.producer.Close()
}
