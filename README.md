# Helios

**CNCF-native network observability platform for Kubernetes.**

Helios provides unified telemetry collection, flow analysis, visualization, and automated remediation for networks of 50 to 8,000+ devices across multiple datacenters.

## Features

- **Streaming Metrics** — gNMI and SNMP collection with Prometheus sharding and Thanos federation
- **Flow Analysis** — NetFlow/IPFIX/sFlow ingestion via goflow2, enriched with NetBox metadata and GeoIP, stored in ClickHouse with automatic rollup
- **14 Pre-built Dashboards** — 8 metrics dashboards (device health, interface analytics, BGP, latency) and 6 flow dashboards (top talkers, traffic matrix, AS paths, geographic)
- **Automated Runbooks** — Kubernetes CRDs for gNMI-driven remediation with approval workflows, rollback, and audit trails
- **NetBox Integration** — Automatic device discovery and target generation from NetBox inventory
- **Multi-datacenter** — Thanos federation, MinIO site replication, ArgoCD GitOps deployment
- **Scale Tiers** — Pre-tuned profiles from 50 devices (small) to 8,000+ (XL)

## Architecture

Helios runs in six isolated Kubernetes namespaces:

```
helios-integration    NetBox sync, target generation
helios-collection     gnmic, SNMP exporter, blackbox exporter
helios-storage        Prometheus (sharded), Thanos, MinIO
helios-visualization  Grafana, Alertmanager, alert rules
helios-automation     Runbook Operator, CRDs
helios-flows          goflow2, Kafka (Strimzi), Flow Enricher, ClickHouse
```

### Data Flow

```
Network Devices
  │
  ├── gNMI/SNMP ──→ gnmic/snmp_exporter ──→ Prometheus ──→ Thanos ──→ Grafana
  │
  └── NetFlow/sFlow ──→ goflow2 ──→ Kafka ──→ Flow Enricher ──→ ClickHouse ──→ Grafana
                                                  │
                                          NetBox + GeoIP enrichment
```

### Custom Services

| Service | Purpose |
|---------|---------|
| **Flow Enricher** | Consumes raw flows from Kafka, enriches with NetBox device metadata and MaxMind GeoIP, produces to ClickHouse |
| **Target Generator** | CronJob that queries NetBox for monitored devices and generates collector ConfigMaps |
| **Runbook Operator** | Kubernetes operator managing Runbook/RunbookExecution CRDs with approval workflows |

## Quick Start

### Prerequisites

- Kubernetes 1.28+
- Helm 3.14+
- Prometheus Operator, Strimzi, and Altinity ClickHouse Operator (installed by chart dependencies)

### Install

```bash
helm install helios ./helm/helios \
  -f ./helm/helios/values-medium.yaml \
  --namespace helios-system \
  --create-namespace \
  --wait --timeout 10m
```

### Configure Device Targets

**Option A: NetBox integration** (recommended)

Set `netbox.url` and `netbox.apiToken` in your values. Devices with the `helios_monitor: true` custom field are automatically discovered.

**Option B: Static targets**

Edit the targets ConfigMap in `helios-collection` with your device IPs and credentials.

### Create Secrets

```bash
# gNMI credentials
kubectl create secret generic gnmic-credentials \
  --from-literal=username=admin \
  --from-literal=password=changeme \
  -n helios-collection

# SNMP community
kubectl create secret generic snmp-credentials \
  --from-literal=community=public \
  -n helios-collection
```

### Access Grafana

```bash
kubectl port-forward svc/grafana 3000:3000 -n helios-visualization
# Open http://localhost:3000 (admin/admin)
```

## Scale Tiers

| Tier | Devices | Prometheus Shards | gnmic Replicas | ClickHouse |
|------|---------|-------------------|----------------|------------|
| Small | 50–200 | 1 | 2 | 1 shard |
| Medium | 50–500 | 2 | 3 | 1 shard × 2 replicas |
| Large | 500–2,000 | 4 | 5 | 2 shards × 2 replicas |
| XL | 2,000–8,000+ | 8 | 10 | 4 shards × 2 replicas |

```bash
# Select a tier at install time
helm install helios ./helm/helios -f ./helm/helios/values-large.yaml
```

### Storage Topologies

Overlay a storage profile on top of your scale tier:

```bash
# Multi-datacenter with MinIO site replication
helm install helios ./helm/helios \
  -f ./helm/helios/values-large.yaml \
  -f ./helm/helios/values-storage-multi-dc.yaml

# Cloud S3 backend
helm install helios ./helm/helios \
  -f ./helm/helios/values-medium.yaml \
  -f ./helm/helios/values-storage-cloud-s3.yaml
```

Available: `values-storage-single-dc.yaml`, `values-storage-multi-dc.yaml`, `values-storage-cloud-s3.yaml`, `values-storage-hybrid.yaml`

## Data Retention

| Data | Raw | 5-min Downsample | 1-hour Downsample |
|------|-----|-------------------|-------------------|
| Metrics (Prometheus/Thanos) | 14 days | 30 days | 90 days |
| Flows (ClickHouse) | 7 days | 30 days (1-min agg) | 90 days (1-hour agg) |

ClickHouse materialized views automatically populate rollup tables. TTL-based deletion requires no manual intervention.

## Dashboards

### Metrics
- Network Overview — device count, utilization, availability
- Interface Analytics — traffic, errors, discards
- Device Health — CPU, memory, uptime
- Site Overview — aggregated site metrics
- Latency & Reachability — blackbox probe results
- BGP Status — neighbor state, session changes
- Top Talkers — metrics-based
- Alerts Overview — firing alerts, trends

### Flows
- Top Talkers — IP pairs, protocols, bytes
- Traffic Matrix — AS-to-AS, site-to-site
- AS Paths — BGP route diversity
- Protocol Distribution — IPv4/IPv6/TCP/UDP breakdown
- Geographic Flows — source/destination countries
- Interface Correlation — cross-datasource (flows + metrics)

## Alerting

Pre-configured alert rules include:

- Device unreachable (2-minute threshold)
- Interface error rate > 1%
- BGP session state change
- CPU > 80%, Memory > 85%
- Flow rate anomaly (>2 std dev from baseline)
- Target sync failure (>15 minutes stale)

Alerts route through Alertmanager with support for PagerDuty, Slack, and email.

## Runbook Automation

Define remediation procedures as Kubernetes CRDs:

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: interface-bounce
spec:
  description: "Bounce a network interface"
  riskLevel: medium
  requiresApproval: true
  parameters:
    - name: device
      type: string
      required: true
    - name: interface
      type: string
      required: true
  steps:
    - name: disable-interface
      type: gnmi-set
      config:
        path: "/interfaces/interface[name={{.interface}}]/config/enabled"
        value: "false"
    - name: wait
      type: wait
      config:
        duration: "10s"
    - name: enable-interface
      type: gnmi-set
      config:
        path: "/interfaces/interface[name={{.interface}}]/config/enabled"
        value: "true"
```

Execute with:
```yaml
apiVersion: helios.io/v1alpha1
kind: RunbookExecution
metadata:
  name: bounce-eth1-router1
spec:
  runbookRef:
    name: interface-bounce
  parameters:
    device: "router1.dc1.example.com"
    interface: "Ethernet1"
```

## GitOps Deployment

An ArgoCD ApplicationSet is provided for multi-cluster deployment:

```bash
kubectl apply -f deploy/argocd/applicationset.yaml
```

Clusters are selected by labels: `helios.io/enabled`, `helios.io/scale-tier`, `helios.io/datacenter`. Scale tier and storage topology values are automatically injected.

## Development

### Build

```bash
make all          # lint + test + build
make build        # build service binaries
make test         # run tests with -race
make lint         # golangci-lint
make proto-gen    # regenerate protobuf Go code
make docker-build # multi-arch Docker images
make helm-lint    # lint Helm charts
make helm-unittest # chart unit tests
```

### Project Structure

```
helios/
├── helm/helios/                    # Umbrella Helm chart + sub-charts
├── services/
│   ├── flow-enricher/              # Kafka → enrich → ClickHouse
│   ├── target-generator/           # NetBox → ConfigMaps
│   └── runbook-operator/           # Runbook CRD operator
├── config/
│   ├── alerts/rules/               # PrometheusRule definitions
│   ├── dashboards/{metrics,flows}/ # 14 Grafana dashboards
│   ├── gnmic/subscriptions/        # gNMI subscription configs
│   ├── snmp/modules/               # SNMP exporter modules
│   └── runbooks/samples/           # 10 sample runbooks
├── clickhouse/migrations/          # ClickHouse schema DDL
├── proto/                          # Protobuf definitions
├── deploy/argocd/                  # GitOps ApplicationSet
├── .github/workflows/              # CI/CD pipelines
└── tests/topology/                 # Containerlab test topology
```

### Service Configuration

| Variable | Service | Description |
|----------|---------|-------------|
| `KAFKA_BROKERS` | Flow Enricher | Kafka bootstrap servers |
| `NETBOX_API_URL` | Flow Enricher | NetBox API endpoint |
| `NETBOX_API_TOKEN` | Flow Enricher, Target Generator | NetBox API token |
| `GEOIP_CITY_DB` | Flow Enricher | Path to MaxMind GeoLite2-City database |
| `GEOIP_ASN_DB` | Flow Enricher | Path to MaxMind GeoLite2-ASN database |
| `TARGET_NAMESPACE` | Target Generator | Namespace for generated ConfigMaps |
| `EXECUTOR_IMAGE` | Runbook Operator | Container image for runbook job pods |

### Docker Images

All services use multi-stage builds with `gcr.io/distroless/static-debian12:nonroot` as the runtime base. Multi-arch support for `linux/amd64` and `linux/arm64`.

```bash
# Build all images
REGISTRY=ghcr.io/your-org/helios make docker-build
```

## CI/CD

GitHub Actions workflows:

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `build-services` | PRs touching `services/` | Go tests, lint, multi-arch image build |
| `helm-lint` | PRs touching `helm/` | Helm lint + chart unit tests |
| `validate-config` | PRs touching `config/` | YAML schema validation |
| `integration-test` | PRs to `main` | Containerlab + kind cluster + end-to-end |

## License

See [LICENSE](LICENSE) for details.
