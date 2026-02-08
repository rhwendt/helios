package kubernetes

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	configMapUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "helios_target_sync_configmap_updates_total",
		Help: "Total ConfigMap update operations",
	}, []string{"name", "namespace", "status"})
)

// ConfigMapUpdater manages atomic ConfigMap updates with safety guarantees.
type ConfigMapUpdater struct {
	client    kubernetes.Interface
	logger    *slog.Logger
	namespace string
}

// NewConfigMapUpdater creates a new updater for the given namespace.
func NewConfigMapUpdater(client kubernetes.Interface, namespace string, logger *slog.Logger) *ConfigMapUpdater {
	return &ConfigMapUpdater{
		client:    client,
		namespace: namespace,
		logger:    logger,
	}
}

// UpdateConfigMap atomically updates a ConfigMap's data, preserving the existing
// ConfigMap on error. If the new data is empty, the update is skipped to prevent
// accidentally removing all targets.
func (u *ConfigMapUpdater) UpdateConfigMap(ctx context.Context, name string, data map[string]string, labels map[string]string) error {
	if len(data) == 0 {
		u.logger.Warn("skipping ConfigMap update with empty data to prevent target loss", "name", name)
		return nil
	}

	annotations := map[string]string{
		"helios.io/last-sync":    time.Now().UTC().Format(time.RFC3339),
		"helios.io/device-count": fmt.Sprintf("%d", countTargets(data)),
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   u.namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: data,
	}

	existing, err := u.client.CoreV1().ConfigMaps(u.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		// ConfigMap doesn't exist yet, create it
		_, err = u.client.CoreV1().ConfigMaps(u.namespace).Create(ctx, cm, metav1.CreateOptions{})
		if err != nil {
			configMapUpdates.WithLabelValues(name, u.namespace, "error").Inc()
			return fmt.Errorf("creating ConfigMap %s: %w", name, err)
		}
		u.logger.Info("created ConfigMap", "name", name, "namespace", u.namespace)
		configMapUpdates.WithLabelValues(name, u.namespace, "created").Inc()
		return nil
	}

	// Update existing ConfigMap
	existing.Data = data
	existing.Labels = labels
	existing.Annotations = annotations

	_, err = u.client.CoreV1().ConfigMaps(u.namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		configMapUpdates.WithLabelValues(name, u.namespace, "error").Inc()
		return fmt.Errorf("updating ConfigMap %s: %w", name, err)
	}

	u.logger.Info("updated ConfigMap", "name", name, "namespace", u.namespace)
	configMapUpdates.WithLabelValues(name, u.namespace, "updated").Inc()
	return nil
}

func countTargets(data map[string]string) int {
	// Approximate count based on number of data keys
	return len(data)
}
