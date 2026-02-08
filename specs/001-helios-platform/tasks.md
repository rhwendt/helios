---
description: "Task list for Helios Network Observability Platform"
---

# Tasks: Helios Network Observability Platform

**Input**: Design documents from `/specs/001-helios-platform/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: Included per user guidance. Go unit tests, helm unittest, Containerlab integration tests.

**Organization**: Tasks grouped by user story. Each story is independently testable and deliverable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

- **Helm charts**: `helm/helios/` (umbrella) + `helm/helios/charts/helios-*/` (sub-charts)
- **Custom services**: `services/flow-enricher/`, `services/runbook-operator/`, `services/target-generator/`
- **Config content**: `config/snmp/`, `config/gnmic/`, `config/dashboards/`, `config/alerts/`, `config/runbooks/`
- **ClickHouse migrations**: `clickhouse/migrations/`
- **Protobuf**: `proto/flow.proto`
- **CI/CD**: `.github/workflows/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Repository structure, Helm umbrella chart skeleton, CI pipeline scaffolding

- [ ] T001 Create repository directory structure per plan.md layout: `helm/`, `services/`, `config/`, `clickhouse/`, `proto/`, `deploy/`, `.github/workflows/`
- [ ] T002 Initialize Helm umbrella chart in `helm/helios/Chart.yaml` with dependencies (kube-prometheus-stack, thanos, minio, strimzi-kafka-operator, clickhouse-operator) and create `values.yaml` with global defaults
- [ ] T003 [P] Create 4 scale-tier values files: `helm/helios/values-small.yaml`, `values-medium.yaml`, `values-large.yaml`, `values-xl.yaml` with resource limits, replica counts, and shard counts per architecture spec
- [ ] T004 [P] Create 6 sub-chart skeletons with `Chart.yaml` and `values.yaml` in `helm/helios/charts/`: helios-integration, helios-collection, helios-storage, helios-visualization, helios-automation, helios-flows
- [ ] T005 [P] Create namespace template in `helm/helios/templates/namespaces.yaml` defining 6 namespaces: helios-integration, helios-collection, helios-storage, helios-visualization, helios-automation, helios-flows
- [ ] T006 [P] Create `.golangci.yml` at repo root with Go 1.22+ configuration for all custom services
- [ ] T007 [P] Create `Makefile` at repo root with targets: lint, test, build, docker-build, helm-lint, helm-unittest, proto-gen
- [ ] T008 [P] Create `.github/workflows/helm-lint.yaml` CI workflow running `helm lint` and `helm unittest` on PRs touching `helm/`
- [ ] T009 [P] Create `.github/workflows/build-services.yaml` CI workflow building multi-arch Docker images (amd64+arm64) via Buildx for all 3 custom services on PRs touching `services/`
- [ ] T010 [P] Create `.github/workflows/validate-config.yaml` CI workflow validating SNMP modules, gnmic configs, Prometheus rules, and dashboard JSON on PRs touching `config/`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core storage and networking infrastructure that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T011 Create helios-storage sub-chart Prometheus Operator CR in `helm/helios/charts/helios-storage/templates/prometheus.yaml` with sharding support (`shards` field), Thanos Sidecar container, and external_labels (cluster, datacenter, replica)
- [ ] T012 Create Thanos objstore Secret template in `helm/helios/charts/helios-storage/templates/thanos-objstore-secret.yaml` with configurable S3/MinIO endpoint, bucket, access key, secret key
- [ ] T013 [P] Create Thanos Query Deployment in `helm/helios/charts/helios-storage/templates/thanos-query.yaml` with StoreAPI discovery for Sidecar and Store Gateway endpoints
- [ ] T014 [P] Create Thanos Store Gateway StatefulSet in `helm/helios/charts/helios-storage/templates/thanos-store.yaml` with anti-affinity and cache sizing from values
- [ ] T015 [P] Create Thanos Compactor StatefulSet (singleton) in `helm/helios/charts/helios-storage/templates/thanos-compactor.yaml` with downsampling and retention config (raw 14d, 5m 30d, 1h 90d)
- [ ] T016 [P] Create MinIO StatefulSet in `helm/helios/charts/helios-storage/templates/minio.yaml` with distributed erasure coding (EC:4), bucket auto-creation for Thanos, and PVC templates
- [ ] T017 Create global NetworkPolicy templates in `helm/helios/templates/networkpolicies-global.yaml` defining default deny-all ingress per namespace and allowed cross-namespace traffic patterns
- [ ] T018 [P] Create RBAC ClusterRole definitions for helios-viewer, helios-engineer, helios-admin in `helm/helios/charts/helios-storage/templates/rbac.yaml`
- [ ] T019 Create `helm/helios/charts/helios-storage/tests/` directory with helm unittest YAML tests for Prometheus CR shard count, Thanos Compactor singleton, and MinIO replica count across all 4 scale tiers

**Checkpoint**: Storage foundation ready — Prometheus, Thanos, MinIO operational. User story implementation can begin.

---

## Phase 3: User Story 1 — Metrics Collection & Storage (Priority: P1) 🎯 MVP

**Goal**: gNMI + SNMP + synthetic probe metrics flowing into Prometheus → Thanos → MinIO

**Independent Test**: `helm install` on kind cluster + Containerlab virtual devices → metrics in Prometheus within 30s, queryable via Thanos Query

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T020 [P] [US1] Helm unittest for gnmic StatefulSet in `helm/helios/charts/helios-collection/tests/statefulset-gnmic_test.yaml`: verify replica count from values, headless Service, clustering args, volume mounts for config and targets
- [ ] T021 [P] [US1] Helm unittest for snmp_exporter Deployment in `helm/helios/charts/helios-collection/tests/deployment-snmp-exporter_test.yaml`: verify ConfigMap volume mount for modules, ServiceMonitor creation
- [ ] T022 [P] [US1] Helm unittest for blackbox_exporter Deployment in `helm/helios/charts/helios-collection/tests/deployment-blackbox-exporter_test.yaml`: verify probe module configuration
- [ ] T023 [P] [US1] Helm unittest for HPA in `helm/helios/charts/helios-collection/tests/hpa-gnmic_test.yaml`: verify min/max replicas from scale-tier values

### Implementation for User Story 1

- [ ] T024 [P] [US1] Create gnmic StatefulSet in `helm/helios/charts/helios-collection/templates/statefulset-gnmic.yaml` with K8s clustering (--clustering-enable, headless Service, Lease RBAC), Prometheus write endpoint, and target ConfigMap volume mount
- [ ] T025 [P] [US1] Create gnmic configuration ConfigMap in `helm/helios/charts/helios-collection/templates/configmap-gnmic.yaml` with default gNMI subscriptions (interfaces/counters, system/state, BGP neighbors), output Prometheus remote-write, and clustering config
- [ ] T026 [P] [US1] Create static target ConfigMap in `helm/helios/charts/helios-collection/templates/configmap-targets.yaml` with example targets in gnmic YAML format (used before Target Generator is deployed)
- [ ] T027 [P] [US1] Create gnmic headless Service in `helm/helios/charts/helios-collection/templates/service-gnmic.yaml` with ClusterIP: None for peer discovery and API port 7890
- [ ] T028 [P] [US1] Create snmp_exporter Deployment in `helm/helios/charts/helios-collection/templates/deployment-snmp-exporter.yaml` with ConfigMap volume for custom MIB modules and Service on port 9116
- [ ] T029 [P] [US1] Create blackbox_exporter Deployment in `helm/helios/charts/helios-collection/templates/deployment-blackbox-exporter.yaml` with probe module configuration (icmp, tcp_connect, http_2xx) and Service on port 9115
- [ ] T030 [US1] Create gnmic HPA in `helm/helios/charts/helios-collection/templates/hpa-gnmic.yaml` with CPU-based scaling, min/max from scale-tier values
- [ ] T031 [US1] Create ServiceMonitor for gnmic in `helm/helios/charts/helios-collection/templates/servicemonitor.yaml` scraping port 9804 (gnmic Prometheus endpoint) with relabeling for device/site/vendor labels
- [ ] T032 [P] [US1] Create default SNMP modules in `config/snmp/modules/arista_eos.yml`, `cisco_iosxe.yml`, `juniper_junos.yml`, `paloalto_panos.yml` with interface counters, system stats, and hardware health OIDs
- [ ] T033 [P] [US1] Create default gNMI subscription files in `config/gnmic/subscriptions/default-counters.yaml`, `default-bgp.yaml`, `default-system.yaml` with OpenConfig paths for interface counters, BGP neighbor state, CPU/memory
- [ ] T034 [US1] Create NetworkPolicy for helios-collection namespace in `helm/helios/charts/helios-collection/templates/networkpolicy.yaml` restricting egress to management CIDRs on gNMI (6030/57400), SNMP (161/UDP), and Prometheus (9090) ports
- [ ] T035 [US1] Create Thanos Ruler StatefulSet in `helm/helios/charts/helios-storage/templates/thanos-ruler.yaml` for evaluating recording rules across Thanos data (e.g., long-range downsampled aggregations)

**Checkpoint**: Metrics pipeline complete. `helm install` → gnmic/SNMP/blackbox collect → Prometheus stores → Thanos uploads to MinIO → Thanos Query serves unified view.

---

## Phase 4: User Story 2 — Visualization & Alerting (Priority: P2)

**Goal**: 8 pre-built Grafana dashboards + tiered alert routing via Alertmanager

**Independent Test**: Open Grafana → all 8 dashboards render with metrics data → trigger device-down alert → verify PagerDuty/Slack routing

### Tests for User Story 2 ⚠️

- [ ] T036 [P] [US2] Helm unittest for Grafana datasources in `helm/helios/charts/helios-visualization/tests/grafana-datasources_test.yaml`: verify Thanos Query and Alertmanager datasource URLs
- [ ] T037 [P] [US2] Helm unittest for dashboard ConfigMap count in `helm/helios/charts/helios-visualization/tests/grafana-dashboards_test.yaml`: verify 8 metrics dashboards are created with `grafana_dashboard: "1"` label
- [ ] T038 [P] [US2] Helm unittest for alert rules in `helm/helios/charts/helios-visualization/tests/prometheus-rules_test.yaml`: verify PrometheusRule resource includes device-down, interface-errors, BGP-session-loss, CPU, memory rules

### Implementation for User Story 2

- [ ] T039 [P] [US2] Create Grafana Deployment in `helm/helios/charts/helios-visualization/templates/grafana-deployment.yaml` with sidecar for dashboard provisioning, PostgreSQL backend config, OIDC auth config from values, and resource limits
- [ ] T040 [P] [US2] Create Grafana datasource ConfigMap in `helm/helios/charts/helios-visualization/templates/grafana-datasources.yaml` provisioning Thanos Query (Prometheus type) and Alertmanager datasources
- [ ] T041 [P] [US2] Create Alertmanager config template in `helm/helios/charts/helios-visualization/templates/alertmanager-config.yaml` with tiered routing: critical → PagerDuty, warning → Slack, info → email, receiver URLs from values
- [ ] T042 [US2] Create PrometheusRule resource in `helm/helios/charts/helios-visualization/templates/prometheus-rules.yaml` with default alert rules: DeviceUnreachable (2m), InterfaceErrorRate (>1%, 5m), BGPSessionDown (1m), HighCPU (>80%, 5m), HighMemory (>85%, 5m)
- [ ] T043 [P] [US2] Create Network Overview dashboard JSON in `config/dashboards/metrics/network-overview.json` with panels: device count, interface utilization heatmap, top-N error interfaces, BGP session summary, site availability map
- [ ] T044 [P] [US2] Create Interface Analytics dashboard JSON in `config/dashboards/metrics/interface-analytics.json` with panels: per-interface traffic rate, error/discard counters, utilization percentage, top talkers by interface
- [ ] T045 [P] [US2] Create Device Health dashboard JSON in `config/dashboards/metrics/device-health.json` with panels: CPU utilization, memory usage, uptime, temperature, power supply status per device
- [ ] T046 [P] [US2] Create Site Overview dashboard JSON in `config/dashboards/metrics/site-overview.json` with panels: devices per site, aggregate bandwidth, site availability percentage, alert count by site
- [ ] T047 [P] [US2] Create Latency & Reachability dashboard JSON in `config/dashboards/metrics/latency-reachability.json` with panels: ICMP RTT, packet loss, TCP connect time, HTTP probe duration per target
- [ ] T048 [P] [US2] Create BGP Status dashboard JSON in `config/dashboards/metrics/bgp-status.json` with panels: BGP neighbor state, prefixes received/sent, session uptime, flap count
- [ ] T049 [P] [US2] Create Top Talkers (metrics) dashboard JSON in `config/dashboards/metrics/top-talkers-metrics.json` with panels: top interfaces by throughput, top devices by total traffic, bandwidth utilization ranking
- [ ] T050 [P] [US2] Create Alerts Overview dashboard JSON in `config/dashboards/metrics/alerts-overview.json` with panels: active alerts table, alert history timeline, alerts by severity, alerts by site
- [ ] T051 [US2] Create dashboard ConfigMap templates in `helm/helios/charts/helios-visualization/templates/grafana-dashboards/` that load JSON files from `config/dashboards/metrics/` into ConfigMaps with `grafana_dashboard: "1"` label
- [ ] T052 [US2] Create default Prometheus alert rules YAML in `config/alerts/rules/helios-defaults.yaml` with all 5 default alert rules (device-down, interface-errors, BGP-session, CPU, memory) using PromQL from spec

**Checkpoint**: Visualization complete. 8 dashboards auto-provisioned, alerts fire and route to configured receivers.

---

## Phase 5: User Story 3 — Flow Collection & Analysis (Priority: P3)

**Goal**: NetFlow/IPFIX/sFlow → Kafka → Flow Enricher → ClickHouse → 6 flow dashboards in Grafana

**Independent Test**: Send synthetic NetFlow to goflow2 → enriched records in ClickHouse within 60s → Top Talkers dashboard renders

### Tests for User Story 3 ⚠️

- [ ] T053 [P] [US3] Unit tests for Flow Enricher enrichment logic in `services/flow-enricher/internal/enricher/enricher_test.go`: test NetBox cache lookup, GeoIP lookup, protobuf serialization, cache miss handling
- [ ] T054 [P] [US3] Unit tests for Flow Enricher Kafka consumer in `services/flow-enricher/internal/kafka/kafka_test.go`: test message deserialization, consumer group rebalancing, producer batching
- [ ] T055 [P] [US3] Helm unittest for goflow2 StatefulSet in `helm/helios/charts/helios-flows/tests/statefulset-goflow2_test.yaml`: verify UDP ports 2055/6343, Kafka output config, replica count
- [ ] T056 [P] [US3] Helm unittest for ClickHouse Installation in `helm/helios/charts/helios-flows/tests/clickhouse-installation_test.yaml`: verify shard/replica count per scale tier

### Implementation for User Story 3

- [ ] T057 [US3] Create canonical protobuf definition in `proto/flow.proto` with EnrichedFlow message per contracts/flow.proto spec (timestamp, flow_type, exporter metadata, L3/L4 fields, counters, GeoIP, ASN enrichment)
- [ ] T058 [US3] Initialize Go module for Flow Enricher in `services/flow-enricher/go.mod` with dependencies: confluent-kafka-go, protobuf, oschwald/maxminddb-golang, netbox-community/go-netbox
- [ ] T059 [P] [US3] Implement NetBox cache in `services/flow-enricher/internal/enricher/netbox.go`: periodic refresh from NetBox API (every 5 min), device→metadata map keyed by IP, interface index→name map
- [ ] T060 [P] [US3] Implement GeoIP lookup in `services/flow-enricher/internal/enricher/geoip.go`: MaxMind GeoLite2-City and GeoLite2-ASN database readers, IP→country/city/ASN resolution
- [ ] T061 [US3] Implement enrichment pipeline in `services/flow-enricher/internal/enricher/enricher.go`: accept raw flow protobuf, apply NetBox metadata (exporter_name, site, role, interface names), apply GeoIP (countries, cities, ASN names), output enriched protobuf
- [ ] T062 [P] [US3] Implement Kafka consumer in `services/flow-enricher/internal/kafka/consumer.go`: consumer group for `helios-flows-raw` topic, protobuf deserialization, batch processing
- [ ] T063 [P] [US3] Implement Kafka producer in `services/flow-enricher/internal/kafka/producer.go`: produce enriched protobuf to `helios-flows-enriched` topic, batched writes, delivery confirmation
- [ ] T064 [US3] Implement Flow Enricher main entrypoint in `services/flow-enricher/cmd/flow-enricher/main.go`: wire config, start Kafka consumer, start enrichment pipeline, start Prometheus metrics endpoint (:8080/metrics), graceful shutdown
- [ ] T065 [US3] Create Flow Enricher Dockerfile in `services/flow-enricher/Dockerfile`: multi-stage build (Go builder → distroless), multi-arch compatible, copy GeoIP database into image
- [ ] T066 [P] [US3] Create goflow2 StatefulSet in `helm/helios/charts/helios-flows/templates/statefulset-goflow2.yaml` with UDP listeners (2055 NetFlow, 6343 sFlow), Kafka protobuf output to `helios-flows-raw` topic
- [ ] T067 [P] [US3] Create goflow2 UDP Service in `helm/helios/charts/helios-flows/templates/service-goflow2-udp.yaml` as LoadBalancer type exposing ports 2055 and 6343
- [ ] T068 [P] [US3] Create Strimzi Kafka Cluster CR in `helm/helios/charts/helios-flows/templates/kafka-cluster.yaml` with 3 brokers, log retention from values, JMX metrics export
- [ ] T069 [P] [US3] Create KafkaTopic CRs in `helm/helios/charts/helios-flows/templates/kafka-topics.yaml` for `helios-flows-raw` (24 partitions) and `helios-flows-enriched` (24 partitions)
- [ ] T070 [US3] Create Flow Enricher Deployment in `helm/helios/charts/helios-flows/templates/deployment-flow-enricher.yaml` with Kafka connection config, NetBox API URL, GeoIP database mount, Prometheus metrics port
- [ ] T071 [P] [US3] Create ClickHouseInstallation CR in `helm/helios/charts/helios-flows/templates/clickhouse-installation.yaml` via Altinity Operator with shard/replica count from values, PVC templates, ZooKeeper config
- [ ] T072 [US3] Create ClickHouse schema migration `clickhouse/migrations/001_create_flows_raw.sql` with flows_kafka (Kafka engine), flows_raw (ReplicatedMergeTree, TTL 7d), and materialized view per data-model.md
- [ ] T073 [US3] Create ClickHouse migration `clickhouse/migrations/002_create_flows_1m.sql` with flows_1m (ReplicatedAggregatingMergeTree, TTL 30d) and materialized view from flows_raw
- [ ] T074 [US3] Create ClickHouse migration `clickhouse/migrations/003_create_flows_1h.sql` with flows_1h (ReplicatedAggregatingMergeTree, TTL 90d) and materialized view from flows_raw
- [ ] T075 [US3] Create ClickHouse schema init Job template in `helm/helios/charts/helios-flows/templates/clickhouse-schema.yaml` that runs migrations on install/upgrade
- [ ] T076 [P] [US3] Create goflow2 HPA in `helm/helios/charts/helios-flows/templates/hpa-goflow2.yaml` with CPU-based scaling, min/max from scale-tier values
- [ ] T077 [P] [US3] Create Flow Enricher HPA in `helm/helios/charts/helios-flows/templates/hpa-flow-enricher.yaml` with CPU-based scaling, min/max from scale-tier values
- [ ] T078 [US3] Create helios-flows NetworkPolicy in `helm/helios/charts/helios-flows/templates/networkpolicy.yaml` allowing ingress UDP 2055/6343, egress to Kafka 9092, ClickHouse 8123/9000
- [ ] T079 [US3] Add ClickHouse datasource to Grafana provisioning in `helm/helios/charts/helios-visualization/templates/grafana-datasources.yaml` (update existing file)
- [ ] T080 [P] [US3] Create Top Talkers flow dashboard JSON in `config/dashboards/flows/top-talkers.json` with panels: top source/dest IPs by bytes, top conversations, top protocols by traffic volume
- [ ] T081 [P] [US3] Create Traffic Matrix dashboard JSON in `config/dashboards/flows/traffic-matrix.json` with panels: site-to-site traffic matrix, AS-to-AS matrix, protocol distribution per path
- [ ] T082 [P] [US3] Create AS Paths dashboard JSON in `config/dashboards/flows/as-paths.json` with panels: top source/dest ASNs, AS path analysis, transit traffic
- [ ] T083 [P] [US3] Create Protocol Distribution dashboard JSON in `config/dashboards/flows/protocol-distribution.json` with panels: protocol breakdown by bytes/packets, top ports, TCP flag distribution
- [ ] T084 [P] [US3] Create Geographic Flows dashboard JSON in `config/dashboards/flows/geographic-flows.json` with panels: flows by country (world map), top countries by traffic, cross-border traffic analysis
- [ ] T085 [P] [US3] Create Interface Correlation dashboard JSON in `config/dashboards/flows/interface-correlation.json` with panels: cross-datasource combining Thanos interface metrics with ClickHouse flow data per interface
- [ ] T086 [US3] Create flow dashboard ConfigMap templates in `helm/helios/charts/helios-visualization/templates/grafana-dashboards/` loading JSON from `config/dashboards/flows/` with `grafana_dashboard: "1"` label

**Checkpoint**: Flow pipeline complete. NetFlow/sFlow → Kafka → Enricher → ClickHouse. 6 flow dashboards auto-provisioned in Grafana.

---

## Phase 6: User Story 4 — Device Discovery & Integration (Priority: P4)

**Goal**: NetBox → Target Generator CronJob → ConfigMaps → gnmic/SNMP/blackbox targets auto-synced

**Independent Test**: Add device to NetBox → wait ≤5 min → device appears in gnmic targets → metrics flowing

### Tests for User Story 4 ⚠️

- [ ] T087 [P] [US4] Unit tests for NetBox client in `services/target-generator/internal/netbox/client_test.go`: test device list query, custom field extraction, error handling, pagination
- [ ] T088 [P] [US4] Unit tests for ConfigMap generators in `services/target-generator/internal/generator/generator_test.go`: test gnmic target YAML generation, SNMP SD JSON generation, blackbox SD JSON generation, label taxonomy correctness
- [ ] T089 [P] [US4] Helm unittest for CronJob in `helm/helios/charts/helios-integration/tests/cronjob-target-sync_test.yaml`: verify schedule, RBAC, NetBox secret mount

### Implementation for User Story 4

- [ ] T090 [US4] Initialize Go module for Target Generator in `services/target-generator/go.mod` with dependencies: k8s.io/client-go, netbox-community/go-netbox, sigs.k8s.io/yaml
- [ ] T091 [US4] Implement NetBox API client in `services/target-generator/internal/netbox/client.go`: query devices with `helios_monitor=true`, extract custom fields (gnmi_enabled, gnmi_port, snmp_enabled, snmp_module, telemetry_profile, monitoring_tier, blackbox_probes), handle pagination
- [ ] T092 [P] [US4] Implement gnmic target generator in `services/target-generator/internal/generator/gnmic.go`: convert NetBox devices to gnmic targets YAML format with labels (device, site, region, vendor, platform, role, tier), subscription assignment by telemetry_profile
- [ ] T093 [P] [US4] Implement SNMP target generator in `services/target-generator/internal/generator/snmp.go`: convert NetBox devices to Prometheus file_sd JSON format with __param_module label from snmp_module custom field
- [ ] T094 [P] [US4] Implement blackbox target generator in `services/target-generator/internal/generator/blackbox.go`: convert NetBox devices to Prometheus file_sd JSON for each enabled probe type (icmp, tcp, http, dns)
- [ ] T095 [P] [US4] Implement Prometheus SD generator in `services/target-generator/internal/generator/prometheus_sd.go`: generate file-based service discovery JSON for all target types with consistent label taxonomy
- [ ] T096 [US4] Implement Kubernetes ConfigMap writer in `services/target-generator/internal/kubernetes/configmap.go`: atomic ConfigMap update (create new → verify → swap), preserve existing on error (no empty-write), Prometheus metrics for sync status
- [ ] T097 [US4] Implement Target Generator main entrypoint in `services/target-generator/cmd/target-generator/main.go`: wire config, query NetBox, generate all targets, update ConfigMaps, expose metrics (:8080/metrics), exit (CronJob pattern)
- [ ] T098 [US4] Create Target Generator Dockerfile in `services/target-generator/Dockerfile`: multi-stage build, multi-arch, minimal image
- [ ] T099 [P] [US4] Create CronJob template in `helm/helios/charts/helios-integration/templates/cronjob-target-sync.yaml` with configurable schedule (default: */5 * * * *), NetBox secret mount, ServiceAccount with ConfigMap update RBAC
- [ ] T100 [P] [US4] Create NetBox connection ConfigMap in `helm/helios/charts/helios-integration/templates/configmap-netbox.yaml` with NetBox URL, filters, custom field names
- [ ] T101 [US4] Create RBAC for Target Generator in `helm/helios/charts/helios-integration/templates/rbac.yaml`: ServiceAccount, Role (get/create/update ConfigMaps in helios-collection), RoleBinding
- [ ] T102 [US4] Create target sync alert rule in `config/alerts/rules/helios-target-sync.yaml`: alert if `helios_target_sync_errors_total` increases for >15 minutes or last successful sync >15 minutes ago

**Checkpoint**: NetBox integration complete. Devices auto-discovered, targets auto-generated, collection auto-configured.

---

## Phase 7: User Story 5 — Automated Remediation (Priority: P5)

**Goal**: Runbook CRDs + Operator + gNMI execution + approval workflows + audit trails + sample runbooks

**Independent Test**: Apply interface-bounce Runbook → create RunbookExecution → operator executes gNMI Set on Containerlab device → audit trail in CRD status

### Tests for User Story 5 ⚠️

- [ ] T103 [P] [US5] Unit tests for Runbook Operator controller in `services/runbook-operator/controllers/controller_test.go`: test state machine transitions (Pending→Running→Completed, Pending→PendingApproval→Approved→Running, Running→Failed→RollingBack→RolledBack), Job creation, status updates
- [ ] T104 [P] [US5] Unit tests for gNMI client wrapper in `services/runbook-operator/pkg/gnmic/client_test.go`: test Set, Get, Subscribe operations, connection handling, timeout behavior
- [ ] T105 [P] [US5] Unit tests for template engine in `services/runbook-operator/pkg/template/engine_test.go`: test parameter substitution, Go template rendering, validation
- [ ] T106 [P] [US5] Helm unittest for operator Deployment in `helm/helios/charts/helios-automation/tests/deployment-operator_test.yaml`: verify leader election, RBAC, CRD installation

### Implementation for User Story 5

- [ ] T107 [US5] Initialize kubebuilder project for Runbook Operator in `services/runbook-operator/`: scaffold with `kubebuilder init --domain helios.io --repo github.com/rhwendt/helios/services/runbook-operator`, create API types, generate deepcopy
- [ ] T108 [US5] Define Runbook CRD types in `services/runbook-operator/api/v1alpha1/runbook_types.go` per contracts/runbook-crd.yaml: RunbookSpec (name, category, riskLevel, requiresApproval, approvers, parameters, steps, rollback), RunbookStatus
- [ ] T109 [US5] Define RunbookExecution CRD types in `services/runbook-operator/api/v1alpha1/runbookexecution_types.go` per contracts/runbookexecution-crd.yaml: RunbookExecutionSpec (runbookRef, parameters, triggeredBy, triggerSource, dryRun), RunbookExecutionStatus (phase state machine, step tracking, audit fields)
- [ ] T110 [US5] Implement Runbook controller in `services/runbook-operator/controllers/runbook_controller.go`: validate runbook schema on create/update, set Ready condition, index by category
- [ ] T111 [US5] Implement RunbookExecution controller in `services/runbook-operator/controllers/runbookexecution_controller.go`: full state machine reconciliation (Pending→PendingApproval→Approved→Running→Completed/Failed, rollback flow), Job creation with executor image, status updates with step tracking
- [ ] T112 [P] [US5] Implement gNMI client wrapper in `services/runbook-operator/pkg/gnmic/client.go`: connection management with TLS, credentials from Secret
- [ ] T113 [P] [US5] Implement gNMI Set operations in `services/runbook-operator/pkg/gnmic/set.go`: update, replace, delete operations with JSON_IETF encoding
- [ ] T114 [P] [US5] Implement gNMI Get operations in `services/runbook-operator/pkg/gnmic/get.go`: single get and poll with retryUntil support
- [ ] T115 [P] [US5] Implement gNMI Subscribe in `services/runbook-operator/pkg/gnmic/subscribe.go`: streaming telemetry subscription for validation steps
- [ ] T116 [US5] Implement template engine in `services/runbook-operator/pkg/template/engine.go`: Go template rendering for parameter substitution in step configs, with custom functions for device resolution
- [ ] T117 [US5] Implement approval handler in `services/runbook-operator/pkg/approval/approver.go`: notification dispatch (Slack webhook, Teams webhook), approval status check via CRD status patch
- [ ] T118 [US5] Implement audit logger in `services/runbook-operator/pkg/audit/logger.go`: structured logging of all execution events (step start/end, outputs, errors, approvals) to both CRD status and stdout
- [ ] T119 [US5] Implement executor Job entrypoint in `services/runbook-operator/cmd/executor/main.go`: receive runbook spec + parameters via ConfigMap mount, execute steps sequentially (gnmi_set, gnmi_get, wait, notify, condition, script), report results back via CRD status
- [ ] T120 [US5] Create Runbook Operator Dockerfile in `services/runbook-operator/Dockerfile` and executor Dockerfile in `services/runbook-operator/Dockerfile.executor`: multi-stage, multi-arch
- [ ] T121 [US5] Create operator Deployment template in `helm/helios/charts/helios-automation/templates/deployment-operator.yaml` with leader election, RBAC (manage CRDs, create Jobs), ServiceMonitor for Prometheus metrics
- [ ] T122 [US5] Create CRD templates in `helm/helios/charts/helios-automation/templates/crd-runbook.yaml` and `crd-runbookexecution.yaml` from contracts/ spec with printer columns (Category, Risk, Phase, Duration)
- [ ] T123 [US5] Create operator RBAC in `helm/helios/charts/helios-automation/templates/rbac.yaml`: ClusterRole for CRD management, Role for Job creation in helios-automation namespace, ServiceAccount
- [ ] T124 [US5] Create operator ServiceMonitor in `helm/helios/charts/helios-automation/templates/servicemonitor.yaml` scraping operator metrics (executions_total, execution_duration, pending_approvals, active_executions)
- [ ] T125 [P] [US5] Create sample runbook: interface-bounce in `config/runbooks/samples/interface-bounce.yaml` (risk: medium, no approval, steps: gnmi_set disable → wait → gnmi_set enable → gnmi_get verify UP)
- [ ] T126 [P] [US5] Create sample runbook: collect-diagnostics in `config/runbooks/samples/collect-diagnostics.yaml` (risk: low, no approval, steps: gnmi_get system state → gnmi_get interfaces → gnmi_get BGP → notify results)
- [ ] T127 [P] [US5] Create sample runbook: clear-bgp-neighbor in `config/runbooks/samples/clear-bgp-neighbor.yaml` (risk: high, approval required, steps: gnmi_get current state → approval gate → gnmi_set clear → wait → gnmi_get verify recovered)
- [ ] T128 [P] [US5] Create sample runbooks: drain-interface, backup-config, device-maintenance-mode, exit-maintenance-mode in `config/runbooks/samples/` per runbook library spec
- [ ] T129 [P] [US5] Create vendor-specific sample runbooks: arista-mlag-check, cisco-reload-in, paloalto-commit in `config/runbooks/samples/` per runbook library spec
- [ ] T130 [US5] Create runbook execution alert rules in `config/alerts/rules/helios-runbook.yaml`: alert if pending approval >1 hour, alert on execution failure, alert on rollback triggered

**Checkpoint**: Automation complete. Runbook Operator manages CRDs, executes gNMI operations, supports approval/rollback, 10 sample runbooks deployed.

---

## Phase 8: User Story 6 — Multi-Datacenter Federation (Priority: P6)

**Goal**: Selectable storage topologies, Thanos cross-DC federation, Compactor singleton coordination

**Independent Test**: Two kind clusters with Helios → central Thanos Query → cross-DC queries return merged results

### Tests for User Story 6 ⚠️

- [ ] T131 [P] [US6] Helm unittest for storage topology selection in `helm/helios/charts/helios-storage/tests/storage-topology_test.yaml`: verify each of 4 topologies (single-dc-minio, multi-dc-minio, cloud-s3, hybrid) renders correct Thanos objstore config and MinIO manifest
- [ ] T132 [P] [US6] Helm unittest for Compactor singleton in `helm/helios/charts/helios-storage/tests/compactor-singleton_test.yaml`: verify replicas=1, PDB maxUnavailable=1 across all topologies

### Implementation for User Story 6

- [ ] T133 [US6] Add `global.storageTopology` value to `helm/helios/values.yaml` with enum (single-dc-minio, multi-dc-minio, cloud-s3, hybrid) defaulting to single-dc-minio
- [ ] T134 [US6] Create conditional Thanos objstore config in `helm/helios/charts/helios-storage/templates/thanos-objstore-secret.yaml` that renders different S3/MinIO config based on `global.storageTopology` value (update existing template with topology switch)
- [ ] T135 [US6] Create conditional MinIO manifest in `helm/helios/charts/helios-storage/templates/minio.yaml` that deploys MinIO for single-dc and multi-dc topologies, skips for cloud-s3, deploys hot-tier for hybrid (update existing template)
- [ ] T136 [US6] Add MinIO site replication config to `helm/helios/charts/helios-storage/templates/minio-site-replication.yaml` (Job) for multi-dc-minio topology: configure remote endpoint, replication rule, bucket sync
- [ ] T137 [US6] Update Thanos Query template `helm/helios/charts/helios-storage/templates/thanos-query.yaml` to accept additional `--store` endpoints from values for cross-DC StoreAPI federation
- [ ] T138 [US6] Update Thanos Compactor template `helm/helios/charts/helios-storage/templates/thanos-compactor.yaml` to disable in non-primary DCs for multi-dc topology via `global.compactor.enabled` flag
- [ ] T139 [US6] Add external_labels DC identifier to Prometheus CR in `helm/helios/charts/helios-storage/templates/prometheus.yaml` using `global.datacenter` value for cross-DC deduplication (update existing template)
- [ ] T140 [US6] Create ArgoCD ApplicationSet in `deploy/argocd/applicationset.yaml` with per-cluster generator, scale-tier and storage-topology value selection per cluster
- [ ] T141 [US6] Create storage topology values overlay files: `helm/helios/values-storage-single-dc.yaml`, `values-storage-multi-dc.yaml`, `values-storage-cloud-s3.yaml`, `values-storage-hybrid.yaml` with topology-specific MinIO/S3 configuration

**Checkpoint**: Multi-DC complete. Storage topology selectable, Thanos federation across DCs, Compactor safely singleton.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: CI/CD completion, integration testing, performance validation, documentation

- [ ] T142 [P] Create `.github/workflows/integration-test.yaml` CI workflow: spin up kind cluster + Containerlab topology, deploy Helios at Small scale, run smoke tests (metrics in Prometheus, dashboards load, flow pipeline E2E)
- [ ] T143 [P] Run `helm lint` on umbrella chart and all sub-charts, fix any warnings
- [ ] T144 [P] Run `golangci-lint` on all 3 Go services, fix any issues
- [ ] T145 Verify `go test -cover ./...` reports >70% coverage for all 3 Go services, add tests where needed
- [ ] T146 Run `helm unittest` on all chart test files, verify all pass
- [ ] T147 [P] Build multi-arch Docker images for all 3 services, verify they run on both amd64 and arm64
- [ ] T148 Validate quickstart.md procedure end-to-end: fresh kind cluster → `helm install` → first metrics in Grafana within 30 minutes
- [ ] T149 [P] Verify all 14 Grafana dashboards (8 metrics + 6 flow) load without errors and display data correctly
- [ ] T150 [P] Verify all alert rules fire correctly by simulating threshold breaches in Containerlab topology

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — BLOCKS all user stories
- **Phase 3 (US1 Metrics)**: Depends on Phase 2 — MVP delivery
- **Phase 4 (US2 Dashboards)**: Depends on Phase 2 — can start in parallel with US1 (chart templates) but dashboards need metrics data to test
- **Phase 5 (US3 Flows)**: Depends on Phase 2 — independent Go service + Helm charts, can start in parallel with US1/US2
- **Phase 6 (US4 Discovery)**: Depends on Phase 2 — Go service development can start in parallel, but integration testing needs US1
- **Phase 7 (US5 Automation)**: Depends on Phase 2 — Go service development can start in parallel, integration needs devices
- **Phase 8 (US6 Multi-DC)**: Depends on Phase 2 + Phase 3 (requires working single-DC Thanos stack)
- **Phase 9 (Polish)**: Depends on all user stories being implemented

### User Story Dependencies

- **US1 (P1) Metrics**: Independent after Phase 2. **This is the MVP.**
- **US2 (P2) Dashboards**: Chart work independent; testing requires US1 metrics data
- **US3 (P3) Flows**: Fully independent Go service; Grafana datasource update requires US2 Grafana deployment
- **US4 (P4) Discovery**: Fully independent Go service; integration test requires US1 gnmic deployment
- **US5 (P5) Automation**: Fully independent Go operator; integration test requires at least one device target
- **US6 (P6) Multi-DC**: Requires US1 Thanos stack working in single-DC first

### Within Each User Story

1. Tests (helm unittest, Go unit tests) written FIRST, verified to FAIL
2. Helm chart templates before config content
3. Go service packages before main entrypoint
4. Core logic before integration (e.g., enricher before Kafka consumer)
5. NetworkPolicies and HPAs after core components

### Parallel Opportunities

- Phase 1: T003–T010 can all run in parallel
- Phase 2: T013–T016, T018 can run in parallel (after T011, T012)
- US1: T024–T029 and T032–T033 can all run in parallel
- US2: T039–T041, T043–T050 can all run in parallel
- US3: T059–T060, T062–T063, T066–T069, T071, T076–T077, T080–T085 can run in parallel
- US4: T092–T095, T099–T100 can run in parallel
- US5: T112–T115, T125–T129 can run in parallel
- US3/US4/US5 Go service development can all run in parallel since they are separate modules

---

## Parallel Example: User Story 1

```bash
# Launch all helm unit tests in parallel:
Task: "Helm unittest for gnmic StatefulSet"
Task: "Helm unittest for snmp_exporter Deployment"
Task: "Helm unittest for blackbox_exporter Deployment"
Task: "Helm unittest for HPA"

# Launch all collection chart templates in parallel:
Task: "Create gnmic StatefulSet template"
Task: "Create gnmic config ConfigMap"
Task: "Create static targets ConfigMap"
Task: "Create gnmic headless Service"
Task: "Create snmp_exporter Deployment"
Task: "Create blackbox_exporter Deployment"

# Launch all config content in parallel:
Task: "Create SNMP modules for Arista, Cisco, Juniper, Palo Alto"
Task: "Create gNMI subscription files (counters, BGP, system)"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001–T010)
2. Complete Phase 2: Foundational (T011–T019)
3. Complete Phase 3: User Story 1 — Metrics (T020–T035)
4. **STOP and VALIDATE**: `helm install` → gnmic connects → metrics in Prometheus → Thanos Query works
5. Deploy on real hardware for validation

### Incremental Delivery

1. Setup + Foundational → Storage foundation operational
2. US1 Metrics → `helm install` produces working metrics pipeline (MVP!)
3. US2 Dashboards → Engineers can visualize and receive alerts
4. US3 Flows → Flow analytics available
5. US4 Discovery → NetBox-driven automation
6. US5 Automation → Runbook operator for remediation
7. US6 Multi-DC → Federation across datacenters

### Parallel Team Strategy

With multiple developers after Phase 2:

- **Developer A**: US1 (Helm charts for metrics collection)
- **Developer B**: US3 (Flow Enricher Go service) + US5 (Runbook Operator Go service)
- **Developer C**: US2 (Dashboards + Alerts) + US4 (Target Generator Go service)
- **Developer D**: US6 (Multi-DC topology extensions)

---

## Notes

- [P] tasks = different files, no dependencies on incomplete tasks
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Verify tests fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- All Helm templates must pass `helm lint` before merging
- All Go code must pass `golangci-lint` before merging
- Multi-arch images must be verified on both amd64 and arm64
