# Research: Helios Network Observability Platform

**Branch**: `001-helios-platform` | **Date**: 2026-02-07

## R-001: gnmic Kubernetes Clustering Mode

**Decision**: Deploy gnmic as a StatefulSet with Kubernetes-native clustering
using `--clustering-enable` with `--api-address :7890` for leader election and
hash-mod target distribution.

**Rationale**: gnmic's built-in K8s clustering mode uses leader election via the
Kubernetes API (coordination/v1 Lease). The leader assigns targets to cluster
members using consistent hashing (hash-mod on target address). This eliminates
the need for external coordination (Consul, etcd) and is the recommended
deployment pattern for K8s. A headless Service enables peer discovery via DNS.

**Alternatives considered**:
- *External service discovery (Consul)*: Adds operational complexity. gnmic's
  native K8s clustering is simpler and sufficient.
- *Prometheus-style hashmod relabeling*: Only works for scrape targets, not
  gNMI subscriptions. gnmic needs its own clustering.
- *Single gnmic instance*: Does not scale beyond ~200 devices with streaming
  telemetry at default subscription intervals.

## R-002: Prometheus Sharding Strategy

**Decision**: Use the Prometheus Operator's `shards` field on the Prometheus CR.
Each shard runs as a separate StatefulSet. Thanos Sidecar uploads blocks from
each shard independently. Thanos Query merges results across shards.

**Rationale**: The Operator's sharding splits targets by consistent hashing on
`__address__`. Combined with Thanos Sidecar, each shard independently uploads
2-hour blocks to object storage. Thanos Query's StoreAPI fanout naturally
merges results across shards with deduplication via `external_labels`.

**Alternatives considered**:
- *Cortex/Mimir*: Significant operational overhead for the scale target. Thanos
  is lighter-weight and sufficient for 8K devices.
- *Victoria Metrics*: Strong alternative but less ecosystem support for Thanos
  long-term storage patterns.
- *Single Prometheus*: Cannot handle >500 devices at standard scrape intervals
  without memory pressure.

## R-003: ClickHouse Deployment Pattern

**Decision**: Use the Altinity ClickHouse Operator with a
ClickHouseInstallation CR. 2 shards × 2 replicas for Large scale, scaling to
4×2 at XL. ReplicatedMergeTree for data safety.

**Rationale**: The Altinity Operator manages ClickHouse cluster lifecycle
(rolling upgrades, shard management, schema migrations) natively on K8s. It
supports ReplicatedMergeTree with ZooKeeper or ClickHouse Keeper for
replication coordination. TTL policies on MergeTree tables handle automatic
data expiry without external cron jobs.

**Alternatives considered**:
- *TimescaleDB*: Good for time-series but ClickHouse's columnar storage is
  significantly faster for flow analytics (aggregation, top-N, grouping).
- *Elasticsearch*: Higher resource consumption, not optimized for numeric
  aggregation at flow scale.
- *Plain ClickHouse StatefulSet*: Misses operator benefits (automated
  recovery, rolling upgrades, schema management).

## R-004: Flow Pipeline Kafka vs Direct Ingestion

**Decision**: Use Kafka (Strimzi Operator) between goflow2 and ClickHouse to
decouple collection from storage. goflow2 publishes to `helios-flows-raw`,
Flow Enricher consumes, enriches, publishes to `helios-flows-enriched`,
ClickHouse consumes via Kafka engine table.

**Rationale**: Kafka provides backpressure handling — if ClickHouse slows down
(compaction, maintenance), flows buffer in Kafka rather than being dropped at
the UDP receiver. Kafka also enables multiple consumers (ClickHouse + future
analytics) and replay for reprocessing. Strimzi manages the Kafka cluster
lifecycle on K8s.

**Alternatives considered**:
- *goflow2 → ClickHouse direct*: No buffer for backpressure. ClickHouse
  maintenance causes flow drops.
- *NATS/JetStream*: Lighter but less ecosystem tooling for ClickHouse
  integration (no native Kafka engine equivalent).
- *Apache Pulsar*: More complex to operate, smaller community for this
  use case.

## R-005: Flow Enricher Design

**Decision**: Custom Go service consuming from Kafka, enriching with in-memory
caches of NetBox device data and MaxMind GeoIP databases, producing enriched
protobuf to a second Kafka topic.

**Rationale**: Enrichment must happen at flow rate (up to 5M FPS at XL). An
in-memory cache with periodic refresh (every 5 minutes from NetBox API, GeoIP
database reload on file change) avoids per-flow API calls. Go provides the
concurrency model and performance needed. Protobuf encoding is compact and
directly consumable by ClickHouse's Kafka engine.

**Alternatives considered**:
- *ClickHouse dictionaries*: Can do IP→GeoIP lookup but cannot query NetBox
  for device metadata at query time with acceptable latency.
- *Kafka Streams*: Java ecosystem, adds JVM dependency. Go service is simpler
  and aligns with the operator codebase.
- *Enrichment at query time (Grafana transforms)*: Too slow for aggregated
  queries over millions of flows.

## R-006: Object Storage Topology Selection

**Decision**: Support 4 topologies selectable via `global.storageTopology`
Helm value: `single-dc-minio` (default), `multi-dc-minio`, `cloud-s3`,
`hybrid`. Each topology maps to a different Thanos objstore config and MinIO
deployment pattern.

**Rationale**: Different deployment environments have different storage
requirements. Home lab uses Single-DC MinIO. Multi-site enterprise uses
Multi-DC with site replication. Cloud-native deployments use S3 directly.
Hybrid allows hot-tier local MinIO with cold-tier cloud S3. The Helm chart
abstracts these into a single value selection.

**Alternatives considered**:
- *S3-only*: Excludes on-prem/home-lab deployments where cloud egress costs
  are prohibitive.
- *MinIO-only*: Excludes cloud-native deployments that already have S3.
- *Single topology*: Forces all users into one pattern regardless of
  environment.

## R-007: Runbook Operator Framework

**Decision**: Build with kubebuilder (controller-runtime). Two CRDs: Runbook
(template) and RunbookExecution (instance). Execution runs as a K8s Job
spawned by the controller. gNMI operations via gnmic Go library.

**Rationale**: Kubebuilder is the standard framework for K8s operators in Go.
The Job-based execution model provides resource isolation (each execution gets
its own pod), automatic cleanup, and retry semantics. The controller manages
the state machine (Pending → PendingApproval → Running → Completed/Failed →
RollingBack → RolledBack). Separating Runbook (reusable template) from
RunbookExecution (instance) follows the Job/CronJob pattern.

**Alternatives considered**:
- *Ansible AWX/Tower*: Heavy, not K8s-native, separate authentication system.
- *Argo Workflows*: Good workflow engine but adds a large dependency for what
  is essentially a sequential step executor with gNMI support.
- *Tekton*: CI/CD focused, poor fit for network operations with approval
  workflows and gNMI integration.

## R-008: Target Generator Implementation

**Decision**: Go-based CronJob querying the NetBox REST API every 5 minutes.
Generates ConfigMaps for gnmic, snmp_exporter, and blackbox_exporter targets,
plus Prometheus file-based service discovery JSON. Atomic ConfigMap updates
(write-new-then-swap) to prevent empty target lists on failure.

**Rationale**: Go aligns with the other custom services. The CronJob model is
simple and fits the 5-minute sync interval. ConfigMap-based targets allow gnmic
and exporters to detect changes via K8s volume mount inotify without pod
restarts. File-based SD is the standard Prometheus pattern for dynamic targets.

**Alternatives considered**:
- *Python (pynetbox)*: Originally considered but Go provides better K8s client
  library support and aligns with the rest of the codebase.
- *Webhook-triggered sync*: More responsive but requires NetBox webhook
  configuration and adds complexity. 5-minute polling is acceptable.
- *Prometheus Operator ServiceMonitor CRDs*: Only works for Prometheus scrape
  targets, not gnmic gNMI subscriptions.

## R-009: Multi-Arch Build Strategy

**Decision**: Use Docker Buildx with `--platform linux/amd64,linux/arm64` for
all custom service images. GitHub Actions runners build natively on amd64;
arm64 uses QEMU emulation via `docker/setup-qemu-action`.

**Rationale**: The target hardware includes both x86_64 (Miniforum MS-01) and
aarch64 (Raspberry Pi 5). Buildx manifest lists allow a single image tag to
resolve to the correct architecture. QEMU emulation is slow but acceptable for
CI builds of Go services (which cross-compile natively).

**Alternatives considered**:
- *Native ARM runners*: Faster but GitHub-hosted ARM runners have limited
  availability. Can be added later as an optimization.
- *Go cross-compilation only*: Works for Go binaries but doesn't handle C
  dependencies (e.g., MaxMind GeoIP library cgo bindings).
- *Separate image tags per arch*: Requires arch-aware Helm values, adds
  deployment complexity.

## R-010: Grafana Dashboard Provisioning Strategy

**Decision**: Dashboard JSON files stored in `config/dashboards/` as part of
the Git-managed config repository. Helm chart creates ConfigMaps with the
`grafana_dashboard: "1"` label, which Grafana's sidecar provisioner detects
and loads automatically.

**Rationale**: The Grafana sidecar (part of the kube-prometheus-stack chart)
watches for ConfigMaps with the dashboard label and automatically provisions
them. This is the standard GitOps pattern for Grafana on K8s. Dashboard JSON
is version-controlled, peer-reviewed, and reproducible across environments.

**Alternatives considered**:
- *Grafana API provisioning*: Requires imperative scripts. Violates
  Constitution Principle IV (Extensibility via Git).
- *Grafana Terraform provider*: Adds Terraform as a dependency. Overkill for
  dashboard management.
- *Grafonnet/Jsonnet*: Good for programmatic dashboard generation but adds
  build complexity. Raw JSON is more accessible for the target audience
  (network engineers).
