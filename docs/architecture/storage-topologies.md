#architecture #helios #storage #minio #thanos #multi-dc

# Helios Object Storage Topologies

**Version**: 1.0
**Date**: 2026-01-09
**Author**: Architecture Review
**Status**: Draft
**Related**: [[architecture]], [[../decisions/adr-001-storage-backend]]

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Topology Overview](#topology-overview)
3. [Topology A: Single-DC MinIO](#topology-a-single-dc-minio)
4. [Topology B: Multi-DC MinIO with Replication](#topology-b-multi-dc-minio-with-replication)
5. [Topology C: Cloud Object Storage](#topology-c-cloud-object-storage)
6. [Topology D: Hybrid MinIO + Cloud Tiering](#topology-d-hybrid-minio--cloud-tiering)
7. [MinIO Architecture Deep Dive](#minio-architecture-deep-dive)
8. [Site Replication Design](#site-replication-design)
9. [Thanos Integration Patterns](#thanos-integration-patterns)
10. [Network Considerations](#network-considerations)
11. [Backup and Disaster Recovery](#backup-and-disaster-recovery)
12. [Cost Analysis](#cost-analysis)
13. [Configuration Templates](#configuration-templates)
14. [Migration Paths](#migration-paths)
15. [Decision Matrix](#decision-matrix)

---

## Executive Summary

This document defines four object storage deployment topologies for Helios, supporting scales from 50 to 8,000+ devices across single or multiple datacenters. Each topology is designed for Thanos integration with specific trade-offs for cost, complexity, and resilience.

### Topology Selection Guide

| Topology | Best For | Complexity | Cost | DR Capability |
|----------|----------|------------|------|---------------|
| A: Single-DC MinIO | Dev/staging, single-site production | Low | Low | Limited |
| B: Multi-DC MinIO | Multi-site enterprise, strict data locality | High | Medium | Excellent |
| C: Cloud Object Storage | Cloud-native, hybrid cloud | Low | Variable | Good |
| D: Hybrid MinIO + Cloud | Cost optimization, tiered retention | Medium | Optimized | Good |

---

## Topology Overview

### Architecture Comparison

```
TOPOLOGY A: Single-DC MinIO
+------------------+
|    Datacenter    |
|  +------------+  |
|  |   MinIO    |  |
|  |  Cluster   |  |
|  +-----+------+  |
|        |         |
|  +-----v------+  |
|  |   Thanos   |  |
|  | Components |  |
|  +------------+  |
+------------------+

TOPOLOGY B: Multi-DC MinIO
+------------------+     +------------------+
|   Datacenter 1   |     |   Datacenter 2   |
|  +------------+  |     |  +------------+  |
|  |   MinIO    |<-------->|   MinIO    |  |
|  |  Cluster   |  |     |  |  Cluster   |  |
|  +-----+------+  |     |  +-----+------+  |
|        |         |     |        |         |
|  +-----v------+  |     |  +-----v------+  |
|  |   Thanos   |  |     |  |   Thanos   |  |
|  | Store GW   |  |     |  | Store GW   |  |
|  +------------+  |     |  +------------+  |
+--------+---------+     +---------+--------+
         |                         |
         +------------+------------+
                      |
              +-------v-------+
              | Thanos Query  |
              |   (Global)    |
              +---------------+

TOPOLOGY C: Cloud Object Storage
+------------------+     +------------------+
|   Datacenter 1   |     |   Datacenter 2   |
|  +------------+  |     |  +------------+  |
|  | Prometheus |  |     |  | Prometheus |  |
|  | + Sidecar  |  |     |  | + Sidecar  |  |
|  +-----+------+  |     |  +-----+------+  |
+--------|----------     +--------|----------
         |                        |
         +------------+-----------+
                      |
              +-------v-------+
              |   AWS S3 /    |
              |   GCS / Azure |
              +-------+-------+
                      |
              +-------v-------+
              | Thanos Store  |
              |   Gateway     |
              +---------------+

TOPOLOGY D: Hybrid (MinIO + Cloud Tiering)
+------------------+
|    Datacenter    |
|  +------------+  |
|  |   MinIO    |--+---> (Lifecycle Policy)
|  |  (Hot)     |  |            |
|  | 0-14 days  |  |     +------v------+
|  +-----+------+  |     |   Cloud S3  |
|        |         |     |   (Warm)    |
|  +-----v------+  |     | 14-90 days  |
|  |   Thanos   |  |     +-------------+
|  | Components |  |
|  +------------+  |
+------------------+
```

---

## Topology A: Single-DC MinIO

### Overview

A single MinIO cluster deployed in one datacenter, with all Thanos components co-located. This is the simplest deployment suitable for development, staging, or single-site production environments.

### Architecture Diagram

```
                    +-------------------------------------------+
                    |            Datacenter / K8s Cluster       |
                    |                                           |
                    |  +-------------------------------------+  |
                    |  |         MinIO Distributed Mode       |  |
                    |  |                                      |  |
                    |  |  +-------+  +-------+  +-------+     |  |
                    |  |  |minio-0|  |minio-1|  |minio-2| ... |  |
                    |  |  +---+---+  +---+---+  +---+---+     |  |
                    |  |      |          |          |         |  |
                    |  |  +---v----------v----------v---+     |  |
                    |  |  |      Erasure Coding         |     |  |
                    |  |  |    (EC:4 or EC:8 parity)    |     |  |
                    |  |  +-----------------------------+     |  |
                    |  +-------------------------------------+  |
                    |               |                           |
                    |  +------------v-----------+               |
                    |  |    helios-metrics      |               |
                    |  |       (Bucket)         |               |
                    |  +------------+-----------+               |
                    |               |                           |
                    |  +------------v--------------------------+|
                    |  |         Thanos Components             ||
                    |  |                                       ||
                    |  | +----------+  +----------+  +-------+ ||
                    |  | |  Store   |  |  Query   |  |Compact| ||
                    |  | | Gateway  |  |          |  |  or   | ||
                    |  | +----------+  +----------+  +-------+ ||
                    |  |                                       ||
                    |  | +----------+  +----------+            ||
                    |  | |Prometheus|  |Prometheus|  (shards)  ||
                    |  | |+ Sidecar |  |+ Sidecar |            ||
                    |  | +----------+  +----------+            ||
                    |  +---------------------------------------+|
                    +-------------------------------------------+
```

### Component Specifications

| Component | Replicas | Purpose |
|-----------|----------|---------|
| MinIO StatefulSet | 4-16 nodes | Distributed object storage |
| Thanos Store Gateway | 2 | Query historical data from MinIO |
| Thanos Query | 2-3 | Federate queries |
| Thanos Query Frontend | 2 | Query caching and splitting |
| Thanos Compactor | 1 | Downsampling and compaction |
| Prometheus + Sidecar | 2 per shard | Metrics collection and upload |

### Pros

1. **Simplicity**: Single cluster to manage
2. **Low Latency**: All components in same network
3. **Cost Effective**: No cross-DC bandwidth costs
4. **Easy Operations**: Standard K8s tooling applies
5. **Fast Recovery**: No cross-DC coordination needed

### Cons

1. **Single Point of Failure**: DC failure loses all data
2. **Limited DR**: Requires backup to external location
3. **Capacity Ceiling**: Limited by single DC resources
4. **No Geographic Redundancy**: All data in one location

### When to Use

- Development and staging environments
- Single-site production with acceptable RPO/RTO
- Small to medium scale (< 500 devices)
- Budget-constrained deployments
- Proof of concept / initial deployment

### Sizing Recommendations

| Scale | MinIO Nodes | Drives/Node | Drive Size | Total Capacity | Usable (EC:4) |
|-------|-------------|-------------|------------|----------------|---------------|
| Small | 4 | 4 | 100 GB | 1.6 TB | 1.2 TB |
| Medium | 4 | 4 | 500 GB | 8 TB | 6 TB |
| Large | 8 | 4 | 1 TB | 32 TB | 24 TB |
| XL | 16 | 4 | 2 TB | 128 TB | 96 TB |

---

## Topology B: Multi-DC MinIO with Replication

### Overview

Independent MinIO clusters in each datacenter with active-active site replication. Thanos Store Gateways read from local MinIO, with global Thanos Query for unified view.

### Architecture Diagram

```
+------------------------------------+     +------------------------------------+
|          Datacenter 1              |     |          Datacenter 2              |
|                                    |     |                                    |
|  +------------------------------+  |     |  +------------------------------+  |
|  |      MinIO Cluster (DC1)     |  |     |  |      MinIO Cluster (DC2)     |  |
|  |                              |  |     |  |                              |  |
|  | +------+ +------+ +------+   |  |     |  | +------+ +------+ +------+   |  |
|  | |node-0| |node-1| |node-2|...|  |     |  | |node-0| |node-1| |node-2|...|  |
|  | +------+ +------+ +------+   |  |     |  | +------+ +------+ +------+   |  |
|  |                              |  |     |  |                              |  |
|  +-------------+----------------+  |     |  +-------------+----------------+  |
|                |                   |     |                |                   |
|          Site Replication <---------------------+         |                   |
|          (Active-Active)           |     |     +------------------->          |
|                |                   |     |                |                   |
|  +-------------v----------------+  |     |  +-------------v----------------+  |
|  |    helios-metrics Bucket     |  |     |  |    helios-metrics Bucket     |  |
|  |    (Primary + Replica)       |  |     |  |    (Primary + Replica)       |  |
|  +-------------+----------------+  |     |  +-------------+----------------+  |
|                |                   |     |                |                   |
|  +-------------v----------------+  |     |  +-------------v----------------+  |
|  |      Thanos Components       |  |     |  |      Thanos Components       |  |
|  |                              |  |     |  |                              |  |
|  | +----------+ +----------+    |  |     |  | +----------+ +----------+    |  |
|  | | Store GW | |Compactor*|    |  |     |  | | Store GW | |(standby) |    |  |
|  | +----------+ +----------+    |  |     |  | +----------+ +----------+    |  |
|  |                              |  |     |  |                              |  |
|  | +----------+ +----------+    |  |     |  | +----------+ +----------+    |  |
|  | |Prometheus| |Prometheus|    |  |     |  | |Prometheus| |Prometheus|    |  |
|  | |+ Sidecar | |+ Sidecar |    |  |     |  | |+ Sidecar | |+ Sidecar |    |  |
|  | +----------+ +----------+    |  |     |  | +----------+ +----------+    |  |
|  +------------------------------+  |     |  +------------------------------+  |
+----------------+-------------------+     +-------------------+-----------------+
                 |                                             |
                 |       +---------------------------+         |
                 +------>|    Global Thanos Query    |<--------+
                         |   (Query Federation)      |
                         +-------------+-------------+
                                       |
                         +-------------v-------------+
                         |    Thanos Query Frontend  |
                         |    (With Caching)         |
                         +-------------+-------------+
                                       |
                         +-------------v-------------+
                         |         Grafana           |
                         +---------------------------+

* Thanos Compactor runs as singleton - only one active across all DCs
```

### Site Replication Flow

```
DC1 Write Path:
Prometheus --> Sidecar --> MinIO DC1 --> Site Replication --> MinIO DC2
                               |
                               v
                         Store Gateway (DC1)

DC2 Write Path:
Prometheus --> Sidecar --> MinIO DC2 --> Site Replication --> MinIO DC1
                               |
                               v
                         Store Gateway (DC2)

Query Path (Global):
Grafana --> Query Frontend --> Thanos Query
                                    |
                    +---------------+---------------+
                    |                               |
              Store GW (DC1)                  Store GW (DC2)
                    |                               |
              MinIO (DC1)                     MinIO (DC2)
```

### Thanos Compactor Singleton Strategy

The Thanos Compactor MUST be a singleton across all DCs to prevent data corruption. Options:

**Option 1: Leader Election (Recommended)**
```yaml
# Use K8s lease-based leader election across clusters
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-compactor
  namespace: helios-storage
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: compactor
        args:
          - compact
          - --wait
          - --data-dir=/var/thanos/compact
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --compact.concurrency=1
          # Deduplication based on replica labels
          - --deduplication.replica-label=replica
          - --deduplication.replica-label=datacenter
```

**Option 2: Designate Primary DC**
- Run Compactor only in DC1
- DC2 relies on replicated compacted blocks
- Failover: Manual promotion of DC2 Compactor if DC1 fails

**Option 3: Receive-based Architecture**
- Use Thanos Receive instead of Sidecars
- Receive handles deduplication
- Compactor per-DC with tenant separation

### Pros

1. **High Availability**: Survives complete DC failure
2. **Data Locality**: Queries served from local data
3. **Low RPO**: Near-real-time replication (seconds to minutes)
4. **Compliance**: Data remains in designated regions
5. **Performance**: Local reads, no cross-DC query latency
6. **Active-Active**: Both DCs actively serve traffic

### Cons

1. **Complexity**: Two clusters to manage
2. **Cost**: Double the storage hardware
3. **Bandwidth**: Cross-DC replication traffic
4. **Compactor Coordination**: Singleton complexity
5. **Conflict Resolution**: Potential for replication conflicts
6. **Operational Overhead**: More components to monitor

### When to Use

- Enterprise multi-site deployments
- Strict data residency requirements
- Zero or near-zero RPO requirements
- Large scale (500+ devices per DC)
- Critical production workloads

### Site Replication Configuration

```yaml
# mc command to configure site replication
# Run from admin workstation with access to both clusters

# Add sites
mc alias set dc1-minio https://minio.dc1.example.com ACCESS_KEY SECRET_KEY
mc alias set dc2-minio https://minio.dc2.example.com ACCESS_KEY SECRET_KEY

# Enable site replication
mc admin replicate add dc1-minio dc2-minio

# Verify replication status
mc admin replicate status dc1-minio
```

---

## Topology C: Cloud Object Storage

### Overview

Use cloud-native object storage (AWS S3, GCS, or Azure Blob) as the central storage backend. Thanos Sidecars from multiple DCs write directly to cloud storage.

### Architecture Diagram

```
+------------------------------------+     +------------------------------------+
|          Datacenter 1              |     |          Datacenter 2              |
|                                    |     |                                    |
|  +------------------------------+  |     |  +------------------------------+  |
|  |      Prometheus + Sidecar    |  |     |  |      Prometheus + Sidecar    |  |
|  |                              |  |     |  |                              |  |
|  | +----------+ +----------+    |  |     |  | +----------+ +----------+    |  |
|  | |Prometheus| |Prometheus|    |  |     |  | |Prometheus| |Prometheus|    |  |
|  | |+ Sidecar | |+ Sidecar |    |  |     |  | |+ Sidecar | |+ Sidecar |    |  |
|  | +----+-----+ +----+-----+    |  |     |  | +----+-----+ +----+-----+    |  |
|  +------|------------|----------+  |     |  +------|------------|----------+  |
|         |            |             |     |         |            |             |
+---------+------------+-------------+     +---------+------------+-------------+
          |            |                             |            |
          +------------+-----------------------------+------------+
                                     |
                       +-------------v-------------+
                       |    Cloud Object Storage   |
                       |                           |
                       |  +---------------------+  |
                       |  |   helios-metrics    |  |
                       |  |      (Bucket)       |  |
                       |  +---------------------+  |
                       |                           |
                       |  AWS S3 / GCS / Azure     |
                       +-------------+-------------+
                                     |
                       +-------------v-------------+
                       |   Thanos Store Gateway    |
                       |      (2+ replicas)        |
                       +-------------+-------------+
                                     |
                       +-------------v-------------+
                       |     Thanos Compactor      |
                       |      (1 singleton)        |
                       +-------------+-------------+
                                     |
                       +-------------v-------------+
                       |      Thanos Query         |
                       |   + Query Frontend        |
                       +-------------+-------------+
                                     |
                       +-------------v-------------+
                       |         Grafana           |
                       +---------------------------+
```

### Cloud Provider Configurations

**AWS S3**:
```yaml
# objstore.yaml for AWS S3
type: S3
config:
  bucket: helios-metrics-prod
  region: us-east-1
  endpoint: s3.us-east-1.amazonaws.com
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
  # Optional: Use IAM roles instead of keys
  # aws_sdk_auth: true

  # S3 Select for efficient queries (optional)
  sse_config:
    type: SSE-S3

  # Intelligent tiering
  storage_class: INTELLIGENT_TIERING
```

**Google Cloud Storage**:
```yaml
# objstore.yaml for GCS
type: GCS
config:
  bucket: helios-metrics-prod
  service_account: |
    {
      "type": "service_account",
      "project_id": "helios-prod",
      ...
    }
  # Or use workload identity
  # service_account: ""  # Uses default credentials
```

**Azure Blob Storage**:
```yaml
# objstore.yaml for Azure Blob
type: AZURE
config:
  storage_account: heliosmetrics
  container: thanos
  # Use managed identity or SAS token
  storage_account_key: ${AZURE_STORAGE_KEY}
  # Or: msi_resource: https://storage.azure.com/
```

### Pros

1. **Simplicity**: No storage infrastructure to manage
2. **Durability**: 11 9s durability built-in
3. **Scalability**: Virtually unlimited capacity
4. **Global Access**: Available from any location
5. **Cost Optimization**: Pay-as-you-go, tiered storage classes
6. **DR Built-In**: Cross-region replication available
7. **Compliance**: CSP certifications (SOC2, HIPAA, etc.)

### Cons

1. **Egress Costs**: Significant costs for data retrieval
2. **Latency**: Higher latency than local storage
3. **Vendor Lock-In**: Switching providers requires migration
4. **Internet Dependency**: Requires reliable internet connectivity
5. **Data Sovereignty**: Data leaves on-premises
6. **Variable Costs**: Unpredictable monthly bills

### When to Use

- Cloud-native or hybrid cloud deployments
- Organizations already using cloud infrastructure
- When operational simplicity is priority
- For smaller datasets where egress costs are manageable
- When compliance allows cloud storage

### Cost Optimization Strategies

**1. Lifecycle Policies**:
```json
{
  "Rules": [
    {
      "ID": "TransitionToIA",
      "Status": "Enabled",
      "Filter": {"Prefix": ""},
      "Transitions": [
        {"Days": 30, "StorageClass": "STANDARD_IA"},
        {"Days": 90, "StorageClass": "GLACIER_IR"}
      ]
    }
  ]
}
```

**2. S3 Intelligent-Tiering**:
- Automatic tier movement based on access patterns
- No retrieval fees for Frequent/Infrequent tiers
- Ideal for unpredictable query patterns

**3. Regional Considerations**:
- Deploy Store Gateways in same region as bucket
- Use VPC endpoints to avoid internet egress
- Consider multi-region buckets for global queries

---

## Topology D: Hybrid MinIO + Cloud Tiering

### Overview

MinIO serves as the primary storage for hot data (0-14 days), with automatic lifecycle tiering to cloud storage for older data (14-90 days). This optimizes cost while maintaining local performance for recent data.

### Architecture Diagram

```
                    +-------------------------------------------+
                    |            Datacenter / K8s Cluster       |
                    |                                           |
                    |  +-------------------------------------+  |
                    |  |         MinIO (Hot Storage)         |  |
                    |  |         Retention: 0-14 days        |  |
                    |  |                                      |  |
                    |  | +------+ +------+ +------+ +------+ |  |
                    |  | |node-0| |node-1| |node-2| |node-3| |  |
                    |  | +------+ +------+ +------+ +------+ |  |
                    |  +------------------+------------------+  |
                    |                     |                     |
                    |                     | Lifecycle Policy    |
                    |                     | (Transition >14d)   |
                    |                     |                     |
                    +---------------------|---------------------+
                                          |
                                          v
                    +-------------------------------------------+
                    |         Cloud Object Storage              |
                    |         (Warm/Cold Storage)               |
                    |         Retention: 14-90 days             |
                    |                                           |
                    |  +-------------------------------------+  |
                    |  |   AWS S3 / GCS / Azure Blob         |  |
                    |  |   (STANDARD_IA or equivalent)       |  |
                    |  +-------------------------------------+  |
                    +-------------------------------------------+

                    +-------------------------------------------+
                    |            Thanos Components              |
                    |                                           |
                    |  +-----------------+  +-----------------+ |
                    |  | Store Gateway   |  | Store Gateway   | |
                    |  | (MinIO - Hot)   |  | (Cloud - Warm)  | |
                    |  +-----------------+  +-----------------+ |
                    |           |                    |          |
                    |           +----------+---------+          |
                    |                      |                    |
                    |           +----------v---------+          |
                    |           |    Thanos Query    |          |
                    |           | (Federated View)   |          |
                    |           +--------------------+          |
                    +-------------------------------------------+
```

### MinIO Tiering Configuration

```yaml
# Configure MinIO to tier to AWS S3
# mc ilm tier add minio S3 WARM-TIER \
#   --endpoint https://s3.us-east-1.amazonaws.com \
#   --access-key ${AWS_ACCESS_KEY} \
#   --secret-key ${AWS_SECRET_KEY} \
#   --bucket helios-metrics-warm \
#   --region us-east-1 \
#   --storage-class STANDARD_IA

# Apply lifecycle rule
# mc ilm rule add minio/helios-metrics \
#   --transition-days 14 \
#   --transition-tier WARM-TIER \
#   --noncurrent-transition-days 7 \
#   --noncurrent-transition-tier WARM-TIER
```

### Thanos Multi-Backend Configuration

```yaml
# Store Gateway for MinIO (Hot)
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-store-hot
  namespace: helios-storage
spec:
  template:
    spec:
      containers:
      - name: thanos-store
        args:
          - store
          - --data-dir=/var/thanos/store
          - --objstore.config-file=/etc/thanos/objstore-minio.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
          # Only serve recent data
          - --min-time=-14d
        volumeMounts:
        - name: objstore-config-minio
          mountPath: /etc/thanos
---
# Store Gateway for Cloud (Warm)
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-store-warm
  namespace: helios-storage
spec:
  template:
    spec:
      containers:
      - name: thanos-store
        args:
          - store
          - --data-dir=/var/thanos/store
          - --objstore.config-file=/etc/thanos/objstore-s3.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
          # Only serve older data
          - --max-time=-14d
        volumeMounts:
        - name: objstore-config-s3
          mountPath: /etc/thanos
```

### Data Flow

```
Write Path:
Prometheus --> Thanos Sidecar --> MinIO (Hot)
                                      |
                                      | (after 14 days)
                                      v
                               MinIO Tiering
                                      |
                                      v
                              Cloud S3 (Warm)

Query Path (Recent - < 14 days):
Grafana --> Thanos Query --> Store Gateway (Hot) --> MinIO

Query Path (Historical - > 14 days):
Grafana --> Thanos Query --> Store Gateway (Warm) --> Cloud S3

Query Path (Spanning):
Grafana --> Thanos Query --+--> Store Gateway (Hot) --> MinIO
                           |
                           +--> Store Gateway (Warm) --> Cloud S3
```

### Pros

1. **Cost Optimized**: Hot storage local, cold storage cheap cloud
2. **Performance**: Recent data served locally with low latency
3. **Scalability**: Cloud handles long-term growth
4. **Flexibility**: Adjustable tier boundaries
5. **Data Locality**: Recent data stays on-premises
6. **Reduced Egress**: Only cold queries hit cloud

### Cons

1. **Complexity**: Two storage systems to manage
2. **Transition Lag**: 24-48 hour delay for tier transitions
3. **Query Complexity**: Spanning queries hit both backends
4. **Dependency**: Relies on both MinIO and cloud availability
5. **MinIO Overhead**: Still need to operate MinIO cluster

### When to Use

- Cost-sensitive deployments with significant retention
- When recent data queries are frequent
- Compliance requires recent data on-premises
- Medium to large scale with 30+ day retention
- Organizations with existing cloud accounts

### Tier Boundary Optimization

| Retention | Hot Tier (MinIO) | Warm Tier (S3-IA) | Cold Tier (Glacier) |
|-----------|------------------|-------------------|---------------------|
| 30 days | 7 days | 8-30 days | - |
| 60 days | 14 days | 15-60 days | - |
| 90 days | 14 days | 15-60 days | 61-90 days |
| 365 days | 14 days | 15-90 days | 91-365 days |

---

## MinIO Architecture Deep Dive

### Distributed Mode Requirements

MinIO distributed mode requires a minimum of 4 nodes with erasure coding for data protection.

```
                    MinIO Distributed Cluster
    +-------------------------------------------------------+
    |                                                       |
    |  +----------+  +----------+  +----------+  +--------+ |
    |  | Node 0   |  | Node 1   |  | Node 2   |  | Node 3 | |
    |  |          |  |          |  |          |  |        | |
    |  | +------+ |  | +------+ |  | +------+ |  | +----+ | |
    |  | |Drive0| |  | |Drive0| |  | |Drive0| |  | |D0  | | |
    |  | |Drive1| |  | |Drive1| |  | |Drive1| |  | |D1  | | |
    |  | |Drive2| |  | |Drive2| |  | |Drive2| |  | |D2  | | |
    |  | |Drive3| |  | |Drive3| |  | |Drive3| |  | |D3  | | |
    |  | +------+ |  | +------+ |  | +------+ |  | +----+ | |
    |  +----------+  +----------+  +----------+  +--------+ |
    |                                                       |
    |  Erasure Set: 16 drives (4 nodes x 4 drives)         |
    |  EC:4 = Can lose up to 4 drives                      |
    |  Usable Capacity: 75% of raw                         |
    +-------------------------------------------------------+
```

### Erasure Coding Options

| Configuration | Drives | Data Shards | Parity Shards | Drive Tolerance | Usable Capacity |
|---------------|--------|-------------|---------------|-----------------|-----------------|
| EC:2 | 4 | 2 | 2 | 2 | 50% |
| EC:4 (Default) | 8-16 | N-4 | 4 | 4 | 75% |
| EC:8 | 16+ | N-8 | 8 | 8 | 50% |

**Recommendation**: Use EC:4 for production deployments (balance of protection and efficiency).

### Drive Layout Recommendations

**Small Scale (4 nodes)**:
```yaml
# 4 nodes, 4 drives each = 16 drives, 1 erasure set
nodes:
  - minio-0: [/data/drive0, /data/drive1, /data/drive2, /data/drive3]
  - minio-1: [/data/drive0, /data/drive1, /data/drive2, /data/drive3]
  - minio-2: [/data/drive0, /data/drive1, /data/drive2, /data/drive3]
  - minio-3: [/data/drive0, /data/drive1, /data/drive2, /data/drive3]

erasure_sets: 1
total_drives: 16
ec_parity: 4 (EC:4)
usable_capacity: 75%
```

**Large Scale (16 nodes)**:
```yaml
# 16 nodes, 4 drives each = 64 drives, 4 erasure sets
nodes:
  - minio-[0-15]: [/data/drive0, /data/drive1, /data/drive2, /data/drive3]

erasure_sets: 4
drives_per_set: 16
total_drives: 64
ec_parity: 4 (EC:4)
usable_capacity: 75%
```

### Sizing Tables

#### Small Scale (< 50 devices, ~80 GB storage)

| Resource | Specification |
|----------|---------------|
| Nodes | 4 |
| Drives per Node | 4 |
| Drive Size | 50 GB |
| Total Raw | 800 GB |
| Usable (EC:4) | 600 GB |
| CPU per Node | 2 cores |
| Memory per Node | 4 GB |
| Network | 1 Gbps |

#### Medium Scale (50-500 devices, ~800 GB storage)

| Resource | Specification |
|----------|---------------|
| Nodes | 4 |
| Drives per Node | 4 |
| Drive Size | 500 GB |
| Total Raw | 8 TB |
| Usable (EC:4) | 6 TB |
| CPU per Node | 4 cores |
| Memory per Node | 8 GB |
| Network | 10 Gbps |

#### Large Scale (500-2000 devices, ~3.2 TB storage)

| Resource | Specification |
|----------|---------------|
| Nodes | 8 |
| Drives per Node | 4 |
| Drive Size | 1 TB |
| Total Raw | 32 TB |
| Usable (EC:4) | 24 TB |
| CPU per Node | 8 cores |
| Memory per Node | 16 GB |
| Network | 10 Gbps |
| IOPS per Node | 10,000+ |

#### XL Scale (2000-8000 devices, ~12.8 TB storage)

| Resource | Specification |
|----------|---------------|
| Nodes | 16 |
| Drives per Node | 4 |
| Drive Size | 2 TB |
| Total Raw | 128 TB |
| Usable (EC:4) | 96 TB |
| CPU per Node | 16 cores |
| Memory per Node | 32 GB |
| Network | 25 Gbps |
| IOPS per Node | 50,000+ |

### Kubernetes Deployment Manifests

**MinIO StatefulSet**:

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: minio
  namespace: helios-storage
  labels:
    app: minio
    app.kubernetes.io/name: minio
    app.kubernetes.io/part-of: helios
spec:
  serviceName: minio-headless
  replicas: 4  # Minimum for distributed mode
  podManagementPolicy: Parallel
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchLabels:
                  app: minio
              topologyKey: kubernetes.io/hostname
      containers:
      - name: minio
        image: quay.io/minio/minio:RELEASE.2025-01-01T00-00-00Z
        args:
          - server
          - --console-address
          - ":9001"
          # Distributed mode: specify all nodes and drives
          - http://minio-{0...3}.minio-headless.helios-storage.svc.cluster.local/data/drive{0...3}
        env:
        - name: MINIO_ROOT_USER
          valueFrom:
            secretKeyRef:
              name: minio-credentials
              key: root-user
        - name: MINIO_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: minio-credentials
              key: root-password
        - name: MINIO_PROMETHEUS_AUTH_TYPE
          value: "public"
        ports:
        - name: api
          containerPort: 9000
        - name: console
          containerPort: 9001
        resources:
          requests:
            cpu: 1000m
            memory: 2Gi
          limits:
            cpu: 4000m
            memory: 8Gi
        livenessProbe:
          httpGet:
            path: /minio/health/live
            port: 9000
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /minio/health/ready
            port: 9000
          initialDelaySeconds: 30
          periodSeconds: 15
        volumeMounts:
        - name: data-0
          mountPath: /data/drive0
        - name: data-1
          mountPath: /data/drive1
        - name: data-2
          mountPath: /data/drive2
        - name: data-3
          mountPath: /data/drive3
  volumeClaimTemplates:
  - metadata:
      name: data-0
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: local-nvme  # Or your storage class
      resources:
        requests:
          storage: 500Gi
  - metadata:
      name: data-1
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: local-nvme
      resources:
        requests:
          storage: 500Gi
  - metadata:
      name: data-2
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: local-nvme
      resources:
        requests:
          storage: 500Gi
  - metadata:
      name: data-3
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: local-nvme
      resources:
        requests:
          storage: 500Gi
---
apiVersion: v1
kind: Service
metadata:
  name: minio-headless
  namespace: helios-storage
  labels:
    app: minio
spec:
  type: ClusterIP
  clusterIP: None
  ports:
  - name: api
    port: 9000
    targetPort: 9000
  - name: console
    port: 9001
    targetPort: 9001
  selector:
    app: minio
---
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: helios-storage
  labels:
    app: minio
spec:
  type: ClusterIP
  ports:
  - name: api
    port: 9000
    targetPort: 9000
  - name: console
    port: 9001
    targetPort: 9001
  selector:
    app: minio
---
apiVersion: v1
kind: Secret
metadata:
  name: minio-credentials
  namespace: helios-storage
type: Opaque
stringData:
  root-user: admin
  root-password: CHANGEME_STRONG_PASSWORD_HERE
```

**MinIO Ingress with TLS**:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minio-console
  namespace: helios-storage
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - minio.example.com
    secretName: minio-tls
  rules:
  - host: minio.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: minio
            port:
              number: 9001
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: minio-api
  namespace: helios-storage
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: "0"
    nginx.ingress.kubernetes.io/proxy-buffering: "off"
    nginx.ingress.kubernetes.io/backend-protocol: "HTTPS"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - s3.example.com
    secretName: minio-s3-tls
  rules:
  - host: s3.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: minio
            port:
              number: 9000
```

**MinIO Operator (Alternative)**:

```yaml
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: helios
  namespace: helios-storage
spec:
  image: quay.io/minio/minio:RELEASE.2025-01-01T00-00-00Z
  imagePullPolicy: IfNotPresent

  pools:
  - name: pool-0
    servers: 4
    volumesPerServer: 4
    volumeClaimTemplate:
      metadata:
        name: data
      spec:
        accessModes:
        - ReadWriteOnce
        storageClassName: local-nvme
        resources:
          requests:
            storage: 500Gi
    resources:
      requests:
        cpu: 1000m
        memory: 2Gi
      limits:
        cpu: 4000m
        memory: 8Gi
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              v1.min.io/tenant: helios
          topologyKey: kubernetes.io/hostname

  mountPath: /export

  configuration:
    name: helios-env

  users:
  - name: helios-user-0

  buckets:
  - name: helios-metrics
    objectLock: false

  features:
    enableSFTP: false

  prometheus:
    diskCapacityGB: 1

  console:
    consoleSecret:
      name: helios-console
    replicas: 2
```

---

## Site Replication Design

### Active-Active Configuration

MinIO site replication provides active-active replication between clusters with eventual consistency.

```
Site 1 (DC1)                              Site 2 (DC2)
+-----------------+                       +-----------------+
|    MinIO        |                       |    MinIO        |
|    Cluster      |                       |    Cluster      |
|                 |    Bidirectional      |                 |
|   +----------+  |    Replication        |  +----------+   |
|   | Bucket A |<-------------------------->| Bucket A |   |
|   +----------+  |                       |  +----------+   |
|                 |                       |                 |
|   +----------+  |                       |  +----------+   |
|   | Bucket B |<-------------------------->| Bucket B |   |
|   +----------+  |                       |  +----------+   |
+-----------------+                       +-----------------+
        |                                         |
        v                                         v
  Thanos Sidecar                           Thanos Sidecar
  writes locally                           writes locally
```

### Replication Policies

**What Gets Replicated**:
- Objects (including versions)
- Bucket policies
- Bucket tags
- Object lock configuration
- IAM policies and users
- Bucket notifications

**What Does NOT Get Replicated**:
- Bucket lifecycle rules (must be configured per-site)
- Existing objects before replication enabled (use `mc mirror`)

### Configuration Steps

```bash
#!/bin/bash
# site-replication-setup.sh

# 1. Set up aliases for both sites
mc alias set dc1 https://minio.dc1.example.com ADMIN_ACCESS_KEY ADMIN_SECRET_KEY
mc alias set dc2 https://minio.dc2.example.com ADMIN_ACCESS_KEY ADMIN_SECRET_KEY

# 2. Create bucket on first site (will be replicated)
mc mb dc1/helios-metrics

# 3. Enable site replication
mc admin replicate add dc1 dc2

# 4. Verify replication status
mc admin replicate info dc1

# 5. Check replication lag
mc admin replicate status dc1 --all

# 6. Test replication
echo "test" | mc pipe dc1/helios-metrics/test.txt
sleep 5
mc cat dc2/helios-metrics/test.txt
```

### Bandwidth Considerations

| Scale | Daily Upload | Replication Bandwidth | Recommended WAN |
|-------|--------------|----------------------|-----------------|
| Small | 4 GB/day | ~50 Mbps peak | 100 Mbps |
| Medium | 40 GB/day | ~500 Mbps peak | 1 Gbps |
| Large | 160 GB/day | ~2 Gbps peak | 10 Gbps |
| XL | 640 GB/day | ~8 Gbps peak | 10 Gbps dedicated |

**Bandwidth Calculation**:
```
Replication Bandwidth = (Daily Data Volume / 86400 seconds) * 8 * Safety Factor

Example (Large Scale):
160 GB/day = (160 * 8 Gb) / 86400 = ~15 Mbps sustained
Peak (10x factor) = 150 Mbps
With compression and burst = 500 Mbps - 2 Gbps peaks
```

### Conflict Resolution

MinIO site replication uses **last-write-wins** based on object modification time.

**Conflict Scenarios**:

| Scenario | Resolution | Impact on Thanos |
|----------|------------|------------------|
| Same object written to both sites | Last timestamp wins | Minimal - TSDB blocks are immutable |
| Object deleted in one site | Deletion replicates | Could lose blocks if accidental |
| Network partition | Sites diverge, reconcile on reconnect | Possible duplicate blocks |

**Thanos-Specific Considerations**:
- Thanos blocks are write-once, immutable
- Each Prometheus/Sidecar writes to specific bucket prefix
- Use external labels to identify source: `datacenter: dc1`
- Compactor handles deduplication across DCs

### Failure Scenarios and Recovery

**Scenario 1: DC1 Complete Failure**

```
Before Failure:
DC1 [MinIO] <--> [MinIO] DC2
     |              |
  Prometheus    Prometheus
  + Sidecar     + Sidecar

During DC1 Failure:
DC1 [OFFLINE] <-X-> [MinIO] DC2
                       |
                   Prometheus (DC2 only)
                   + Sidecar

Recovery Actions:
1. DC2 continues operating independently
2. Thanos Query routes all queries to DC2 Store Gateway
3. When DC1 recovers, replication resumes automatically
4. Historical blocks from DC1 are already in DC2
5. Gap in DC1 metrics from failure period (filled by DC2)
```

**Runbook: DC Failure Recovery**

```bash
#!/bin/bash
# dc-failure-recovery.sh

# 1. Confirm replication status
mc admin replicate info dc2

# 2. If DC1 is permanently lost, remove from replication
mc admin replicate rm dc2 dc1

# 3. If DC1 is recovered, re-add
mc admin replicate add dc1 dc2

# 4. Force sync of missed objects
mc admin replicate resync start dc1 --older-than 1d
```

**Scenario 2: Network Partition**

```
During Partition:
DC1 [MinIO]    X    [MinIO] DC2
     |                  |
 Writes OK          Writes OK
 (queued replication)   (queued replication)

After Reconnection:
- Replication queue processes
- Objects sync bidirectionally
- Thanos Query sees unified view
```

**Monitoring Replication Health**:

```yaml
# Prometheus alert for replication lag
groups:
- name: minio-replication
  rules:
  - alert: MinIOReplicationLag
    expr: minio_cluster_replication_last_hour_failed_bytes > 0
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: MinIO replication is lagging
      description: "{{ $value }} bytes failed to replicate in the last hour"

  - alert: MinIOReplicationDown
    expr: minio_cluster_health_status != 1
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: MinIO cluster health degraded
```

---

## Thanos Integration Patterns

### objstore.yaml Configurations

**MinIO (Single-DC)**:

```yaml
# objstore-minio.yaml
type: S3
config:
  bucket: helios-metrics
  endpoint: minio.helios-storage.svc.cluster.local:9000
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}
  insecure: true  # Internal traffic, TLS termination at ingress
  signature_version2: false
  put_user_metadata:
    datacenter: dc1
  http_config:
    idle_conn_timeout: 90s
    response_header_timeout: 2m
    insecure_skip_verify: false
    tls_config:
      ca_file: ""
      cert_file: ""
      key_file: ""
      server_name: ""
```

**MinIO (Multi-DC - write local, read any)**:

```yaml
# objstore-dc1.yaml (for DC1 components)
type: S3
config:
  bucket: helios-metrics
  endpoint: minio.helios-storage.svc.cluster.local:9000  # Local MinIO
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}
  insecure: true
  put_user_metadata:
    datacenter: dc1

# objstore-dc2.yaml (for DC2 components)
type: S3
config:
  bucket: helios-metrics
  endpoint: minio.helios-storage.svc.cluster.local:9000  # Local MinIO
  access_key: ${MINIO_ACCESS_KEY}
  secret_key: ${MINIO_SECRET_KEY}
  insecure: true
  put_user_metadata:
    datacenter: dc2
```

**AWS S3**:

```yaml
# objstore-s3.yaml
type: S3
config:
  bucket: helios-metrics-prod
  region: us-east-1
  endpoint: s3.us-east-1.amazonaws.com
  access_key: ${AWS_ACCESS_KEY_ID}
  secret_key: ${AWS_SECRET_ACCESS_KEY}
  # Or use IAM role
  # signature_version2: false
  # aws_sdk_auth: true
  sse_config:
    type: SSE-S3
  # For reduced costs on infrequent access
  # storage_class: STANDARD_IA
  http_config:
    idle_conn_timeout: 90s
    response_header_timeout: 2m
```

**GCS**:

```yaml
# objstore-gcs.yaml
type: GCS
config:
  bucket: helios-metrics-prod
  service_account: |
    {
      "type": "service_account",
      "project_id": "helios-prod",
      "private_key_id": "...",
      "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
      "client_email": "thanos@helios-prod.iam.gserviceaccount.com",
      "client_id": "...",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token"
    }
  # Or use workload identity (recommended for GKE)
  # service_account: ""
```

**Azure Blob**:

```yaml
# objstore-azure.yaml
type: AZURE
config:
  storage_account: heliosmetrics
  storage_account_key: ${AZURE_STORAGE_KEY}
  container: thanos
  endpoint: blob.core.windows.net
  # Or use managed identity
  # user_assigned_id: <client-id>
  # msi_resource: https://storage.azure.com/
  http_config:
    idle_conn_timeout: 90s
    response_header_timeout: 2m
```

### Store Gateway Placement

**Single-DC Pattern**:

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
      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchLabels:
                app: thanos-store
            topologyKey: kubernetes.io/hostname
      containers:
      - name: thanos-store
        image: quay.io/thanos/thanos:v0.35.0
        args:
          - store
          - --data-dir=/var/thanos/store
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
          - --index-cache-size=2GB
          - --chunk-pool-size=4GB
          - --store.grpc.series-sample-limit=50000
          - --store.grpc.series-max-concurrency=20
        resources:
          requests:
            cpu: 500m
            memory: 4Gi
          limits:
            cpu: 2000m
            memory: 16Gi
```

**Multi-DC Pattern - Store Gateway Per DC**:

```yaml
# DC1 Store Gateway
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-store-dc1
  namespace: helios-storage
  labels:
    datacenter: dc1
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: thanos-store
        datacenter: dc1
    spec:
      containers:
      - name: thanos-store
        image: quay.io/thanos/thanos:v0.35.0
        args:
          - store
          - --data-dir=/var/thanos/store
          - --objstore.config-file=/etc/thanos/objstore-dc1.yaml
          - --grpc-address=0.0.0.0:10901
          - --http-address=0.0.0.0:10902
          # Add labels for routing
          - --label=datacenter=dc1
---
# DC2 Store Gateway (similar, with dc2 config)
```

### Compactor Placement Strategy

The Thanos Compactor is a **singleton** and must not run concurrently. In multi-DC setups:

**Option 1: Primary DC with Failover**

```yaml
# DC1 - Active Compactor
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-compactor
  namespace: helios-storage
spec:
  replicas: 1
  template:
    spec:
      containers:
      - name: compactor
        args:
          - compact
          - --wait
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --data-dir=/var/thanos/compact
          - --retention.resolution-raw=14d
          - --retention.resolution-5m=30d
          - --retention.resolution-1h=90d
          - --compact.concurrency=4
          - --downsample.concurrency=4
          - --deduplication.replica-label=replica
          - --deduplication.replica-label=datacenter
```

**Option 2: Lease-Based Leader Election (Experimental)**

```yaml
# Deploy in both DCs, use K8s lease for leader election
# Note: Requires shared K8s API or external coordination
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-compactor
spec:
  replicas: 1  # Per DC, but only leader runs
  template:
    spec:
      containers:
      - name: compactor
        args:
          - compact
          - --wait
          - --objstore.config-file=/etc/thanos/objstore.yaml
          # Leader election sidecar handles coordination
      - name: leader-election
        image: k8s.gcr.io/leader-elector:0.5
        args:
          - --election=thanos-compactor
          - --http=0.0.0.0:4040
```

### Query Federation Setup

**Multi-DC Query Architecture**:

```
                         +------------------------+
                         |    Global Load         |
                         |    Balancer / DNS      |
                         +-----------+------------+
                                     |
                         +-----------v------------+
                         |   Thanos Query         |
                         |   Frontend             |
                         |   (Query Caching)      |
                         +-----------+------------+
                                     |
                         +-----------v------------+
                         |   Thanos Query         |
                         |   (Global Federation)  |
                         +-----------+------------+
                                     |
             +-------------+---------+---------+-------------+
             |             |                   |             |
    +--------v--------+ +--v--------+ +--------v--+ +--------v--------+
    | Store Gateway   | | Store GW  | | Store GW  | | Prometheus      |
    | (DC1 MinIO)     | | (DC2)     | | (Cloud)   | | (Recent Data)   |
    +-----------------+ +-----------+ +-----------+ +-----------------+
```

**Thanos Query Configuration (Multi-DC)**:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: thanos-query
  namespace: helios-storage
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: query
        image: quay.io/thanos/thanos:v0.35.0
        args:
          - query
          - --http-address=0.0.0.0:9090
          - --grpc-address=0.0.0.0:10901
          # Connect to all Store Gateways
          - --endpoint=dnssrv+_grpc._tcp.thanos-store-dc1.helios-storage.svc
          - --endpoint=dnssrv+_grpc._tcp.thanos-store-dc2.helios-storage.svc
          - --endpoint=dnssrv+_grpc._tcp.thanos-store-cloud.helios-storage.svc
          # Connect to Prometheus Sidecars for recent data
          - --endpoint=dnssrv+_grpc._tcp.prometheus.helios-storage.svc
          # Deduplication
          - --query.replica-label=replica
          - --query.replica-label=datacenter
          # Performance
          - --query.auto-downsampling
          - --query.partial-response
          - --query.timeout=2m
          - --store.response-timeout=30s
        ports:
        - name: http
          containerPort: 9090
        - name: grpc
          containerPort: 10901
```

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
      - name: query-frontend
        image: quay.io/thanos/thanos:v0.35.0
        args:
          - query-frontend
          - --http-address=0.0.0.0:9090
          - --query-frontend.downstream-url=http://thanos-query:9090
          - --query-range.split-interval=24h
          - --query-range.max-retries-per-request=5
          - --query-frontend.log-queries-longer-than=10s
          # In-memory cache (for small deployments)
          - --query-range.response-cache-config=
              type: IN-MEMORY
              config:
                max_size: 1GB
                max_item_size: 50MB
        # For production, use Redis or Memcached
        # - --query-range.response-cache-config-file=/etc/thanos/cache.yaml
```

---

## Network Considerations

### Cilium ClusterMesh Integration

For multi-DC deployments using Cilium, storage traffic can flow over the ClusterMesh overlay.

**Architecture**:

```
DC1 K8s Cluster                           DC2 K8s Cluster
+------------------+                      +------------------+
|   Cilium Agent   |                      |   Cilium Agent   |
|                  |                      |                  |
|  +------------+  |   Cilium Mesh        |  +------------+  |
|  |   Pod      |<----------------------->|  |   Pod      |  |
|  | (Thanos)   |  |   (WireGuard/        |  | (Thanos)   |  |
|  +------------+  |    VXLAN Tunnel)     |  +------------+  |
+------------------+                      +------------------+
         |                                         |
         v                                         v
+------------------+                      +------------------+
|   MinIO Pod      |                      |   MinIO Pod      |
| (Local Storage)  |                      | (Local Storage)  |
+------------------+                      +------------------+
```

**ClusterMesh Configuration for Helios**:

```yaml
# Enable ClusterMesh in Cilium
apiVersion: cilium.io/v2
kind: CiliumClusterConfig
metadata:
  name: dc1-cluster
spec:
  cluster:
    name: dc1
    id: 1
---
# Global Service for MinIO (accessible from both clusters)
apiVersion: v1
kind: Service
metadata:
  name: minio-global
  namespace: helios-storage
  annotations:
    io.cilium/global-service: "true"
    io.cilium/shared-service: "true"
spec:
  type: ClusterIP
  selector:
    app: minio
  ports:
  - port: 9000
    targetPort: 9000
```

**Service Mesh Considerations**:

If using Istio or Linkerd:

```yaml
# Istio ServiceEntry for cross-DC MinIO
apiVersion: networking.istio.io/v1alpha3
kind: ServiceEntry
metadata:
  name: minio-dc2
spec:
  hosts:
  - minio.dc2.example.com
  ports:
  - number: 9000
    name: s3
    protocol: TCP
  resolution: DNS
  location: MESH_EXTERNAL
---
# DestinationRule for connection pooling
apiVersion: networking.istio.io/v1alpha3
kind: DestinationRule
metadata:
  name: minio-dc2
spec:
  host: minio.dc2.example.com
  trafficPolicy:
    connectionPool:
      tcp:
        maxConnections: 100
      http:
        h2UpgradePolicy: UPGRADE
        http2MaxRequests: 1000
```

### Bandwidth Requirements

**Thanos Sidecar Upload**:

| Scale | Metrics/sec | Block Size (2h) | Upload Rate | Bandwidth |
|-------|-------------|-----------------|-------------|-----------|
| Small | 2,500 | ~100 MB | 50 MB/hr | ~110 Kbps |
| Medium | 25,000 | ~1 GB | 500 MB/hr | ~1.1 Mbps |
| Large | 100,000 | ~4 GB | 2 GB/hr | ~4.5 Mbps |
| XL | 400,000 | ~16 GB | 8 GB/hr | ~18 Mbps |

**Site Replication**:

| Scale | Daily Volume | Sustained | Peak (10x) |
|-------|--------------|-----------|------------|
| Small | 1.2 GB | ~0.1 Mbps | ~1 Mbps |
| Medium | 12 GB | ~1 Mbps | ~10 Mbps |
| Large | 48 GB | ~4 Mbps | ~40 Mbps |
| XL | 192 GB | ~18 Mbps | ~180 Mbps |

**Query Traffic**:

| Query Type | Data Transfer | Notes |
|------------|---------------|-------|
| Dashboard load (24h) | 1-5 MB | Per dashboard |
| Large query (7d, many series) | 10-50 MB | Depends on cardinality |
| Long-range query (90d) | 50-200 MB | Downsampled data |

### Latency Requirements

**Thanos Component Latency Tolerance**:

| Component | Acceptable Latency | Impact of Higher Latency |
|-----------|-------------------|--------------------------|
| Sidecar -> Object Storage | < 100ms | Upload delays, local buffer growth |
| Store Gateway -> Object Storage | < 50ms | Query slowdown |
| Query -> Store Gateway | < 10ms | Query timeout risk |
| Query Frontend -> Query | < 5ms | User-visible latency |

**Recommendations for WAN Links**:

| WAN Latency | Suitable Topology | Mitigation |
|-------------|-------------------|------------|
| < 10ms | Any | N/A |
| 10-50ms | B, C, D | Increase timeouts |
| 50-100ms | B, C, D | Local Store Gateways mandatory |
| > 100ms | B (with caching), C (with local cache) | Query caching essential |

---

## Backup and Disaster Recovery

### Backup Strategy

**MinIO Bucket Versioning**:

```bash
# Enable versioning on helios-metrics bucket
mc version enable minio/helios-metrics

# Configure version retention
mc ilm rule add minio/helios-metrics \
  --noncurrent-expire-days 30 \
  --noncurrent-transition-days 7 \
  --noncurrent-transition-tier GLACIER
```

**Cross-Region Replication (Cloud)**:

```json
{
  "Role": "arn:aws:iam::ACCOUNT:role/replication-role",
  "Rules": [
    {
      "ID": "helios-dr",
      "Status": "Enabled",
      "Priority": 1,
      "Filter": {},
      "Destination": {
        "Bucket": "arn:aws:s3:::helios-metrics-dr-region2",
        "StorageClass": "STANDARD_IA"
      },
      "DeleteMarkerReplication": {
        "Status": "Enabled"
      }
    }
  ]
}
```

**Point-in-Time Recovery**:

For MinIO with versioning enabled:

```bash
#!/bin/bash
# restore-pitr.sh

# List object versions at specific time
TIMESTAMP="2026-01-08T12:00:00Z"
BUCKET="helios-metrics"

# Get versions before timestamp
mc ls --versions --rewind $TIMESTAMP minio/$BUCKET

# Restore specific prefix to point-in-time
mc mirror --preserve --older-than $TIMESTAMP minio/$BUCKET minio/$BUCKET-restored
```

### DR Procedures

**DC Failover Runbook**:

```markdown
## DC1 Failure - Failover to DC2

### Detection
1. Monitor alerts:
   - `MinIOClusterDown{datacenter="dc1"}`
   - `ThanosStoreGatewayDown{datacenter="dc1"}`

### Immediate Actions
1. Verify DC1 is unreachable
2. Update DNS/Load Balancer to route to DC2 only
3. Verify DC2 Thanos Query is healthy

### Thanos Configuration
1. Remove DC1 Store Gateway endpoints from Query:
   ```bash
   kubectl patch deployment thanos-query -n helios-storage \
     --type=json \
     -p='[{"op": "remove", "path": "/spec/template/spec/containers/0/args/3"}]'
   ```

2. Verify queries work with DC2 data only

### Recovery (when DC1 returns)
1. Verify MinIO replication status:
   ```bash
   mc admin replicate status dc2
   ```
2. Wait for replication to complete
3. Re-add DC1 Store Gateway endpoints
4. Verify deduplication working
```

**RTO/RPO Targets**:

| Topology | RPO | RTO | Notes |
|----------|-----|-----|-------|
| A: Single-DC | 24h (backup) | 4h | Restore from backup |
| B: Multi-DC | < 1 min | < 5 min | Failover to replica |
| C: Cloud | 0 (11 9s durability) | < 5 min | Cloud SLA |
| D: Hybrid | < 1 min (hot), hours (warm) | < 15 min | Depends on tier |

---

## Cost Analysis

### On-Premises MinIO Costs

**Hardware Costs (one-time)**:

| Scale | Servers | Storage | Network | Total Hardware |
|-------|---------|---------|---------|----------------|
| Small | 4x $1,000 | 16x 100GB SSD $50 | $2,000 | ~$7,000 |
| Medium | 4x $3,000 | 16x 500GB SSD $100 | $5,000 | ~$20,000 |
| Large | 8x $5,000 | 32x 1TB NVMe $150 | $10,000 | ~$55,000 |
| XL | 16x $8,000 | 64x 2TB NVMe $250 | $25,000 | ~$170,000 |

**Operational Costs (annual)**:

| Scale | Power | Rack Space | Admin Time | Total Annual |
|-------|-------|------------|------------|--------------|
| Small | $500 | $1,000 | $5,000 | ~$6,500 |
| Medium | $1,500 | $3,000 | $10,000 | ~$14,500 |
| Large | $4,000 | $8,000 | $20,000 | ~$32,000 |
| XL | $12,000 | $20,000 | $40,000 | ~$72,000 |

### Cloud Object Storage Costs

**AWS S3 (US-East-1)**:

| Scale | Storage | Writes | Reads | Egress | Total Monthly |
|-------|---------|--------|-------|--------|---------------|
| Small (80 GB) | $2 | $5 | $10 | $50 | ~$67 |
| Medium (800 GB) | $18 | $50 | $100 | $200 | ~$368 |
| Large (3.2 TB) | $74 | $200 | $400 | $500 | ~$1,174 |
| XL (12.8 TB) | $294 | $800 | $1,600 | $1,500 | ~$4,194 |

**Cost Reduction Strategies**:

| Strategy | Savings | Implementation |
|----------|---------|----------------|
| Intelligent-Tiering | 30-40% on storage | Storage class setting |
| VPC Endpoints | 100% of cross-AZ | VPC gateway endpoint |
| Reserved Capacity | 30-50% on storage | S3 commitment |
| Query Caching | 50-70% on reads | Query Frontend cache |

### Hybrid Cost Comparison

**3-Year TCO (Medium Scale)**:

| Component | MinIO Only | Cloud Only | Hybrid |
|-----------|------------|------------|--------|
| Hardware (3y) | $20,000 | $0 | $15,000 |
| Storage (3y) | $0 | $648 | $216 |
| Operations (3y) | $43,500 | $5,000 | $35,000 |
| Egress (3y) | $0 | $7,200 | $2,400 |
| **Total 3y TCO** | **$63,500** | **$12,848** | **$52,616** |

**Notes**:
- MinIO TCO dominated by operational overhead
- Cloud costs scale linearly with data
- Hybrid optimizes for local hot access + cheap cold storage

### Cost Comparison Matrix

| Scale | MinIO (3yr) | AWS S3 (3yr) | GCS (3yr) | Hybrid (3yr) |
|-------|-------------|--------------|-----------|--------------|
| Small | $27,500 | $2,412 | $2,160 | $22,000 |
| Medium | $63,500 | $13,248 | $11,880 | $52,616 |
| Large | $151,000 | $42,264 | $37,908 | $130,000 |
| XL | $346,000 | $150,984 | $135,324 | $290,000 |

---

## Configuration Templates

### Helm Values

**values-storage-minio-single.yaml** (Topology A):

```yaml
# Helios Storage Configuration - Single-DC MinIO

storage:
  type: minio
  topology: single-dc

minio:
  enabled: true
  mode: distributed
  replicas: 4
  drivesPerNode: 4

  persistence:
    enabled: true
    storageClass: local-nvme
    size: 500Gi

  resources:
    requests:
      cpu: 1000m
      memory: 2Gi
    limits:
      cpu: 4000m
      memory: 8Gi

  metrics:
    enabled: true
    serviceMonitor:
      enabled: true

  buckets:
    - name: helios-metrics
      policy: none
      purge: false

  users:
    - accessKey: thanos
      secretKey: ${THANOS_SECRET_KEY}
      policy: readwrite

thanos:
  enabled: true

  objstoreConfig:
    type: S3
    config:
      bucket: helios-metrics
      endpoint: minio.helios-storage.svc.cluster.local:9000
      insecure: true

  storegateway:
    enabled: true
    replicas: 2
    resources:
      requests:
        cpu: 500m
        memory: 4Gi

  compactor:
    enabled: true
    replicas: 1
    retentionResolutionRaw: 14d
    retentionResolution5m: 30d
    retentionResolution1h: 90d

  query:
    enabled: true
    replicas: 2

  queryFrontend:
    enabled: true
    replicas: 2
```

**values-storage-minio-multi.yaml** (Topology B):

```yaml
# Helios Storage Configuration - Multi-DC MinIO

storage:
  type: minio
  topology: multi-dc
  datacenters:
    - name: dc1
      primary: true
    - name: dc2
      primary: false

minio:
  enabled: true
  mode: distributed
  replicas: 4

  siteReplication:
    enabled: true
    peers:
      - endpoint: https://minio.dc2.example.com
        accessKey: admin
        secretKeySecretRef:
          name: minio-dc2-credentials
          key: secret-key

thanos:
  enabled: true

  storegateway:
    enabled: true
    replicas: 2
    # Only read from local MinIO
    extraArgs:
      - "--min-time=-2w"  # Recent data

  compactor:
    enabled: true
    replicas: 1
    # Run only in primary DC
    nodeSelector:
      datacenter: dc1

  query:
    enabled: true
    replicas: 3
    stores:
      - dnssrv+_grpc._tcp.thanos-store-dc1.helios-storage.svc
      - dnssrv+_grpc._tcp.thanos-store-dc2.helios-storage.svc
```

**values-storage-cloud-s3.yaml** (Topology C):

```yaml
# Helios Storage Configuration - AWS S3

storage:
  type: s3
  topology: cloud

minio:
  enabled: false

thanos:
  enabled: true

  objstoreConfig:
    type: S3
    config:
      bucket: helios-metrics-prod
      region: us-east-1
      endpoint: s3.us-east-1.amazonaws.com
      sse_config:
        type: SSE-S3

  existingObjstoreSecret: thanos-s3-credentials

  storegateway:
    enabled: true
    replicas: 2
    resources:
      requests:
        cpu: 500m
        memory: 4Gi
      limits:
        cpu: 2000m
        memory: 16Gi

  compactor:
    enabled: true
    replicas: 1

  query:
    enabled: true
    replicas: 2

  queryFrontend:
    enabled: true
    replicas: 2
    # Enable response caching for cost reduction
    extraArgs:
      - "--query-range.response-cache-config={\"type\":\"IN-MEMORY\",\"config\":{\"max_size\":\"2GB\"}}"
```

**values-storage-hybrid.yaml** (Topology D):

```yaml
# Helios Storage Configuration - Hybrid MinIO + Cloud

storage:
  type: hybrid
  topology: tiered
  tiers:
    hot:
      type: minio
      retention: 14d
    warm:
      type: s3
      retention: 90d

minio:
  enabled: true
  mode: distributed
  replicas: 4

  tiering:
    enabled: true
    warmTier:
      type: s3
      endpoint: s3.us-east-1.amazonaws.com
      bucket: helios-metrics-warm
      transitionDays: 14
      storageClass: STANDARD_IA

thanos:
  enabled: true

  # Multiple Store Gateways for different tiers
  storegateway:
    enabled: true

    hot:
      replicas: 2
      objstoreConfig:
        type: S3
        config:
          bucket: helios-metrics
          endpoint: minio.helios-storage.svc.cluster.local:9000
          insecure: true
      extraArgs:
        - "--min-time=-14d"

    warm:
      replicas: 2
      objstoreConfig:
        type: S3
        config:
          bucket: helios-metrics-warm
          region: us-east-1
          endpoint: s3.us-east-1.amazonaws.com
      extraArgs:
        - "--max-time=-14d"
```

### MinIO Tenant CRD (Operator)

```yaml
apiVersion: minio.min.io/v2
kind: Tenant
metadata:
  name: helios
  namespace: helios-storage
  labels:
    app.kubernetes.io/name: minio
    app.kubernetes.io/part-of: helios
spec:
  image: quay.io/minio/minio:RELEASE.2025-01-01T00-00-00Z
  imagePullPolicy: IfNotPresent

  ## Configure TLS
  requestAutoCert: true
  certConfig:
    commonName: "*.helios-storage.svc.cluster.local"
    dnsNames:
      - "minio.helios-storage.svc.cluster.local"
      - "*.minio-headless.helios-storage.svc.cluster.local"

  ## Storage pools
  pools:
    - name: pool-0
      servers: 4
      volumesPerServer: 4
      volumeClaimTemplate:
        metadata:
          name: data
        spec:
          accessModes:
            - ReadWriteOnce
          storageClassName: local-nvme
          resources:
            requests:
              storage: 500Gi

      resources:
        requests:
          cpu: 1000m
          memory: 2Gi
        limits:
          cpu: 4000m
          memory: 8Gi

      affinity:
        podAntiAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            - labelSelector:
                matchLabels:
                  v1.min.io/tenant: helios
              topologyKey: kubernetes.io/hostname

      tolerations: []
      nodeSelector: {}

  ## Bucket configuration
  buckets:
    - name: helios-metrics
      objectLock: false

  ## User configuration
  users:
    - name: helios-thanos

  ## Prometheus monitoring
  prometheusOperator: true

  ## Console configuration
  console:
    replicas: 2
    consoleSecret:
      name: helios-console-secret
```

---

## Migration Paths

### Single-DC to Multi-DC

**Steps**:

1. **Deploy DC2 MinIO Cluster**
   ```bash
   helm install minio-dc2 minio/minio -f values-dc2.yaml
   ```

2. **Configure Site Replication**
   ```bash
   mc admin replicate add dc1 dc2
   ```

3. **Wait for Initial Sync**
   ```bash
   # Monitor sync progress
   mc admin replicate status dc1 --all
   ```

4. **Deploy DC2 Thanos Store Gateway**
   ```yaml
   # Add to existing Thanos deployment
   kubectl apply -f thanos-store-dc2.yaml
   ```

5. **Update Thanos Query Endpoints**
   ```bash
   kubectl patch deployment thanos-query -n helios-storage \
     --type=json \
     -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--endpoint=dnssrv+_grpc._tcp.thanos-store-dc2.helios-storage.svc"}]'
   ```

6. **Validate**
   ```bash
   # Query should return data from both DCs
   curl -s "http://thanos-query:9090/api/v1/query?query=up" | jq '.data.result | length'
   ```

### On-Prem MinIO to Cloud S3

**Steps**:

1. **Create S3 Bucket**
   ```bash
   aws s3 mb s3://helios-metrics-prod --region us-east-1
   ```

2. **Configure MinIO to Tier to S3**
   ```bash
   mc ilm tier add minio S3 CLOUD-TIER \
     --endpoint https://s3.us-east-1.amazonaws.com \
     --access-key $AWS_ACCESS_KEY \
     --secret-key $AWS_SECRET_KEY \
     --bucket helios-metrics-prod
   ```

3. **Migrate Historical Data**
   ```bash
   # Use mc mirror for bulk migration
   mc mirror minio/helios-metrics s3/helios-metrics-prod \
     --older-than 14d \
     --remove
   ```

4. **Add Cloud Store Gateway**
   ```bash
   kubectl apply -f thanos-store-cloud.yaml
   ```

5. **Transition to Cloud-Only (Optional)**
   ```bash
   # After all data migrated
   # Update Sidecar objstore to write to S3
   # Remove MinIO cluster
   ```

### MinIO to S3 (Full Migration)

**Steps**:

1. **Parallel Write Period**
   - Configure Thanos Sidecars to write to both MinIO and S3
   - Use Thanos Receive as intermediary

   ```yaml
   # Thanos Receive configuration
   apiVersion: apps/v1
   kind: StatefulSet
   metadata:
     name: thanos-receive
   spec:
     template:
       spec:
         containers:
         - name: receive
           args:
             - receive
             - --objstore.config-file=/etc/thanos/objstore-minio.yaml
             - --objstore.config-file=/etc/thanos/objstore-s3.yaml
   ```

2. **Migrate Historical Data**
   ```bash
   mc mirror --preserve minio/helios-metrics s3/helios-metrics-prod
   ```

3. **Switch Sidecars to S3**
   ```bash
   kubectl rollout restart statefulset prometheus -n helios-storage
   ```

4. **Validate**
   ```bash
   # Verify S3 has all expected blocks
   thanos tools bucket inspect --objstore.config-file=objstore-s3.yaml
   ```

5. **Decommission MinIO**
   ```bash
   # After validation period (e.g., 7 days)
   helm uninstall minio -n helios-storage
   ```

---

## Decision Matrix

### Topology Selection Criteria

| Criterion | Weight | A: Single-DC | B: Multi-DC | C: Cloud | D: Hybrid |
|-----------|--------|--------------|-------------|----------|-----------|
| **Simplicity** | 20% | 5 | 2 | 4 | 3 |
| **Cost (small)** | 15% | 3 | 2 | 5 | 3 |
| **Cost (large)** | 15% | 4 | 3 | 2 | 4 |
| **DR Capability** | 20% | 2 | 5 | 4 | 4 |
| **Performance** | 15% | 4 | 4 | 3 | 4 |
| **Scalability** | 15% | 3 | 4 | 5 | 4 |

**Scores** (1-5, higher is better):

| Topology | Weighted Score | Best For |
|----------|----------------|----------|
| A: Single-DC MinIO | 3.35 | Dev/staging, simple production |
| B: Multi-DC MinIO | 3.50 | Enterprise, compliance-heavy |
| C: Cloud Object Storage | 3.65 | Cloud-native, simplicity priority |
| D: Hybrid | 3.65 | Cost optimization, mixed workloads |

### Quick Decision Guide

```
START
  |
  v
Do you need multi-DC DR?
  |
  +-- YES --> Do you have strict data locality requirements?
  |             |
  |             +-- YES --> Topology B (Multi-DC MinIO)
  |             |
  |             +-- NO --> Is cloud egress cost acceptable?
  |                          |
  |                          +-- YES --> Topology C (Cloud)
  |                          |
  |                          +-- NO --> Topology D (Hybrid)
  |
  +-- NO --> Is this production?
              |
              +-- YES --> Do you have cloud infrastructure?
              |             |
              |             +-- YES --> Topology C (Cloud)
              |             |
              |             +-- NO --> Topology A (Single-DC MinIO)
              |
              +-- NO --> Topology A (Single-DC MinIO)
```

---

## Related Documents

- [[architecture]] - Main Helios architecture document
- [[../decisions/adr-001-storage-backend]] - Storage backend selection ADR
- [[../decisions/adr-002-gnmic-deployment]] - gnmic deployment model ADR
- [[../overview]] - Helios project overview

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-09 | Architecture Review | Initial storage topologies design |
