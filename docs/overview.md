#project #status/design #observability #network #kubernetes

# Project: Helios

*Illuminate your network*

**Status**: Design Complete
**Priority**: Medium
**Start Date**: 2026-01-09
**Last Updated**: 2026-01-09
**Repository**: TBD
**Local**: TBD

## Overview

Helios is a Kubernetes-native network observability stack designed to provide comprehensive visibility into network infrastructure. It combines streaming telemetry, traditional polling, flow analysis, and modern visualization into a unified platform.

> [!NOTE] Project Phase
> Architecture design complete (metrics + flows). Ready for implementation.

## Goals & Objectives

### Primary Goals
- Build a K8s-native network observability platform
- Unified collection of gNMI streaming telemetry and SNMP metrics
- Flow data collection (NetFlow v5/v9, IPFIX, sFlow) for traffic analysis
- Highly available metrics and flow storage with long-term retention
- Rich visualization and alerting capabilities

### Success Metrics
- Single deployment for complete network visibility
- Sub-minute telemetry for critical metrics
- 90+ days metric retention
- Flow data for traffic forensics and Top-N analysis
- Zero vendor lock-in (fully open source)

## Architecture Overview

```
                                    VISUALIZATION
    +------------------------------------------------------------------+
    |     Grafana      |     Alertmanager    |     Runbook UI          |
    +------------------------------------------------------------------+
                          |                     |
    +---------------------+---------------------+----------------------+
    |      METRICS QUERY              |         FLOW QUERY             |
    |  Thanos Query + Frontend        |      ClickHouse SQL            |
    +---------------------+---------------------+----------------------+
                          |                     |
    +---------------------+---------------------+----------------------+
    |      METRICS STORAGE            |         FLOW STORAGE           |
    |  Thanos Store + Object Storage  |    ClickHouse (tiered TTL)     |
    +---------------------+---------------------+----------------------+
                          |                     |
    +---------------------+---------------------+----------------------+
    |    METRICS COLLECTION           |       FLOW COLLECTION          |
    |  Prometheus --> Thanos Sidecar  | goflow2 --> Kafka --> Enricher |
    +---------------------+---------------------+----------------------+
                          |                     |
    +---------------------+---------------------+----------------------+
    |                       EXPORTER LAYER                             |
    | gnmic (gNMI) | SNMP Exporter | Blackbox | goflow2 (NF/sFlow)     |
    +------------------------------------------------------------------+
                                      |
    +------------------------------------------------------------------+
    |                     INTEGRATION LAYER                            |
    |        NetBox (Source of Truth)  -->  Target Generator           |
    +------------------------------------------------------------------+
                                      |
    +------------------------------------------------------------------+
    |                       NETWORK LAYER                              |
    |   Arista | Cisco | Juniper | Palo Alto | Fortigate | Others      |
    +------------------------------------------------------------------+
```

See [[architecture/architecture]] for metrics design and [[architecture/flow-collection]] for flow design.

## Key Decisions

| Decision | Choice | ADR |
|----------|--------|-----|
| Metrics Storage | Thanos + Object Storage | [[decisions/adr-001-storage-backend]] |
| gnmic Deployment | StatefulSet with K8s Clustering | [[decisions/adr-002-gnmic-deployment]] |
| Flow Collector | goflow2 | [[architecture/flow-collection#adr-008]] |
| Flow Storage | ClickHouse | [[architecture/flow-collection#adr-009]] |
| Flow Message Queue | Kafka (Strimzi) | [[architecture/flow-collection#adr-010]] |
| Target Discovery | NetBox + CronJob Sync | Pending |
| Runbook Automation | Custom K8s Operator | Pending |
| Multi-DC Architecture | Prometheus per-DC + Thanos | Pending |

## Scale Support

### Metrics Scale

| Scale | Devices | gnmic Pods | Prometheus Shards | Storage (90d) |
|-------|---------|------------|-------------------|---------------|
| Small | <50 | 2 | 1 | ~80 GB |
| Medium | 50-500 | 3-5 | 2 | ~800 GB |
| Large | 500-2000 | 5-10 | 4 | ~3.2 TB |
| XL | 2000-8000 | 10-20 | 8 | ~12.8 TB |

### Flow Scale

| Scale | Devices | FPS | goflow2 Pods | ClickHouse Shards | Storage (90d) |
|-------|---------|-----|--------------|-------------------|---------------|
| Small | <50 | 5K | 2 | 1 | ~120 GB |
| Medium | 50-500 | 50K | 3 | 2 | ~1.2 TB |
| Large | 500-2000 | 500K | 5 | 4 | ~12 TB |
| XL | 2000-8000 | 5M | 10 | 8 | ~120 TB |

## Components

### Metrics Collection Layer

| Component | Purpose | Notes |
|-----------|---------|-------|
| **gnmic** | gNMI/OpenConfig streaming telemetry | StatefulSet with clustering |
| **snmp_exporter** | SNMP polling | Legacy device support |
| **blackbox_exporter** | Synthetic probes | Reachability, latency |
| **node_exporter** | Host metrics | For K8s nodes |
| **script_exporter** | Custom scripts | Extensibility |

### Flow Collection Layer

| Component | Purpose | Notes |
|-----------|---------|-------|
| **goflow2** | NetFlow/IPFIX/sFlow collection | StatefulSet, UDP ingress |
| **Kafka (Strimzi)** | Flow message queue | Decouples collection from storage |
| **Flow Enricher** | NetBox + GeoIP enrichment | Custom Go service |
| **ClickHouse** | Flow storage and analytics | Tiered retention |

### Metrics Storage Layer

| Component | Purpose | Notes |
|-----------|---------|-------|
| **Prometheus** | Short-term metrics storage | Sharded with Thanos sidecar |
| **Thanos** | HA + long-term storage | Full stack deployment |
| **MinIO/S3** | Object storage | 90-day retention |

### Visualization Layer

| Component | Purpose | Notes |
|-----------|---------|-------|
| **Grafana** | Dashboards and visualization | Metrics + Flow dashboards |
| **Alertmanager** | Alert routing and grouping | PagerDuty, Slack integration |

### Automation Layer

| Component | Purpose | Notes |
|-----------|---------|-------|
| **Target Generator** | NetBox sync | CronJob-based |
| **Runbook Operator** | Automated remediation | Custom K8s operator |

## Implementation Phases

### Phase 1: Metrics Foundation (Weeks 1-4)
- [ ] Deploy Prometheus Operator + Thanos
- [ ] Deploy MinIO for object storage
- [ ] Deploy Grafana + Alertmanager
- [ ] Create namespaces and RBAC
- [ ] Basic dashboards and alerts

### Phase 2: Metrics Collection (Weeks 5-8)
- [ ] Deploy gnmic with basic subscriptions
- [ ] Deploy SNMP and Blackbox exporters
- [ ] Integrate with NetBox (manual targets initially)
- [ ] Create device-specific dashboards

### Phase 3: Automation (Weeks 9-12)
- [ ] Deploy Target Generator CronJob
- [ ] Create CI/CD pipeline for configs
- [ ] Deploy Runbook Operator
- [ ] Create initial runbook library

### Phase 4: Flow Collection (Weeks 13-18) - v1.x
- [ ] Deploy Strimzi Kafka cluster
- [ ] Deploy ClickHouse cluster
- [ ] Deploy goflow2 collectors
- [ ] Deploy Flow Enricher with NetBox integration
- [ ] Create flow dashboards (Top Talkers, Traffic Matrix, AS Paths)

### Phase 5: Scale & Optimize (Weeks 19-22)
- [ ] Implement sharding based on load
- [ ] Add Query Frontend with caching
- [ ] Tune retention and compaction
- [ ] Performance testing and optimization

## Flow Dashboards (v1.x)

| Dashboard | Purpose |
|-----------|---------|
| **Top Talkers** | Highest traffic sources/destinations by bytes/packets |
| **Traffic Matrix** | Site-to-site and device-to-device flow visualization |
| **AS Path Analysis** | Traffic breakdown by source/destination ASN |
| **Protocol Distribution** | TCP/UDP port analysis, protocol breakdown |
| **Geographic Flows** | Traffic by country, geo-mapping |
| **Interface Correlation** | Combined metrics + flow view per interface |

## Future Considerations

- **Log aggregation** - Loki for syslog/device logs
- **Topology visualization** - Network maps from discovery
- **Containerlab integration** - Test topologies for development
- **DDoS detection** - Flow-based anomaly detection

## Hardware Target

Planned deployment on future cluster:
- 3x Miniforum MS-01 (control plane + heavy workloads)
- 5x Raspberry Pi 5 8GB (distributed workers)

See [[../k8s-ansible/overview]] for cluster details.

## Related

- [[architecture/architecture]] - Full metrics architecture document
- [[architecture/flow-collection]] - Flow collection architecture (v1.x)
- [[decisions/adr-001-storage-backend]] - Storage backend ADR
- [[decisions/adr-002-gnmic-deployment]] - gnmic deployment ADR
- [[../k8s-ansible/overview]] - Cluster provisioning
- [[../apps/overview]] - ArgoCD application deployments

## Resources

- [gnmic documentation](https://gnmic.openconfig.net/)
- [Thanos documentation](https://thanos.io/)
- [Prometheus Operator](https://prometheus-operator.dev/)
- [Grafana documentation](https://grafana.com/docs/)
- [goflow2 GitHub](https://github.com/netsampler/goflow2)
- [ClickHouse documentation](https://clickhouse.com/docs)
- [Strimzi Kafka](https://strimzi.io/)

---

**Tasks**:
- [[tasks/task-001-architecture-design]] - Design detailed architecture (completed)
- Flow collection architecture design (completed) - [[architecture/flow-collection]]
