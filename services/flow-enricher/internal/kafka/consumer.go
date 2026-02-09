package kafka

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"google.golang.org/protobuf/proto"

	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

// MessageHandler processes a batch of flow messages.
type MessageHandler func(ctx context.Context, flows []*flowpb.EnrichedFlow) error

// Consumer reads raw flow protobuf messages from a Kafka topic.
type Consumer struct {
	consumer  *kafka.Consumer
	topic     string
	batchSize int
	handler   MessageHandler
	logger    *slog.Logger
}

// ConsumerConfig holds configuration for the Kafka consumer.
type ConsumerConfig struct {
	Brokers   string
	GroupID   string
	Topic     string
	BatchSize int
}

// NewConsumer creates a new Kafka consumer.
func NewConsumer(cfg ConsumerConfig, handler MessageHandler, logger *slog.Logger) (*Consumer, error) {
	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers":        cfg.Brokers,
		"group.id":                 cfg.GroupID,
		"auto.offset.reset":        "latest",
		"enable.auto.offset.store": false,
		"session.timeout.ms":       30000,
		"max.poll.interval.ms":     300000,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Kafka consumer: %w", err)
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	return &Consumer{
		consumer:  c,
		topic:     cfg.Topic,
		batchSize: batchSize,
		handler:   handler,
		logger:    logger,
	}, nil
}

// Start begins consuming messages. It blocks until the context is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	if err := c.consumer.Subscribe(c.topic, nil); err != nil {
		return fmt.Errorf("subscribing to topic %s: %w", c.topic, err)
	}

	c.logger.Info("Kafka consumer started", "topic", c.topic, "batch_size", c.batchSize)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("shutting down Kafka consumer")
			c.consumer.Close()
			return ctx.Err()
		case <-ticker.C:
			batch, err := c.pollBatch(ctx)
			if err != nil {
				c.logger.Error("error polling batch", "error", err)
				continue
			}
			if len(batch) == 0 {
				continue
			}

			if err := c.handler(ctx, batch); err != nil {
				c.logger.Error("error processing batch", "error", err, "batch_size", len(batch))
			}
		}
	}
}

// pollBatch reads up to batchSize messages from Kafka.
func (c *Consumer) pollBatch(ctx context.Context) ([]*flowpb.EnrichedFlow, error) {
	var batch []*flowpb.EnrichedFlow
	timeout := 100 * time.Millisecond

	for i := 0; i < c.batchSize; i++ {
		select {
		case <-ctx.Done():
			return batch, ctx.Err()
		default:
		}

		ev := c.consumer.Poll(int(timeout.Milliseconds()))
		if ev == nil {
			break
		}

		switch e := ev.(type) {
		case *kafka.Message:
			flow := &flowpb.EnrichedFlow{}
			if err := proto.Unmarshal(e.Value, flow); err != nil {
				c.logger.Warn("failed to unmarshal flow", "error", err)
				continue
			}
			batch = append(batch, flow)

			if _, err := c.consumer.StoreMessage(e); err != nil {
				c.logger.Warn("failed to store offset", "error", err)
			}
		case kafka.Error:
			c.logger.Error("Kafka consumer error", "error", e)
			if e.Code() == kafka.ErrAllBrokersDown {
				return batch, fmt.Errorf("all Kafka brokers down: %w", e)
			}
		}
	}

	return batch, nil
}
