package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rhwendt/helios/services/target-generator/internal/generator"
	k8sclient "github.com/rhwendt/helios/services/target-generator/internal/kubernetes"
	"github.com/rhwendt/helios/services/target-generator/internal/netbox"
)

var (
	syncLastSuccess = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_last_success_timestamp",
		Help: "Unix timestamp of last successful sync",
	})
	syncDuration = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_duration_seconds",
		Help: "Duration of last sync cycle",
	})
	syncDevicesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_devices_total",
		Help: "Total devices discovered in last sync",
	})
	syncGNMITargets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_gnmi_targets",
		Help: "Number of gNMI targets generated",
	})
	syncSNMPTargets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_snmp_targets",
		Help: "Number of SNMP targets generated",
	})
	syncBlackboxTargets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "helios_target_sync_blackbox_targets",
		Help: "Number of blackbox targets generated",
	})
	syncErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "helios_target_sync_errors_total",
		Help: "Total sync errors",
	})
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Start metrics server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	metricsServer := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	if err := runSync(logger); err != nil {
		syncErrors.Inc()
		logger.Error("sync failed", "error", err)
		os.Exit(1)
	}

	logger.Info("target sync completed successfully")
}

func runSync(logger *slog.Logger) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return run(ctx, logger)
}

func run(ctx context.Context, logger *slog.Logger) error {
	start := time.Now()

	netboxURL := envOrDefault("NETBOX_URL", "http://netbox.helios-integration.svc.cluster.local")
	netboxToken := envOrDefault("NETBOX_API_TOKEN", "")
	targetNamespace := envOrDefault("TARGET_NAMESPACE", "helios-collection")

	if netboxToken == "" {
		return fmt.Errorf("NETBOX_API_TOKEN is required")
	}

	// Initialize NetBox client
	nbClient := netbox.NewClient(netboxURL, netboxToken, logger)

	// Initialize Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("getting in-cluster config: %w", err)
	}
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	cmUpdater := k8sclient.NewConfigMapUpdater(k8sClient, targetNamespace, logger)

	// Query NetBox for monitored devices
	devices, err := nbClient.ListMonitoredDevices(ctx)
	if err != nil {
		return fmt.Errorf("listing monitored devices: %w", err)
	}
	syncDevicesTotal.Set(float64(len(devices)))

	// Generate gNMI targets
	gnmicData, gnmicCount, err := generator.GenerateGNMICTargets(devices)
	if err != nil {
		return fmt.Errorf("generating gnmic targets: %w", err)
	}
	syncGNMITargets.Set(float64(gnmicCount))

	err = cmUpdater.UpdateConfigMap(ctx, "helios-gnmic-targets", map[string]string{
		"targets.yaml": string(gnmicData),
	}, map[string]string{
		"app.kubernetes.io/name":      "gnmic",
		"app.kubernetes.io/component": "targets",
		"helios.io/generated-by":      "target-generator",
	})
	if err != nil {
		return fmt.Errorf("updating gnmic ConfigMap: %w", err)
	}

	// Generate SNMP targets
	snmpData, snmpCount, err := generator.GenerateSNMPTargets(devices)
	if err != nil {
		return fmt.Errorf("generating snmp targets: %w", err)
	}
	syncSNMPTargets.Set(float64(snmpCount))

	err = cmUpdater.UpdateConfigMap(ctx, "helios-snmp-targets", map[string]string{
		"snmp-targets.json": string(snmpData),
	}, map[string]string{
		"app.kubernetes.io/name":      "snmp-exporter",
		"app.kubernetes.io/component": "targets",
		"helios.io/generated-by":      "target-generator",
	})
	if err != nil {
		return fmt.Errorf("updating snmp ConfigMap: %w", err)
	}

	// Generate blackbox targets
	bbTargets, bbCount, err := generator.GenerateBlackboxTargets(devices)
	if err != nil {
		return fmt.Errorf("generating blackbox targets: %w", err)
	}
	syncBlackboxTargets.Set(float64(bbCount))

	bbData := make(map[string]string)
	for filename, data := range bbTargets {
		bbData[filename] = string(data)
	}

	err = cmUpdater.UpdateConfigMap(ctx, "helios-blackbox-targets", bbData, map[string]string{
		"app.kubernetes.io/name":      "blackbox-exporter",
		"app.kubernetes.io/component": "targets",
		"helios.io/generated-by":      "target-generator",
	})
	if err != nil {
		return fmt.Errorf("updating blackbox ConfigMap: %w", err)
	}

	duration := time.Since(start)
	syncDuration.Set(duration.Seconds())
	syncLastSuccess.SetToCurrentTime()

	logger.Info("sync complete",
		"devices", len(devices),
		"gnmi_targets", gnmicCount,
		"snmp_targets", snmpCount,
		"blackbox_targets", bbCount,
		"duration", duration,
	)

	return nil
}

func envOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
