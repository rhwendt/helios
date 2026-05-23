# Helios Spec-Kit Prompts

Reference documentation for driving spec-kit slash commands.
Run these from a Claude Code session opened in `~/git/helios/helios/`.

---

## Step 1: `/speckit.constitution`

```
Helios is a CNCF-native, Kubernetes-first network observability platform. Read
the full project documentation from my vaulty notes to derive the constitution:

Source files (read all of these):
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/overview.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/architecture.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/flow-collection.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/runbook-operator.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/storage-topologies.md

ADRs (read for decision context):
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-001-storage-backend.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-002-gnmic-deployment.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-003-target-discovery.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-004-runbook-automation.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-005-multi-datacenter.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-006-alerting-strategy.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/decisions/adr-007-dashboard-provisioning.md

Create 6 core principles based on the architecture's design principles:
1. Cloud-Native First — K8s-native with Helm, ArgoCD, GitOps. No bare-metal deps.
2. Horizontal Scalability — Independent scaling of collection/storage/query layers.
   4 scale tiers: Small (<50), Medium (50-500), Large (500-2000), XL (2000-8000+).
3. Vendor Agnostic — Standardized interfaces (gNMI, SNMP, NetFlow/IPFIX/sFlow).
   NetBox as single source of truth. Vendor logic in config only, never in code.
4. Extensibility via Git — MIBs, subscriptions, alerts, dashboards, runbooks added
   via Git commits. CI/CD validation. No manual Grafana UI changes.
5. Operational Simplicity — Pre-built dashboards, alerts, runbooks out of the box.
   Single Helm umbrella chart. Production-ready defaults for Medium scale.
6. Long-Term Retention with Tiered Storage — 90-day metrics via Thanos + MinIO/S3.
   Flow retention: raw 7d, 1m agg 30d, 1h agg 365d. Selectable storage topology.

Additional sections to include:
- "Architecture Constraints" — Technology stack (metrics pipeline, flow pipeline,
  visualization, alerting, automation, integration, deployment), namespace isolation
  (6 namespaces), security requirements (RBAC, ESO, network policies, mTLS),
  performance budgets (<30s metric latency, 500K fps at Large, <5s dashboard query)
- "Development Workflow" — Code quality (>70% coverage for Go services, helm lint,
  golangci-lint), review process, testing strategy (unit, Containerlab integration,
  helm unittest, staging E2E)

Hardware target: 3x Miniforum MS-01 + 5x Raspberry Pi 5 8GB (mixed x86+ARM).
```

---

## Step 2: `/speckit.specify`

```
Build the complete Helios network observability platform — a Kubernetes-native
system that provides unified metrics collection (gNMI streaming telemetry + SNMP),
flow data collection (NetFlow/IPFIX/sFlow), long-term storage, visualization,
alerting, and automated remediation for network infrastructure.

Read the full project documentation for detailed requirements:
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/overview.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/architecture.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/flow-collection.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/runbook-operator.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/storage-topologies.md

Key user stories to cover (derive from the docs):

P1 - Metrics Collection & Storage: Network engineer deploys Helios and gets
streaming telemetry (gNMI) and SNMP metrics from network devices, stored in
Prometheus with Thanos for 90-day retention on object storage.

P2 - Visualization & Alerting: Network engineer views pre-built Grafana dashboards
(interface utilization, BGP state, hardware health) and receives alerts via
Alertmanager with tiered routing (PagerDuty, Slack, email).

P3 - Flow Collection & Analysis: Network engineer collects NetFlow/IPFIX/sFlow
from network devices, enriched with NetBox metadata and GeoIP, stored in
ClickHouse with tiered retention. Dashboards: Top Talkers, Traffic Matrix,
AS Paths, Protocol Distribution, Geographic Flows, Interface Correlation.

P4 - Device Discovery & Integration: Network engineer configures NetBox as the
source of truth; Target Generator automatically syncs device inventory to gnmic,
SNMP exporter, and goflow2 target lists via ConfigMaps/ServiceDiscovery.

P5 - Automated Remediation: Network engineer defines runbooks as Kubernetes CRDs;
Runbook Operator executes gNMI Set/Get actions, with approval workflows, audit
trails, and rollback capabilities.

P6 - Multi-Datacenter: Platform supports multiple datacenters with Prometheus
per-DC, Thanos global query federation, and selectable storage topologies
(Single-DC MinIO, Multi-DC MinIO replication, Cloud S3, Hybrid).

Scale: 50 to 8,000+ devices. Hardware: 3x Miniforum MS-01 + 5x RPi5 (mixed arch).
Multi-vendor: Arista, Cisco, Juniper, Palo Alto, Fortigate, F5, Cumulus, etc.
```

---

## Step 3: `/speckit.plan`

```
Read the full architecture documentation for technical design details:
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/architecture.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/flow-collection.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/runbook-operator.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/storage-topologies.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/runbooks/README.md

Technical context:
- Language: Go 1.22+ (custom services), YAML/Jsonnet (Helm charts, dashboards)
- Primary Dependencies: gnmic, Prometheus, Thanos, Grafana, ClickHouse, Kafka
  (Strimzi), goflow2, MinIO, Alertmanager, NetBox
- Storage: Prometheus (short-term metrics), Thanos + MinIO/S3 (long-term metrics),
  ClickHouse (flow data), Kafka (flow message queue)
- Testing: Go test, helm unittest, Containerlab (network simulation)
- Target Platform: Kubernetes (mixed x86+ARM), Cilium CNI, ArgoCD
- Project Type: Kubernetes platform (Helm umbrella chart + custom operators)
- Performance: <30s metric latency, 500K fps flow ingestion, <5s dashboard queries
- Scale: 50-8000+ devices across multiple datacenters

The architecture docs contain complete designs including:
- gnmic StatefulSet with K8s clustering and hash-mod target distribution
- Prometheus sharding with Thanos sidecar, Store Gateway, Compactor, Query
- ClickHouse schema (flows_kafka, flows_raw, flows_1m, flows_1h tables)
- Flow Enricher service (Go, NetBox + GeoIP enrichment, Protobuf)
- Runbook Operator CRDs (Runbook, RunbookExecution) with state machine
- 4 storage topologies with Helm values for each
- Complete Grafana dashboard definitions (6 flow dashboards + metrics dashboards)
- Namespace structure, RBAC roles, network policies, resource estimates

Build with: Helm umbrella chart, ArgoCD ApplicationSet for multi-cluster,
GitOps workflow for all configuration. Deploy to mixed-arch K8s cluster.
```

---

## Step 4: `/speckit.tasks`

```
Break the plan into implementation tasks following the 5-phase approach from the
project overview:

Phase 1 (Weeks 1-4): Metrics Foundation
- Prometheus Operator + Thanos, MinIO, Grafana, Alertmanager, namespaces, RBAC

Phase 2 (Weeks 5-8): Metrics Collection
- gnmic with subscriptions, SNMP/Blackbox exporters, NetBox integration, dashboards

Phase 3 (Weeks 9-12): Automation
- Target Generator CronJob, CI/CD pipeline, Runbook Operator, runbook library

Phase 4 (Weeks 13-18): Flow Collection
- Strimzi Kafka, ClickHouse, goflow2, Flow Enricher, flow dashboards

Phase 5 (Weeks 19-22): Scale & Optimize
- Sharding, Query Frontend caching, retention tuning, performance testing

Reference the architecture docs for exact component specifications:
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/architecture.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/flow-collection.md
- /mnt/c/Users/rhwen/Documents/vaulty/projects/helios/architecture/runbook-operator.md

Include tests where applicable (Go unit tests, helm unittest, Containerlab E2E).
```

---

## Optional: `/speckit.clarify`

Run after `/speckit.specify` if there are `[NEEDS CLARIFICATION]` markers.

## Optional: `/speckit.analyze`

Run after `/speckit.tasks` to cross-check consistency across all artifacts.

## Optional: `/speckit.checklist`

Run after `/speckit.plan` to generate quality validation checklists.
