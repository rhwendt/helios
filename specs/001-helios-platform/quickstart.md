# Quickstart: Helios Network Observability Platform

**Goal**: First metrics visible in Grafana within 30 minutes.

## Prerequisites

- Kubernetes cluster (1.28+) with `kubectl` configured
- Helm 3.14+
- At least one network device with gNMI or SNMP enabled
- NetBox instance (optional — can use static targets for quickstart)

## Step 1: Install Helios (5 minutes)

```bash
# Add Helm repositories for dependencies
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo add minio https://operator.min.io/
helm repo add strimzi https://strimzi.io/charts/
helm repo update

# Clone the Helios repository
git clone https://github.com/rhwendt/helios.git
cd helios

# Install with Medium scale defaults (single command)
helm install helios ./helm/helios \
  -f ./helm/helios/values-medium.yaml \
  --create-namespace \
  --namespace helios-system \
  --wait --timeout 10m
```

## Step 2: Configure Device Targets (5 minutes)

### Option A: Static Targets (no NetBox)

Create a target file for your devices:

```bash
cat <<'EOF' > /tmp/my-targets.yaml
targets:
  my-router:6030:
    address: 10.0.1.1:6030
    labels:
      device: my-router
      site: lab
      vendor: arista
      platform: eos
      role: router
      tier: standard
EOF

kubectl create configmap helios-gnmic-targets \
  --from-file=targets.yaml=/tmp/my-targets.yaml \
  -n helios-collection \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Option B: NetBox Integration

```bash
# Configure NetBox connection
kubectl create secret generic helios-netbox-credentials \
  -n helios-integration \
  --from-literal=url=https://netbox.example.com \
  --from-literal=token=your-netbox-api-token

# The Target Generator CronJob runs every 5 minutes automatically.
# Ensure your devices in NetBox have:
#   - Custom field: helios_monitor = true
#   - Custom field: gnmi_enabled = true (for gNMI devices)
#   - Custom field: gnmi_port = 6030 (or your port)
#   - Primary IP assigned
```

## Step 3: Configure Device Credentials (5 minutes)

```bash
# gNMI credentials (for gNMI-enabled devices)
kubectl create secret generic helios-gnmi-credentials \
  -n helios-collection \
  --from-literal=username=admin \
  --from-literal=password=your-password

# SNMP credentials (for SNMP-enabled devices)
kubectl create secret generic helios-snmp-credentials \
  -n helios-collection \
  --from-literal=community=your-community-string
```

## Step 4: Access Grafana (2 minutes)

```bash
# Port-forward Grafana
kubectl port-forward svc/helios-grafana 3000:3000 -n helios-visualization &

# Open in browser
echo "Grafana: http://localhost:3000"
echo "Default credentials: admin / helios-admin (change on first login)"
```

## Step 5: Verify Metrics (5 minutes)

1. Open Grafana at http://localhost:3000
2. Navigate to **Dashboards → Helios → Network Overview**
3. Select your site from the dropdown
4. Verify you see:
   - Device count
   - Interface utilization graphs
   - Device availability status

### Troubleshooting

```bash
# Check gnmic is running and connected
kubectl logs -l app=gnmic -n helios-collection --tail=50

# Check Prometheus is scraping
kubectl port-forward svc/prometheus-operated 9090:9090 -n helios-storage &
# Open http://localhost:9090/targets

# Check target sync status
kubectl get configmap helios-gnmic-targets -n helios-collection -o yaml

# Verify all pods are healthy
kubectl get pods -A -l app.kubernetes.io/part-of=helios
```

## What You Get Out of the Box

### Dashboards (pre-provisioned)
- Network Overview
- Interface Analytics
- Device Health
- Site Overview
- Latency & Reachability
- BGP Status
- Top Talkers (metrics)
- Alerts Overview

### Alert Rules (pre-configured)
- Device unreachable (2 minutes)
- Interface error rate > 1%
- BGP session state change
- CPU > 80%
- Memory > 85%

### Next Steps

- **Add flow collection**: Enable NetFlow/sFlow on your devices, pointing to
  the goflow2 service IP on ports 2055 (NetFlow) / 6343 (sFlow)
- **Configure alerting**: Set up Alertmanager receivers in
  `helm/helios/values.yaml` (PagerDuty, Slack, email)
- **Deploy runbooks**: `kubectl apply -f config/runbooks/samples/`
- **Scale up**: Switch to `values-large.yaml` or `values-xl.yaml` as your
  device count grows
