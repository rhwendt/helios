package main

import (
	"log/slog"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	heliosv1alpha1 "github.com/rhwendt/helios/services/runbook-operator/api/v1alpha1"
	"github.com/rhwendt/helios/services/runbook-operator/controllers"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(heliosv1alpha1.AddToScheme(scheme))
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	metricsAddr := getEnv("METRICS_ADDR", ":8080")
	probeAddr := getEnv("HEALTH_PROBE_ADDR", ":8081")
	executorImage := getEnv("EXECUTOR_IMAGE", "ghcr.io/rhwendt/helios/runbook-executor:latest")
	enableLeaderElection := os.Getenv("ENABLE_LEADER_ELECTION") == "true"

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "runbook-operator.helios.io",
	})
	if err != nil {
		log.Error("unable to start manager", "error", err)
		os.Exit(1)
	}

	if err := (&controllers.RunbookReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Log:    log.With("controller", "runbook"),
	}).SetupWithManager(mgr); err != nil {
		log.Error("unable to create runbook controller", "error", err)
		os.Exit(1)
	}

	if err := (&controllers.RunbookExecutionReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Log:           log.With("controller", "runbookexecution"),
		ExecutorImage: executorImage,
	}).SetupWithManager(mgr); err != nil {
		log.Error("unable to create runbookexecution controller", "error", err)
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error("unable to set up health check", "error", err)
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error("unable to set up ready check", "error", err)
		os.Exit(1)
	}

	log.Info("starting runbook operator")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error("operator exited with error", "error", err)
		os.Exit(1)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
