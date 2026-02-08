# Implementation Plan: Helios Network Observability Platform

**Branch**: `001-helios-platform` | **Date**: 2026-02-07 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-helios-platform/spec.md`

## Summary

Build the complete Helios network observability platform: a Kubernetes-native
system providing unified gNMI/SNMP metrics collection, NetFlow/IPFIX/sFlow
analysis, long-term tiered storage, pre-built Grafana dashboards, tiered
alerting, automated runbook execution, and multi-datacenter federation. The
platform is deployed as a Helm umbrella chart with GitOps delivery via ArgoCD,
targeting mixed amd64+arm64 clusters scaling from 50 to 8,000+ network devices.

## Technical Context

**Language/Version**: Go 1.22+ (custom services: Flow Enricher, Runbook Operator, Target Generator), YAML/JSON (Helm charts, dashboards, alert rules)
**Primary Dependencies**: gnmic, Prometheus (Operator), Thanos, Grafana, ClickHouse (Altinity Operator), Kafka (Strimzi), goflow2, MinIO, Alertmanager, NetBox, ArgoCD
**Storage**: Prometheus (short-term metrics, 2h TSDB blocks), Thanos + MinIO/S3 (long-term metrics, 90-day), ClickHouse (flow data, tiered TTL), Kafka (flow message queue), PostgreSQL (Grafana backend)
**Testing**: `go test` (unit), `helm unittest` (chart rendering), `helm lint`, `golangci-lint`, Containerlab + kind (integration), staging cluster (E2E)
**Target Platform**: Kubernetes (mixed x86_64 + aarch64), Cilium CNI, 3x Miniforum MS-01 + 5x Raspberry Pi 5 8GB
**Project Type**: Kubernetes platform — Helm umbrella chart with sub-charts + 3 custom Go services + config repository
**Performance Goals**: <30s metric ingestion-to-query, 500K FPS flow ingestion at Large scale, <5s dashboard query (24h range), <5min NetBox target sync, <10s runbook execution overhead
**Constraints**: Multi-arch images required (amd64+arm64), 6 isolated namespaces, ESO for secrets (no secrets in Git), NetworkPolicies for egress control, Thanos Compactor singleton
**Scale/Scope**: 50–8,000+ network devices, 4 scale tiers (Small/Medium/Large/XL), multi-vendor (Arista, Cisco, Juniper, Palo Alto, Fortigate, F5, Cumulus), multi-datacenter

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| # | Principle | Status | Evidence |
|---|-----------|--------|----------|
| I | Cloud-Native First | PASS | All components deploy on K8s via Helm umbrella chart. ArgoCD ApplicationSets for GitOps. Multi-arch container images for custom services. No bare-metal dependencies. |
| II | Horizontal Scalability | PASS | 4 scale-tier values files (small/medium/large/xl). gnmic HPA, goflow2 HPA, Flow Enricher HPA. Prometheus sharding via Operator `shards` field. Kafka partition scaling. ClickHouse sharding via Altinity Operator. |
| III | Vendor Agnostic | PASS | gNMI/OpenConfig, SNMP, NetFlow/IPFIX/sFlow only. NetBox as SoT. Vendor logic in config files (gnmic subscriptions, SNMP modules), never in application code. Consistent label taxonomy. |
| IV | Extensibility via Git | PASS | helios-config/ repository with CI validation (snmp-exporter --config.check, gnmic --dry-run, promtool check rules, dashboard JSON schema). ConfigMap generation pipeline. ArgoCD sync. No manual Grafana UI changes. |
| V | Operational Simplicity | PASS | 8 core metrics dashboards + 6 flow dashboards provisioned automatically. Default alert rules included. 10 sample runbooks shipped. Single `helm install` with Medium defaults. Quickstart guide targeting <30min to first metrics. |
| VI | Long-Term Retention | PASS | Thanos: raw 14d, 5m downsampled 30d, 1h downsampled 90d. ClickHouse TTL: raw flows 7d, 1m aggregates 30d, 1h aggregates 90d. Selectable storage topologies (Single-DC MinIO, Multi-DC MinIO, Cloud S3, Hybrid). |

**Gate result**: ALL PASS — proceed to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/001-helios-platform/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── runbook-crd.yaml
│   ├── runbookexecution-crd.yaml
│   ├── flow.proto
│   └── target-generator-output.yaml
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
helios/
├── helm/
│   └── helios/                          # Umbrella chart
│       ├── Chart.yaml                   # Dependencies: kube-prometheus-stack, thanos, minio
│       ├── values.yaml                  # Global defaults (Medium scale)
│       ├── values-small.yaml
│       ├── values-medium.yaml
│       ├── values-large.yaml
│       ├── values-xl.yaml
│       ├── charts/
│       │   ├── helios-integration/      # NetBox sync, target generation
│       │   │   └── templates/
│       │   │       ├── cronjob-target-sync.yaml
│       │   │       ├── configmap-netbox.yaml
│       │   │       └── rbac.yaml
│       │   ├── helios-collection/       # gnmic, SNMP, blackbox exporters
│       │   │   └── templates/
│       │   │       ├── statefulset-gnmic.yaml
│       │   │       ├── deployment-snmp-exporter.yaml
│       │   │       ├── deployment-blackbox-exporter.yaml
│       │   │       ├── configmap-gnmic.yaml
│       │   │       ├── configmap-targets.yaml
│       │   │       ├── hpa-gnmic.yaml
│       │   │       └── servicemonitor.yaml
│       │   ├── helios-storage/          # Prometheus, Thanos, MinIO
│       │   │   └── templates/
│       │   │       ├── prometheus.yaml          # Prometheus Operator CR
│       │   │       ├── thanos-objstore-secret.yaml
│       │   │       ├── thanos-query.yaml
│       │   │       ├── thanos-compactor.yaml
│       │   │       ├── thanos-store.yaml
│       │   │       ├── thanos-ruler.yaml
│       │   │       └── minio.yaml
│       │   ├── helios-visualization/    # Grafana, Alertmanager
│       │   │   └── templates/
│       │   │       ├── grafana-deployment.yaml
│       │   │       ├── grafana-datasources.yaml
│       │   │       ├── grafana-dashboards/      # 14 dashboard ConfigMaps
│       │   │       ├── alertmanager-config.yaml
│       │   │       └── prometheus-rules.yaml
│       │   ├── helios-automation/       # Runbook Operator
│       │   │   └── templates/
│       │   │       ├── deployment-operator.yaml
│       │   │       ├── crd-runbook.yaml
│       │   │       ├── crd-runbookexecution.yaml
│       │   │       ├── rbac.yaml
│       │   │       └── servicemonitor.yaml
│       │   └── helios-flows/            # goflow2, Kafka, Flow Enricher, ClickHouse
│       │       └── templates/
│       │           ├── statefulset-goflow2.yaml
│       │           ├── service-goflow2-udp.yaml
│       │           ├── kafka-cluster.yaml       # Strimzi CR
│       │           ├── kafka-topics.yaml
│       │           ├── deployment-flow-enricher.yaml
│       │           ├── hpa-goflow2.yaml
│       │           ├── hpa-flow-enricher.yaml
│       │           ├── clickhouse-installation.yaml  # Altinity CR
│       │           ├── clickhouse-schema.yaml
│       │           └── networkpolicy.yaml
│       └── templates/
│           ├── namespaces.yaml
│           └── networkpolicies-global.yaml
├── services/
│   ├── flow-enricher/                   # Custom Go service
│   │   ├── cmd/
│   │   │   └── flow-enricher/
│   │   │       └── main.go
│   │   ├── internal/
│   │   │   ├── enricher/               # NetBox + GeoIP lookup logic
│   │   │   │   ├── enricher.go
│   │   │   │   ├── netbox.go
│   │   │   │   ├── geoip.go
│   │   │   │   └── enricher_test.go
│   │   │   ├── kafka/                  # Consumer/producer
│   │   │   │   ├── consumer.go
│   │   │   │   ├── producer.go
│   │   │   │   └── kafka_test.go
│   │   │   └── proto/                  # Generated protobuf
│   │   │       └── flow.pb.go
│   │   ├── proto/
│   │   │   └── flow.proto
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   └── go.sum
│   ├── runbook-operator/                # Custom K8s operator (kubebuilder)
│   │   ├── api/v1alpha1/
│   │   │   ├── runbook_types.go
│   │   │   ├── runbookexecution_types.go
│   │   │   ├── groupversion_info.go
│   │   │   └── zz_generated.deepcopy.go
│   │   ├── controllers/
│   │   │   ├── runbook_controller.go
│   │   │   ├── runbookexecution_controller.go
│   │   │   └── controller_test.go
│   │   ├── pkg/
│   │   │   ├── gnmic/                  # gNMI client wrapper
│   │   │   │   ├── client.go
│   │   │   │   ├── set.go
│   │   │   │   ├── get.go
│   │   │   │   └── subscribe.go
│   │   │   ├── template/               # Go template engine for parameters
│   │   │   │   └── engine.go
│   │   │   ├── approval/               # Notification integrations
│   │   │   │   └── approver.go
│   │   │   └── audit/                  # Audit logging
│   │   │       └── logger.go
│   │   ├── cmd/
│   │   │   └── executor/
│   │   │       └── main.go             # Executor Job entrypoint
│   │   ├── config/
│   │   │   ├── crd/bases/
│   │   │   ├── rbac/
│   │   │   └── manager/
│   │   ├── Dockerfile
│   │   ├── Dockerfile.executor
│   │   ├── go.mod
│   │   └── main.go
│   └── target-generator/               # NetBox sync CronJob
│       ├── cmd/
│       │   └── target-generator/
│       │       └── main.go
│       ├── internal/
│       │   ├── netbox/                 # NetBox API client
│       │   │   ├── client.go
│       │   │   └── client_test.go
│       │   ├── generator/              # ConfigMap/SD generation
│       │   │   ├── gnmic.go
│       │   │   ├── snmp.go
│       │   │   ├── blackbox.go
│       │   │   ├── prometheus_sd.go
│       │   │   └── generator_test.go
│       │   └── kubernetes/             # K8s ConfigMap writer
│       │       └── configmap.go
│       ├── Dockerfile
│       ├── go.mod
│       └── go.sum
├── config/                              # helios-config repository content
│   ├── snmp/
│   │   └── modules/                    # SNMP exporter modules
│   ├── gnmic/
│   │   └── subscriptions/              # gNMI subscription definitions
│   ├── dashboards/
│   │   ├── metrics/                    # 8 core metrics dashboards (JSON)
│   │   └── flows/                      # 6 flow dashboards (JSON)
│   ├── alerts/
│   │   └── rules/                      # PrometheusRule YAML files
│   └── runbooks/
│       └── samples/                    # 10 sample runbook YAMLs
├── deploy/
│   └── argocd/
│       └── applicationset.yaml         # ArgoCD ApplicationSet
├── clickhouse/
│   └── migrations/
│       ├── 001_create_flows_raw.sql
│       ├── 002_create_flows_1m.sql
│       ├── 003_create_flows_1h.sql
│       └── 004_create_materialized_views.sql
├── proto/
│   └── flow.proto                      # Canonical protobuf definition
├── .github/
│   └── workflows/
│       ├── validate-config.yaml        # CI: lint SNMP, gnmic, dashboards, alerts
│       ├── build-services.yaml         # CI: build + push multi-arch images
│       ├── helm-lint.yaml              # CI: helm lint + unittest
│       └── integration-test.yaml       # CI: Containerlab + kind
├── .golangci.yml
├── Makefile
└── README.md
```

**Structure Decision**: Kubernetes platform layout with Helm umbrella chart at
`helm/helios/`, three custom Go services under `services/`, declarative config
under `config/`, ClickHouse migrations under `clickhouse/`, and ArgoCD
deployment manifests under `deploy/`. This separates concerns cleanly: Helm
charts are the deployment mechanism, services are the custom code, config is the
user-extensible content, and deploy is the GitOps glue.

## Complexity Tracking

> No constitution violations — table intentionally empty.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
