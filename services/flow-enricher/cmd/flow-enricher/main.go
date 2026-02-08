package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rhwendt/helios/services/flow-enricher/internal/enricher"
	flowkafka "github.com/rhwendt/helios/services/flow-enricher/internal/kafka"
	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("starting flow-enricher")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Configuration from environment
	kafkaBrokers := envOrDefault("KAFKA_BROKERS", "localhost:9092")
	consumerTopic := envOrDefault("KAFKA_CONSUMER_TOPIC", "helios-flows-raw")
	consumerGroup := envOrDefault("KAFKA_CONSUMER_GROUP", "flow-enricher")
	producerTopic := envOrDefault("KAFKA_PRODUCER_TOPIC", "helios-flows-enriched")
	netboxURL := envOrDefault("NETBOX_API_URL", "")
	netboxToken := envOrDefault("NETBOX_API_TOKEN", "")
	geoipCityDB := envOrDefault("GEOIP_CITY_DB", "/var/lib/geoip/GeoLite2-City.mmdb")
	geoipASNDB := envOrDefault("GEOIP_ASN_DB", "/var/lib/geoip/GeoLite2-ASN.mmdb")
	metricsAddr := envOrDefault("METRICS_ADDR", ":8080")

	// Initialize NetBox cache
	netboxCache := enricher.NewNetBoxCache(netboxURL, netboxToken, 5*time.Minute, logger)

	// Initialize GeoIP reader
	var geoipReader *enricher.GeoIPReader
	var err error
	geoipReader, err = enricher.NewGeoIPReader(geoipCityDB, geoipASNDB, logger)
	if err != nil {
		logger.Warn("GeoIP databases not available, continuing without GeoIP enrichment", "error", err)
		geoipReader = nil
	}

	// Initialize enricher
	e := enricher.New(netboxCache, geoipReader, logger)

	// Initialize Kafka producer
	producer, err := flowkafka.NewProducer(flowkafka.ProducerConfig{
		Brokers: kafkaBrokers,
		Topic:   producerTopic,
	}, logger)
	if err != nil {
		logger.Error("failed to create Kafka producer", "error", err)
		os.Exit(1)
	}
	defer producer.Close()

	// Message handler: enrich and produce
	handler := func(ctx context.Context, flows []*flowpb.EnrichedFlow) error {
		for _, flow := range flows {
			e.Enrich(flow)
		}
		return producer.ProduceBatch(ctx, flows)
	}

	// Initialize Kafka consumer
	consumer, err := flowkafka.NewConsumer(flowkafka.ConsumerConfig{
		Brokers:   kafkaBrokers,
		GroupID:   consumerGroup,
		Topic:     consumerTopic,
		BatchSize: 100,
	}, handler, logger)
	if err != nil {
		logger.Error("failed to create Kafka consumer", "error", err)
		os.Exit(1)
	}

	// Start metrics server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	server := &http.Server{Addr: metricsAddr, Handler: mux}

	var wg sync.WaitGroup

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("metrics server starting", "addr", metricsAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	// Start NetBox cache refresh
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := netboxCache.Start(ctx); err != nil && err != context.Canceled {
			logger.Error("NetBox cache error", "error", err)
		}
	}()

	// Start Kafka consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := consumer.Start(ctx); err != nil && err != context.Canceled {
			logger.Error("Kafka consumer error", "error", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("shutting down")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)

	if geoipReader != nil {
		geoipReader.Close()
	}

	wg.Wait()
	logger.Info("shutdown complete")
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
