# Data Model: Helios Network Observability Platform

**Branch**: `001-helios-platform` | **Date**: 2026-02-07

## Entities

### Device (NetBox — source of truth)

Represents a network device managed in NetBox. Not stored by Helios directly;
queried via the NetBox REST API by the Target Generator.

| Field | Type | Source | Description |
|-------|------|--------|-------------|
| name | string | NetBox `name` | Unique device hostname |
| management_ip | string | NetBox `primary_ip4` | Management IPv4 address |
| platform | string | NetBox `platform.slug` | OS platform (eos, iosxe, junos, panos, etc.) |
| vendor | string | NetBox `manufacturer.slug` | Hardware vendor |
| site | string | NetBox `site.slug` | Physical site code |
| region | string | NetBox `site.region.slug` | Geographic region |
| role | string | NetBox `role.slug` | Device role (core-router, access-switch, firewall, etc.) |
| tier | string | NetBox CF `monitoring_tier` | Monitoring tier: critical, standard, best-effort |
| gnmi_enabled | bool | NetBox CF `gnmi_enabled` | Whether gNMI collection is active |
| gnmi_port | int | NetBox CF `gnmi_port` | gNMI port (default: 6030) |
| snmp_enabled | bool | NetBox CF `snmp_enabled` | Whether SNMP collection is active |
| snmp_version | string | NetBox CF `snmp_version` | SNMP version: v2c, v3 |
| snmp_module | string | NetBox CF `snmp_module` | SNMP exporter module name |
| telemetry_profile | string | NetBox CF `telemetry_profile` | Subscription depth: minimal, default, detailed, custom |
| blackbox_probes | []string | NetBox CF `blackbox_probes` | Probe types: icmp, tcp, http, dns |
| helios_monitor | bool | NetBox CF `helios_monitor` | Master monitoring toggle |

**Relationships**:
- Device → Site (belongs to)
- Device → Platform (has one)
- Device → Role (has one)
- Device → Interface[] (has many)

### Interface (NetBox — enrichment source)

| Field | Type | Source | Description |
|-------|------|--------|-------------|
| name | string | NetBox `name` | Interface name (e.g., Ethernet1/1) |
| snmp_index | int | NetBox CF `snmp_index` | SNMP interface index for metric correlation |
| speed | int | NetBox `speed` | Interface speed in Kbps |
| enabled | bool | NetBox `enabled` | Admin state |
| device | string | NetBox `device.name` | Parent device |

### Metric (Prometheus — TSDB)

Time-series data point. Not modeled as a traditional entity; defined by
Prometheus's dimensional data model (metric name + label set + timestamp + value).

**Standard label taxonomy** (applied across all metric sources):

| Label | Source | Example |
|-------|--------|---------|
| `device` | Target Generator | `core-rtr-01` |
| `site` | Target Generator | `dc1` |
| `region` | Target Generator | `us-east` |
| `vendor` | Target Generator | `arista` |
| `platform` | Target Generator | `eos` |
| `role` | Target Generator | `core-router` |
| `tier` | Target Generator | `critical` |
| `interface` | gnmic/SNMP | `Ethernet1/1` |
| `job` | Prometheus | `gnmic`, `snmp`, `blackbox` |
| `instance` | Prometheus | `10.0.1.1:6030` |

**Key metric families**:

| Metric | Source | Type | Description |
|--------|--------|------|-------------|
| `gnmic_interfaces_interface_state_counters_*` | gnmic | counter | Interface byte/packet counters |
| `gnmic_interfaces_interface_state_oper_status` | gnmic | gauge | Interface oper status (1=UP, 2=DOWN) |
| `gnmic_system_state_*` | gnmic | gauge | CPU, memory, uptime |
| `gnmic_network_instances_protocol_bgp_neighbors_*` | gnmic | gauge | BGP neighbor state |
| `ifHCInOctets` / `ifHCOutOctets` | SNMP | counter | Interface octets (64-bit) |
| `sysUpTime` | SNMP | gauge | System uptime |
| `probe_success` | blackbox | gauge | Probe success (1=up, 0=down) |
| `probe_duration_seconds` | blackbox | gauge | Probe RTT |

### Flow Record (ClickHouse — flows_raw)

A single network flow record after enrichment.

| Column | Type | Description |
|--------|------|-------------|
| timestamp | DateTime64(3) | Flow observation time |
| flow_type | Enum8 | netflow_v5, netflow_v9, ipfix, sflow |
| exporter_ip | IPv4 | Device management IP |
| exporter_name | LowCardinality(String) | Device name (from NetBox) |
| exporter_site | LowCardinality(String) | Site code (from NetBox) |
| exporter_region | LowCardinality(String) | Region (from NetBox) |
| exporter_role | LowCardinality(String) | Device role (from NetBox) |
| in_if / out_if | UInt32 | SNMP interface index |
| in_if_name / out_if_name | LowCardinality(String) | Interface name (from NetBox) |
| in_if_speed / out_if_speed | UInt64 | Interface speed |
| src_ip / dst_ip | IPv6 | Source/destination IP (IPv4-mapped IPv6) |
| ip_version | UInt8 | 4 or 6 |
| protocol | UInt8 | IP protocol number |
| src_port / dst_port | UInt16 | L4 ports |
| tcp_flags | UInt8 | TCP flags bitmask |
| bytes | UInt64 | Flow byte count |
| packets | UInt64 | Flow packet count |
| sampling_rate | UInt32 | Sampler rate (1:N) |
| src_as / dst_as | UInt32 | BGP AS numbers |
| next_hop | IPv4 | BGP next-hop |
| src_country / dst_country | LowCardinality(String) | GeoIP country code |
| src_city / dst_city | LowCardinality(String) | GeoIP city |
| src_as_name / dst_as_name | LowCardinality(String) | AS organization name |
| src_vlan / dst_vlan | UInt16 | VLAN IDs |
| direction | Enum8 | ingress, egress, unknown |

**Partition**: `toYYYYMMDD(timestamp)`
**Order**: `(exporter_site, exporter_name, timestamp, src_ip, dst_ip)`
**TTL**: `timestamp + INTERVAL 7 DAY DELETE`
**Engine**: `ReplicatedMergeTree`

### Flow Aggregate 1-Minute (ClickHouse — flows_1m)

| Column | Type | Description |
|--------|------|-------------|
| timestamp | DateTime | Truncated to minute |
| exporter_site | LowCardinality(String) | Site code |
| exporter_name | LowCardinality(String) | Device name |
| in_if_name / out_if_name | LowCardinality(String) | Interface names |
| src_ip / dst_ip | IPv6 | IP addresses |
| protocol | UInt8 | IP protocol |
| src_port / dst_port | UInt16 | L4 ports |
| src_as / dst_as | UInt32 | AS numbers |
| src_country / dst_country | LowCardinality(String) | GeoIP countries |
| bytes_sum | AggregateFunction(sum, UInt64) | Total bytes |
| packets_sum | AggregateFunction(sum, UInt64) | Total packets |
| flow_count | AggregateFunction(count, UInt64) | Number of flows |

**TTL**: `timestamp + INTERVAL 30 DAY DELETE`
**Engine**: `ReplicatedAggregatingMergeTree`

### Flow Aggregate 1-Hour (ClickHouse — flows_1h)

Same structure as flows_1m but with:
- `timestamp` truncated to hour
- `src_network / dst_network` (aggregated to /24 IPv4 or /48 IPv6) instead of full IPs
- No port-level granularity (only protocol + dst_port)
- **TTL**: `timestamp + INTERVAL 90 DAY DELETE`

### Runbook (Kubernetes CRD — helios.io/v1alpha1)

| Field | Type | Description |
|-------|------|-------------|
| name | string | Human-readable name |
| description | string | What the runbook does |
| category | enum | interface, bgp, system, security, diagnostic, custom |
| riskLevel | enum | low, medium, high, critical |
| requiresApproval | bool | Whether human approval is needed |
| approvers | []Approver | Who can approve (type: user/group, name) |
| approvalTimeout | duration | Max wait for approval (default: 1h) |
| allowedRoles | []string | RBAC groups that can execute |
| cooldown | duration | Min time between executions on same target |
| parameters | []Parameter | Input parameter definitions |
| steps | []Step | Ordered execution steps |
| rollback | []Step | Steps to run on failure |

**State transitions**: N/A (Runbook is a template, not stateful)

### RunbookExecution (Kubernetes CRD — helios.io/v1alpha1)

| Field | Type | Description |
|-------|------|-------------|
| runbookRef | ObjectRef | Reference to Runbook (name, namespace) |
| parameters | map[string]any | Resolved parameter values |
| triggeredBy | string | User identity (email) |
| triggerSource | enum | manual, alert, scheduled, api |
| dryRun | bool | Simulate without applying changes |

**State machine**:

```
Pending → PendingApproval → Approved → Running → Completed
                                          ↓
                                        Failed → RollingBack → RolledBack
                                          ↓
                                       Cancelled
```

| State | Description |
|-------|-------------|
| Pending | Created, waiting for controller to process |
| PendingApproval | Requires approval; notification sent |
| Approved | Approval received; queued for execution |
| Running | Executor Job active; steps executing |
| Completed | All steps succeeded |
| Failed | A step failed (before rollback) |
| RollingBack | Rollback steps executing |
| RolledBack | Rollback complete |
| Cancelled | Manually cancelled or approval timed out |

### Target ConfigMap (Kubernetes — generated)

Generated by Target Generator. Not a persistent entity; regenerated every sync
cycle.

**gnmic targets format**:
```yaml
targets:
  core-rtr-01:6030:
    address: 10.0.1.1:6030
    labels:
      device: core-rtr-01
      site: dc1
      region: us-east
      vendor: arista
      platform: eos
      role: core-router
      tier: critical
```

**Prometheus file-based SD format**:
```json
[
  {
    "targets": ["10.0.1.1:161"],
    "labels": {
      "device": "core-rtr-01",
      "site": "dc1",
      "region": "us-east",
      "__param_module": "arista_eos"
    }
  }
]
```

## Entity Relationships

```
NetBox Device ──────┬──→ gnmic Target ConfigMap ──→ gnmic StatefulSet ──→ Prometheus
                    ├──→ SNMP Target ConfigMap ──→ snmp_exporter ──→ Prometheus
                    ├──→ Blackbox Target ConfigMap ──→ blackbox_exporter ──→ Prometheus
                    └──→ Flow Enricher Cache (device metadata lookup)

Prometheus ──→ Thanos Sidecar ──→ Object Storage (MinIO/S3)
                                        ↓
                              Thanos Store Gateway
                                        ↓
                                  Thanos Query ──→ Grafana

goflow2 ──→ Kafka (raw) ──→ Flow Enricher ──→ Kafka (enriched) ──→ ClickHouse
                                                                        ↓
                                                                    Grafana

Runbook CRD ←── kubectl apply ── User/Alert
     ↓
RunbookExecution CRD ──→ Runbook Operator ──→ K8s Job (Executor)
                                                    ↓
                                              gNMI Set/Get ──→ Device
```
