# Feature Specification: Helios Network Observability Platform

**Feature Branch**: `001-helios-platform`
**Created**: 2026-02-07
**Status**: Draft
**Input**: User description: "Build the complete Helios network observability platform"

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Metrics Collection & Storage (Priority: P1)

A network engineer deploys Helios and gets streaming telemetry (gNMI) and SNMP
metrics from network devices, stored in Prometheus with Thanos for 90-day
retention on object storage.

**Why this priority**: Without metrics ingestion and storage the platform has no
data. Every other story (dashboards, alerts, flows, automation) depends on a
functioning metrics pipeline. This is the foundational MVP.

**Independent Test**: Deploy the Helm umbrella chart on a kind cluster with
Containerlab virtual devices exporting gNMI and SNMP data. Verify metrics appear
in Prometheus within 30 seconds and are queryable through Thanos Query after
upload to MinIO.

**Acceptance Scenarios**:

1. **Given** a Kubernetes cluster with the Helios Helm chart installed at Medium
   scale defaults, **When** a gnmic StatefulSet pod starts with a valid target
   list, **Then** gNMI streaming telemetry subscriptions are established and
   metrics (interface counters, BGP state, hardware health) are written to
   Prometheus within 30 seconds of device connection.

2. **Given** SNMP-capable devices listed in the target ConfigMap, **When**
   snmp_exporter scrapes targets on the configured interval, **Then** SNMP
   metrics (sysUpTime, ifHCInOctets, ifHCOutOctets, CPU, memory) are ingested
   into Prometheus with correct labels (device, site, region, vendor, platform,
   role, tier).

3. **Given** Prometheus shards running with Thanos Sidecar, **When** metrics
   blocks complete a 2-hour TSDB block, **Then** blocks are uploaded to the
   configured object storage bucket (MinIO or S3) and become queryable through
   Thanos Query within 10 minutes.

4. **Given** Thanos Compactor running as a singleton, **When** uploaded blocks
   exceed the downsampling threshold, **Then** 5-minute and 1-hour downsampled
   blocks are created and raw blocks older than 14 days are marked for deletion
   per the retention policy.

5. **Given** a blackbox_exporter deployment, **When** synthetic probe targets
   (ICMP, TCP, HTTP) are configured, **Then** latency, packet loss, and
   reachability metrics are collected and stored alongside device telemetry.

---

### User Story 2 — Visualization & Alerting (Priority: P2)

A network engineer views pre-built Grafana dashboards covering interface
utilization, BGP state, and hardware health, and receives alerts via
Alertmanager with tiered routing to PagerDuty, Slack, and email.

**Why this priority**: Metrics without visualization and alerting provide no
operational value. Dashboards and alerts are the primary interface through which
engineers interact with the platform.

**Independent Test**: With the metrics pipeline from US1 running, open Grafana
and confirm all 8 core dashboards render with real data. Trigger a simulated
device-down condition and verify an alert fires and routes to the correct
receiver.

**Acceptance Scenarios**:

1. **Given** Grafana deployed with provisioned datasources (Thanos Query,
   Alertmanager), **When** the Helios chart is installed, **Then** 8 core
   dashboards are automatically provisioned: Network Overview, Interface
   Analytics, Device Health, Site Overview, Latency & Reachability, BGP Status,
   Top Talkers (metrics-based), and Alerts Overview.

2. **Given** the Network Overview dashboard is loaded, **When** the user selects
   a site and time range, **Then** the dashboard displays aggregated interface
   utilization, device availability, top-N error interfaces, and BGP session
   counts with query response times under 5 seconds for a 24-hour range.

3. **Given** alert rules deployed via ConfigMap (device down, interface errors
   >1% threshold, BGP session loss, CPU >80%, memory >85%), **When** a
   threshold is breached for the configured duration, **Then** Alertmanager
   fires the alert with correct severity label and routes to the appropriate
   receiver (critical → PagerDuty, warning → Slack, info → email).

4. **Given** Grafana configured with OIDC authentication, **When** users with
   different RBAC roles (helios-viewer, helios-engineer, helios-admin) log in,
   **Then** dashboard access and annotation permissions match their role tier.

---

### User Story 3 — Flow Collection & Analysis (Priority: P3)

A network engineer collects NetFlow/IPFIX/sFlow from network devices, enriched
with NetBox metadata and GeoIP data, stored in ClickHouse with tiered retention.
Dedicated Grafana dashboards provide Top Talkers, Traffic Matrix, AS Path, Protocol
Distribution, Geographic Flow, and Interface Correlation views.

**Why this priority**: Flow data provides traffic-level visibility that
metrics alone cannot. It enables capacity planning, security forensics, and
traffic engineering — but requires the metrics pipeline (US1) and dashboards
(US2) as a foundation.

**Independent Test**: Deploy the flow pipeline (goflow2, Kafka, Flow Enricher,
ClickHouse) alongside the metrics stack. Send synthetic NetFlow v9 data from a
Containerlab topology. Verify enriched records appear in ClickHouse and the Top
Talkers dashboard renders within 60 seconds.

**Acceptance Scenarios**:

1. **Given** goflow2 deployed with UDP listeners on ports 2055 (NetFlow) and
   6343 (sFlow), **When** network devices send flow records, **Then** goflow2
   ingests flows and publishes protobuf-encoded messages to the Kafka
   `helios-flows-raw` topic at the configured scale tier rate (5K–5M FPS).

2. **Given** the Flow Enricher consuming from Kafka, **When** a raw flow record
   is received, **Then** it is enriched with NetBox metadata (device name, site,
   role, tenant) and GeoIP data (country, city, ASN, AS name) for both source
   and destination IPs, and published to the `helios-flows-enriched` topic.

3. **Given** ClickHouse with the flows schema deployed (flows_raw MergeTree,
   flows_1m AggregatingMergeTree, flows_1h AggregatingMergeTree), **When**
   enriched flow records are consumed from Kafka via the Kafka engine table,
   **Then** raw flows are inserted and materialized views populate 1-minute
   and 1-hour aggregate tables automatically.

4. **Given** ClickHouse TTL policies configured, **When** data ages past the
   retention threshold, **Then** raw flows older than 7 days, 1-minute
   aggregates older than 30 days, and 1-hour aggregates older than 90 days
   are automatically purged.

5. **Given** Grafana with the ClickHouse datasource configured, **When** the
   user opens a flow dashboard, **Then** 6 flow dashboards are available:
   Top Talkers, Traffic Matrix, AS Paths, Protocol Distribution, Geographic
   Flows, and Interface Correlation (cross-datasource combining Thanos
   metrics and ClickHouse flow data).

---

### User Story 4 — Device Discovery & Integration (Priority: P4)

A network engineer configures NetBox as the source of truth for device inventory.
The Target Generator automatically syncs device inventory to gnmic, SNMP
exporter, and goflow2 target lists via ConfigMaps and Prometheus service
discovery files.

**Why this priority**: Manual target management does not scale beyond a handful
of devices. NetBox integration enables the platform to manage hundreds to
thousands of devices with zero manual target configuration — but requires the
collection layer (US1) to be functional first.

**Independent Test**: Add a device to NetBox with the `helios_monitor: true`
custom field. Wait for the Target Generator CronJob to run (≤5 minutes). Verify
the device appears in gnmic targets and metrics begin flowing without manual
intervention.

**Acceptance Scenarios**:

1. **Given** NetBox populated with network devices tagged with custom field
   `helios_monitor: true` and attributes (platform, site, region, role, tier,
   management IP, gNMI port, SNMP community), **When** the Target Generator
   CronJob executes, **Then** it queries the NetBox API, generates target
   ConfigMaps for gnmic, SNMP exporter, and blackbox exporter, and writes
   Prometheus file-based service discovery JSON.

2. **Given** a newly generated target ConfigMap, **When** the ConfigMap is
   updated in Kubernetes, **Then** gnmic detects the configuration change and
   establishes gNMI subscriptions to new targets without pod restart within 5
   minutes of the NetBox change.

3. **Given** a device is removed from NetBox or its `helios_monitor` field is
   set to false, **When** the Target Generator runs, **Then** the device is
   removed from all target lists and stale metrics are no longer scraped.

4. **Given** the Target Generator encounters a NetBox API error or timeout,
   **When** the sync fails, **Then** the existing target ConfigMaps are
   preserved (no empty-write), a Prometheus metric
   `helios_target_sync_errors_total` is incremented, and an alert fires if
   sync has not succeeded for >15 minutes.

---

### User Story 5 — Automated Remediation (Priority: P5)

A network engineer defines runbooks as Kubernetes CRDs. The Runbook Operator
executes gNMI Set/Get actions with approval workflows, audit trails, and
rollback capabilities.

**Why this priority**: Automation builds on top of all other layers — it
requires working metrics (US1) for trigger conditions, alerts (US2) for
notifications, and device integration (US4) for target resolution. It is the
highest-value differentiator but the last to deliver safely.

**Independent Test**: Apply a Runbook CRD for "interface bounce" to the cluster.
Create a RunbookExecution targeting a Containerlab device. Verify the operator
executes the gNMI Set (disable/enable interface), logs each step, and records
the execution in the audit trail.

**Acceptance Scenarios**:

1. **Given** the Runbook Operator deployed in the `helios-automation` namespace
   with its CRDs registered (Runbook, RunbookExecution), **When** a Runbook
   resource is applied with steps (gNMI Set, gNMI Get, wait, notify, condition,
   script), **Then** the operator validates the runbook schema, stores it, and
   reports status as Ready.

2. **Given** a valid Runbook exists, **When** a RunbookExecution is created
   targeting a device (resolved via label selector or explicit target), **Then**
   the operator spawns a Kubernetes Job that executes each step sequentially,
   recording step start/end times, outputs, and success/failure status.

3. **Given** a Runbook with `approvalRequired: true` and `riskLevel: high`,
   **When** a RunbookExecution is created, **Then** the execution enters
   `PendingApproval` state and does not proceed until an authorized user
   (helios-admin role) approves it via the API or CLI.

4. **Given** a running RunbookExecution where a step fails, **When** the
   runbook has `rollbackOnFailure: true` with defined rollback steps, **Then**
   the operator transitions to `RollingBack` state, executes rollback steps
   in reverse order, and reports final status as `RolledBack` with full audit
   trail.

5. **Given** completed RunbookExecutions, **When** querying the operator API or
   inspecting CRD status, **Then** full audit records are available including
   executor identity, approval chain, step-by-step outputs, timestamps, and
   device responses. Prometheus metrics track `helios_runbook_executions_total`,
   `helios_runbook_execution_duration_seconds`, and
   `helios_runbook_pending_approvals`.

---

### User Story 6 — Multi-Datacenter Federation (Priority: P6)

The platform supports multiple datacenters with per-DC Prometheus instances,
Thanos global query federation, and selectable storage topologies (Single-DC
MinIO, Multi-DC MinIO replication, Cloud S3, Hybrid).

**Why this priority**: Multi-DC is an architectural extension that builds on the
single-DC foundation from US1–US5. It adds operational complexity (Thanos Query
federation, Compactor coordination, cross-DC replication) and should only be
tackled once the single-DC stack is proven stable.

**Independent Test**: Deploy two kind clusters simulating two datacenters, each
running Helios at Small scale. Configure Thanos Query in a hub cluster to
federate across both. Verify cross-DC queries return merged results and storage
replication (if Multi-DC MinIO topology) keeps buckets synchronized.

**Acceptance Scenarios**:

1. **Given** two or more Kubernetes clusters each running a Helios metrics stack
   (Prometheus + Thanos Sidecar), **When** a central Thanos Query instance is
   configured with StoreAPI endpoints from each DC, **Then** queries spanning
   multiple DCs return merged, deduplicated results with correct
   `external_labels` identifying the source DC.

2. **Given** Multi-DC MinIO storage topology selected via Helm values, **When**
   site replication is configured between MinIO clusters, **Then** metrics
   blocks uploaded in DC-A are replicated to DC-B with eventual consistency
   (active-active, last-write-wins).

3. **Given** Thanos Compactor must run as a singleton across DCs, **When**
   the Multi-DC topology is active, **Then** exactly one Compactor instance
   runs (in the designated primary DC), preventing data corruption from
   concurrent compaction.

4. **Given** the Helm umbrella chart with storage topology selection, **When**
   the operator sets `global.storageTopology` to one of `single-dc-minio`,
   `multi-dc-minio`, `cloud-s3`, or `hybrid`, **Then** the chart renders the
   correct MinIO/S3 configuration, Thanos objstore config, bucket policies,
   and replication settings without requiring manual YAML editing.

---

### Edge Cases

- What happens when gnmic loses connection to a device mid-stream? gnmic MUST
  reconnect automatically with exponential backoff. Prometheus staleness markers
  MUST be applied to interrupted series.
- What happens when Kafka is temporarily unavailable? goflow2 MUST buffer
  locally (in-memory ring buffer) and resume publishing when Kafka recovers.
  The Flow Enricher MUST handle consumer group rebalancing gracefully.
- What happens when the NetBox API is unreachable during a Target Generator
  sync? The generator MUST preserve existing ConfigMaps and not write empty
  target lists. A `helios_target_sync_errors_total` metric MUST be incremented.
- What happens when ClickHouse runs out of disk? TTL-based deletion MUST
  continue operating. An alert MUST fire when disk usage exceeds 80%.
  ClickHouse MUST reject inserts rather than corrupt data.
- What happens when a runbook execution targets a device that has been removed
  from NetBox? The operator MUST fail the execution with a clear error status
  rather than silently skip it.
- What happens when Prometheus shards are rebalanced (shard count changed)?
  Thanos Query MUST handle overlapping series from old and new shards via
  deduplication. No manual intervention should be required.
- What happens when MinIO replication lags between DCs? Thanos Query MUST
  return results from whichever Store has the data available. Eventual
  consistency is acceptable; data loss is not.
- What happens when a Runbook with `approvalRequired: true` has no available
  approver? The execution MUST remain in `PendingApproval` indefinitely and
  an alert MUST fire if pending for >1 hour.

## Requirements *(mandatory)*

### Functional Requirements

**Metrics Pipeline**

- **FR-001**: System MUST collect gNMI streaming telemetry via gnmic deployed as
  a Kubernetes StatefulSet with clustering support (leader election, target
  distribution).
- **FR-002**: System MUST collect SNMP metrics via Prometheus snmp_exporter with
  custom MIB modules loaded from ConfigMaps.
- **FR-003**: System MUST collect synthetic probe metrics (ICMP, TCP, HTTP) via
  blackbox_exporter.
- **FR-004**: System MUST store metrics in sharded Prometheus instances managed
  by the Prometheus Operator, with shard count configurable via Helm values.
- **FR-005**: System MUST upload metrics to object storage via Thanos Sidecar and
  support querying across shards and time ranges via Thanos Query.
- **FR-006**: System MUST downsample metrics to 5-minute and 1-hour resolution
  via Thanos Compactor, with configurable retention per resolution tier.

**Flow Pipeline**

- **FR-007**: System MUST collect NetFlow v5/v9, IPFIX, and sFlow via goflow2
  with UDP listeners on configurable ports.
- **FR-008**: System MUST decouple flow collection from storage via Kafka
  (Strimzi operator), with topic partitioning aligned to scale tier.
- **FR-009**: System MUST enrich flow records with NetBox device metadata
  (device, site, role, tenant) and MaxMind GeoIP data (country, city, ASN)
  via a custom Go service (Flow Enricher).
- **FR-010**: System MUST store enriched flows in ClickHouse with automatic
  aggregation via materialized views (1-minute and 1-hour granularity).
- **FR-011**: System MUST enforce TTL-based retention in ClickHouse: 7 days raw,
  30 days 1-minute aggregates, 90 days 1-hour aggregates.

**Visualization & Alerting**

- **FR-012**: System MUST provision Grafana with datasources (Thanos Query,
  Alertmanager, ClickHouse) automatically via Helm-managed ConfigMaps.
- **FR-013**: System MUST deploy 8 core metrics dashboards and 6 flow dashboards
  as provisioned JSON, not manually created.
- **FR-014**: System MUST deploy Alertmanager with tiered routing: critical →
  PagerDuty, warning → Slack, info → email. Receiver configuration MUST be
  settable via Helm values.
- **FR-015**: System MUST include default alert rules for: device unreachable,
  interface error rate >1%, BGP session state change, CPU >80%, memory >85%,
  flow rate anomaly (>2 stddev from baseline).

**Device Integration**

- **FR-016**: System MUST sync device inventory from NetBox via a CronJob-based
  Target Generator running at a configurable interval (default: 5 minutes).
- **FR-017**: System MUST generate target ConfigMaps for gnmic, snmp_exporter,
  blackbox_exporter, and Prometheus file-based service discovery JSON from
  NetBox query results.
- **FR-018**: System MUST apply a consistent label taxonomy across all metrics
  and flow data: device, site, region, vendor, platform, role, tier.

**Automation**

- **FR-019**: System MUST define runbooks as Kubernetes CRDs with typed
  parameters, ordered steps (gNMI Set, gNMI Get, gNMI Subscribe, wait, notify,
  condition, script), and optional rollback steps.
- **FR-020**: System MUST execute runbooks via Kubernetes Jobs spawned by the
  Runbook Operator, with each execution tracked as a RunbookExecution CRD.
- **FR-021**: System MUST support approval workflows for high-risk runbooks,
  with executions blocked in `PendingApproval` state until authorized.
- **FR-022**: System MUST record full audit trails for all executions: executor
  identity, approval chain, step outputs, timestamps, device responses.
- **FR-023**: System MUST support automatic rollback on step failure when
  `rollbackOnFailure: true` is set on the runbook.

**Deployment & Operations**

- **FR-024**: System MUST be deployable via a single Helm umbrella chart with
  scale-tier values files (values-small.yaml through values-xl.yaml).
- **FR-025**: System MUST support GitOps delivery via ArgoCD ApplicationSets
  with one Application per Helios namespace.
- **FR-026**: System MUST produce multi-arch container images (amd64 + arm64)
  for all custom-built components (Flow Enricher, Runbook Operator, Target
  Generator).
- **FR-027**: System MUST deploy across 6 isolated namespaces:
  helios-integration, helios-collection, helios-storage, helios-visualization,
  helios-automation, helios-flows.
- **FR-028**: System MUST manage device credentials via External Secrets
  Operator (ESO) syncing from Vault or cloud secret managers. Secrets MUST
  NOT appear in Git or ConfigMaps.
- **FR-029**: System MUST enforce NetworkPolicies restricting collection
  namespace egress to management network CIDRs and specific ports (gNMI
  6030/57400, SNMP 161/UDP, flow UDP 2055/6343).

**Multi-Datacenter**

- **FR-030**: System MUST support Thanos Query federation across multiple DC
  Prometheus instances via StoreAPI endpoints.
- **FR-031**: System MUST support selectable storage topologies via Helm values:
  Single-DC MinIO, Multi-DC MinIO (site replication), Cloud S3, Hybrid.
- **FR-032**: System MUST ensure Thanos Compactor runs as a singleton across
  all DCs to prevent data corruption from concurrent compaction.

### Key Entities

- **Device**: A network device (router, switch, firewall, load balancer)
  managed in NetBox with attributes: name, management IP, platform, vendor,
  site, region, role, tier, gNMI port, SNMP community, monitoring flags
  (`helios_monitor`, `helios_gnmi`, `helios_snmp`, `helios_flow_source`).

- **Site**: A physical location in the NetBox hierarchy containing devices.
  Sites belong to regions. Used for dashboard filtering and alert grouping.

- **Metric**: A time-series data point collected via gNMI, SNMP, or synthetic
  probe. Stored in Prometheus with standard label taxonomy. Uploaded to object
  storage via Thanos for long-term retention.

- **Flow Record**: A network traffic record (NetFlow/IPFIX/sFlow) describing a
  communication between source and destination IPs. Enriched with NetBox and
  GeoIP metadata. Stored in ClickHouse with tiered aggregation.

- **Runbook**: A Kubernetes CRD defining a sequence of automation steps with
  typed parameters, risk level, approval requirements, and optional rollback.
  Stored as a cluster-scoped resource.

- **RunbookExecution**: A Kubernetes CRD representing a single execution of a
  Runbook against a target device. Tracks state machine transitions
  (Pending → PendingApproval → Approved → Running → Completed/Failed →
  RollingBack → RolledBack).

- **Target ConfigMap**: A Kubernetes ConfigMap generated by the Target Generator
  containing device connection details for gnmic, snmp_exporter, or
  blackbox_exporter.

- **Storage Topology**: A deployment configuration (Single-DC MinIO, Multi-DC
  MinIO, Cloud S3, Hybrid) governing how Thanos stores and replicates metrics
  blocks across object storage backends.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: `helm install helios ./charts/helios -f values-medium.yaml`
  produces a fully operational stack on a fresh Kubernetes cluster with no
  manual post-install steps required.
- **SC-002**: Metric ingestion-to-query latency is under 30 seconds measured
  from device emission to Thanos Query availability.
- **SC-003**: Flow ingestion sustains 500,000 FPS at Large scale tier (500–2000
  devices) without data loss, measured end-to-end from goflow2 UDP receive to
  ClickHouse insert.
- **SC-004**: All 14 Grafana dashboards (8 metrics + 6 flow) render correctly
  with query response times under 5 seconds for 24-hour time ranges.
- **SC-005**: Target Generator syncs NetBox inventory changes to collection
  targets within 5 minutes of the change.
- **SC-006**: Runbook execution overhead (operator processing time, excluding
  device RTT) is under 10 seconds per execution.
- **SC-007**: Thanos retains queryable metrics for 90 days with correct
  downsampling: full resolution for 14 days, 5-minute for 30 days, 1-hour for
  90 days.
- **SC-008**: ClickHouse TTL enforcement purges expired data automatically with
  no manual intervention: raw flows at 7 days, 1-minute aggregates at 30 days,
  1-hour aggregates at 90 days.
- **SC-009**: Custom Go services (Flow Enricher, Runbook Operator, Target
  Generator) maintain >70% test coverage as measured by `go test -cover`.
- **SC-010**: All Helm charts pass `helm lint` and `helm unittest` in CI.
- **SC-011**: Multi-arch container images (amd64 + arm64) build and run
  correctly on both Miniforum MS-01 (x86_64) and Raspberry Pi 5 (aarch64)
  nodes.
- **SC-012**: A quickstart guide enables first metrics visibility within 30
  minutes of deploying Helios on a cluster with at least one gNMI or
  SNMP-capable device.
