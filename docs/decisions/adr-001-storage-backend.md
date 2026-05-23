#adr #decision #storage #thanos

# ADR-001: Storage Backend Selection

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #storage

## Context

Helios requires a metrics storage backend that can:

1. **Scale**: Handle 25,000 to 4,000,000+ active time series
2. **Retain**: Store 90 days of metrics cost-effectively
3. **HA**: Survive node and datacenter failures
4. **Query**: Provide fast queries across the entire retention window
5. **Integrate**: Work well with Prometheus ecosystem (PromQL, Grafana)

The storage backend is the most critical architectural decision as it affects:
- Operational complexity
- Cost (compute, storage, network)
- Query performance
- Data durability

## Decision

We will use **Thanos** with object storage (MinIO for on-prem, S3 for cloud).

### Architecture

```
Prometheus (2h retention) --> Thanos Sidecar --> Object Storage
                                   |
                                   v
                            Thanos Store Gateway
                                   |
                                   v
                            Thanos Query (global view)
                                   |
                                   v
                            Thanos Compactor (downsampling)
```

### Key Components

| Component | Purpose | Deployment |
|-----------|---------|------------|
| Thanos Sidecar | Upload blocks to object storage | Per Prometheus |
| Thanos Store Gateway | Query historical data from object storage | 2+ replicas |
| Thanos Query | Federate queries across stores | 3+ replicas |
| Thanos Query Frontend | Query caching and splitting | 2+ replicas |
| Thanos Compactor | Downsampling and deduplication | 1 singleton |

### Object Storage

- **On-Premises**: MinIO (S3-compatible)
- **Cloud**: Native S3, GCS, or Azure Blob

## Consequences

### Positive

1. **Unlimited Retention**: Object storage provides cost-effective infinite retention
2. **Horizontal Scaling**: Add Store Gateways and Query nodes as needed
3. **High Availability**: Object storage provides 11 9s durability
4. **Global View**: Query across all Prometheus instances with deduplication
5. **Downsampling**: 5-minute and 1-hour resolution reduces storage for old data
6. **CNCF Project**: Active community, good documentation, Prometheus ecosystem native
7. **Incremental Migration**: Can add Thanos sidecars to existing Prometheus

### Negative

1. **Operational Complexity**: More components to deploy and monitor
2. **Query Latency**: Historical queries slower than local Prometheus
3. **Object Storage Dependency**: Requires reliable object storage
4. **Compactor Singleton**: Single point of failure (mitigated by object storage durability)

### Neutral

1. **Learning Curve**: Team needs to learn Thanos operations
2. **Resource Overhead**: Sidecar adds ~500Mi memory per Prometheus

## Alternatives Considered

### Option 1: VictoriaMetrics

**Description**: High-performance, cost-effective Prometheus-compatible TSDB

**Pros**:
- Lower resource usage (up to 10x less than Prometheus)
- Simpler architecture (single binary)
- Built-in clustering (vmcluster)
- MetricsQL superset of PromQL

**Cons**:
- Smaller community than Prometheus/Thanos
- Less mature ecosystem integration
- Custom protocol for replication

**Why Not Selected**: While VictoriaMetrics is excellent for pure performance, Thanos provides better integration with existing Prometheus deployments and the broader CNCF ecosystem. VictoriaMetrics is recommended as an alternative for organizations prioritizing operational simplicity.

### Option 2: Grafana Mimir

**Description**: Grafana's horizontally scalable Prometheus-compatible TSDB

**Pros**:
- Native Grafana integration
- Multi-tenancy built-in
- Backed by Grafana Labs

**Cons**:
- Newer project (forked from Cortex in 2022)
- More complex than Thanos
- Tighter coupling to Grafana ecosystem

**Why Not Selected**: Mimir is a strong option but relatively newer. Thanos has more production deployments and a longer track record. Consider Mimir for v2 if multi-tenancy becomes a requirement.

### Option 3: Prometheus Only (Federation)

**Description**: Use Prometheus federation with remote-write for centralization

**Pros**:
- Simplest architecture
- No additional components
- Well understood

**Cons**:
- No HA without external solutions
- Limited retention (local disk)
- Federation has scaling limits
- Higher storage costs (no object storage tiering)

**Why Not Selected**: Does not meet 90-day retention requirement without excessive local storage costs. No native HA.

### Option 4: Cortex

**Description**: CNCF-graduated horizontally scalable Prometheus

**Pros**:
- CNCF graduated project
- Multi-tenant
- Production proven at large scale

**Cons**:
- More complex than Thanos
- Less active development (Mimir fork)
- Requires more components

**Why Not Selected**: Cortex development has slowed since the Mimir fork. Thanos is simpler for single-tenant use cases.

## Implementation Notes

### Retention Configuration

```yaml
# Thanos Compactor retention
--retention.resolution-raw=14d     # Keep raw data 14 days
--retention.resolution-5m=30d      # Keep 5m samples 30 days
--retention.resolution-1h=90d      # Keep 1h samples 90 days
```

### Object Storage Sizing

| Scale | 90-Day Raw | After Compaction |
|-------|------------|------------------|
| Small (50 devices) | 360 GB | ~80 GB |
| Medium (500 devices) | 3.6 TB | ~800 GB |
| Large (2000 devices) | 14.4 TB | ~3.2 TB |
| XL (8000 devices) | 57.6 TB | ~12.8 TB |

## References

- [Thanos Documentation](https://thanos.io/tip/thanos/getting-started.md/)
- [Thanos vs VictoriaMetrics](https://docs.victoriametrics.com/FAQ.html#what-is-the-difference-between-victoriametrics-and-thanos)
- [Grafana Mimir](https://grafana.com/oss/mimir/)
- [[../architecture/architecture]] - Full architecture document
