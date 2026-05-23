#architecture #helios #flow #netflow #sflow #ipfix

# Helios Flow Collection Architecture

**Version**: 1.0
**Date**: 2026-01-09
**Author**: Architecture Review
**Status**: Draft
**Feature**: v1.x

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Requirements Analysis](#requirements-analysis)
3. [Technology Evaluation](#technology-evaluation)
4. [Recommended Architecture](#recommended-architecture)
5. [Component Deep Dive](#component-deep-dive)
6. [Data Model](#data-model)
7. [Grafana Integration](#grafana-integration)
8. [Scaling Strategy](#scaling-strategy)
9. [Helios Integration](#helios-integration)
10. [Kubernetes Manifests](#kubernetes-manifests)
11. [ADR Recommendations](#adr-recommendations)

---

## Executive Summary

This document defines the flow data collection pipeline for Helios v1.x, adding NetFlow v5/v9, IPFIX, and sFlow support to the existing metrics infrastructure. The design prioritizes integration with existing Helios components while using purpose-built storage for flow data.

### Key Design Decisions

| Component | Choice | Rationale |
|-----------|--------|-----------|
| **Collector** | goflow2 | High performance, K8s-native, Cloudflare-proven |
| **Message Queue** | Kafka | Decouples collection from storage, handles bursts |
| **Storage** | ClickHouse | Purpose-built for flow analytics, 10-50x better than alternatives |
| **Visualization** | Grafana + ClickHouse datasource | Consistent with Helios metrics dashboards |
| **Enrichment** | Custom enricher service | NetBox integration for device/site context |

### Architecture Overview

```
+-----------------------------------------------------------------------------+
|                              NETWORK DEVICES                                 |
|   Routers | Switches | Firewalls | Load Balancers | Virtual Appliances      |
+------+----------+----------+----------+----------+----------+---------------+
       |          |          |          |          |          |
       | NetFlow  | NetFlow  |  IPFIX   |  IPFIX   |  sFlow   |  sFlow
       | v5       | v9       |          |          |          |
       v          v          v          v          v          v
+-----------------------------------------------------------------------------+
|                           COLLECTION LAYER                                   |
|                                                                             |
|  +------------------+  +------------------+  +------------------+           |
|  |    goflow2-0     |  |    goflow2-1     |  |    goflow2-N     |           |
|  |   (StatefulSet)  |  |   (StatefulSet)  |  |   (StatefulSet)  |           |
|  |                  |  |                  |  |                  |           |
|  | UDP 2055 (NF)    |  | UDP 2055 (NF)    |  | UDP 2055 (NF)    |           |
|  | UDP 6343 (sFlow) |  | UDP 6343 (sFlow) |  | UDP 6343 (sFlow) |           |
|  +--------+---------+  +--------+---------+  +--------+---------+           |
|           |                     |                     |                     |
|           +---------------------+---------------------+                     |
|                                 |                                           |
|                                 v                                           |
|                    +------------------------+                               |
|                    |    Kafka (Strimzi)     |                               |
|                    |  Topic: helios-flows   |                               |
|                    |  Partitions: 12-48     |                               |
|                    +------------------------+                               |
|                                 |                                           |
+-----------------------------------------------------------------------------+
                                  |
+-----------------------------------------------------------------------------+
|                          PROCESSING LAYER                                    |
|                                                                             |
|  +------------------------------------------------------------------+       |
|  |                    Flow Enricher Service                          |       |
|  |                                                                  |       |
|  |  +------------------+  +------------------+  +------------------+ |       |
|  |  |  NetBox Client   |  |   GeoIP/ASN     |  |  Flow Aggregator | |       |
|  |  | (Device/Site)    |  | (MaxMind/IPinfo) |  |  (1m/5m buckets) | |       |
|  |  +------------------+  +------------------+  +------------------+ |       |
|  +------------------------------------------------------------------+       |
|                                 |                                           |
+-----------------------------------------------------------------------------+
                                  |
+-----------------------------------------------------------------------------+
|                           STORAGE LAYER                                      |
|                                                                             |
|  +------------------------------------------------------------------+       |
|  |                     ClickHouse Cluster                            |       |
|  |                                                                  |       |
|  |  +-------------------+  +-------------------+  +---------------+ |       |
|  |  |   flows_raw       |  |  flows_1m         |  | flows_1h      | |       |
|  |  | (7 days, raw)     |  | (30 days, 1m agg) |  | (90d, 1h agg) | |       |
|  |  +-------------------+  +-------------------+  +---------------+ |       |
|  |                                                                  |       |
|  |  Storage Engine: MergeTree with TTL                             |       |
|  |  Compression: LZ4 (10-20x ratio typical)                        |       |
|  +------------------------------------------------------------------+       |
|                                 |                                           |
+-----------------------------------------------------------------------------+
                                  |
+-----------------------------------------------------------------------------+
|                        VISUALIZATION LAYER                                   |
|                                                                             |
|  +------------------------------------------------------------------+       |
|  |                         Grafana                                   |       |
|  |                                                                  |       |
|  |  +---------------+  +---------------+  +---------------+         |       |
|  |  | Top Talkers   |  | Traffic Matrix|  | AS Path       |         |       |
|  |  | Dashboard     |  | Dashboard     |  | Dashboard     |         |       |
|  |  +---------------+  +---------------+  +---------------+         |       |
|  |                                                                  |       |
|  |  +---------------+  +---------------+  +---------------+         |       |
|  |  | Protocol      |  | Geographic    |  | Interface     |         |       |
|  |  | Distribution  |  | Flow Map      |  | Correlation   |         |       |
|  |  +---------------+  +---------------+  +---------------+         |       |
|  +------------------------------------------------------------------+       |
|                                                                             |
+-----------------------------------------------------------------------------+
```

---

## Requirements Analysis

### Protocol Support Matrix

| Protocol | Version | Typical Sources | Priority |
|----------|---------|-----------------|----------|
| **NetFlow v5** | 5 | Legacy Cisco IOS, older devices | High |
| **NetFlow v9** | 9 | Cisco IOS, IOS-XE, NX-OS | High |
| **IPFIX** | 10 | Modern Cisco, Juniper, Arista, Palo Alto | High |
| **sFlow** | 5 | Arista, Cumulus, Barefoot, Memory-constrained | High |

### Scale Requirements

| Scale | Devices | Expected FPS | Peak FPS | Storage/Day |
|-------|---------|--------------|----------|-------------|
| Small | <50 | 1,000 | 5,000 | ~2 GB |
| Medium | 50-500 | 10,000 | 50,000 | ~20 GB |
| Large | 500-2000 | 100,000 | 500,000 | ~200 GB |
| XL | 2000-8000 | 1,000,000 | 5,000,000 | ~2 TB |

**FPS** = Flows Per Second

### Retention Requirements

| Data Type | Retention | Aggregation |
|-----------|-----------|-------------|
| Raw flows | 7 days | None |
| 1-minute aggregates | 30 days | Per src/dst/port/protocol |
| 1-hour aggregates | 90 days | Per src/dst/port/protocol |
| Daily summaries | 1 year | Per src/dst network |

---

## Technology Evaluation

### Flow Collectors Comparison

| Collector | Language | Performance | K8s Native | Output Formats | Verdict |
|-----------|----------|-------------|------------|----------------|---------|
| **goflow2** | Go | Excellent | Yes | Kafka, Protobuf, JSON | **Recommended** |
| **Akvorado** | Go | Excellent | Docker | Kafka -> ClickHouse | Alternative |
| **pmacct** | C | Excellent | Docker | Kafka, DB, File | Legacy Alternative |
| **nfdump** | C | Good | No | File-based | Not Recommended |
| **vflow** | Go | Good | Yes | Kafka, IPFIX mirror | Specialized |

#### goflow2 (Recommended)

**Pros**:
- Cloudflare-proven at massive scale
- Native Kafka output with Protobuf serialization
- Lightweight, single binary
- Active community (netsampler/goflow2 fork)
- Kubernetes enricher available (netobserv/goflow2-kube-enricher)

**Cons**:
- No built-in enrichment (requires external service)
- No UI (pure collector)

#### Akvorado (Strong Alternative)

**Pros**:
- Complete solution (collector + enricher + UI + ClickHouse integration)
- NetBox-like interface enrichment
- GeoIP/ASN built-in
- Native Grafana plugin

**Cons**:
- More opinionated architecture
- Harder to customize
- Newer project (2022)

**Recommendation**: Use goflow2 for flexibility and Helios integration, with custom enrichment service. Consider Akvorado for standalone deployments.

### Message Queue Comparison

| Queue | Throughput | Durability | K8s Native | Helios Fit |
|-------|------------|------------|------------|------------|
| **Kafka** | 2M+ msg/s | Excellent | Strimzi | **Recommended** |
| **NATS** | 10M+ msg/s | Good (JetStream) | Excellent | Alternative |
| **Direct Write** | N/A | N/A | N/A | Not Recommended |

**Kafka Selection Rationale**:
- Strimzi operator provides production-grade Kafka on K8s
- Excellent durability for flow data (can replay)
- Partitioning enables parallel processing
- Industry standard for flow pipelines
- Akvorado, goflow2, and most tools support Kafka natively

### Storage Backend Comparison

| Database | Flow Query Speed | Compression | K8s Operator | Cost |
|----------|------------------|-------------|--------------|------|
| **ClickHouse** | Excellent | 10-20x | Yes (Altinity) | Low |
| **TimescaleDB** | Good | 5-10x | Yes | Medium |
| **Elasticsearch** | Good | 2-5x | Yes (ECK) | High |
| **Prometheus** | Poor | Good | Yes | N/A |
| **InfluxDB** | Good | 5-10x | Limited | Medium |

#### ClickHouse (Recommended)

Based on [2025 benchmarks](https://sanj.dev/post/clickhouse-timescaledb-influxdb-time-series-comparison):
- **Ingestion**: 2-3M points/second in multi-threaded scenarios
- **Storage**: 1.7x less disk than alternatives, LZ4 compression
- **Queries**: Dominates aggregation across large time windows
- **Flow-specific**: Ideal for Top-N, aggregations, and traffic analysis

**ClickHouse vs Prometheus for Flows**:

| Aspect | ClickHouse | Prometheus |
|--------|------------|------------|
| Data Model | Columnar, flow records | Time series, metrics |
| Cardinality | Unlimited (IP pairs) | Limited (label explosion) |
| Query Type | SQL (aggregations) | PromQL (rates, alerts) |
| Storage per Flow | ~50-100 bytes compressed | N/A (not designed for flows) |
| Retention | Tiered (raw + aggregated) | Single resolution |

**Verdict**: ClickHouse for flows, Prometheus/Thanos for metrics. They serve different purposes.

---

## Recommended Architecture

### Component Summary

| Component | Implementation | Replicas | Purpose |
|-----------|---------------|----------|---------|
| goflow2 | StatefulSet | 2-8 | Flow collection (UDP) |
| Kafka (Strimzi) | StatefulSet | 3 | Message queue, buffering |
| Flow Enricher | Deployment | 2-6 | NetBox + GeoIP enrichment |
| ClickHouse | StatefulSet | 3 (cluster) | Flow storage |
| Grafana | Deployment | 2 | Visualization (existing) |

### Namespace Structure

```
helios-flows/
├── goflow2           # Flow collectors
├── kafka             # Strimzi Kafka cluster
├── flow-enricher     # Custom enrichment service
├── clickhouse        # ClickHouse cluster
└── (uses helios-visualization for Grafana)
```

### Data Flow

```
1. FLOW INGESTION
   Device (UDP 2055/6343) --> goflow2 --> Kafka (protobuf)

2. ENRICHMENT
   Kafka --> Flow Enricher --> Kafka (enriched topic)

   Enricher lookups:
   - NetBox: device name, site, role, region
   - GeoIP: country, city, coordinates
   - ASN: AS number, AS name

3. STORAGE
   Kafka (enriched) --> ClickHouse Kafka Engine --> MergeTree tables

4. AGGREGATION (ClickHouse Materialized Views)
   flows_raw --> flows_1m (SummingMergeTree)
   flows_raw --> flows_1h (SummingMergeTree)

5. VISUALIZATION
   Grafana --> ClickHouse Datasource --> flows_* tables
```

---

## Component Deep Dive

### goflow2 Collector

#### Configuration

```yaml
# goflow2-config.yaml
---
# Listener configuration
listen:
  # NetFlow v5/v9/IPFIX
  - type: netflow
    addr: ":2055"
    workers: 4
  # sFlow
  - type: sflow
    addr: ":6343"
    workers: 2

# Transport to Kafka
transport:
  type: kafka
  kafka:
    brokers:
      - kafka-0.kafka.helios-flows.svc:9092
      - kafka-1.kafka.helios-flows.svc:9092
      - kafka-2.kafka.helios-flows.svc:9092
    topic: helios-flows-raw
    # Protobuf for efficiency
    serialization: protobuf
    # Partition by exporter for ordering
    partition-key: sampler-address

    # Producer tuning
    batch-size: 16384
    batch-bytes: 1048576
    flush-interval: 100ms

    # Compression
    compression: snappy

# Metrics for Prometheus
metrics:
  addr: ":8080"
  path: /metrics

# Logging
log:
  level: info
  format: json
```

#### StatefulSet Manifest

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: goflow2
  namespace: helios-flows
  labels:
    app.kubernetes.io/name: goflow2
    app.kubernetes.io/component: collector
    app.kubernetes.io/part-of: helios
spec:
  serviceName: goflow2
  replicas: 2  # Scale based on FPS
  podManagementPolicy: Parallel
  selector:
    matchLabels:
      app: goflow2
  template:
    metadata:
      labels:
        app: goflow2
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      # Anti-affinity for HA
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: goflow2
              topologyKey: kubernetes.io/hostname

      containers:
      - name: goflow2
        image: netsampler/goflow2:v2.1.3
        args:
          - -config=/etc/goflow2/config.yaml
        ports:
        - name: netflow
          containerPort: 2055
          protocol: UDP
        - name: sflow
          containerPort: 6343
          protocol: UDP
        - name: metrics
          containerPort: 8080
          protocol: TCP

        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi

        volumeMounts:
        - name: config
          mountPath: /etc/goflow2

        livenessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10

        readinessProbe:
          httpGet:
            path: /metrics
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5

        # Security context
        securityContext:
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 1000
          capabilities:
            drop:
              - ALL

      volumes:
      - name: config
        configMap:
          name: goflow2-config

---
# Service for UDP traffic (LoadBalancer or NodePort)
apiVersion: v1
kind: Service
metadata:
  name: goflow2-udp
  namespace: helios-flows
  annotations:
    # MetalLB annotation for on-prem
    metallb.universe.tf/allow-shared-ip: "helios-flows"
spec:
  type: LoadBalancer
  loadBalancerIP: 10.0.100.50  # Static IP for network device config
  externalTrafficPolicy: Local  # Preserve source IP
  ports:
  - name: netflow
    port: 2055
    targetPort: 2055
    protocol: UDP
  - name: sflow
    port: 6343
    targetPort: 6343
    protocol: UDP
  selector:
    app: goflow2

---
# Headless service for internal communication
apiVersion: v1
kind: Service
metadata:
  name: goflow2
  namespace: helios-flows
spec:
  clusterIP: None
  ports:
  - name: metrics
    port: 8080
  selector:
    app: goflow2
```

### Kafka (Strimzi)

#### Kafka Cluster

```yaml
apiVersion: kafka.strimzi.io/v1beta2
kind: Kafka
metadata:
  name: helios-flows
  namespace: helios-flows
spec:
  kafka:
    version: 3.7.0
    replicas: 3

    listeners:
      - name: plain
        port: 9092
        type: internal
        tls: false
      - name: tls
        port: 9093
        type: internal
        tls: true

    config:
      # Performance tuning for flow data
      num.partitions: 12
      default.replication.factor: 2
      min.insync.replicas: 1

      # Retention (flows buffer, not long-term storage)
      log.retention.hours: 24
      log.retention.bytes: 107374182400  # 100GB per partition

      # Segment tuning
      log.segment.bytes: 1073741824  # 1GB segments
      log.cleanup.policy: delete

      # Compression
      compression.type: snappy

      # Transaction (if needed for exactly-once)
      transaction.state.log.replication.factor: 2
      transaction.state.log.min.isr: 1

    storage:
      type: jbod
      volumes:
      - id: 0
        type: persistent-claim
        size: 200Gi
        class: fast-ssd
        deleteClaim: false

    resources:
      requests:
        memory: 4Gi
        cpu: 1000m
      limits:
        memory: 8Gi
        cpu: 4000m

    jvmOptions:
      -Xms: 2048m
      -Xmx: 4096m

    metricsConfig:
      type: jmxPrometheusExporter
      valueFrom:
        configMapKeyRef:
          name: kafka-metrics
          key: kafka-metrics-config.yml

  zookeeper:
    replicas: 3
    storage:
      type: persistent-claim
      size: 10Gi
      class: fast-ssd
    resources:
      requests:
        memory: 1Gi
        cpu: 500m
      limits:
        memory: 2Gi
        cpu: 1000m

  entityOperator:
    topicOperator: {}
    userOperator: {}

---
# Flow topics
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: helios-flows-raw
  namespace: helios-flows
  labels:
    strimzi.io/cluster: helios-flows
spec:
  partitions: 24
  replicas: 2
  config:
    retention.ms: 86400000  # 24 hours
    retention.bytes: -1
    cleanup.policy: delete
    compression.type: snappy

---
apiVersion: kafka.strimzi.io/v1beta2
kind: KafkaTopic
metadata:
  name: helios-flows-enriched
  namespace: helios-flows
  labels:
    strimzi.io/cluster: helios-flows
spec:
  partitions: 24
  replicas: 2
  config:
    retention.ms: 86400000
    compression.type: snappy
```

### Flow Enricher Service

A custom Go service that consumes raw flows from Kafka, enriches them with context from NetBox and GeoIP, and produces to the enriched topic.

#### Enricher Design

```
+---------------------+
|   Kafka Consumer    |
| (helios-flows-raw)  |
+---------+-----------+
          |
          v
+---------------------+     +------------------+
|   Flow Decoder      | --> |  Lookup Cache    |
|   (protobuf)        |     |  (in-memory LRU) |
+---------------------+     +------------------+
          |                          ^
          v                          |
+---------------------+     +------------------+
|  Enrichment Pipeline|     |  NetBox Client   |
|                     | <-- |  (periodic sync) |
+---------------------+     +------------------+
          |                          |
          v                 +------------------+
+---------------------+     |  GeoIP/ASN DB    |
|   Kafka Producer    |     |  (MaxMind)       |
| (helios-flows-      |     +------------------+
|       enriched)     |
+---------------------+
```

#### Enricher ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: flow-enricher-config
  namespace: helios-flows
data:
  config.yaml: |
    kafka:
      brokers:
        - kafka-0.kafka.helios-flows.svc:9092
        - kafka-1.kafka.helios-flows.svc:9092
        - kafka-2.kafka.helios-flows.svc:9092
      consumer:
        group: flow-enricher
        topic: helios-flows-raw
        auto-offset-reset: latest
      producer:
        topic: helios-flows-enriched
        compression: snappy

    netbox:
      url: http://netbox.helios-integration.svc:8080
      token: ${NETBOX_TOKEN}
      sync-interval: 5m
      cache-ttl: 10m

    geoip:
      # MaxMind GeoLite2 databases
      asn-db: /var/lib/geoip/GeoLite2-ASN.mmdb
      city-db: /var/lib/geoip/GeoLite2-City.mmdb
      # Auto-update via geoipupdate sidecar

    enrichment:
      # Lookup exporter IP in NetBox devices
      device-lookup: true
      # Add site, region, role labels
      site-labels: true
      # GeoIP for external IPs
      geoip-external: true
      # ASN for all IPs
      asn-lookup: true

    aggregation:
      # Pre-aggregate in enricher (optional)
      enabled: false
      # Let ClickHouse handle aggregation via materialized views

    metrics:
      addr: ":8080"
      path: /metrics

    log:
      level: info
      format: json
```

#### Enricher Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flow-enricher
  namespace: helios-flows
spec:
  replicas: 3
  selector:
    matchLabels:
      app: flow-enricher
  template:
    metadata:
      labels:
        app: flow-enricher
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
    spec:
      containers:
      - name: enricher
        image: helios/flow-enricher:v1.0.0
        args:
          - --config=/etc/enricher/config.yaml
        ports:
        - name: metrics
          containerPort: 8080

        env:
        - name: NETBOX_TOKEN
          valueFrom:
            secretKeyRef:
              name: netbox-credentials
              key: token

        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi

        volumeMounts:
        - name: config
          mountPath: /etc/enricher
        - name: geoip
          mountPath: /var/lib/geoip

        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10

        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5

      # Sidecar for GeoIP database updates
      - name: geoipupdate
        image: maxmindinc/geoipupdate:v6.1
        env:
        - name: GEOIPUPDATE_ACCOUNT_ID
          valueFrom:
            secretKeyRef:
              name: maxmind-credentials
              key: account-id
        - name: GEOIPUPDATE_LICENSE_KEY
          valueFrom:
            secretKeyRef:
              name: maxmind-credentials
              key: license-key
        - name: GEOIPUPDATE_EDITION_IDS
          value: "GeoLite2-ASN GeoLite2-City"
        - name: GEOIPUPDATE_FREQUENCY
          value: "24"
        volumeMounts:
        - name: geoip
          mountPath: /usr/share/GeoIP

      volumes:
      - name: config
        configMap:
          name: flow-enricher-config
      - name: geoip
        emptyDir: {}
```

### ClickHouse Cluster

#### ClickHouse Operator Installation

```yaml
# Using Altinity ClickHouse Operator
apiVersion: clickhouse.altinity.com/v1
kind: ClickHouseInstallation
metadata:
  name: helios-flows
  namespace: helios-flows
spec:
  configuration:
    clusters:
      - name: flows
        layout:
          shardsCount: 2
          replicasCount: 2
        templates:
          podTemplate: clickhouse-pod
          volumeClaimTemplate: data-volume

    settings:
      # Compression
      compression/method: lz4

      # Memory limits
      max_memory_usage: 10000000000  # 10GB
      max_bytes_before_external_group_by: 5000000000  # 5GB

      # Kafka engine settings
      kafka_max_block_size: 65536
      kafka_skip_broken_messages: 100

    users:
      grafana/password: ${CLICKHOUSE_GRAFANA_PASSWORD}
      grafana/networks/ip:
        - "10.0.0.0/8"
      grafana/profile: readonly

      admin/password: ${CLICKHOUSE_ADMIN_PASSWORD}
      admin/networks/ip:
        - "10.0.0.0/8"

    profiles:
      readonly/readonly: 1

    zookeeper:
      nodes:
        - host: zookeeper-0.zookeeper.helios-flows.svc
          port: 2181
        - host: zookeeper-1.zookeeper.helios-flows.svc
          port: 2181
        - host: zookeeper-2.zookeeper.helios-flows.svc
          port: 2181

  templates:
    podTemplates:
      - name: clickhouse-pod
        spec:
          containers:
            - name: clickhouse
              image: clickhouse/clickhouse-server:24.3
              resources:
                requests:
                  memory: 8Gi
                  cpu: 2000m
                limits:
                  memory: 16Gi
                  cpu: 8000m

    volumeClaimTemplates:
      - name: data-volume
        spec:
          accessModes:
            - ReadWriteOnce
          resources:
            requests:
              storage: 500Gi
          storageClassName: fast-ssd
```

---

## Data Model

### ClickHouse Schema

```sql
-- Database for flow data
CREATE DATABASE IF NOT EXISTS helios_flows;

-- Raw flow table (Kafka engine for ingestion)
CREATE TABLE helios_flows.flows_kafka (
    -- Timestamp
    timestamp DateTime64(3),

    -- Flow identifiers
    flow_type Enum8('netflow_v5' = 1, 'netflow_v9' = 2, 'ipfix' = 3, 'sflow' = 4),

    -- Exporter info
    exporter_ip IPv4,
    exporter_name LowCardinality(String),  -- From NetBox
    exporter_site LowCardinality(String),  -- From NetBox
    exporter_region LowCardinality(String), -- From NetBox
    exporter_role LowCardinality(String),  -- From NetBox

    -- Interface info
    in_if UInt32,
    out_if UInt32,
    in_if_name LowCardinality(String),  -- From SNMP/NetBox
    out_if_name LowCardinality(String),
    in_if_speed UInt64,
    out_if_speed UInt64,

    -- Layer 3
    src_ip IPv6,  -- IPv6 for both v4 and v6
    dst_ip IPv6,
    src_ip_v4 IPv4,  -- Alias for convenience
    dst_ip_v4 IPv4,
    ip_version UInt8,
    protocol UInt8,
    tos UInt8,
    ttl UInt8,

    -- Layer 4
    src_port UInt16,
    dst_port UInt16,
    tcp_flags UInt8,
    icmp_type UInt8,
    icmp_code UInt8,

    -- Counters
    bytes UInt64,
    packets UInt64,

    -- Sampling
    sampling_rate UInt32,

    -- Duration (for NetFlow)
    flow_start DateTime64(3),
    flow_end DateTime64(3),

    -- BGP/Routing
    src_as UInt32,
    dst_as UInt32,
    next_hop IPv4,
    src_mask UInt8,
    dst_mask UInt8,

    -- GeoIP enrichment
    src_country LowCardinality(String),
    src_city LowCardinality(String),
    dst_country LowCardinality(String),
    dst_city LowCardinality(String),

    -- ASN enrichment
    src_as_name LowCardinality(String),
    dst_as_name LowCardinality(String),

    -- VLAN
    src_vlan UInt16,
    dst_vlan UInt16,

    -- Direction (inferred or from IPFIX)
    direction Enum8('ingress' = 0, 'egress' = 1, 'unknown' = 2)
)
ENGINE = Kafka()
SETTINGS
    kafka_broker_list = 'kafka-0.kafka.helios-flows.svc:9092,kafka-1.kafka.helios-flows.svc:9092,kafka-2.kafka.helios-flows.svc:9092',
    kafka_topic_list = 'helios-flows-enriched',
    kafka_group_name = 'clickhouse-flows',
    kafka_format = 'Protobuf',
    kafka_schema = 'flow.proto:EnrichedFlow',
    kafka_num_consumers = 4,
    kafka_max_block_size = 65536;

-- Raw flow storage table (MergeTree)
CREATE TABLE helios_flows.flows_raw (
    timestamp DateTime64(3),
    flow_type Enum8('netflow_v5' = 1, 'netflow_v9' = 2, 'ipfix' = 3, 'sflow' = 4),

    exporter_ip IPv4,
    exporter_name LowCardinality(String),
    exporter_site LowCardinality(String),
    exporter_region LowCardinality(String),
    exporter_role LowCardinality(String),

    in_if UInt32,
    out_if UInt32,
    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),

    src_ip IPv6,
    dst_ip IPv6,
    ip_version UInt8,
    protocol UInt8,

    src_port UInt16,
    dst_port UInt16,
    tcp_flags UInt8,

    bytes UInt64,
    packets UInt64,
    sampling_rate UInt32,

    src_as UInt32,
    dst_as UInt32,

    src_country LowCardinality(String),
    dst_country LowCardinality(String),
    src_as_name LowCardinality(String),
    dst_as_name LowCardinality(String),

    direction Enum8('ingress' = 0, 'egress' = 1, 'unknown' = 2),

    -- Calculated fields
    bytes_adjusted UInt64 MATERIALIZED bytes * sampling_rate,
    packets_adjusted UInt64 MATERIALIZED packets * sampling_rate
)
ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/flows_raw', '{replica}')
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (exporter_site, exporter_name, timestamp, src_ip, dst_ip)
TTL timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view: Kafka to raw table
CREATE MATERIALIZED VIEW helios_flows.flows_kafka_mv TO helios_flows.flows_raw AS
SELECT * FROM helios_flows.flows_kafka;

-- 1-minute aggregate table
CREATE TABLE helios_flows.flows_1m (
    timestamp DateTime,  -- Truncated to minute

    exporter_site LowCardinality(String),
    exporter_name LowCardinality(String),

    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),

    src_ip IPv6,
    dst_ip IPv6,
    protocol UInt8,
    src_port UInt16,
    dst_port UInt16,

    src_as UInt32,
    dst_as UInt32,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),

    -- Aggregated counters
    bytes_sum AggregateFunction(sum, UInt64),
    packets_sum AggregateFunction(sum, UInt64),
    flow_count AggregateFunction(count, UInt64)
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/flows_1m', '{replica}')
PARTITION BY toYYYYMM(timestamp)
ORDER BY (exporter_site, timestamp, src_ip, dst_ip, protocol, dst_port)
TTL timestamp + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view for 1-minute aggregation
CREATE MATERIALIZED VIEW helios_flows.flows_1m_mv TO helios_flows.flows_1m AS
SELECT
    toStartOfMinute(timestamp) AS timestamp,
    exporter_site,
    exporter_name,
    in_if_name,
    out_if_name,
    src_ip,
    dst_ip,
    protocol,
    src_port,
    dst_port,
    src_as,
    dst_as,
    src_country,
    dst_country,
    sumState(bytes_adjusted) AS bytes_sum,
    sumState(packets_adjusted) AS packets_sum,
    countState() AS flow_count
FROM helios_flows.flows_raw
GROUP BY
    timestamp, exporter_site, exporter_name,
    in_if_name, out_if_name,
    src_ip, dst_ip, protocol, src_port, dst_port,
    src_as, dst_as, src_country, dst_country;

-- 1-hour aggregate table
CREATE TABLE helios_flows.flows_1h (
    timestamp DateTime,  -- Truncated to hour

    exporter_site LowCardinality(String),
    exporter_name LowCardinality(String),

    -- Aggregated to /24 networks for space savings
    src_network IPv6,  -- /24 for IPv4, /48 for IPv6
    dst_network IPv6,
    protocol UInt8,
    dst_port UInt16,

    src_as UInt32,
    dst_as UInt32,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),

    bytes_sum AggregateFunction(sum, UInt64),
    packets_sum AggregateFunction(sum, UInt64),
    flow_count AggregateFunction(count, UInt64)
)
ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/flows_1h', '{replica}')
PARTITION BY toYYYYMM(timestamp)
ORDER BY (exporter_site, timestamp, src_network, dst_network, protocol, dst_port)
TTL timestamp + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view for 1-hour aggregation (from 1m table)
CREATE MATERIALIZED VIEW helios_flows.flows_1h_mv TO helios_flows.flows_1h AS
SELECT
    toStartOfHour(timestamp) AS timestamp,
    exporter_site,
    exporter_name,
    -- Aggregate to /24 networks
    toIPv6(IPv4CIDRToRange(toIPv4(src_ip), 24).1) AS src_network,
    toIPv6(IPv4CIDRToRange(toIPv4(dst_ip), 24).1) AS dst_network,
    protocol,
    dst_port,
    src_as,
    dst_as,
    src_country,
    dst_country,
    sumMergeState(bytes_sum) AS bytes_sum,
    sumMergeState(packets_sum) AS packets_sum,
    countMergeState(flow_count) AS flow_count
FROM helios_flows.flows_1m
GROUP BY
    timestamp, exporter_site, exporter_name,
    src_network, dst_network, protocol, dst_port,
    src_as, dst_as, src_country, dst_country;
```

### Enriched Flow Protobuf Schema

```protobuf
// flow.proto
syntax = "proto3";

package helios.flows;

message EnrichedFlow {
    // Timestamp (milliseconds since epoch)
    int64 timestamp_ms = 1;

    // Flow type
    enum FlowType {
        UNKNOWN = 0;
        NETFLOW_V5 = 1;
        NETFLOW_V9 = 2;
        IPFIX = 3;
        SFLOW = 4;
    }
    FlowType flow_type = 2;

    // Exporter
    fixed32 exporter_ip = 3;  // IPv4 as uint32
    string exporter_name = 4;
    string exporter_site = 5;
    string exporter_region = 6;
    string exporter_role = 7;

    // Interfaces
    uint32 in_if = 10;
    uint32 out_if = 11;
    string in_if_name = 12;
    string out_if_name = 13;
    uint64 in_if_speed = 14;
    uint64 out_if_speed = 15;

    // Layer 3
    bytes src_ip = 20;  // 4 or 16 bytes
    bytes dst_ip = 21;
    uint32 ip_version = 22;  // 4 or 6
    uint32 protocol = 23;
    uint32 tos = 24;
    uint32 ttl = 25;

    // Layer 4
    uint32 src_port = 30;
    uint32 dst_port = 31;
    uint32 tcp_flags = 32;
    uint32 icmp_type = 33;
    uint32 icmp_code = 34;

    // Counters
    uint64 bytes = 40;
    uint64 packets = 41;
    uint32 sampling_rate = 42;

    // BGP
    uint32 src_as = 50;
    uint32 dst_as = 51;
    fixed32 next_hop = 52;

    // GeoIP
    string src_country = 60;
    string src_city = 61;
    string dst_country = 62;
    string dst_city = 63;

    // ASN names
    string src_as_name = 70;
    string dst_as_name = 71;

    // Direction
    enum Direction {
        UNKNOWN_DIR = 0;
        INGRESS = 1;
        EGRESS = 2;
    }
    Direction direction = 80;
}
```

---

## Grafana Integration

### ClickHouse Datasource

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasource-clickhouse
  namespace: helios-visualization
  labels:
    grafana_datasource: "1"
data:
  clickhouse-flows.yaml: |
    apiVersion: 1
    datasources:
      - name: ClickHouse-Flows
        type: grafana-clickhouse-datasource
        access: proxy
        url: http://helios-flows-clickhouse.helios-flows.svc:8123
        jsonData:
          defaultDatabase: helios_flows
          port: 9000
          server: helios-flows-clickhouse.helios-flows.svc
          username: grafana
          tlsSkipVerify: true
        secureJsonData:
          password: ${CLICKHOUSE_GRAFANA_PASSWORD}
        editable: false
```

### Dashboard: Top Talkers

```json
{
  "title": "Flow Analytics - Top Talkers",
  "uid": "helios-flows-top-talkers",
  "tags": ["helios", "flows", "network"],
  "templating": {
    "list": [
      {
        "name": "site",
        "type": "query",
        "datasource": "ClickHouse-Flows",
        "query": "SELECT DISTINCT exporter_site FROM helios_flows.flows_1m ORDER BY exporter_site",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "device",
        "type": "query",
        "datasource": "ClickHouse-Flows",
        "query": "SELECT DISTINCT exporter_name FROM helios_flows.flows_1m WHERE exporter_site IN ($site) ORDER BY exporter_name",
        "multi": true,
        "includeAll": true
      },
      {
        "name": "topN",
        "type": "custom",
        "query": "10,20,50,100",
        "current": {"value": "20"}
      },
      {
        "name": "interval",
        "type": "interval",
        "query": "1m,5m,15m,1h",
        "current": {"value": "5m"}
      }
    ]
  },
  "panels": [
    {
      "title": "Top Talkers by Bytes (Source)",
      "type": "bargauge",
      "gridPos": {"h": 8, "w": 12, "x": 0, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  IPv6NumToString(src_ip) AS source,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND exporter_site IN ($site)\n  AND exporter_name IN ($device)\nGROUP BY src_ip\nORDER BY bytes DESC\nLIMIT $topN",
          "format": "table"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "bytes",
          "thresholds": {
            "mode": "absolute",
            "steps": [
              {"color": "green", "value": null},
              {"color": "yellow", "value": 1000000000},
              {"color": "red", "value": 10000000000}
            ]
          }
        }
      }
    },
    {
      "title": "Top Talkers by Bytes (Destination)",
      "type": "bargauge",
      "gridPos": {"h": 8, "w": 12, "x": 12, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  IPv6NumToString(dst_ip) AS destination,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND exporter_site IN ($site)\n  AND exporter_name IN ($device)\nGROUP BY dst_ip\nORDER BY bytes DESC\nLIMIT $topN",
          "format": "table"
        }
      ]
    },
    {
      "title": "Traffic Over Time by Top Sources",
      "type": "timeseries",
      "gridPos": {"h": 8, "w": 24, "x": 0, "y": 8},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "WITH top_sources AS (\n  SELECT src_ip\n  FROM helios_flows.flows_1m\n  WHERE timestamp >= toDateTime($__from / 1000)\n    AND timestamp < toDateTime($__to / 1000)\n    AND exporter_site IN ($site)\n  GROUP BY src_ip\n  ORDER BY sumMerge(bytes_sum) DESC\n  LIMIT 10\n)\nSELECT\n  toStartOfInterval(timestamp, INTERVAL $interval) AS time,\n  IPv6NumToString(src_ip) AS source,\n  sumMerge(bytes_sum) * 8 / 60 AS bits_per_second\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND src_ip IN (SELECT src_ip FROM top_sources)\nGROUP BY time, src_ip\nORDER BY time",
          "format": "time_series"
        }
      ],
      "fieldConfig": {
        "defaults": {
          "unit": "bps"
        }
      }
    },
    {
      "title": "Top Conversations",
      "type": "table",
      "gridPos": {"h": 10, "w": 24, "x": 0, "y": 16},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  IPv6NumToString(src_ip) AS source,\n  IPv6NumToString(dst_ip) AS destination,\n  dictGet('protocols', 'name', toUInt64(protocol)) AS protocol,\n  dst_port AS port,\n  sumMerge(bytes_sum) AS bytes,\n  sumMerge(packets_sum) AS packets,\n  countMerge(flow_count) AS flows\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND exporter_site IN ($site)\nGROUP BY src_ip, dst_ip, protocol, dst_port\nORDER BY bytes DESC\nLIMIT $topN",
          "format": "table"
        }
      ],
      "transformations": [
        {
          "id": "organize",
          "options": {
            "renameByName": {
              "bytes": "Bytes",
              "packets": "Packets",
              "flows": "Flows"
            }
          }
        }
      ]
    }
  ]
}
```

### Dashboard: Traffic Matrix

```json
{
  "title": "Flow Analytics - Traffic Matrix",
  "uid": "helios-flows-matrix",
  "panels": [
    {
      "title": "Site-to-Site Traffic Matrix",
      "type": "state-timeline",
      "gridPos": {"h": 12, "w": 24, "x": 0, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  toStartOfInterval(timestamp, INTERVAL $interval) AS time,\n  concat(f1.exporter_site, ' -> ', f2.exporter_site) AS flow_path,\n  sumMerge(f1.bytes_sum) AS bytes\nFROM helios_flows.flows_1m f1\nINNER JOIN (\n  SELECT DISTINCT exporter_name, exporter_site, src_ip\n  FROM helios_flows.flows_1m\n  WHERE timestamp >= toDateTime($__from / 1000)\n) f2 ON f1.dst_ip = f2.src_ip\nWHERE f1.timestamp >= toDateTime($__from / 1000)\n  AND f1.timestamp < toDateTime($__to / 1000)\nGROUP BY time, flow_path\nORDER BY time",
          "format": "time_series"
        }
      ]
    },
    {
      "title": "AS Path Sankey",
      "type": "netsage-sankey-panel",
      "gridPos": {"h": 12, "w": 24, "x": 0, "y": 12},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  src_as_name AS source,\n  dst_as_name AS target,\n  sumMerge(bytes_sum) AS value\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND src_as > 0\n  AND dst_as > 0\nGROUP BY src_as_name, dst_as_name\nORDER BY value DESC\nLIMIT 50",
          "format": "table"
        }
      ]
    }
  ]
}
```

### Dashboard: AS Path Analysis

```json
{
  "title": "Flow Analytics - AS Paths",
  "uid": "helios-flows-as-paths",
  "panels": [
    {
      "title": "Top Source ASNs by Traffic",
      "type": "piechart",
      "gridPos": {"h": 10, "w": 12, "x": 0, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  concat(toString(src_as), ' - ', src_as_name) AS asn,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND src_as > 0\nGROUP BY src_as, src_as_name\nORDER BY bytes DESC\nLIMIT 15"
        }
      ]
    },
    {
      "title": "Top Destination ASNs by Traffic",
      "type": "piechart",
      "gridPos": {"h": 10, "w": 12, "x": 12, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  concat(toString(dst_as), ' - ', dst_as_name) AS asn,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND dst_as > 0\nGROUP BY dst_as, dst_as_name\nORDER BY bytes DESC\nLIMIT 15"
        }
      ]
    },
    {
      "title": "ASN Traffic Over Time",
      "type": "timeseries",
      "gridPos": {"h": 10, "w": 24, "x": 0, "y": 10},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "WITH top_asns AS (\n  SELECT dst_as\n  FROM helios_flows.flows_1m\n  WHERE timestamp >= toDateTime($__from / 1000)\n  GROUP BY dst_as\n  ORDER BY sumMerge(bytes_sum) DESC\n  LIMIT 10\n)\nSELECT\n  toStartOfInterval(timestamp, INTERVAL $interval) AS time,\n  concat('AS', toString(dst_as), ' - ', dst_as_name) AS asn,\n  sumMerge(bytes_sum) * 8 / 60 AS bps\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND dst_as IN (SELECT dst_as FROM top_asns)\nGROUP BY time, dst_as, dst_as_name\nORDER BY time"
        }
      ]
    }
  ]
}
```

### Dashboard: Protocol Distribution

```json
{
  "title": "Flow Analytics - Protocol Distribution",
  "uid": "helios-flows-protocols",
  "panels": [
    {
      "title": "Protocol Distribution by Bytes",
      "type": "piechart",
      "gridPos": {"h": 10, "w": 8, "x": 0, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  CASE protocol\n    WHEN 6 THEN 'TCP'\n    WHEN 17 THEN 'UDP'\n    WHEN 1 THEN 'ICMP'\n    WHEN 47 THEN 'GRE'\n    WHEN 50 THEN 'ESP'\n    ELSE concat('Proto-', toString(protocol))\n  END AS protocol_name,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\nGROUP BY protocol\nORDER BY bytes DESC\nLIMIT 10"
        }
      ]
    },
    {
      "title": "Top TCP Ports by Bytes",
      "type": "bargauge",
      "gridPos": {"h": 10, "w": 8, "x": 8, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  CASE dst_port\n    WHEN 80 THEN 'HTTP (80)'\n    WHEN 443 THEN 'HTTPS (443)'\n    WHEN 22 THEN 'SSH (22)'\n    WHEN 53 THEN 'DNS (53)'\n    WHEN 3389 THEN 'RDP (3389)'\n    ELSE concat('Port-', toString(dst_port))\n  END AS port,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND protocol = 6\nGROUP BY dst_port\nORDER BY bytes DESC\nLIMIT 15"
        }
      ]
    },
    {
      "title": "Top UDP Ports by Bytes",
      "type": "bargauge",
      "gridPos": {"h": 10, "w": 8, "x": 16, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  CASE dst_port\n    WHEN 53 THEN 'DNS (53)'\n    WHEN 123 THEN 'NTP (123)'\n    WHEN 500 THEN 'IKE (500)'\n    WHEN 4500 THEN 'IKE-NAT (4500)'\n    WHEN 514 THEN 'Syslog (514)'\n    ELSE concat('Port-', toString(dst_port))\n  END AS port,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND protocol = 17\nGROUP BY dst_port\nORDER BY bytes DESC\nLIMIT 15"
        }
      ]
    }
  ]
}
```

### Dashboard: Geographic Flows

```json
{
  "title": "Flow Analytics - Geographic",
  "uid": "helios-flows-geo",
  "panels": [
    {
      "title": "Traffic by Source Country",
      "type": "geomap",
      "gridPos": {"h": 12, "w": 12, "x": 0, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  src_country AS country,\n  sumMerge(bytes_sum) AS value\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND src_country != ''\nGROUP BY src_country\nORDER BY value DESC"
        }
      ],
      "options": {
        "view": {
          "id": "zero"
        },
        "layers": [
          {
            "type": "markers",
            "config": {
              "showLegend": true
            }
          }
        ]
      }
    },
    {
      "title": "Traffic by Destination Country",
      "type": "geomap",
      "gridPos": {"h": 12, "w": 12, "x": 12, "y": 0},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  dst_country AS country,\n  sumMerge(bytes_sum) AS value\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND dst_country != ''\nGROUP BY dst_country\nORDER BY value DESC"
        }
      ]
    },
    {
      "title": "Top Country-to-Country Flows",
      "type": "table",
      "gridPos": {"h": 10, "w": 24, "x": 0, "y": 12},
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  src_country AS source,\n  dst_country AS destination,\n  sumMerge(bytes_sum) AS bytes,\n  sumMerge(packets_sum) AS packets,\n  countMerge(flow_count) AS flows\nFROM helios_flows.flows_1m\nWHERE timestamp >= toDateTime($__from / 1000)\n  AND timestamp < toDateTime($__to / 1000)\n  AND src_country != ''\n  AND dst_country != ''\nGROUP BY src_country, dst_country\nORDER BY bytes DESC\nLIMIT 20"
        }
      ]
    }
  ]
}
```

---

## Scaling Strategy

### Scaling Tiers

| Component | Small | Medium | Large | XL |
|-----------|-------|--------|-------|-----|
| **goflow2** | 2 | 3 | 5 | 10 |
| **Kafka brokers** | 3 | 3 | 5 | 7 |
| **Kafka partitions** | 12 | 24 | 48 | 96 |
| **Flow Enricher** | 2 | 4 | 8 | 16 |
| **ClickHouse shards** | 1 | 2 | 4 | 8 |
| **ClickHouse replicas** | 2 | 2 | 2 | 3 |

### Resource Requirements by Scale

#### Small (< 50 devices, ~5K FPS)

| Component | Replicas | CPU Request | Memory Request | Storage |
|-----------|----------|-------------|----------------|---------|
| goflow2 | 2 | 500m | 512Mi | - |
| Kafka | 3 | 500m | 2Gi | 50Gi |
| Flow Enricher | 2 | 500m | 512Mi | - |
| ClickHouse | 2 | 1000m | 4Gi | 200Gi |
| ZooKeeper | 3 | 250m | 512Mi | 10Gi |
| **Total** | 12 | 5.75 cores | 16Gi | 330Gi |

#### Medium (50-500 devices, ~50K FPS)

| Component | Replicas | CPU Request | Memory Request | Storage |
|-----------|----------|-------------|----------------|---------|
| goflow2 | 3 | 1000m | 1Gi | - |
| Kafka | 3 | 1000m | 4Gi | 200Gi |
| Flow Enricher | 4 | 1000m | 1Gi | - |
| ClickHouse | 4 | 2000m | 8Gi | 500Gi |
| ZooKeeper | 3 | 500m | 1Gi | 20Gi |
| **Total** | 17 | 17.5 cores | 51Gi | 1.5TB |

#### Large (500-2000 devices, ~500K FPS)

| Component | Replicas | CPU Request | Memory Request | Storage |
|-----------|----------|-------------|----------------|---------|
| goflow2 | 5 | 2000m | 2Gi | - |
| Kafka | 5 | 2000m | 8Gi | 500Gi |
| Flow Enricher | 8 | 2000m | 2Gi | - |
| ClickHouse | 8 | 4000m | 16Gi | 1Ti |
| ZooKeeper | 3 | 1000m | 2Gi | 50Gi |
| **Total** | 29 | 61 cores | 138Gi | 7TB |

#### XL (2000-8000 devices, ~5M FPS)

| Component | Replicas | CPU Request | Memory Request | Storage |
|-----------|----------|-------------|----------------|---------|
| goflow2 | 10 | 4000m | 4Gi | - |
| Kafka | 7 | 4000m | 16Gi | 1Ti |
| Flow Enricher | 16 | 4000m | 4Gi | - |
| ClickHouse | 24 | 8000m | 32Gi | 2Ti |
| ZooKeeper | 5 | 2000m | 4Gi | 100Gi |
| **Total** | 62 | 256 cores | 524Gi | 25TB |

### Horizontal Pod Autoscaler

```yaml
# goflow2 HPA
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: goflow2-hpa
  namespace: helios-flows
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: goflow2
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Pods
    pods:
      metric:
        name: goflow2_flows_received_total
      target:
        type: AverageValue
        averageValue: "100000"  # 100k flows/minute per pod

---
# Flow Enricher HPA
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: flow-enricher-hpa
  namespace: helios-flows
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: flow-enricher
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Pods
    pods:
      metric:
        name: kafka_consumer_lag
      target:
        type: AverageValue
        averageValue: "10000"  # Scale up if lag > 10k
```

### Storage Calculations

**Daily Storage Estimate Formula**:
```
Daily Storage (GB) = (FPS * 86400 * bytes_per_flow) / (1024^3 * compression_ratio)

Where:
- FPS = average flows per second
- bytes_per_flow = ~100 bytes (compressed in ClickHouse)
- compression_ratio = ~10 for LZ4
```

**Example Calculations**:

| Scale | FPS | Raw/Day | Compressed/Day | 7-Day Raw | 30-Day 1m | 90-Day 1h |
|-------|-----|---------|----------------|-----------|-----------|-----------|
| Small | 5K | 43 GB | 4.3 GB | 30 GB | 50 GB | 40 GB |
| Medium | 50K | 430 GB | 43 GB | 300 GB | 500 GB | 400 GB |
| Large | 500K | 4.3 TB | 430 GB | 3 TB | 5 TB | 4 TB |
| XL | 5M | 43 TB | 4.3 TB | 30 TB | 50 TB | 40 TB |

---

## Helios Integration

### Correlation with Metrics (gnmic)

Flow data and metrics serve different purposes but can be correlated:

```
+-------------------+     +-------------------+
|   Flow Data       |     |   Metrics Data    |
|   (ClickHouse)    |     |   (Thanos)        |
+--------+----------+     +--------+----------+
         |                         |
         v                         v
+-------------------------------------------+
|              Grafana Dashboard            |
|                                           |
|  +---------------+    +---------------+   |
|  | Interface     |    | Interface     |   |
|  | Traffic (Flow)|    | Utilization   |   |
|  | - bytes/pkts  |    | (gnmic)       |   |
|  | - top talkers |    | - bps in/out  |   |
|  | - protocols   |    | - errors      |   |
|  +---------------+    +---------------+   |
|           \              /                |
|            \            /                 |
|             v          v                  |
|        +------------------+               |
|        | Interface Panel  |               |
|        | (Combined View)  |               |
|        +------------------+               |
+-------------------------------------------+
```

#### Cross-Datasource Dashboard

```json
{
  "title": "Interface Analysis - Combined",
  "uid": "helios-interface-combined",
  "panels": [
    {
      "title": "Interface Utilization (from gNMI)",
      "type": "timeseries",
      "datasource": "Thanos",
      "targets": [
        {
          "expr": "rate(gnmic_interfaces_interface_state_counters_in_octets{device=\"$device\", interface=\"$interface\"}[5m]) * 8",
          "legendFormat": "In bps"
        },
        {
          "expr": "rate(gnmic_interfaces_interface_state_counters_out_octets{device=\"$device\", interface=\"$interface\"}[5m]) * 8",
          "legendFormat": "Out bps"
        }
      ]
    },
    {
      "title": "Top Talkers on Interface (from Flows)",
      "type": "table",
      "datasource": "ClickHouse-Flows",
      "targets": [
        {
          "rawSql": "SELECT\n  IPv6NumToString(src_ip) AS source,\n  IPv6NumToString(dst_ip) AS destination,\n  sumMerge(bytes_sum) AS bytes\nFROM helios_flows.flows_1m\nWHERE exporter_name = '$device'\n  AND in_if_name = '$interface'\n  AND timestamp >= toDateTime($__from / 1000)\nGROUP BY src_ip, dst_ip\nORDER BY bytes DESC\nLIMIT 10"
        }
      ]
    },
    {
      "title": "Interface Errors (from gNMI)",
      "type": "stat",
      "datasource": "Thanos",
      "targets": [
        {
          "expr": "increase(gnmic_interfaces_interface_state_counters_in_errors{device=\"$device\", interface=\"$interface\"}[1h])"
        }
      ]
    }
  ]
}
```

### NetBox Integration

The Flow Enricher service integrates with NetBox similarly to the gnmic Target Generator:

```python
# Enricher NetBox sync pseudocode
class NetBoxSync:
    def __init__(self):
        self.nb = pynetbox.api(url=NETBOX_URL, token=NETBOX_TOKEN)
        self.device_cache = {}  # IP -> device info
        self.interface_cache = {}  # (device, if_index) -> interface info

    def sync(self):
        """Sync device and interface data from NetBox."""
        for device in self.nb.dcim.devices.filter(status='active'):
            if device.primary_ip:
                ip = device.primary_ip.address.split('/')[0]
                self.device_cache[ip] = {
                    'name': device.name,
                    'site': device.site.slug,
                    'region': device.site.region.slug if device.site.region else '',
                    'role': device.device_role.slug,
                }

            # Sync interfaces with SNMP index
            for iface in device.interfaces.all():
                if iface.custom_fields.get('snmp_index'):
                    key = (device.name, iface.custom_fields['snmp_index'])
                    self.interface_cache[key] = {
                        'name': iface.name,
                        'description': iface.description,
                        'speed': iface.speed or 0,
                    }

    def lookup_device(self, exporter_ip: str) -> dict:
        return self.device_cache.get(exporter_ip, {})

    def lookup_interface(self, device_name: str, if_index: int) -> dict:
        return self.interface_cache.get((device_name, if_index), {})
```

### Alert Integration

Flow anomalies can trigger Prometheus alerts via a bridge:

```yaml
# ClickHouse -> Prometheus bridge for flow alerts
apiVersion: v1
kind: ConfigMap
metadata:
  name: flow-alerting-config
  namespace: helios-flows
data:
  alerts.yaml: |
    rules:
      - name: high_traffic_source
        query: |
          SELECT
            exporter_site,
            IPv6NumToString(src_ip) AS src,
            sumMerge(bytes_sum) AS bytes
          FROM helios_flows.flows_1m
          WHERE timestamp >= now() - INTERVAL 5 MINUTE
          GROUP BY exporter_site, src_ip
          HAVING bytes > 10737418240  -- 10GB in 5 min
        labels:
          severity: warning
        annotations:
          summary: "High traffic from {{ $labels.src }} at {{ $labels.exporter_site }}"

      - name: new_external_asn
        query: |
          SELECT DISTINCT dst_as, dst_as_name
          FROM helios_flows.flows_1m
          WHERE timestamp >= now() - INTERVAL 1 HOUR
            AND dst_as > 0
          EXCEPT
          SELECT DISTINCT dst_as, dst_as_name
          FROM helios_flows.flows_1m
          WHERE timestamp BETWEEN now() - INTERVAL 1 DAY AND now() - INTERVAL 1 HOUR
        labels:
          severity: info
        annotations:
          summary: "New external ASN detected: AS{{ $labels.dst_as }} ({{ $labels.dst_as_name }})"
```

---

## Kubernetes Manifests

### Complete Namespace and RBAC

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: helios-flows
  labels:
    app.kubernetes.io/part-of: helios
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: goflow2
  namespace: helios-flows
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: flow-enricher
  namespace: helios-flows
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: flow-enricher
  namespace: helios-flows
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: flow-enricher
  namespace: helios-flows
subjects:
  - kind: ServiceAccount
    name: flow-enricher
roleRef:
  kind: Role
  name: flow-enricher
  apiGroup: rbac.authorization.k8s.io
```

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: goflow2-ingress
  namespace: helios-flows
spec:
  podSelector:
    matchLabels:
      app: goflow2
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow UDP from network devices (management network)
    - from:
        - ipBlock:
            cidr: 10.0.0.0/8
      ports:
        - protocol: UDP
          port: 2055
        - protocol: UDP
          port: 6343
    # Allow Prometheus scrape
    - from:
        - namespaceSelector:
            matchLabels:
              name: helios-storage
      ports:
        - protocol: TCP
          port: 8080
  egress:
    # Allow Kafka
    - to:
        - podSelector:
            matchLabels:
              strimzi.io/cluster: helios-flows
      ports:
        - protocol: TCP
          port: 9092
    # Allow DNS
    - to:
        - namespaceSelector: {}
          podSelector:
            matchLabels:
              k8s-app: kube-dns
      ports:
        - protocol: UDP
          port: 53

---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: clickhouse-ingress
  namespace: helios-flows
spec:
  podSelector:
    matchLabels:
      clickhouse.altinity.com/chi: helios-flows
  policyTypes:
    - Ingress
  ingress:
    # Allow from Grafana
    - from:
        - namespaceSelector:
            matchLabels:
              name: helios-visualization
      ports:
        - protocol: TCP
          port: 8123
        - protocol: TCP
          port: 9000
    # Allow from Kafka (ingest)
    - from:
        - podSelector:
            matchLabels:
              strimzi.io/cluster: helios-flows
      ports:
        - protocol: TCP
          port: 9000
    # Allow internal replication
    - from:
        - podSelector:
            matchLabels:
              clickhouse.altinity.com/chi: helios-flows
      ports:
        - protocol: TCP
          port: 9009
```

### Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: netbox-credentials
  namespace: helios-flows
type: Opaque
stringData:
  url: "http://netbox.helios-integration.svc:8080"
  token: "${NETBOX_TOKEN}"

---
apiVersion: v1
kind: Secret
metadata:
  name: maxmind-credentials
  namespace: helios-flows
type: Opaque
stringData:
  account-id: "${MAXMIND_ACCOUNT_ID}"
  license-key: "${MAXMIND_LICENSE_KEY}"

---
apiVersion: v1
kind: Secret
metadata:
  name: clickhouse-credentials
  namespace: helios-flows
type: Opaque
stringData:
  admin-password: "${CLICKHOUSE_ADMIN_PASSWORD}"
  grafana-password: "${CLICKHOUSE_GRAFANA_PASSWORD}"
```

### ServiceMonitor for Prometheus

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: goflow2
  namespace: helios-flows
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      app: goflow2
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics

---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: flow-enricher
  namespace: helios-flows
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      app: flow-enricher
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics

---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: clickhouse
  namespace: helios-flows
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      clickhouse.altinity.com/chi: helios-flows
  endpoints:
    - port: prometheus
      interval: 30s
```

---

## ADR Recommendations

### ADR-008: Flow Collector Selection

**Recommendation**: goflow2

| Option | Pros | Cons | Verdict |
|--------|------|------|---------|
| **goflow2** | High performance, Cloudflare-proven, K8s-native | No built-in enrichment | **Recommended** |
| Akvorado | Complete solution, NetBox-like | More opinionated, harder to customize | Alternative |
| pmacct | Mature, feature-rich, BGP correlation | C-based, complex config | Legacy alternative |
| nfdump | Simple, well-known | File-based, not K8s-native | Not recommended |

**Key Decision Points**:
- goflow2 handles high FPS with low resource usage
- Native Kafka output with Protobuf efficiency
- Separate enrichment service provides flexibility
- Active community (netsampler fork)

---

### ADR-009: Flow Storage Backend

**Recommendation**: ClickHouse

| Option | Flow Query Speed | Compression | Cardinality | Verdict |
|--------|------------------|-------------|-------------|---------|
| **ClickHouse** | Excellent | 10-20x | Unlimited | **Recommended** |
| TimescaleDB | Good | 5-10x | Good | Alternative |
| Elasticsearch | Good | 2-5x | Good | High cost |
| Prometheus | Poor | Good | Limited | Not suitable |

**Key Decision Points**:
- Flow data is high-cardinality (IP pairs) - Prometheus unsuitable
- ClickHouse 10-20x compression reduces storage costs
- SQL queries for Top-N, aggregations perform excellently
- MergeTree + TTL provides automatic tiered retention

---

### ADR-010: Flow Message Queue

**Recommendation**: Kafka (Strimzi)

| Option | Throughput | Durability | K8s Native | Verdict |
|--------|------------|------------|------------|---------|
| **Kafka** | 2M+ msg/s | Excellent | Strimzi | **Recommended** |
| NATS | 10M+ msg/s | Good | Excellent | Alternative |
| Direct | N/A | N/A | N/A | Not recommended |

**Key Decision Points**:
- Kafka provides replay capability for reprocessing
- Strimzi operator is production-grade
- Partitioning enables parallel consumption
- Industry standard for flow pipelines

---

### ADR-011: Flow Data Retention Strategy

**Recommendation**: Tiered retention with aggregation

| Tier | Resolution | Retention | Purpose |
|------|------------|-----------|---------|
| Raw | Full | 7 days | Forensics, debugging |
| 1-minute | Aggregated | 30 days | Operational analysis |
| 1-hour | Aggregated | 90 days | Trending, capacity |

**Key Decision Points**:
- Raw flows expensive but needed for forensics
- Aggregation reduces storage 50-100x
- ClickHouse materialized views automate aggregation
- TTL automatically purges old data

---

## Implementation Phases

### Phase 1: Foundation (Week 1-2)

1. Deploy Strimzi Kafka operator
2. Create Kafka cluster and topics
3. Deploy ClickHouse operator
4. Create ClickHouse cluster and schema
5. Configure Grafana ClickHouse datasource

### Phase 2: Collection (Week 3-4)

1. Deploy goflow2 StatefulSet
2. Configure UDP LoadBalancer service
3. Point test devices to collector
4. Validate flow ingestion to Kafka
5. Create basic monitoring for collectors

### Phase 3: Enrichment (Week 5-6)

1. Deploy Flow Enricher service
2. Integrate with NetBox
3. Configure GeoIP databases
4. Test enrichment pipeline
5. Validate data in ClickHouse

### Phase 4: Visualization (Week 7-8)

1. Create Top Talkers dashboard
2. Create Traffic Matrix dashboard
3. Create Protocol Distribution dashboard
4. Create Geographic Flows dashboard
5. Create AS Path Analysis dashboard

### Phase 5: Integration (Week 9-10)

1. Create combined interface dashboards (flows + metrics)
2. Implement flow-based alerts
3. Document operational procedures
4. Performance tuning
5. Capacity planning validation

---

## Related Documents

- [[architecture]] - Main Helios architecture
- [[../decisions/adr-001-storage-backend]] - Metrics storage (Thanos)
- [[../decisions/adr-002-gnmic-deployment]] - gNMI collector
- [[../overview]] - Project overview

---

## References

- [goflow2 GitHub](https://github.com/netsampler/goflow2) - Primary collector
- [Akvorado](https://github.com/akvorado/akvorado) - Alternative complete solution
- [ClickHouse Documentation](https://clickhouse.com/docs) - Storage backend
- [Strimzi Documentation](https://strimzi.io/documentation/) - Kafka on K8s
- [ClickHouse Time Series Performance](https://altinity.com/blog/clickhouse-for-time-series) - Benchmarks
- [Grafana ClickHouse Plugin](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/) - Visualization
- [Akvorado Grafana Plugin](https://grafana.com/grafana/plugins/ovhcloud-akvorado-datasource/) - Alternative visualization

---

**Document History**:

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-09 | Architecture Review | Initial flow collection design |
