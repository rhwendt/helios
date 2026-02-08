<!--
Sync Impact Report
==================
- Version change: 0.0.0 (new) → 1.0.0
- Modified principles: N/A (initial ratification)
- Added sections:
  - Core Principles (6 principles)
  - Architecture Constraints
  - Development Workflow
  - Governance
- Removed sections: N/A
- Templates requiring updates:
  - .specify/templates/plan-template.md: ✅ no update needed
    (Constitution Check section already references constitution file
    dynamically; gates will be derived from these principles at plan time)
  - .specify/templates/spec-template.md: ✅ no update needed
    (Requirements and Success Criteria sections are generic; performance
    budgets from this constitution apply at spec-fill time)
  - .specify/templates/tasks-template.md: ✅ no update needed
    (Phase structure is generic; Helm/Go/GitOps task types will be
    instantiated from these principles when tasks are generated)
- Follow-up TODOs: None
-->

# Helios Constitution

## Core Principles

### I. Cloud-Native First

All Helios components MUST be deployable on Kubernetes via Helm
charts and managed through GitOps workflows (ArgoCD). There MUST
be no bare-metal or VM-only dependencies. Every custom service
MUST produce a container image published to a registry.

- Deployment tooling: Helm umbrella chart, ArgoCD ApplicationSets.
- Configuration MUST be declarative (YAML/JSON in Git), never
  imperative scripts executed ad-hoc on clusters.
- Platform target: mixed amd64 + arm64 nodes (3x Miniforum MS-01,
  5x Raspberry Pi 5 8GB). Multi-arch images MUST be provided for
  all custom-built components.

**Rationale**: Kubernetes-native deployment is the foundation of
the entire stack. Every ADR, scaling decision, and operational
runbook assumes K8s primitives (StatefulSets, CronJobs, CRDs,
RBAC, NetworkPolicies). Violating this principle invalidates the
architecture.

### II. Horizontal Scalability

Collection, storage, and query layers MUST scale independently.
The system MUST support four discrete scale tiers without
architectural changes:

| Tier | Devices | gnmic Pods | Prom Shards | Flow FPS |
|------|---------|------------|-------------|----------|
| Small | <50 | 2 | 1 | 5K |
| Medium | 50-500 | 3-5 | 2 | 50K |
| Large | 500-2000 | 5-10 | 4 | 500K |
| XL | 2000-8000+ | 10-20 | 8 | 5M |

- Scaling MUST be achievable by changing Helm values files
  (`values-small.yaml` through `values-xl.yaml`) without code
  changes.
- HPAs MUST be defined for gnmic, goflow2, and the Flow Enricher.
- Prometheus sharding MUST use the Operator's `shards` field with
  Thanos Sidecar for upload.

**Rationale**: The architecture spans a 160x device range. Each
layer (gnmic clustering, Prometheus sharding, Thanos federation,
ClickHouse sharding, Kafka partitioning) was designed to scale
its axis independently so cost and capacity are proportional.

### III. Vendor Agnostic

Helios MUST interface with network devices exclusively through
standardized protocols: gNMI/OpenConfig, SNMP, and flow protocols
(NetFlow v5/v9, IPFIX, sFlow). Vendor-specific logic MUST reside
in configuration files (gnmic subscriptions, SNMP modules, alert
rules), never in application code.

- NetBox MUST be the single source of truth for device inventory,
  site hierarchy, and monitoring attributes.
- New vendor support MUST be achievable by adding configuration
  files and committing to the config repository—no service
  redeployment required.
- Label taxonomy (device, site, region, vendor, platform, role,
  tier) MUST be consistent across metrics and flow pipelines.

**Rationale**: Network environments are inherently multi-vendor.
Encoding vendor knowledge in code creates coupling that scales
linearly with vendor count. Configuration-driven support keeps
the platform maintainable.

### IV. Extensibility via Git

MIBs, OpenConfig subscriptions, alert rules, Grafana dashboards,
and runbook definitions MUST be added via Git commits to a config
repository. CI/CD pipelines MUST validate changes before merge.
Manual changes through Grafana UI or direct API calls are
prohibited in production.

- The config repository (`helios-config/`) MUST contain
  directories: `snmp/`, `gnmic/`, `dashboards/`, `alerts/`,
  `runbooks/`, `scripts/`.
- CI MUST validate: SNMP modules (`snmp-exporter --config.check`),
  gnmic configs (`gnmic --dry-run`), Prometheus rules
  (`promtool check rules`), and dashboard JSON schema.
- Deployment MUST flow through ConfigMap generation and ArgoCD
  sync.

**Rationale**: Git-based extensibility provides version control,
peer review, rollback capability, and an audit trail. It
eliminates configuration drift between environments and enables
reproducible deployments.

### V. Operational Simplicity

Helios MUST ship with production-ready defaults for Medium scale.
Pre-built dashboards, alert rules, and runbooks MUST cover common
network operations scenarios out of the box. A single
`helm install` with default values MUST produce a working stack.

- Core dashboards (Network Overview, Interface Analytics, Device
  Health, Site Overview, Latency & Reachability, BGP Status, Top
  Talkers, Alerts) MUST be provisioned automatically.
- Alert rules for device down, interface errors, BGP session loss,
  high CPU/memory, and flow anomalies MUST be included by default.
- The Runbook Operator MUST ship with example runbooks (interface
  bounce, collect diagnostics, clear BGP session).
- Documentation MUST include a quickstart guide that achieves
  first metrics within 30 minutes of deployment.

**Rationale**: Operational complexity is the primary adoption
barrier for observability platforms. Helios competes with
commercial products by providing equivalent out-of-box
functionality with zero vendor lock-in.

### VI. Long-Term Retention with Tiered Storage

Metrics MUST be retained for a minimum of 90 days using Thanos
with object storage (MinIO or S3-compatible). Flow data MUST use
tiered retention in ClickHouse:

| Data Type | Retention | Resolution |
|-----------|-----------|------------|
| Raw metrics | 14 days | Full |
| 5m downsampled | 30 days | 5-minute |
| 1h downsampled | 90 days | 1-hour |
| Raw flows | 7 days | Full |
| 1m flow aggregates | 30 days | 1-minute |
| 1h flow aggregates | 90 days | 1-hour |

- Object storage topology MUST be selectable per deployment
  (Single-DC MinIO, Multi-DC MinIO, Cloud S3, Hybrid tiering).
- Thanos Compactor MUST run as a singleton; in multi-DC setups
  it MUST be coordinated to prevent data corruption.
- ClickHouse TTL policies MUST enforce automatic data expiry.

**Rationale**: Network forensics and capacity planning require
historical data. Tiered retention balances storage cost against
query granularity, ensuring recent data is high-resolution while
long-term data remains queryable at reduced resolution.

## Architecture Constraints

### Technology Stack

| Layer | Component | Purpose |
|-------|-----------|---------|
| Metrics Collection | gnmic, snmp_exporter, blackbox_exporter | gNMI, SNMP, synthetic probes |
| Flow Collection | goflow2 | NetFlow/IPFIX/sFlow |
| Flow Queue | Kafka (Strimzi) | Decouples collection from storage |
| Flow Enrichment | Custom Go service | NetBox + GeoIP context |
| Metrics Storage | Prometheus + Thanos + MinIO/S3 | HA metrics with long-term retention |
| Flow Storage | ClickHouse | Columnar analytics for flow data |
| Visualization | Grafana | Unified dashboards |
| Alerting | Alertmanager + Thanos Ruler | Notification routing |
| Automation | Custom K8s Operator (Go) | Runbook execution via CRDs |
| Integration | NetBox + Target Generator | Device inventory sync |
| Deployment | Helm + ArgoCD | GitOps delivery |

### Namespace Isolation

All Helios components MUST be deployed across isolated namespaces:

- `helios-integration` — NetBox sync, target generation
- `helios-collection` — gnmic, SNMP/Blackbox exporters
- `helios-storage` — Prometheus, Thanos, MinIO
- `helios-visualization` — Grafana, Alertmanager
- `helios-automation` — Runbook Operator, execution jobs
- `helios-flows` — goflow2, Kafka, ClickHouse, Flow Enricher

Cross-namespace traffic MUST be governed by NetworkPolicies.

### Security Requirements

- Device credentials MUST be managed through External Secrets
  Operator (ESO) syncing from Vault or cloud secret managers.
  Secrets MUST NOT appear in Git or ConfigMaps.
- RBAC MUST define three tiers: `helios-viewer` (read-only),
  `helios-engineer` (execute runbooks), `helios-admin` (full
  access including config and approval).
- NetworkPolicies MUST restrict collection namespace egress to
  management network CIDRs and specific ports (gNMI 6030/57400,
  SNMP 161/UDP, flow UDP 2055/6343).
- mTLS SHOULD be used for inter-component communication where
  supported by the service mesh (Cilium, Istio).

### Performance Budgets

| Metric | Target |
|--------|--------|
| Metric ingestion-to-query latency | < 30 seconds |
| Flow ingestion at Large scale | 500K FPS sustained |
| Dashboard query response (24h range) | < 5 seconds |
| Target sync from NetBox | < 5 minutes |
| Runbook execution overhead | < 10 seconds (excluding device RTT) |

## Development Workflow

### Code Quality

- Custom Go services (Flow Enricher, Runbook Operator, Target
  Generator) MUST maintain > 70% test coverage.
- All Go code MUST pass `golangci-lint` with the project's
  `.golangci.yml` configuration.
- Helm charts MUST pass `helm lint` and `helm unittest`.
- ClickHouse schema changes MUST include migration scripts.

### Review Process

- All changes MUST go through pull request review.
- Helm value changes that affect production scale tiers MUST
  have two approvals.
- Runbook definitions with `riskLevel: high` or
  `riskLevel: critical` MUST be reviewed by a network-admin.

### Testing Strategy

| Test Type | Scope | Tools |
|-----------|-------|-------|
| Unit | Go service logic | `go test` |
| Helm | Chart rendering | `helm unittest` |
| Integration | Multi-component flows | Containerlab + kind |
| E2E | Full stack validation | Staging cluster |

- Integration tests SHOULD use Containerlab to simulate network
  topologies with virtual devices exporting gNMI/SNMP/flow data.
- Staging E2E tests MUST run against a Small-scale deployment
  before production promotion.

## Governance

This constitution is the authoritative source of architectural
principles and constraints for the Helios project. All pull
requests, design reviews, and architecture decisions MUST be
evaluated against these principles.

### Amendment Procedure

1. Propose changes via pull request to this file.
2. Changes MUST include rationale and impact assessment.
3. Principle additions or removals (MAJOR version bump) require
   review from the project maintainer.
4. Clarifications and wording fixes (PATCH) require standard
   review.

### Versioning Policy

This constitution follows semantic versioning:

- **MAJOR**: Principle added, removed, or fundamentally redefined.
- **MINOR**: New section added or existing guidance materially
  expanded.
- **PATCH**: Wording clarification, typo fix, non-semantic
  refinement.

### Compliance Review

- The plan template's "Constitution Check" section MUST validate
  every new feature against these principles before implementation
  begins.
- Deviations MUST be documented in the plan's "Complexity
  Tracking" table with justification and rejected alternatives.

**Version**: 1.0.0 | **Ratified**: 2026-02-07 | **Last Amended**: 2026-02-07
