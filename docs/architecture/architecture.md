#architecture #helios #network #observability

# Helios Architecture Document

**Version**: 1.0
**Date**: 2026-01-09
**Author**: Architecture Review
**Status**: Draft

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [High-Level Architecture](#high-level-architecture)
3. [Component Deep Dive](#component-deep-dive)
4. [Scalability Design](#scalability-design)
5. [Deployment Architecture](#deployment-architecture)
6. [Data Model](#data-model)
7. [Extensibility Architecture](#extensibility-architecture)
8. [Runbook Automation Design](#runbook-automation-design)
9. [Security Considerations](#security-considerations)
10. [Resource Estimates](#resource-estimates)
11. [ADR Recommendations](#adr-recommendations)

---

## Executive Summary

Helios is a CNCF-native, all-in-one network observability platform designed for Kubernetes environments. It provides unified collection, storage, visualization, and alerting for network infrastructure metrics from 50 to 8,000+ devices across multiple datacenters connected via Cilium overlay networking.

### Design Principles

1. **Cloud-Native First**: Built for Kubernetes with Helm, ArgoCD, and GitOps workflows
2. **Horizontal Scalability**: Scale collection and storage independently based on load
3. **Vendor Agnostic**: Support multiple network vendors through standardized interfaces
4. **Extensibility**: Easy addition of new MIBs, OpenConfig paths, and custom scripts via Git workflows
5. **Operational Simplicity**: Pre-built dashboards, alerts, and runbooks for common scenarios
6. **Long-Term Retention**: 90-day metric retention with cost-effective tiered storage

---

## High-Level Architecture

### System Overview

```
                                    VISUALIZATION LAYER
    +------------------------------------------------------------------+
    |                                                                  |
    |  +------------------+  +------------------+  +----------------+  |
    |  |     Grafana      |  |  Alertmanager   |  |   Runbook UI   |  |
    |  |   (Dashboards)   |  |  (Notifications)|  |   (Automation) |  |
    |  +--------+---------+  +--------+--------+  +-------+--------+  |
    |           |                     |                   |           |
    +-----------|---------------------|-------------------|------------+
                |                     |                   |
    +-----------v---------------------v-------------------v------------+
    |                        QUERY LAYER                               |
    |                                                                  |
    |  +------------------------------------------------------------+  |
    |  |                    Thanos Query                            |  |
    |  |          (Global Query Federation + Deduplication)         |  |
    |  +-----+------------------+------------------+----------------+  |
    |        |                  |                  |                   |
    +--------|------------------|------------------|-------------------+
             |                  |                  |
    +--------v------------------v------------------v-------------------+
    |                        STORAGE LAYER                             |
    |                                                                  |
    |  +----------------+  +----------------+  +-------------------+   |
    |  | Thanos Store   |  | Thanos Store   |  |   Thanos Ruler    |   |
    |  | Gateway (DC1)  |  | Gateway (DC2)  |  | (Recording Rules) |   |
    |  +-------+--------+  +-------+--------+  +---------+---------+   |
    |          |                   |                     |             |
    |  +-------v-------------------v---------------------v---------+   |
    |  |              Object Storage (MinIO / S3)                  |   |
    |  |              90-Day Retention + Compaction                |   |
    |  +-----------------------------------------------------------+   |
    |                                                                  |
    +------------------------------------------------------------------+
             ^                  ^                  ^
             |                  |                  |
    +--------|------------------|------------------|-------------------+
    |        |    COLLECTION LAYER                 |                   |
    |  +-----+------+    +------+-----+    +------+------+            |
    |  | Prometheus |    | Prometheus |    | Prometheus  |            |
    |  | Shard 1    |    | Shard 2    |    | Shard N     |            |
    |  +-----+------+    +-----+------+    +------+------+            |
    |        |                 |                  |                   |
    |  +-----v-----------------v------------------v------+            |
    |  |              Thanos Sidecar (per Prometheus)    |            |
    |  +--------------------------------------------------+            |
    |                                                                  |
    +------------------------------------------------------------------+
             ^                  ^                  ^
             |                  |                  |
    +--------|------------------|------------------|-------------------+
    |        |    EXPORTER LAYER                   |                   |
    |                                                                  |
    |  +-----------+  +-----------+  +-----------+  +-----------+     |
    |  |   gnmic   |  |   gnmic   |  |   gnmic   |  |   gnmic   |     |
    |  | Cluster 1 |  | Cluster 2 |  | Cluster 3 |  | Cluster N |     |
    |  +-----------+  +-----------+  +-----------+  +-----------+     |
    |                                                                  |
    |  +-----------+  +-------------+  +-------------+  +---------+   |
    |  |   SNMP    |  |  Blackbox   |  |    Node     |  | Script  |   |
    |  |  Exporter |  |  Exporter   |  |  Exporter   |  | Exporter|   |
    |  +-----------+  +-------------+  +-------------+  +---------+   |
    |                                                                  |
    +------------------------------------------------------------------+
             ^                  ^                  ^
             |                  |                  |
    +--------|------------------|------------------|-------------------+
    |                    INTEGRATION LAYER                             |
    |                                                                  |
    |  +-------------------+     +----------------------------------+  |
    |  |    NetBox Sync    |     |     Target Generator Service    |  |
    |  | (Device Inventory)|     | (ConfigMap + ServiceDiscovery)  |  |
    |  +---------+---------+     +-----------------+----------------+  |
    |            |                                 |                   |
    |  +---------v---------------------------------v----------------+  |
    |  |                      NetBox API                            |  |
    |  |              (Source of Truth for Devices)                 |  |
    |  +------------------------------------------------------------+  |
    |                                                                  |
    +------------------------------------------------------------------+
             ^                  ^                  ^
             |                  |                  |
    +--------|------------------|------------------|-------------------+
    |                    NETWORK LAYER                                 |
    |                                                                  |
    |   +----------+  +----------+  +----------+  +----------+        |
    |   |  Arista  |  |  Cisco   |  | Juniper  |  | Palo Alto|        |
    |   |  Switch  |  |  Router  |  | Firewall |  | Firewall |        |
    |   +----------+  +----------+  +----------+  +----------+        |
    |                                                                  |
    |   +----------+  +----------+  +----------+  +----------+        |
    |   | Fortigate|  |   F5     |  |  Cumulus |  |  Other   |        |
    |   | Firewall |  |   LB     |  |  Switch  |  | Devices  |        |
    |   +----------+  +----------+  +----------+  +----------+        |
    |                                                                  |
    +------------------------------------------------------------------+
```

### Data Flow

```
1. DEVICE DISCOVERY FLOW
   NetBox --> Target Generator --> ConfigMaps --> gnmic/Exporters

2. METRICS COLLECTION FLOW
   Devices --> gnmic (gNMI) --------+
   Devices --> SNMP Exporter -------+--> Prometheus --> Thanos Sidecar
   Devices --> Blackbox Exporter ---+                        |
   Hosts   --> Node Exporter -------+                        v
   Custom  --> Script Exporter -----+              Object Storage

3. QUERY FLOW
   Grafana --> Thanos Query --> Thanos Store Gateway --> Object Storage
                          +--> Prometheus (recent data)

4. ALERTING FLOW
   Prometheus --> Alertmanager --> PagerDuty/Slack/Email

5. AUTOMATION FLOW
   Alert/Manual --> Runbook Operator --> gnmic Write --> Device
```

---

## Component Deep Dive

### Collection Layer

#### gnmic Deployment Strategy

gnmic provides gNMI streaming telemetry collection with Prometheus output.

**Deployment Model**: StatefulSet with horizontal scaling

```yaml
# gnmic-cluster.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: gnmic
  namespace: helios-collection
spec:
  serviceName: gnmic
  replicas: 3  # Scale based on device count
  selector:
    matchLabels:
      app: gnmic
  template:
    spec:
      containers:
      - name: gnmic
        image: ghcr.io/openconfig/gnmic:latest
        args:
          - subscribe
          - --config=/etc/gnmic/gnmic.yaml
        ports:
        - containerPort: 9804  # Prometheus metrics
        - containerPort: 57400 # gRPC for clustering
        volumeMounts:
        - name: config
          mountPath: /etc/gnmic
        - name: targets
          mountPath: /etc/gnmic/targets
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
      volumes:
      - name: config
        configMap:
          name: gnmic-config
      - name: targets
        configMap:
          name: gnmic-targets  # Auto-generated from NetBox
```

**gnmic Clustering Configuration**:

```yaml
# gnmic.yaml
clustering:
  cluster-name: helios-gnmic
  instance-name: ${POD_NAME}
  service-address: ${POD_IP}:57400
  services-watch-timer: 30s
  targets-watch-timer: 30s
  leader-wait-timer: 10s

  locker:
    type: k8s
    # Uses K8s leases for leader election and target distribution

targets-file: /etc/gnmic/targets/targets.yaml

subscriptions:
  interfaces:
    paths:
      - /interfaces/interface/state/counters
      - /interfaces/interface/state/oper-status
    mode: stream
    stream-mode: sample
    sample-interval: 10s

  system:
    paths:
      - /system/state
      - /system/memory/state
      - /system/cpus/cpu/state
    mode: stream
    stream-mode: sample
    sample-interval: 30s

outputs:
  prometheus:
    type: prometheus
    listen: :9804
    path: /metrics
    metric-prefix: gnmic
    append-subscription-name: true
    export-timestamps: true
    strings-as-labels: true
```

**Target Distribution**:
- gnmic clustering automatically distributes targets across replicas
- Uses Kubernetes leases for leader election
- Targets are rebalanced when pods scale or fail
- Each device is subscribed to by exactly one gnmic instance

#### Prometheus Exporters

**SNMP Exporter** (for legacy devices):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: snmp-exporter
  namespace: helios-collection
spec:
  replicas: 2  # HA deployment
  template:
    spec:
      containers:
      - name: snmp-exporter
        image: prom/snmp-exporter:latest
        ports:
        - containerPort: 9116
        volumeMounts:
        - name: snmp-config
          mountPath: /etc/snmp_exporter
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

**Blackbox Exporter** (synthetic monitoring):

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: blackbox-exporter
  namespace: helios-collection
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: blackbox-exporter
        image: prom/blackbox-exporter:latest
        args:
          - --config.file=/etc/blackbox/blackbox.yaml
        ports:
        - containerPort: 9115
```

**Blackbox Configuration**:

```yaml
# blackbox.yaml
modules:
  icmp:
    prober: icmp
    timeout: 5s
    icmp:
      preferred_ip_protocol: ip4

  tcp_connect:
    prober: tcp
    timeout: 5s

  http_2xx:
    prober: http
    timeout: 5s
    http:
      valid_http_versions: ["HTTP/1.1", "HTTP/2.0"]
      valid_status_codes: [200, 301, 302]

  dns:
    prober: dns
    timeout: 5s
    dns:
      query_name: "example.com"
      query_type: "A"
```

### Storage Layer

#### Thanos Component Topology

```
                           +------------------+
                           |   Thanos Query   |
                           | (Query Frontend) |
                           +--------+---------+
                                    |
              +---------------------+---------------------+
              |                     |                     |
    +---------v--------+  +---------v--------+  +---------v--------+
    |  Thanos Store    |  |  Thanos Store    |  |    Prometheus    |
    |  Gateway (DC1)   |  |  Gateway (DC2)   |  |    (Sidecar)     |
    +--------+---------+  +--------+---------+  +--------+---------+
             |                     |                     |
    +--------v---------------------v---------------------v--------+
    |                     Object Storage                          |
    |  +------------------+  +------------------+  +-----------+  |
    |  | dc1-metrics/     |  | dc2-metrics/     |  | compacted/|  |
    |  | (raw blocks)     |  | (raw blocks)     |  | (merged)  |  |
    |  +------------------+  +------------------+  +-----------+  |
    +-------------------------------------------------------------+
                              ^
                              |
                    +---------+---------+
                    |  Thanos Compactor |
                    | (Downsampling +   |
                    |  Deduplication)   |
                    +-------------------+
```

**Thanos Sidecar** (per Prometheus):

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: prometheus
  namespace: helios-storage
spec:
  replicas: 2  # HA pair
  template:
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:latest
        args:
          - --config.file=/etc/prometheus/prometheus.yaml
          - --storage.tsdb.path=/prometheus
          - --storage.tsdb.retention.time=2h  # Short retention, Thanos handles long-term
          - --storage.tsdb.min-block-duration=2h
          - --storage.tsdb.max-block-duration=2h
          - --web.enable-lifecycle
        volumeMounts:
        - name: prometheus-data
          mountPath: /prometheus

      - name: thanos-sidecar
        image: quay.io/thanos/thanos:latest
        args:
          - sidecar
          - --tsdb.path=/prometheus
          - --prometheus.url=http://localhost:9090
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
        volumeMounts:
        - name: prometheus-data
          mountPath: /prometheus
        - name: objstore-config
          mountPath: /etc/thanos
```

**Thanos Store Gateway**:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-store
  namespace: helios-storage
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: thanos-store
        image: quay.io/thanos/thanos:latest
        args:
          - store
          - --data-dir=/var/thanos/store
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
          - --index-cache-size=1GB
          - --chunk-pool-size=2GB
        resources:
          requests:
            cpu: 500m
            memory: 2Gi
          limits:
            cpu: 2000m
            memory: 8Gi
```

**Thanos Query**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thanos-query
  namespace: helios-storage
spec:
  replicas: 3  # HA for query layer
  template:
    spec:
      containers:
      - name: thanos-query
        image: quay.io/thanos/thanos:latest
        args:
          - query
          - --http-address=0.0.0.0:9090
          - --grpc-address=0.0.0.0:10901
          - --store=dnssrv+_grpc._tcp.thanos-store.helios-storage.svc
          - --store=dnssrv+_grpc._tcp.prometheus.helios-storage.svc
          - --query.replica-label=replica
          - --query.auto-downsampling
```

**Thanos Compactor** (singleton):

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-compactor
  namespace: helios-storage
spec:
  replicas: 1  # Must be singleton
  template:
    spec:
      containers:
      - name: thanos-compactor
        image: quay.io/thanos/thanos:latest
        args:
          - compact
          - --data-dir=/var/thanos/compact
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --retention.resolution-raw=14d     # Keep raw for 14 days
          - --retention.resolution-5m=30d      # Keep 5m samples for 30 days
          - --retention.resolution-1h=90d      # Keep 1h samples for 90 days
          - --compact.concurrency=4
          - --downsample.concurrency=4
          - --wait
```

**Object Storage Configuration**:

```yaml
# objstore.yaml (MinIO example)
type: S3
config:
  bucket: helios-metrics
  endpoint: minio.helios-storage.svc:9000
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}
  insecure: true  # For internal MinIO

# objstore.yaml (AWS S3 example)
type: S3
config:
  bucket: helios-metrics-prod
  region: us-east-1
  endpoint: s3.us-east-1.amazonaws.com
```

### Integration Layer

#### NetBox Sync Architecture

```
+-------------+     +------------------+     +-------------------+
|   NetBox    | --> | Target Generator | --> |   ConfigMaps      |
|   (API)     |     |   (CronJob/      |     | - gnmic-targets   |
|             |     |    Controller)   |     | - snmp-targets    |
+-------------+     +------------------+     | - blackbox-targets|
                            |               +-------------------+
                            v                        |
                    +---------------+                v
                    | Prometheus SD |         +-------------+
                    | (HTTP SD)     | <-------| Service     |
                    +---------------+         | Discovery   |
                                              +-------------+
```

**Target Generator Service**:

```python
# target_generator.py - Deployed as CronJob or Controller
import pynetbox
import yaml
from kubernetes import client, config

class TargetGenerator:
    def __init__(self):
        self.nb = pynetbox.api(
            url=os.environ['NETBOX_URL'],
            token=os.environ['NETBOX_TOKEN']
        )
        config.load_incluster_config()
        self.k8s = client.CoreV1Api()

    def get_gnmic_targets(self) -> dict:
        """Generate gnmic targets from NetBox devices with gNMI support."""
        targets = {}

        # Query devices with gnmi custom field
        devices = self.nb.dcim.devices.filter(
            status='active',
            cf_gnmi_enabled=True
        )

        for device in devices:
            target_name = device.name
            targets[target_name] = {
                'address': f"{device.primary_ip.address.split('/')[0]}:6030",
                'username': '${GNMI_USERNAME}',
                'password': '${GNMI_PASSWORD}',
                'skip-verify': True,
                'labels': {
                    'device': device.name,
                    'site': device.site.slug,
                    'role': device.device_role.slug,
                    'vendor': device.device_type.manufacturer.slug,
                    'platform': device.platform.slug if device.platform else 'unknown',
                    'region': device.site.region.slug if device.site.region else 'unknown',
                }
            }

        return {'targets': targets}

    def get_snmp_targets(self) -> list:
        """Generate SNMP targets for Prometheus static config."""
        targets = []

        devices = self.nb.dcim.devices.filter(
            status='active',
            cf_snmp_enabled=True
        )

        for device in devices:
            targets.append({
                'targets': [device.primary_ip.address.split('/')[0]],
                'labels': {
                    '__param_module': device.custom_fields.get('snmp_module', 'if_mib'),
                    'device': device.name,
                    'site': device.site.slug,
                    'role': device.device_role.slug,
                    'vendor': device.device_type.manufacturer.slug,
                }
            })

        return targets

    def update_configmaps(self):
        """Update Kubernetes ConfigMaps with generated targets."""
        gnmic_targets = self.get_gnmic_targets()
        snmp_targets = self.get_snmp_targets()

        # Update gnmic targets ConfigMap
        self.k8s.patch_namespaced_config_map(
            name='gnmic-targets',
            namespace='helios-collection',
            body={
                'data': {
                    'targets.yaml': yaml.dump(gnmic_targets)
                }
            }
        )

        # Update SNMP targets ConfigMap
        self.k8s.patch_namespaced_config_map(
            name='snmp-targets',
            namespace='helios-collection',
            body={
                'data': {
                    'targets.json': json.dumps(snmp_targets)
                }
            }
        )
```

**CronJob for Target Sync**:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: netbox-target-sync
  namespace: helios-integration
spec:
  schedule: "*/5 * * * *"  # Every 5 minutes
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: target-generator
            image: helios/target-generator:latest
            env:
            - name: NETBOX_URL
              valueFrom:
                secretKeyRef:
                  name: netbox-credentials
                  key: url
            - name: NETBOX_TOKEN
              valueFrom:
                secretKeyRef:
                  name: netbox-credentials
                  key: token
          restartPolicy: OnFailure
```

### Visualization Layer

#### Grafana Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: helios-visualization
spec:
  replicas: 2  # HA with shared database
  template:
    spec:
      containers:
      - name: grafana
        image: grafana/grafana:latest
        env:
        - name: GF_DATABASE_TYPE
          value: postgres
        - name: GF_DATABASE_HOST
          value: postgres.helios-visualization.svc:5432
        - name: GF_UNIFIED_ALERTING_ENABLED
          value: "true"
        volumeMounts:
        - name: dashboards
          mountPath: /etc/grafana/provisioning/dashboards
        - name: datasources
          mountPath: /etc/grafana/provisioning/datasources
        - name: dashboard-files
          mountPath: /var/lib/grafana/dashboards
```

**Pre-built Dashboard Categories**:

```yaml
# dashboards/provisioning.yaml
apiVersion: 1
providers:
  - name: 'helios-core'
    folder: 'Network Overview'
    type: file
    options:
      path: /var/lib/grafana/dashboards/core

  - name: 'helios-vendor'
    folder: 'Vendor Specific'
    type: file
    options:
      path: /var/lib/grafana/dashboards/vendor

  - name: 'helios-user'
    folder: 'Custom'
    type: file
    options:
      path: /var/lib/grafana/dashboards/user
```

**Core Dashboards to Include**:

| Dashboard | Purpose | Key Metrics |
|-----------|---------|-------------|
| Network Overview | Fleet-wide health | Up/down status, error rates |
| Interface Analytics | Per-interface drill-down | Traffic, errors, discards |
| Device Health | Per-device metrics | CPU, memory, uptime |
| Site Overview | Datacenter view | Aggregated by site |
| Latency & Reachability | Blackbox results | Ping, TCP, HTTP probes |
| BGP Status | Routing health | Session state, prefixes |
| Top Talkers | Traffic analysis | Highest bandwidth interfaces |
| Alerts Dashboard | Alert overview | Active, silenced, history |

---

## Scalability Design

### gnmic Scaling Strategy

**Horizontal Pod Autoscaler**:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: gnmic-hpa
  namespace: helios-collection
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: gnmic
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Pods
    pods:
      metric:
        name: gnmic_subscription_active_count
      target:
        type: AverageValue
        averageValue: "500"  # Target 500 subscriptions per pod
```

**Scaling Tiers**:

| Scale | Devices | gnmic Replicas | Subscriptions/Pod |
|-------|---------|----------------|-------------------|
| Small | <50 | 2 | 25 |
| Medium | 50-500 | 3-5 | 100-150 |
| Large | 500-2000 | 5-10 | 200-400 |
| XL | 2000-8000 | 10-20 | 400-500 |

**Resource Scaling per gnmic Pod**:

```yaml
# Small scale (< 50 devices)
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# XL scale (8000+ devices)
resources:
  requests:
    cpu: 1000m
    memory: 2Gi
  limits:
    cpu: 4000m
    memory: 8Gi
```

### Prometheus Sharding Strategy

**Sharding by HashMod**:

```yaml
# prometheus-shard-0.yaml
scrape_configs:
  - job_name: 'gnmic'
    relabel_configs:
      - source_labels: [__address__]
        modulus: 4
        target_label: __tmp_hash
        action: hashmod
      - source_labels: [__tmp_hash]
        regex: ^0$
        action: keep
```

**Prometheus Operator Sharding**:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: helios
  namespace: helios-storage
spec:
  replicas: 2       # HA per shard
  shards: 4         # Number of shards
  retention: 2h     # Short retention, Thanos for long-term

  externalLabels:
    cluster: production
    replica: $(POD_NAME)

  thanos:
    image: quay.io/thanos/thanos:latest
    objectStorageConfig:
      name: thanos-objstore
      key: objstore.yaml
```

**Sharding Recommendations**:

| Scale | Prometheus Shards | Replicas per Shard | Total Prometheus Pods |
|-------|-------------------|--------------------|-----------------------|
| Small | 1 | 2 | 2 |
| Medium | 2 | 2 | 4 |
| Large | 4 | 2 | 8 |
| XL | 8 | 2 | 16 |

### Thanos Query Federation

**Query Frontend with Caching**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thanos-query-frontend
  namespace: helios-storage
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: thanos-query-frontend
        image: quay.io/thanos/thanos:latest
        args:
          - query-frontend
          - --http-address=0.0.0.0:9090
          - --query-frontend.downstream-url=http://thanos-query:9090
          - --query-range.split-interval=24h
          - --query-range.max-retries-per-request=3
          - --query-frontend.log-queries-longer-than=10s
          - --cache-compression-type=snappy
```

**Multi-Datacenter Query**:

```
                    +------------------------+
                    | Thanos Query (Global)  |
                    +------------------------+
                              |
            +-----------------+-----------------+
            |                 |                 |
    +-------v-------+ +-------v-------+ +-------v-------+
    | Store Gateway | | Store Gateway | | Store Gateway |
    |    (DC1)      | |    (DC2)      | |    (DC3)      |
    +---------------+ +---------------+ +---------------+
            |                 |                 |
    +-------v-------+ +-------v-------+ +-------v-------+
    |    MinIO      | |    MinIO      | |     S3        |
    |    (DC1)      | |    (DC2)      | |   (Cloud)     |
    +---------------+ +---------------+ +---------------+
```

---

## Deployment Architecture

### Namespace Structure

```
helios-system/
├── helios-integration/     # NetBox sync, target generation
├── helios-collection/      # gnmic, exporters
├── helios-storage/         # Prometheus, Thanos, MinIO
├── helios-visualization/   # Grafana, Alertmanager
└── helios-automation/      # Runbook operator, jobs
```

**Namespace Definitions**:

```yaml
# namespaces.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: helios-integration
  labels:
    app.kubernetes.io/part-of: helios
---
apiVersion: v1
kind: Namespace
metadata:
  name: helios-collection
  labels:
    app.kubernetes.io/part-of: helios
---
apiVersion: v1
kind: Namespace
metadata:
  name: helios-storage
  labels:
    app.kubernetes.io/part-of: helios
---
apiVersion: v1
kind: Namespace
metadata:
  name: helios-visualization
  labels:
    app.kubernetes.io/part-of: helios
---
apiVersion: v1
kind: Namespace
metadata:
  name: helios-automation
  labels:
    app.kubernetes.io/part-of: helios
```

### Helm Chart Organization

**Recommended: Umbrella Chart Structure**

```
helios/
├── Chart.yaml                    # Umbrella chart
├── values.yaml                   # Default values
├── values-small.yaml             # Small scale overrides
├── values-medium.yaml            # Medium scale overrides
├── values-large.yaml             # Large scale overrides
├── values-xl.yaml                # XL scale overrides
├── charts/
│   ├── helios-integration/       # Custom chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── cronjob-target-sync.yaml
│   │       ├── configmap-netbox.yaml
│   │       └── secret-netbox.yaml
│   │
│   ├── helios-collection/        # Custom chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── statefulset-gnmic.yaml
│   │       ├── deployment-snmp-exporter.yaml
│   │       ├── deployment-blackbox-exporter.yaml
│   │       ├── configmap-gnmic.yaml
│   │       ├── configmap-snmp.yaml
│   │       └── servicemonitor.yaml
│   │
│   ├── helios-visualization/     # Custom chart
│   │   ├── Chart.yaml
│   │   ├── values.yaml
│   │   └── templates/
│   │       ├── deployment-grafana.yaml
│   │       ├── configmap-dashboards.yaml
│   │       └── configmap-datasources.yaml
│   │
│   └── helios-automation/        # Custom chart
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── deployment-runbook-operator.yaml
│           ├── crd-runbook.yaml
│           └── rbac.yaml
│
└── dependencies/                 # External chart references
    ├── kube-prometheus-stack     # From prometheus-community
    ├── thanos                    # From bitnami
    └── minio                     # From minio
```

**Chart.yaml (Umbrella)**:

```yaml
apiVersion: v2
name: helios
description: CNCF-native network observability platform
version: 0.1.0
appVersion: "1.0.0"

dependencies:
  - name: kube-prometheus-stack
    version: "56.x.x"
    repository: https://prometheus-community.github.io/helm-charts
    condition: prometheus.enabled

  - name: thanos
    version: "12.x.x"
    repository: https://charts.bitnami.com/bitnami
    condition: thanos.enabled

  - name: minio
    version: "5.x.x"
    repository: https://charts.min.io/
    condition: minio.enabled

  - name: helios-integration
    version: "0.1.0"

  - name: helios-collection
    version: "0.1.0"

  - name: helios-visualization
    version: "0.1.0"

  - name: helios-automation
    version: "0.1.0"
```

### ArgoCD ApplicationSet Patterns

**ApplicationSet for Helios Deployment**:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: helios
  namespace: argocd
spec:
  generators:
    - list:
        elements:
          - cluster: production
            scale: large
            objectStorage: s3
          - cluster: staging
            scale: small
            objectStorage: minio

  template:
    metadata:
      name: 'helios-{{cluster}}'
    spec:
      project: helios
      source:
        repoURL: https://github.com/org/helios.git
        targetRevision: HEAD
        path: deploy/helm
        helm:
          valueFiles:
            - values.yaml
            - values-{{scale}}.yaml
            - values-{{objectStorage}}.yaml

      destination:
        server: '{{cluster}}'
        namespace: helios-system

      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
```

**App-of-Apps Pattern**:

```yaml
# apps/helios.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: helios
  namespace: argocd
  finalizers:
    - resources-finalizer.argocd.argoproj.io
spec:
  project: default
  source:
    repoURL: https://github.com/org/helios.git
    targetRevision: HEAD
    path: deploy/argocd
  destination:
    server: https://kubernetes.default.svc
    namespace: argocd
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

---

## Data Model

### NetBox Custom Fields

**Required Custom Fields for Devices**:

| Field Name | Type | Description | Example Values |
|------------|------|-------------|----------------|
| `gnmi_enabled` | Boolean | Device supports gNMI | true/false |
| `gnmi_port` | Integer | gNMI port (default 6030) | 6030, 57400 |
| `snmp_enabled` | Boolean | Device supports SNMP | true/false |
| `snmp_version` | Selection | SNMP version | 2c, 3 |
| `snmp_module` | Text | SNMP exporter module | if_mib, arista_sw |
| `blackbox_probes` | Multi-select | Probes to run | icmp, tcp_22, http_443 |
| `telemetry_profile` | Text | OpenConfig subscription profile | default, detailed, minimal |
| `monitoring_tier` | Selection | Monitoring priority | critical, standard, best-effort |

**Custom Field Definitions (NetBox API)**:

```python
custom_fields = [
    {
        "name": "gnmi_enabled",
        "type": "boolean",
        "object_types": ["dcim.device"],
        "label": "gNMI Enabled",
        "description": "Device supports gNMI streaming telemetry",
        "default": False,
    },
    {
        "name": "telemetry_profile",
        "type": "select",
        "object_types": ["dcim.device"],
        "label": "Telemetry Profile",
        "description": "OpenConfig subscription profile",
        "choices": ["minimal", "default", "detailed", "custom"],
        "default": "default",
    },
    {
        "name": "monitoring_tier",
        "type": "select",
        "object_types": ["dcim.device"],
        "label": "Monitoring Tier",
        "description": "Monitoring priority level",
        "choices": ["critical", "standard", "best-effort"],
        "default": "standard",
    },
]
```

### Label Taxonomy for Prometheus Metrics

**Standard Labels (Applied to All Metrics)**:

| Label | Source | Description | Example |
|-------|--------|-------------|---------|
| `device` | NetBox | Device hostname | core-rtr-01 |
| `site` | NetBox | Site slug | dc1-east |
| `region` | NetBox | Region slug | us-east |
| `vendor` | NetBox | Manufacturer | arista |
| `platform` | NetBox | Platform/OS | eos |
| `role` | NetBox | Device role | core-router |
| `tier` | NetBox | Monitoring tier | critical |
| `cluster` | K8s | Prometheus cluster | production |

**Interface-Specific Labels**:

| Label | Source | Description | Example |
|-------|--------|-------------|---------|
| `interface` | gNMI/SNMP | Interface name | Ethernet1/1 |
| `if_alias` | gNMI/SNMP | Interface description | uplink-to-spine |
| `if_type` | gNMI/SNMP | Interface type | ethernetCsmacd |
| `speed` | gNMI/SNMP | Interface speed | 100000 |

**Relabeling Configuration**:

```yaml
# prometheus relabel configs
relabel_configs:
  # Standardize vendor names
  - source_labels: [vendor]
    regex: 'arista.*'
    target_label: vendor
    replacement: 'arista'

  # Add monitoring tier from device role
  - source_labels: [role]
    regex: '(core-router|core-switch)'
    target_label: tier
    replacement: 'critical'

  # Drop high-cardinality labels
  - action: labeldrop
    regex: '__meta_.*'
```

### Dashboard Variables and Filtering

**Standard Dashboard Variables**:

```json
{
  "templating": {
    "list": [
      {
        "name": "region",
        "type": "query",
        "query": "label_values(up{job=\"gnmic\"}, region)",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "site",
        "type": "query",
        "query": "label_values(up{job=\"gnmic\", region=~\"$region\"}, site)",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "device",
        "type": "query",
        "query": "label_values(up{job=\"gnmic\", site=~\"$site\"}, device)",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "role",
        "type": "query",
        "query": "label_values(up{job=\"gnmic\"}, role)",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "vendor",
        "type": "query",
        "query": "label_values(up{job=\"gnmic\"}, vendor)",
        "multi": true,
        "includeAll": true
      }
    ]
  }
}
```

**Variable Hierarchy**:

```
Region (us-east, us-west, eu-central)
  └── Site (dc1-east, dc2-west, dc3-central)
       └── Device (core-rtr-01, dist-sw-01)
            └── Interface (Ethernet1/1, Ethernet2/1)
```

---

## Extensibility Architecture

### Git Repository Structure for User Customizations

```
helios-config/                    # User-managed config repository
├── README.md
├── .github/
│   └── workflows/
│       ├── validate.yaml         # CI: Validate configs
│       └── deploy.yaml           # CD: Deploy to cluster
│
├── snmp/
│   ├── modules/                  # Custom SNMP modules
│   │   ├── custom_vendor.yml
│   │   └── proprietary_mib.yml
│   └── mibs/                     # MIB files
│       ├── VENDOR-MIB.txt
│       └── PROPRIETARY-MIB.txt
│
├── gnmic/
│   ├── subscriptions/            # Custom OpenConfig subscriptions
│   │   ├── vendor_specific.yaml
│   │   └── custom_paths.yaml
│   └── processors/               # Event processors
│       └── custom_transforms.yaml
│
├── scripts/
│   └── exporters/                # Custom script_exporter scripts
│       ├── check_bgp_neighbors.sh
│       └── validate_config.py
│
├── dashboards/
│   ├── custom/                   # User-created dashboards
│   │   ├── my_network_view.json
│   │   └── vendor_specific.json
│   └── overrides/                # Modifications to default dashboards
│       └── interface_analytics.json
│
├── alerts/
│   ├── rules/                    # Custom alert rules
│   │   ├── custom_alerts.yaml
│   │   └── vendor_specific.yaml
│   └── receivers/                # Alert routing overrides
│       └── custom_routes.yaml
│
└── runbooks/
    ├── definitions/              # Runbook YAML definitions
    │   ├── interface_bounce.yaml
    │   └── clear_bgp_session.yaml
    └── scripts/                  # Runbook scripts
        ├── interface_bounce.py
        └── clear_bgp.py
```

### CI/CD Pipeline for Config Validation and Deployment

**GitHub Actions Workflow: Validation**:

```yaml
# .github/workflows/validate.yaml
name: Validate Helios Config

on:
  pull_request:
    paths:
      - 'snmp/**'
      - 'gnmic/**'
      - 'dashboards/**'
      - 'alerts/**'
      - 'runbooks/**'

jobs:
  validate-snmp:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate SNMP modules
        run: |
          docker run --rm -v $PWD/snmp:/snmp \
            prom/snmp-exporter:latest \
            --config.check --config.file=/snmp/modules/*.yml

  validate-gnmic:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate gnmic configs
        run: |
          docker run --rm -v $PWD/gnmic:/config \
            ghcr.io/openconfig/gnmic:latest \
            --config /config/subscriptions/*.yaml \
            --dry-run

  validate-dashboards:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate Grafana dashboards
        run: |
          npm install -g grafana-toolkit
          for f in dashboards/**/*.json; do
            grafana-toolkit validate "$f"
          done

  validate-alerts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate Prometheus rules
        run: |
          docker run --rm -v $PWD/alerts:/alerts \
            prom/prometheus:latest \
            promtool check rules /alerts/rules/*.yaml

  validate-runbooks:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Validate runbook definitions
        run: |
          python scripts/validate_runbooks.py runbooks/definitions/*.yaml
```

**GitHub Actions Workflow: Deployment**:

```yaml
# .github/workflows/deploy.yaml
name: Deploy Helios Config

on:
  push:
    branches: [main]
    paths:
      - 'snmp/**'
      - 'gnmic/**'
      - 'dashboards/**'
      - 'alerts/**'
      - 'runbooks/**'

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate ConfigMaps
        run: |
          ./scripts/generate-configmaps.sh

      - name: Commit ConfigMaps to GitOps repo
        uses: cpina/github-action-push-to-another-repository@main
        with:
          source-directory: 'generated/'
          destination-github-username: 'org'
          destination-repository-name: 'helios-deploy'
          target-branch: 'main'
          target-directory: 'config/'
```

### ConfigMap Generation Process

**ConfigMap Generator Script**:

```bash
#!/bin/bash
# scripts/generate-configmaps.sh

OUTPUT_DIR="generated/configmaps"
mkdir -p $OUTPUT_DIR

# Generate SNMP exporter ConfigMap
cat <<EOF > $OUTPUT_DIR/snmp-exporter-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: snmp-exporter-config
  namespace: helios-collection
data:
$(for f in snmp/modules/*.yml; do
  echo "  $(basename $f): |"
  sed 's/^/    /' "$f"
done)
EOF

# Generate gnmic subscriptions ConfigMap
cat <<EOF > $OUTPUT_DIR/gnmic-subscriptions.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: gnmic-subscriptions
  namespace: helios-collection
data:
$(for f in gnmic/subscriptions/*.yaml; do
  echo "  $(basename $f): |"
  sed 's/^/    /' "$f"
done)
EOF

# Generate Grafana dashboards ConfigMap
cat <<EOF > $OUTPUT_DIR/grafana-dashboards-custom.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards-custom
  namespace: helios-visualization
  labels:
    grafana_dashboard: "1"
data:
$(for f in dashboards/custom/*.json; do
  echo "  $(basename $f): |"
  sed 's/^/    /' "$f"
done)
EOF

# Generate Prometheus rules ConfigMap
cat <<EOF > $OUTPUT_DIR/prometheus-rules-custom.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: helios-custom-rules
  namespace: helios-storage
  labels:
    prometheus: helios
    role: alert-rules
spec:
$(cat alerts/rules/*.yaml | sed 's/^/  /')
EOF
```

---

## Runbook Automation Design

### Runbook Definition Format

**Custom Resource Definition (CRD)**:

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: runbooks.helios.io
spec:
  group: helios.io
  names:
    kind: Runbook
    listKind: RunbookList
    plural: runbooks
    singular: runbook
    shortNames:
      - rb
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: [name, description, steps]
              properties:
                name:
                  type: string
                description:
                  type: string
                category:
                  type: string
                  enum: [interface, bgp, system, custom]
                requiresApproval:
                  type: boolean
                  default: false
                allowedRoles:
                  type: array
                  items:
                    type: string
                parameters:
                  type: array
                  items:
                    type: object
                    properties:
                      name:
                        type: string
                      type:
                        type: string
                        enum: [string, integer, boolean, device, interface]
                      required:
                        type: boolean
                      description:
                        type: string
                steps:
                  type: array
                  items:
                    type: object
                    properties:
                      name:
                        type: string
                      action:
                        type: string
                        enum: [gnmi_set, gnmi_get, script, wait, notify]
                      config:
                        type: object
                        x-kubernetes-preserve-unknown-fields: true
```

**Example Runbook Definition**:

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: interface-bounce
  namespace: helios-automation
spec:
  name: Bounce Interface
  description: |
    Administratively disables and re-enables an interface.
    Useful for clearing interface errors or resetting port state.
  category: interface
  requiresApproval: false
  allowedRoles:
    - network-engineer
    - network-admin

  parameters:
    - name: device
      type: device
      required: true
      description: Target device hostname
    - name: interface
      type: interface
      required: true
      description: Interface to bounce
    - name: wait_seconds
      type: integer
      required: false
      description: Seconds to wait before re-enabling (default 5)

  steps:
    - name: Disable Interface
      action: gnmi_set
      config:
        path: /interfaces/interface[name={{interface}}]/config/enabled
        value: false
        encoding: json_ietf

    - name: Wait for Link Down
      action: wait
      config:
        duration: "{{wait_seconds | default(5)}}s"

    - name: Enable Interface
      action: gnmi_set
      config:
        path: /interfaces/interface[name={{interface}}]/config/enabled
        value: true
        encoding: json_ietf

    - name: Verify Interface Up
      action: gnmi_get
      config:
        path: /interfaces/interface[name={{interface}}]/state/oper-status
        expected: UP
        timeout: 30s

    - name: Notify Completion
      action: notify
      config:
        channel: slack
        message: "Interface {{interface}} on {{device}} bounced successfully"
```

### Execution Mechanism

**Runbook Operator Architecture**:

```
+-------------------+     +--------------------+     +----------------+
|   UI / CLI        | --> | Runbook Operator   | --> |  gnmic Client  |
| (trigger runbook) |     | (K8s Controller)   |     |  (gNMI write)  |
+-------------------+     +----------+---------+     +----------------+
                                     |
                          +----------v---------+
                          |  RunbookExecution  |
                          |  (Custom Resource) |
                          +----------+---------+
                                     |
                          +----------v---------+
                          |   Execution Job    |
                          |   (K8s Job)        |
                          +--------------------+
```

**RunbookExecution CRD**:

```yaml
apiVersion: helios.io/v1alpha1
kind: RunbookExecution
metadata:
  name: interface-bounce-20260109-143022
  namespace: helios-automation
spec:
  runbookRef:
    name: interface-bounce
  parameters:
    device: core-rtr-01
    interface: Ethernet1/1
    wait_seconds: 10
  triggeredBy: jsmith@example.com
  triggeredAt: "2026-01-09T14:30:22Z"

status:
  phase: Completed  # Pending, Running, Completed, Failed, Cancelled
  startTime: "2026-01-09T14:30:23Z"
  completionTime: "2026-01-09T14:30:45Z"
  steps:
    - name: Disable Interface
      status: Completed
      startTime: "2026-01-09T14:30:23Z"
      completionTime: "2026-01-09T14:30:24Z"
    - name: Wait for Link Down
      status: Completed
      startTime: "2026-01-09T14:30:24Z"
      completionTime: "2026-01-09T14:30:34Z"
    - name: Enable Interface
      status: Completed
      startTime: "2026-01-09T14:30:34Z"
      completionTime: "2026-01-09T14:30:35Z"
    - name: Verify Interface Up
      status: Completed
      startTime: "2026-01-09T14:30:35Z"
      completionTime: "2026-01-09T14:30:42Z"
    - name: Notify Completion
      status: Completed
      startTime: "2026-01-09T14:30:42Z"
      completionTime: "2026-01-09T14:30:43Z"
```

**Operator Controller (Go)**:

```go
// controllers/runbookexecution_controller.go
func (r *RunbookExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var execution heliosv1alpha1.RunbookExecution
    if err := r.Get(ctx, req.NamespacedName, &execution); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    switch execution.Status.Phase {
    case "":
        // New execution - validate and start
        if err := r.validateExecution(&execution); err != nil {
            return r.markFailed(ctx, &execution, err)
        }
        return r.startExecution(ctx, &execution)

    case "Running":
        // Check job status
        return r.checkProgress(ctx, &execution)

    case "PendingApproval":
        // Check if approved
        return r.checkApproval(ctx, &execution)
    }

    return ctrl.Result{}, nil
}

func (r *RunbookExecutionReconciler) startExecution(ctx context.Context, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
    // Create execution job
    job := &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      exec.Name,
            Namespace: exec.Namespace,
            Labels: map[string]string{
                "helios.io/runbook-execution": exec.Name,
            },
        },
        Spec: batchv1.JobSpec{
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    Containers: []corev1.Container{{
                        Name:  "executor",
                        Image: "helios/runbook-executor:latest",
                        Args: []string{
                            "--execution", exec.Name,
                            "--namespace", exec.Namespace,
                        },
                        EnvFrom: []corev1.EnvFromSource{{
                            SecretRef: &corev1.SecretEnvSource{
                                LocalObjectReference: corev1.LocalObjectReference{
                                    Name: "gnmi-credentials",
                                },
                            },
                        }},
                    }},
                    RestartPolicy: corev1.RestartPolicyNever,
                },
            },
        },
    }

    if err := r.Create(ctx, job); err != nil {
        return ctrl.Result{}, err
    }

    exec.Status.Phase = "Running"
    exec.Status.StartTime = &metav1.Time{Time: time.Now()}
    return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, exec)
}
```

### Audit Trail and Approval Workflow

**Audit Logging**:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: runbook-audit-config
  namespace: helios-automation
data:
  audit.yaml: |
    sinks:
      - type: elasticsearch
        url: http://elasticsearch.logging.svc:9200
        index: helios-runbook-audit

      - type: webhook
        url: https://audit.example.com/webhook
        headers:
          Authorization: Bearer ${AUDIT_TOKEN}

    events:
      - runbook.triggered
      - runbook.approved
      - runbook.rejected
      - runbook.started
      - runbook.step_completed
      - runbook.completed
      - runbook.failed
      - runbook.cancelled
```

**Approval Workflow**:

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: clear-bgp-all
  namespace: helios-automation
spec:
  name: Clear All BGP Sessions
  description: Clear all BGP sessions on a device (disruptive!)
  category: bgp
  requiresApproval: true
  approvers:
    - group: network-admin
    - user: emergency-approver@example.com
  approvalTimeout: 1h

  steps:
    - name: Clear BGP Sessions
      action: script
      config:
        script: clear_bgp.py
        args:
          - --device={{device}}
          - --all-neighbors
```

**Approval UI Integration**:

```
+------------------+     +------------------+     +------------------+
|  Slack / Teams   | --> |  Approval API    | --> |  Runbook         |
|  (approval msg)  |     |  (webhook)       |     |  Operator        |
+------------------+     +------------------+     +------------------+
        |                         |
        v                         v
+------------------+     +------------------+
|  Approve/Reject  |     |  Update CR       |
|  (button click)  |     |  (status)        |
+------------------+     +------------------+
```

---

## Security Considerations

### RBAC for Different User Roles

**Role Definitions**:

```yaml
# Network Viewer - Read-only access to dashboards and metrics
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: helios-viewer
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    resourceNames: ["grafana-dashboards-*"]
    verbs: ["get", "list"]
  - apiGroups: ["helios.io"]
    resources: ["runbooks"]
    verbs: ["get", "list"]
  - apiGroups: ["helios.io"]
    resources: ["runbookexecutions"]
    verbs: ["get", "list"]
---
# Network Engineer - Can execute runbooks, view configs
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: helios-engineer
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list"]
  - apiGroups: ["helios.io"]
    resources: ["runbooks"]
    verbs: ["get", "list"]
  - apiGroups: ["helios.io"]
    resources: ["runbookexecutions"]
    verbs: ["get", "list", "create"]
---
# Network Admin - Full access including approvals and config changes
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: helios-admin
rules:
  - apiGroups: [""]
    resources: ["configmaps", "secrets"]
    verbs: ["*"]
  - apiGroups: ["helios.io"]
    resources: ["*"]
    verbs: ["*"]
  - apiGroups: ["monitoring.coreos.com"]
    resources: ["prometheusrules", "servicemonitors"]
    verbs: ["*"]
```

**Grafana RBAC Integration**:

```yaml
# grafana.ini
[auth.generic_oauth]
enabled = true
name = Kubernetes OIDC
client_id = grafana
client_secret = ${OAUTH_SECRET}
scopes = openid profile email groups
auth_url = https://dex.example.com/auth
token_url = https://dex.example.com/token
api_url = https://dex.example.com/userinfo

role_attribute_path = contains(groups[*], 'network-admin') && 'Admin' || contains(groups[*], 'network-engineer') && 'Editor' || 'Viewer'
```

### Secrets Management

**Device Credentials Architecture**:

```
+------------------+     +------------------+     +------------------+
|   Vault / AWS    | --> |  External        | --> |  K8s Secrets     |
|   Secrets Mgr    |     |  Secrets Op      |     |  (synced)        |
+------------------+     +------------------+     +------------------+
                                                         |
                    +------------------------------------+
                    |                    |               |
              +-----v-----+        +-----v-----+   +-----v-----+
              |   gnmic   |        |   SNMP    |   |  Runbook  |
              | (gNMI)    |        |  Exporter |   |  Executor |
              +-----------+        +-----------+   +-----------+
```

**External Secrets Operator Configuration**:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: gnmi-credentials
  namespace: helios-collection
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: gnmi-credentials
    creationPolicy: Owner
  data:
    - secretKey: username
      remoteRef:
        key: helios/gnmi
        property: username
    - secretKey: password
      remoteRef:
        key: helios/gnmi
        property: password
---
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: snmp-credentials
  namespace: helios-collection
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: snmp-credentials
    creationPolicy: Owner
  dataFrom:
    - extract:
        key: helios/snmp/communities
```

**Secret Rotation**:

```yaml
# CronJob for credential rotation notification
apiVersion: batch/v1
kind: CronJob
metadata:
  name: credential-rotation-check
  namespace: helios-automation
spec:
  schedule: "0 8 * * 1"  # Weekly Monday 8am
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: checker
            image: helios/credential-checker:latest
            env:
            - name: VAULT_ADDR
              value: https://vault.example.com
            - name: MAX_AGE_DAYS
              value: "90"
            - name: NOTIFY_SLACK_WEBHOOK
              valueFrom:
                secretKeyRef:
                  name: slack-webhook
                  key: url
          restartPolicy: OnFailure
```

### Network Segmentation for Collection

**Network Policy for Collection Namespace**:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: collection-egress
  namespace: helios-collection
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    # Allow gNMI to network devices (specific CIDRs)
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8  # Management network
      ports:
        - protocol: TCP
          port: 6030  # gNMI
        - protocol: TCP
          port: 57400  # gNMI alt

    # Allow SNMP to network devices
    - to:
        - ipBlock:
            cidr: 10.0.0.0/8
      ports:
        - protocol: UDP
          port: 161  # SNMP

    # Allow Prometheus scraping
    - to:
        - namespaceSelector:
            matchLabels:
              name: helios-storage
      ports:
        - protocol: TCP
          port: 9090

    # Allow DNS
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - protocol: UDP
          port: 53
```

**Cilium Network Policy (if using Cilium)**:

```yaml
apiVersion: cilium.io/v2
kind: CiliumNetworkPolicy
metadata:
  name: gnmic-to-devices
  namespace: helios-collection
spec:
  endpointSelector:
    matchLabels:
      app: gnmic
  egress:
    - toEntities:
        - world
      toPorts:
        - ports:
            - port: "6030"
              protocol: TCP
            - port: "57400"
              protocol: TCP
      # Optional: L7 policy for gRPC
      rules:
        http:
          - method: "POST"
            path: "/gnmi.gNMI/.*"
```

---

## Resource Estimates

### CPU/Memory Requirements by Scale

#### Small Scale (< 50 devices)

| Component | Replicas | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|----------|-------------|-----------|----------------|--------------|
| gnmic | 2 | 250m | 1000m | 256Mi | 1Gi |
| SNMP Exporter | 1 | 100m | 500m | 128Mi | 512Mi |
| Blackbox Exporter | 1 | 100m | 500m | 128Mi | 256Mi |
| Prometheus | 2 | 500m | 2000m | 2Gi | 4Gi |
| Thanos Sidecar | 2 | 100m | 500m | 128Mi | 512Mi |
| Thanos Query | 2 | 250m | 1000m | 512Mi | 2Gi |
| Thanos Store | 1 | 250m | 1000m | 1Gi | 4Gi |
| Thanos Compactor | 1 | 250m | 1000m | 512Mi | 2Gi |
| MinIO | 1 | 250m | 1000m | 1Gi | 4Gi |
| Grafana | 1 | 250m | 1000m | 256Mi | 1Gi |
| Alertmanager | 2 | 100m | 500m | 128Mi | 256Mi |
| Target Generator | 1 | 100m | 250m | 128Mi | 256Mi |
| **Total** | **17** | **2.6 cores** | **11 cores** | **7Gi** | **24Gi** |

#### Medium Scale (50-500 devices)

| Component | Replicas | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|----------|-------------|-----------|----------------|--------------|
| gnmic | 4 | 500m | 2000m | 512Mi | 2Gi |
| SNMP Exporter | 2 | 250m | 1000m | 256Mi | 1Gi |
| Blackbox Exporter | 2 | 100m | 500m | 128Mi | 512Mi |
| Prometheus | 4 | 1000m | 4000m | 4Gi | 8Gi |
| Thanos Sidecar | 4 | 250m | 1000m | 256Mi | 1Gi |
| Thanos Query | 3 | 500m | 2000m | 1Gi | 4Gi |
| Thanos Store | 2 | 500m | 2000m | 2Gi | 8Gi |
| Thanos Compactor | 1 | 500m | 2000m | 1Gi | 4Gi |
| MinIO | 4 | 500m | 2000m | 2Gi | 8Gi |
| Grafana | 2 | 500m | 2000m | 512Mi | 2Gi |
| Alertmanager | 3 | 100m | 500m | 128Mi | 512Mi |
| Target Generator | 1 | 250m | 500m | 256Mi | 512Mi |
| **Total** | **32** | **10 cores** | **42 cores** | **24Gi** | **80Gi** |

#### Large Scale (500-2000 devices)

| Component | Replicas | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|----------|-------------|-----------|----------------|--------------|
| gnmic | 8 | 1000m | 4000m | 1Gi | 4Gi |
| SNMP Exporter | 4 | 500m | 2000m | 512Mi | 2Gi |
| Blackbox Exporter | 4 | 250m | 1000m | 256Mi | 1Gi |
| Prometheus | 8 | 2000m | 8000m | 8Gi | 16Gi |
| Thanos Sidecar | 8 | 500m | 2000m | 512Mi | 2Gi |
| Thanos Query | 4 | 1000m | 4000m | 2Gi | 8Gi |
| Thanos Query Frontend | 2 | 500m | 2000m | 1Gi | 4Gi |
| Thanos Store | 4 | 1000m | 4000m | 4Gi | 16Gi |
| Thanos Compactor | 1 | 1000m | 4000m | 2Gi | 8Gi |
| MinIO | 8 | 1000m | 4000m | 4Gi | 16Gi |
| Grafana | 3 | 1000m | 4000m | 1Gi | 4Gi |
| Alertmanager | 3 | 250m | 1000m | 256Mi | 1Gi |
| Target Generator | 2 | 500m | 1000m | 512Mi | 1Gi |
| **Total** | **59** | **32 cores** | **128 cores** | **68Gi** | **216Gi** |

#### XL Scale (2000-8000+ devices)

| Component | Replicas | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|----------|-------------|-----------|----------------|--------------|
| gnmic | 20 | 2000m | 8000m | 2Gi | 8Gi |
| SNMP Exporter | 8 | 1000m | 4000m | 1Gi | 4Gi |
| Blackbox Exporter | 8 | 500m | 2000m | 512Mi | 2Gi |
| Prometheus | 16 | 4000m | 16000m | 16Gi | 32Gi |
| Thanos Sidecar | 16 | 1000m | 4000m | 1Gi | 4Gi |
| Thanos Query | 6 | 2000m | 8000m | 4Gi | 16Gi |
| Thanos Query Frontend | 4 | 1000m | 4000m | 2Gi | 8Gi |
| Thanos Store | 8 | 2000m | 8000m | 8Gi | 32Gi |
| Thanos Compactor | 2 | 2000m | 8000m | 4Gi | 16Gi |
| MinIO (distributed) | 16 | 2000m | 8000m | 8Gi | 32Gi |
| Grafana | 4 | 2000m | 8000m | 2Gi | 8Gi |
| Alertmanager | 5 | 500m | 2000m | 512Mi | 2Gi |
| Target Generator | 3 | 1000m | 2000m | 1Gi | 2Gi |
| **Total** | **116** | **116 cores** | **464 cores** | **212Gi** | **680Gi** |

### Storage Requirements

#### Metric Storage Sizing

**Calculation Formula**:
```
Daily Storage = (samples/day) * (bytes/sample) * (active series)

Where:
- samples/day = 86400 / scrape_interval
- bytes/sample = ~2 bytes (Prometheus TSDB)
- active series = devices * metrics_per_device
```

**Estimates by Scale**:

| Scale | Devices | Metrics/Device | Active Series | Daily Storage | 90-Day Storage |
|-------|---------|----------------|---------------|---------------|----------------|
| Small | 50 | 500 | 25,000 | ~4 GB | ~360 GB |
| Medium | 500 | 500 | 250,000 | ~40 GB | ~3.6 TB |
| Large | 2,000 | 500 | 1,000,000 | ~160 GB | ~14.4 TB |
| XL | 8,000 | 500 | 4,000,000 | ~640 GB | ~57.6 TB |

**With Thanos Compaction** (reduces storage by ~50-70%):

| Scale | Raw 90-Day | After Compaction | Downsampled (5m, 1h) |
|-------|------------|------------------|----------------------|
| Small | 360 GB | 120 GB | 80 GB |
| Medium | 3.6 TB | 1.2 TB | 800 GB |
| Large | 14.4 TB | 4.8 TB | 3.2 TB |
| XL | 57.6 TB | 19.2 TB | 12.8 TB |

#### Object Storage Sizing

```yaml
# MinIO storage recommendations
scale:
  small:
    drives: 4
    size_per_drive: 100GB
    total: 400GB

  medium:
    drives: 8
    size_per_drive: 500GB
    total: 4TB

  large:
    drives: 16
    size_per_drive: 1TB
    total: 16TB

  xl:
    drives: 32
    size_per_drive: 2TB
    total: 64TB
```

---

## ADR Recommendations

### ADR-001: Storage Backend Selection

**Recommendation**: Thanos with Object Storage

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Thanos + MinIO | HA, unlimited retention, S3-compatible | Operational complexity | **Recommended** |
| VictoriaMetrics | Lower resources, simpler | Less ecosystem integration | Good alternative |
| Mimir | Native Grafana, multi-tenant | Newer, less mature | Consider for v2 |
| Prometheus only | Simplest | No HA, limited retention | Not recommended |

**Key Decision Points**:
- Object storage enables infinite retention with cost-effective tiering
- Thanos query layer provides global view across shards
- Thanos sidecar pattern allows incremental migration

---

### ADR-002: gnmic Deployment Model

**Recommendation**: StatefulSet with Kubernetes-based clustering

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| StatefulSet + K8s clustering | Auto-scaling, HA, target distribution | Requires K8s | **Recommended** |
| DaemonSet (per-node) | Simple, predictable placement | Wastes resources, no target balance | Not recommended |
| Deployment | Simple scaling | No stable identity for clustering | Not recommended |
| External (VM-based) | Full control | Not cloud-native | Not recommended |

**Key Decision Points**:
- gnmic native clustering handles target distribution automatically
- StatefulSet provides stable network identities for cluster membership
- HPA can scale based on subscription count

---

### ADR-003: Target Discovery Strategy

**Recommendation**: NetBox as source of truth with CronJob sync

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| NetBox + CronJob | Single source of truth, simple | 5-min sync delay | **Recommended** |
| NetBox + Operator | Real-time sync | More complex | Consider for v2 |
| File-based | Simple | No automation | Not recommended |
| Prometheus SD | Native | Requires custom SD provider | Alternative |

**Key Decision Points**:
- NetBox already contains device inventory
- CronJob simplicity outweighs sync delay
- ConfigMap pattern integrates well with GitOps

---

### ADR-004: Runbook Automation Framework

**Recommendation**: Custom Kubernetes Operator with CRDs

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Custom K8s Operator | Native K8s, audit trail, RBAC | Development effort | **Recommended** |
| Ansible AWX/Tower | Existing tool, GUI | Not K8s-native | Alternative |
| Rundeck | Mature, GUI | Separate system | Not recommended |
| StackStorm | Event-driven | Complex | Not recommended |

**Key Decision Points**:
- CRD-based runbooks version-controlled in Git
- K8s Jobs provide isolation and logging
- Native RBAC integration
- gnmic write operations for gNMI Set

---

### ADR-005: Multi-Datacenter Architecture

**Recommendation**: Prometheus per-DC with Thanos global query

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Prometheus per-DC + Thanos | Fault isolation, global view | Complexity | **Recommended** |
| Single global Prometheus | Simple | Single point of failure | Not recommended |
| Remote-write federation | Lower local storage | Network dependent | Alternative |

**Key Decision Points**:
- Each DC operates independently if connectivity fails
- Thanos Query provides unified view when connected
- Object storage can be local or shared based on connectivity

---

### ADR-006: Alerting Strategy

**Recommendation**: Recording rules in Thanos Ruler, alerts in Prometheus

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Prometheus alerts + Thanos recording | Low latency alerts, global aggregation | Split config | **Recommended** |
| Thanos Ruler only | Single location | Query latency for alerts | Not recommended |
| Prometheus only | Simplest | No global aggregation | Not recommended |

**Key Decision Points**:
- Alerting needs low latency (Prometheus sidecar)
- Recording rules can tolerate Thanos query latency
- Alertmanager cluster for HA notification delivery

---

### ADR-007: Dashboard Provisioning Strategy

**Recommendation**: Git-based with ConfigMap provisioning

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| Git + ConfigMaps | Version controlled, GitOps | Two-step deploy | **Recommended** |
| Grafana UI + database | Easy for users | No version control | Not recommended |
| Grafana Operator | Native K8s | Less mature | Consider for v2 |

**Key Decision Points**:
- All dashboards version controlled in Git
- PR workflow for review and validation
- Provisioning sidecars for automatic reload

---

## Implementation Phases

### Phase 1: Foundation (Weeks 1-4)

1. Deploy Prometheus Operator + Thanos
2. Deploy MinIO for object storage
3. Deploy Grafana + Alertmanager
4. Create namespaces and RBAC
5. Basic dashboards and alerts

### Phase 2: Collection (Weeks 5-8)

1. Deploy gnmic with basic subscriptions
2. Deploy SNMP and Blackbox exporters
3. Integrate with NetBox (manual targets initially)
4. Create device-specific dashboards

### Phase 3: Automation (Weeks 9-12)

1. Deploy Target Generator CronJob
2. Create CI/CD pipeline for configs
3. Deploy Runbook Operator
4. Create initial runbook library

### Phase 4: Scale & Optimize (Weeks 13-16)

1. Implement sharding based on load
2. Add Query Frontend with caching
3. Tune retention and compaction
4. Performance testing and optimization

---

## Related Documents

- [[../overview]] - Project overview
- [[../tasks/task-001-architecture-design]] - Design task
- [[../../apps/overview]] - Existing apps infrastructure
- [[../../k8s-ansible/overview]] - Cluster infrastructure

---

**Document History**:

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-09 | Architecture Review | Initial architecture design |
