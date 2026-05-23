#adr #decision #multi-dc #federation

# ADR-005: Multi-Datacenter Architecture

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #multi-datacenter #thanos #federation

## Context

Helios must support network monitoring across multiple datacenters connected via Cilium overlay networking. The system must:

1. **Fault Isolation**: Each datacenter should continue operating if connectivity to other DCs is lost
2. **Global View**: Operators need a unified view across all datacenters when connected
3. **Local Performance**: Queries for local data should not depend on remote DC connectivity
4. **Scalability**: Support independent scaling per datacenter based on local device count
5. **Data Durability**: Metrics must survive DC failures with appropriate redundancy
6. **Cost Efficiency**: Minimize cross-DC bandwidth for normal operations

Network topology:
- 2-5 datacenters initially, expandable to more
- Cilium overlay networking provides L3 connectivity between K8s clusters
- Variable latency between DCs (1-50ms)
- Bandwidth between DCs is limited and costly
- Each DC has its own network devices to monitor (50-2000 per DC)

## Decision

We will implement **Prometheus per-DC with Thanos global query federation**.

### Architecture

```
                              GLOBAL LAYER (Any DC or Central)
    +------------------------------------------------------------------------+
    |                                                                        |
    |  +--------------------+     +--------------------+     +-------------+ |
    |  |   Thanos Query     |     |   Thanos Query     |     |   Grafana   | |
    |  |   (Global View)    |     |   Frontend         |     |   (HA)      | |
    |  +----------+---------+     +----------+---------+     +------+------+ |
    |             |                          |                      |        |
    +-------------|--------------------------|----------------------|--------+
                  |                          |                      |
                  +------------+-------------+----------------------+
                               |
         +---------------------+---------------------+---------------------+
         |                     |                     |                     |
         v                     v                     v                     v
+------------------+  +------------------+  +------------------+  +------------------+
|    DC1 Layer     |  |    DC2 Layer     |  |    DC3 Layer     |  |   Cloud Layer    |
|                  |  |                  |  |                  |  |                  |
| +-------------+  |  | +-------------+  |  | +-------------+  |  | +-------------+  |
| |Thanos Store |  |  | |Thanos Store |  |  | |Thanos Store |  |  | |Thanos Store |  |
| | Gateway     |  |  | | Gateway     |  |  | | Gateway     |  |  | | Gateway     |  |
| +------+------+  |  | +------+------+  |  | +------+------+  |  | +------+------+  |
|        |         |  |        |         |  |        |         |  |        |         |
| +------v------+  |  | +------v------+  |  | +------v------+  |  | +------v------+  |
| |   MinIO     |  |  | |   MinIO     |  |  | |   MinIO     |  |  | |     S3      |  |
| | (Local)     |  |  | | (Local)     |  |  | | (Local)     |  |  | |   (AWS)     |  |
| +-------------+  |  | +-------------+  |  | +-------------+  |  | +-------------+  |
|        ^         |  |        ^         |  |        ^         |  |        ^         |
| +------+------+  |  | +------+------+  |  | +------+------+  |  | +------+------+  |
| |Thanos       |  |  | |Thanos       |  |  | |Thanos       |  |  | |Thanos       |  |
| |Sidecar      |  |  | |Sidecar      |  |  | |Sidecar      |  |  | |Sidecar      |  |
| +------+------+  |  | +------+------+  |  | +------+------+  |  | +------+------+  |
|        |         |  |        |         |  |        |         |  |        |         |
| +------+------+  |  | +------+------+  |  | +------+------+  |  | +------+------+  |
| | Prometheus  |  |  | | Prometheus  |  |  | | Prometheus  |  |  | | Prometheus  |  |
| | (Sharded)   |  |  | | (Sharded)   |  |  | | (Sharded)   |  |  | | (Single)    |  |
| +-------------+  |  | +-------------+  |  | +-------------+  |  | +-------------+  |
|        ^         |  |        ^         |  |        ^         |  |        ^         |
| +------+------+  |  | +------+------+  |  | +------+------+  |  | +------+------+  |
| |   gnmic     |  |  | |   gnmic     |  |  | |   gnmic     |  |  | |   gnmic     |  |
| | (Clustered) |  |  | | (Clustered) |  |  | | (Clustered) |  |  | | (Minimal)   |  |
| +-------------+  |  | +-------------+  |  | +-------------+  |  | +-------------+  |
|                  |  |                  |  |                  |  |                  |
|  Network Devices |  |  Network Devices |  |  Network Devices |  |  Cloud Resources |
+------------------+  +------------------+  +------------------+  +------------------+
```

### Per-DC Deployment

Each datacenter runs a complete, independent monitoring stack:

```yaml
# Per-DC Helm values example (dc1-values.yaml)
global:
  datacenter: dc1
  region: us-east

prometheus:
  replicas: 2
  shards: 2
  retention: 2h
  externalLabels:
    dc: dc1
    region: us-east
    cluster: production

thanos:
  sidecar:
    enabled: true

  objectStorage:
    type: s3
    bucket: helios-metrics-dc1
    endpoint: minio.dc1.local:9000

gnmic:
  replicas: 4

minio:
  replicas: 4
  mode: distributed
```

### Global Query Layer

The global Thanos Query connects to all per-DC stores:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thanos-query-global
  namespace: helios-storage
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: thanos-query
        image: quay.io/thanos/thanos:v0.34.0
        args:
          - query
          - --http-address=0.0.0.0:9090
          - --grpc-address=0.0.0.0:10901
          # DC1 Store Gateway
          - --store=thanos-store.dc1.helios.svc.cluster.local:10901
          # DC2 Store Gateway
          - --store=thanos-store.dc2.helios.svc.cluster.local:10901
          # DC3 Store Gateway
          - --store=thanos-store.dc3.helios.svc.cluster.local:10901
          # Cloud Store Gateway
          - --store=thanos-store.cloud.helios.svc.cluster.local:10901
          # Also query live Prometheus for recent data
          - --store=dnssrv+_grpc._tcp.prometheus-headless.dc1.helios.svc.cluster.local
          - --store=dnssrv+_grpc._tcp.prometheus-headless.dc2.helios.svc.cluster.local
          - --store=dnssrv+_grpc._tcp.prometheus-headless.dc3.helios.svc.cluster.local
          # Deduplication
          - --query.replica-label=replica
          - --query.replica-label=prometheus_replica
          # Auto-downsampling for historical queries
          - --query.auto-downsampling
          # Partial response for fault tolerance
          - --query.partial-response
        ports:
        - name: http
          containerPort: 9090
        - name: grpc
          containerPort: 10901
```

### Cilium Network Connectivity

Cross-DC connectivity via Cilium ClusterMesh:

```yaml
# CiliumClusterMeshConfig
apiVersion: cilium.io/v2alpha1
kind: CiliumClusterMeshConfig
metadata:
  name: helios-mesh
spec:
  clusters:
    - name: dc1
      address: dc1-mesh.example.com:2379
    - name: dc2
      address: dc2-mesh.example.com:2379
    - name: dc3
      address: dc3-mesh.example.com:2379
---
# Global Service for Thanos Query
apiVersion: cilium.io/v2
kind: CiliumGlobalService
metadata:
  name: thanos-query-global
  namespace: helios-storage
  annotations:
    io.cilium/global-service: "true"
spec:
  selector:
    matchLabels:
      app: thanos-query
      tier: global
```

### Object Storage Topology

Two supported topologies based on requirements:

#### Option A: Per-DC Object Storage (Recommended)

```
DC1: MinIO --> helios-dc1-bucket (local)
DC2: MinIO --> helios-dc2-bucket (local)
DC3: MinIO --> helios-dc3-bucket (local)

Thanos Query reads from all stores via gRPC
```

**Pros**:
- No cross-DC traffic for writes
- Survives DC isolation
- Lower latency for local queries

**Cons**:
- No automatic cross-DC replication
- More storage infrastructure

#### Option B: Centralized Object Storage

```
All DCs --> Central S3/GCS (via WAN)

Thanos Sidecars write to central bucket
Thanos Store Gateways can be anywhere
```

**Pros**:
- Single storage location
- Simpler backup strategy
- Built-in geo-redundancy (cloud providers)

**Cons**:
- Cross-DC bandwidth for all writes
- Central storage is SPOF for writes
- Higher write latency

### Configuration for Fault Tolerance

```yaml
# Thanos Query with partial response enabled
thanos:
  query:
    args:
      # Return partial results if some stores are unavailable
      - --query.partial-response
      # Set timeout for store responses
      - --store.response-timeout=30s
      # Replica labels for deduplication
      - --query.replica-label=replica
      - --query.replica-label=dc
```

### Alerting in Multi-DC

```yaml
# Prometheus alerting config (per-DC)
alerting:
  alertmanagers:
    # Local Alertmanager cluster
    - static_configs:
        - targets:
            - alertmanager-0.dc1.helios.svc:9093
            - alertmanager-1.dc1.helios.svc:9093
    # Cross-DC Alertmanager for HA
    - static_configs:
        - targets:
            - alertmanager.dc2.helios.svc:9093
      timeout: 10s
```

## Consequences

### Positive

1. **Fault Isolation**: Each DC operates independently during network partitions
2. **Local Performance**: Queries stay local when data is local
3. **Horizontal Scaling**: Each DC scales independently based on device count
4. **Global View**: Operators get unified dashboards when DCs are connected
5. **Flexible Topology**: Supports various storage configurations per DC
6. **Graceful Degradation**: Partial response allows queries when some DCs are down
7. **Bandwidth Efficiency**: Only query traffic crosses DC boundaries
8. **Future-Proof**: Easy to add new DCs without redesign

### Negative

1. **Operational Complexity**: Multiple Prometheus/Thanos stacks to manage
2. **Cross-DC Latency**: Global queries slower than local queries
3. **Infrastructure Cost**: Each DC needs storage and compute
4. **Consistency Delay**: Brief inconsistency during uploads to object storage
5. **Monitoring Overhead**: Need to monitor the monitoring system per DC

### Neutral

1. **Helm Complexity**: Separate values files per DC
2. **RBAC Scope**: Decisions on global vs per-DC permissions
3. **Dashboard Design**: Need DC-aware and global dashboards

## Alternatives Considered

### Option 1: Single Global Prometheus

**Description**: Deploy one large Prometheus instance that scrapes all DCs via WAN

**Pros**:
- Simplest architecture
- Single source of truth
- No federation complexity

**Cons**:
- Single point of failure
- All collection traffic crosses WAN
- High scrape latency for remote DCs
- Scaling limited to single instance

**Why Not Selected**: Does not meet fault isolation requirement. WAN bandwidth would be excessive.

### Option 2: Remote-Write Only (No Thanos)

**Description**: Per-DC Prometheus remote-writes to central TSDB (VictoriaMetrics cluster)

**Pros**:
- Simpler query layer
- VictoriaMetrics efficient for remote-write
- No Thanos complexity

**Cons**:
- All data crosses WAN
- Central TSDB is write SPOF
- Higher bandwidth requirements
- No local query capability during partition

**Why Not Selected**: Still requires WAN for all writes. No local query capability during isolation.

### Option 3: Grafana Mimir Multi-Tenant

**Description**: Deploy Mimir with per-DC tenants

**Pros**:
- Built for multi-tenant scale
- Native Grafana integration
- Single query endpoint

**Cons**:
- More complex than Thanos
- Requires central infrastructure
- Less proven in network monitoring
- Tighter Grafana coupling

**Why Not Selected**: Mimir is excellent but adds complexity. Thanos is simpler for our use case. Consider Mimir for v2 if multi-tenancy becomes important.

### Option 4: Per-DC VictoriaMetrics Cluster

**Description**: Deploy VictoriaMetrics cluster per DC with vmselect federation

**Pros**:
- Better resource efficiency than Prometheus
- Built-in clustering
- Compatible with PromQL

**Cons**:
- Different operational model from Prometheus
- Smaller community
- Less integration documentation
- Cross-DC federation less mature than Thanos

**Why Not Selected**: VictoriaMetrics is excellent but Thanos has more production deployments in similar architectures. Could be reconsidered for specific DCs with resource constraints.

## Implementation Notes

### Prometheus External Labels

Critical for proper federation and deduplication:

```yaml
# Per-DC Prometheus config
global:
  external_labels:
    dc: dc1
    region: us-east
    cluster: production
    replica: $(POD_NAME)
```

### Thanos Compactor Per-DC

Each DC should run its own compactor against its bucket:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-compactor
  namespace: helios-storage
spec:
  replicas: 1  # Must be singleton per bucket
  template:
    spec:
      containers:
      - name: compactor
        image: quay.io/thanos/thanos:v0.34.0
        args:
          - compact
          - --data-dir=/var/thanos/compact
          - --objstore.config-file=/etc/thanos/objstore.yaml
          # Retention policies
          - --retention.resolution-raw=14d
          - --retention.resolution-5m=30d
          - --retention.resolution-1h=90d
          # Wait for more data before compacting
          - --wait
          - --wait-interval=5m
          # Downsampling
          - --downsampling.disable=false
```

### Cross-DC Service Discovery

Using Cilium ClusterMesh for service discovery:

```yaml
# Headless service for Thanos Store Gateway
apiVersion: v1
kind: Service
metadata:
  name: thanos-store-headless
  namespace: helios-storage
  annotations:
    io.cilium/global-service: "true"
spec:
  clusterIP: None
  selector:
    app: thanos-store
  ports:
    - name: grpc
      port: 10901
      targetPort: 10901
```

### Grafana Multi-DC Datasource

```yaml
apiVersion: 1
datasources:
  # Global view (default)
  - name: Thanos-Global
    type: prometheus
    url: http://thanos-query-global.helios.svc:9090
    isDefault: true
    jsonData:
      timeInterval: "15s"

  # Per-DC datasources for local queries
  - name: Thanos-DC1
    type: prometheus
    url: http://thanos-query.dc1.helios.svc:9090
    jsonData:
      timeInterval: "15s"

  - name: Thanos-DC2
    type: prometheus
    url: http://thanos-query.dc2.helios.svc:9090
    jsonData:
      timeInterval: "15s"
```

### Failover Behavior

```
Normal Operation:
  Dashboard -> Thanos Global -> All Stores -> Unified Results

DC2 Network Partition:
  Dashboard -> Thanos Global -> DC1 + DC3 Stores -> Partial Results
  (DC2 continues operating locally, data syncs when reconnected)

Central Query Failure:
  Dashboard -> Local Thanos Query (per DC) -> Local Metrics Only
```

## References

- [Thanos Multi-Cluster Guide](https://thanos.io/tip/operating/cross-cluster-tls-communication.md/)
- [Cilium ClusterMesh](https://docs.cilium.io/en/stable/network/clustermesh/)
- [Prometheus Federation](https://prometheus.io/docs/prometheus/latest/federation/)
- [[adr-001-storage-backend]] - Storage backend decision
- [[adr-006-alerting-strategy]] - Alerting strategy
- [[../architecture/architecture]] - Full architecture document
