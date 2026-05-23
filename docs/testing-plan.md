# Helios Testing Plan

## Option A: Minikube on Workstation (Recommended for initial testing)

Ryzen 7800X3D, 64GB RAM, amd64. Plenty of resources to run the full stack on a single node.

### Phase 0: Prerequisites

#### 0.1 Start minikube
```bash
# Give it plenty of resources — Helios runs Kafka, ClickHouse, Prometheus, etc.
minikube start \
  --cpus=8 \
  --memory=32g \
  --disk-size=100g \
  --driver=docker \
  --kubernetes-version=v1.29.0

# Verify
minikube status
kubectl get nodes
```

#### 0.2 Enable addons
```bash
minikube addons enable metrics-server
minikube addons enable storage-provisioner
# storage-provisioner gives you a default StorageClass for PVCs
kubectl get storageclass
```

#### 0.3 Build and load images into minikube
```bash
# Point docker CLI at minikube's daemon (avoids needing a registry)
eval $(minikube docker-env)

cd ~/git/helios
for svc in flow-enricher target-generator runbook-operator; do
  echo "==> Building ${svc}"
  docker build \
    --tag ghcr.io/rhwendt/helios/${svc}:dev \
    --file services/${svc}/Dockerfile \
    services/${svc}
done

# Verify images are loaded
docker images | grep helios
```

> With `eval $(minikube docker-env)`, images built locally are immediately available to minikube pods. Set `imagePullPolicy: IfNotPresent` or `Never` in your values to avoid pulling from a registry.

---

## Option B: Raspberry Pi 5 Cluster (for realistic multi-node testing)

5x Raspberry Pi 5 (arm64). Resource-constrained.

<details>
<summary>Click to expand Pi cluster layout</summary>

| Node | Role | Workloads |
|------|------|-----------|
| pi-1 | control-plane | k8s API, etcd, ArgoCD (optional) |
| pi-2 | worker | Prometheus, Thanos Query, Grafana, Alertmanager |
| pi-3 | worker | Kafka (Strimzi, single broker), goflow2, Flow Enricher |
| pi-4 | worker | ClickHouse (single shard, single replica), MinIO |
| pi-5 | worker | gnmic, SNMP exporter, blackbox exporter, Target Generator, Runbook Operator |

Use `values-small.yaml` profile. Memory is the bottleneck — use USB SSD boot for Kafka/ClickHouse nodes.

Build arm64 images:
```bash
cd ~/git/helios
for svc in flow-enricher target-generator runbook-operator; do
  docker build \
    --tag ghcr.io/rhwendt/helios/${svc}:dev \
    --file services/${svc}/Dockerfile \
    services/${svc}
done
# Load into k3s: sudo k3s ctr images import <image.tar>
```
</details>

---

## Phase 1: Install Operators (Day 1)

Helios depends on several operators. Install them first and verify they're healthy before deploying Helios itself.

### 1.1 Prometheus Operator
```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus-operator prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set prometheus.enabled=false \
  --set alertmanager.enabled=false \
  --set grafana.enabled=false \
  --set nodeExporter.enabled=false \
  --wait --timeout 5m
```
**Validate:** `kubectl get crd prometheuses.monitoring.coreos.com`

### 1.2 Strimzi Kafka Operator
```bash
helm repo add strimzi https://strimzi.io/charts/
helm install strimzi strimzi/strimzi-kafka-operator \
  --namespace kafka --create-namespace \
  --set resources.requests.memory=256Mi \
  --wait --timeout 5m
```
**Validate:** `kubectl get crd kafkas.kafka.strimzi.io`

### 1.3 Altinity ClickHouse Operator
```bash
helm repo add altinity https://altinity.github.io/clickhouse-operator
helm install clickhouse-operator altinity/altinity-clickhouse-operator \
  --namespace clickhouse --create-namespace \
  --wait --timeout 5m
```
**Validate:** `kubectl get crd clickhouseinstallations.clickhouse.altinity.com`

### 1.4 Verify all operators running
```bash
kubectl get pods -A | grep -E '(operator|controller)'
# All should be Running 1/1
```

**CHECKPOINT:** All three CRDs exist and operator pods are Running. If any operator won't start on Pi (OOM, image pull issues), troubleshoot before proceeding.

---

## Phase 2: Deploy Helios (Day 1-2)

### 2.1 Install
```bash
cd ~/git/helios

# Minikube (64GB workstation) — use medium profile, you have the resources
helm install helios ./helm/helios \
  -f ./helm/helios/values-medium.yaml \
  --namespace helios-system \
  --create-namespace \
  --wait --timeout 15m

# Pi cluster — use small profile
# helm install helios ./helm/helios \
#   -f ./helm/helios/values-small.yaml \
#   --namespace helios-system \
#   --create-namespace \
#   --wait --timeout 15m
```

> If this times out, install without `--wait` and watch pods come up:
> ```bash
> watch kubectl get pods -A | grep helios
> ```

### 2.2 Check every namespace
```bash
for ns in helios-integration helios-collection helios-storage \
          helios-visualization helios-automation helios-flows; do
  echo "=== $ns ==="
  kubectl get pods -n $ns
done
```

### 2.3 Document what's running vs failing
Create a checklist — mark each component:

| Component | Namespace | Status | Notes |
|-----------|-----------|--------|-------|
| Prometheus | helios-storage | | |
| Thanos Query | helios-storage | | |
| Thanos Store | helios-storage | | |
| Thanos Compactor | helios-storage | | |
| MinIO | helios-storage | | |
| Grafana | helios-visualization | | |
| Alertmanager | helios-visualization | | |
| Kafka | helios-flows | | |
| goflow2 | helios-flows | | |
| Flow Enricher | helios-flows | | |
| ClickHouse | helios-flows | | |
| gnmic | helios-collection | | |
| SNMP exporter | helios-collection | | |
| Blackbox exporter | helios-collection | | |
| Target Generator | helios-integration | | |
| Runbook Operator | helios-automation | | |

**CHECKPOINT:** All pods Running or clearly failing with a reason you can investigate. OOMKilled is the most likely issue on Pis — reduce resource requests in values.

### 2.4 Common Pi-specific issues

**OOMKilled:** Reduce memory requests. Create a `values-pi.yaml` overlay:
```yaml
# values-pi.yaml — resource overrides for Raspberry Pi 5
kafka:
  resources:
    requests:
      memory: 512Mi
      cpu: 250m
    limits:
      memory: 1Gi

clickhouse:
  resources:
    requests:
      memory: 512Mi
      cpu: 250m
    limits:
      memory: 1Gi

prometheus:
  resources:
    requests:
      memory: 512Mi
    limits:
      memory: 1Gi
```

**Image pull failures:** Some upstream images may not have arm64 variants. Check:
```bash
kubectl describe pod <failing-pod> -n <namespace> | grep -A5 "Events"
```

---

## Phase 3: Validate Metrics Pipeline (Day 2-3)

### 3.1 Verify Prometheus is scraping
```bash
kubectl port-forward svc/prometheus-operated 9090:9090 -n helios-storage &

# Check targets
curl -s http://localhost:9090/api/v1/targets | python3 -m json.tool | head -50

# Check a basic metric exists
curl -s 'http://localhost:9090/api/v1/query?query=up' | python3 -m json.tool
```
**Pass criteria:** At least 1 target is "up", `up` metric returns results.

### 3.2 Add a test scrape target
If you don't have real network devices, deploy a dummy exporter:
```bash
# Deploy prometheus node-exporter on one Pi as a test target
kubectl apply -f - <<'EOF'
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: test-node-exporter
  namespace: helios-collection
spec:
  selector:
    matchLabels:
      app: node-exporter
  endpoints:
    - port: metrics
      interval: 30s
EOF
```

### 3.3 Verify Thanos Query
```bash
kubectl port-forward svc/thanos-query 10902:10902 -n helios-storage &
curl -s 'http://localhost:10902/api/v1/query?query=up' | python3 -m json.tool
```
**Pass criteria:** Thanos Query returns same results as Prometheus (federated).

### 3.4 Verify Grafana
```bash
kubectl port-forward svc/grafana 3000:3000 -n helios-visualization &
# Open http://localhost:3000
# Default: admin/admin

# Or use minikube service (opens browser automatically):
# minikube service grafana -n helios-visualization
```
**Pass criteria:**
- [ ] Grafana loads
- [ ] Thanos datasource is configured and working (green checkmark in Data Sources)
- [ ] Network Overview dashboard loads (even if empty)
- [ ] At least one dashboard shows data if Prometheus has targets

---

## Phase 4: Validate Flow Pipeline (Day 3-4)

This is the most complex path: goflow2 → Kafka → Flow Enricher → ClickHouse.

### 4.1 Verify Kafka is running
```bash
kubectl get kafka -n helios-flows
kubectl get kafkatopic -n helios-flows
```
**Pass criteria:** Kafka cluster is Ready, topics `helios-flows-raw` and `helios-flows-enriched` exist.

### 4.2 Send test flows to goflow2
From any machine that can reach the cluster:
```bash
# Find goflow2 NodePort or port-forward
kubectl port-forward svc/goflow2-udp 2055:2055 -n helios-flows &

# Send a test NetFlow v5 packet using nflow-generator (or nfgen)
# Install: go install github.com/nerdalert/nflow-generator@latest
nflow-generator -t 127.0.0.1 -p 2055 -c 100
```

If you don't have nflow-generator, use a simple Python script:
```python
#!/usr/bin/env python3
"""Send a minimal NetFlow v5 packet for testing."""
import socket, struct, time

# NetFlow v5 header + 1 flow record
header = struct.pack('!HHIIIIBBH',
    5,           # version
    1,           # count (1 flow)
    int(time.time() * 1000) & 0xFFFFFFFF,  # sysUptime
    int(time.time()),  # unix_secs
    0,           # unix_nsecs
    0,           # flow_sequence
    0,           # engine_type
    0,           # engine_id
    0,           # sampling_interval
)

flow = struct.pack('!IIIHHIIIIHHBBBBHHBBH',
    0x0A000001,  # srcaddr 10.0.0.1
    0x0A000002,  # dstaddr 10.0.0.2
    0,           # nexthop
    0, 0,        # input/output interface
    1000,        # dPkts
    64000,       # dOctets
    0, 0,        # first/last
    8080,        # srcport
    443,         # dstport
    0,           # pad1
    6,           # tcp_flags
    6,           # prot (TCP)
    0,           # tos
    0, 0,        # src_as, dst_as
    0, 0,        # src_mask, dst_mask
    0,           # pad2
)

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.sendto(header + flow, ('127.0.0.1', 2055))
print('Sent 1 NetFlow v5 record')
```

### 4.3 Verify flow reaches Kafka
```bash
# Exec into Kafka pod and consume from raw topic
kubectl exec -it helios-kafka-0 -n helios-flows -- \
  bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic helios-flows-raw \
  --from-beginning --max-messages 1 --timeout-ms 10000
```
**Pass criteria:** At least 1 message appears in the topic.

### 4.4 Verify Flow Enricher is processing
```bash
# Check enricher logs
kubectl logs -l app=flow-enricher -n helios-flows --tail=20

# Check enricher metrics
kubectl port-forward svc/flow-enricher 8080:8080 -n helios-flows &
curl -s http://localhost:8080/metrics | grep flows
```
**Pass criteria:** Logs show flow processing. Metrics show `flows_processed_total > 0`.

### 4.5 Verify ClickHouse has data
```bash
kubectl exec -it chi-helios-clickhouse-0-0-0 -n helios-flows -- \
  clickhouse-client --query "SELECT count() FROM flows_raw"
```
**Pass criteria:** Count > 0.

```bash
# Verify materialized views are populating
kubectl exec -it chi-helios-clickhouse-0-0-0 -n helios-flows -- \
  clickhouse-client --query "SELECT count() FROM flows_1m"
```

### 4.6 Verify flow dashboard in Grafana
- Open Grafana → Top Talkers dashboard
- Set time range to "Last 15 minutes"
- **Pass criteria:** Flow data appears in at least one panel

---

## Phase 5: Validate NetBox Integration (Day 4)

NetBox is an external dependency — Helios connects to it but doesn't deploy it. For testing, run NetBox locally with Docker Compose alongside your minikube cluster.

### 5.1 Deploy NetBox with Docker Compose

Create a directory for the NetBox test instance:
```bash
mkdir -p ~/helios-test/netbox && cd ~/helios-test/netbox
```

Create `docker-compose.yml`:
```yaml
# ~/helios-test/netbox/docker-compose.yml
services:
  netbox:
    image: netboxcommunity/netbox:v4.1
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    ports:
      - "8000:8080"
    environment:
      SUPERUSER_API_TOKEN: "helios-test-token-0123456789abcdef"
      SUPERUSER_EMAIL: "admin@example.com"
      SUPERUSER_NAME: "admin"
      SUPERUSER_PASSWORD: "admin"
      ALLOWED_HOSTS: "*"
      DB_HOST: postgres
      DB_NAME: netbox
      DB_USER: netbox
      DB_PASSWORD: netbox
      REDIS_HOST: redis
      REDIS_PASSWORD: ""
      REDIS_CACHE_HOST: redis
      REDIS_CACHE_PASSWORD: ""
      SECRET_KEY: "test-secret-key-not-for-production-use-change-me"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/api/"]
      interval: 15s
      timeout: 5s
      retries: 20
      start_period: 120s

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: netbox
      POSTGRES_USER: netbox
      POSTGRES_PASSWORD: netbox
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U netbox"]
      interval: 5s
      timeout: 3s
      retries: 10

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
```

Start it up:
```bash
docker compose up -d

# Wait for NetBox to be healthy (~2-3 minutes on first start)
echo "Waiting for NetBox..."
until curl -sf http://localhost:8000/api/ > /dev/null 2>&1; do
  sleep 5
  echo "  still starting..."
done
echo "NetBox is ready!"
```

Verify:
```bash
# Should return API root
curl -s http://localhost:8000/api/ | python3 -m json.tool | head -10

# Verify auth token works
curl -s -H "Authorization: Token helios-test-token-0123456789abcdef" \
  http://localhost:8000/api/dcim/devices/ | python3 -m json.tool
```

### 5.2 Create the helios_monitor custom field

Helios discovers devices via a `helios_monitor` custom field. Create it first:
```bash
NETBOX_URL="http://localhost:8000"
TOKEN="helios-test-token-0123456789abcdef"
AUTH="Authorization: Token ${TOKEN}"
CT="Content-Type: application/json"

# Create the helios_monitor custom field on devices
curl -s -X POST "${NETBOX_URL}/api/extras/custom-fields/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{
    "name": "helios_monitor",
    "label": "Helios Monitor",
    "type": "boolean",
    "default": false,
    "object_types": ["dcim.device"],
    "description": "Enable Helios monitoring for this device",
    "required": false,
    "filter_logic": "exact"
  }' | python3 -m json.tool

# Also create snmp_index custom field on interfaces (used by Flow Enricher)
curl -s -X POST "${NETBOX_URL}/api/extras/custom-fields/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{
    "name": "snmp_index",
    "label": "SNMP Index",
    "type": "integer",
    "object_types": ["dcim.interface"],
    "description": "SNMP interface index (ifIndex)",
    "required": false
  }' | python3 -m json.tool
```

### 5.3 Seed test devices

This script creates a site, manufacturer, device type, device role, and 5 test devices with interfaces — everything Helios needs:
```bash
#!/bin/bash
# ~/helios-test/netbox/seed-devices.sh
NETBOX_URL="http://localhost:8000"
TOKEN="helios-test-token-0123456789abcdef"
AUTH="Authorization: Token ${TOKEN}"
CT="Content-Type: application/json"

echo "=== Creating site ==="
curl -s -X POST "${NETBOX_URL}/api/dcim/sites/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{"name": "DC1", "slug": "dc1", "status": "active"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))"

echo "=== Creating region ==="
curl -s -X POST "${NETBOX_URL}/api/dcim/regions/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{"name": "US-East", "slug": "us-east"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))"

echo "=== Creating manufacturer ==="
curl -s -X POST "${NETBOX_URL}/api/dcim/manufacturers/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{"name": "Arista", "slug": "arista"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))"

echo "=== Creating device type ==="
curl -s -X POST "${NETBOX_URL}/api/dcim/device-types/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{"manufacturer": 1, "model": "DCS-7050TX", "slug": "dcs-7050tx"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))"

echo "=== Creating device role ==="
curl -s -X POST "${NETBOX_URL}/api/dcim/device-roles/" \
  -H "${AUTH}" -H "${CT}" \
  -d '{"name": "Spine", "slug": "spine", "color": "ff0000"}' | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))"

# Create 5 test devices
for i in 1 2 3 4 5; do
  echo "=== Creating device: spine-${i}.dc1 ==="
  DEV_ID=$(curl -s -X POST "${NETBOX_URL}/api/dcim/devices/" \
    -H "${AUTH}" -H "${CT}" \
    -d "{
      \"name\": \"spine-${i}.dc1\",
      \"device_type\": 1,
      \"role\": 1,
      \"site\": 1,
      \"status\": \"active\",
      \"custom_fields\": {\"helios_monitor\": true}
    }" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))")
  echo "  device_id=${DEV_ID}"

  # Create interfaces with SNMP indexes
  for ifnum in 1 2 3 4; do
    IF_ID=$(curl -s -X POST "${NETBOX_URL}/api/dcim/interfaces/" \
      -H "${AUTH}" -H "${CT}" \
      -d "{
        \"device\": ${DEV_ID},
        \"name\": \"Ethernet${ifnum}\",
        \"type\": \"10gbase-t\",
        \"speed\": 10000000,
        \"custom_fields\": {\"snmp_index\": ${ifnum}}
      }" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))")
    echo "  interface Ethernet${ifnum} id=${IF_ID}"
  done

  # Create a management interface and assign a primary IP
  MGMT_IF_ID=$(curl -s -X POST "${NETBOX_URL}/api/dcim/interfaces/" \
    -H "${AUTH}" -H "${CT}" \
    -d "{
      \"device\": ${DEV_ID},
      \"name\": \"Management1\",
      \"type\": \"1000base-t\",
      \"mgmt_only\": true
    }" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))")

  IP_ID=$(curl -s -X POST "${NETBOX_URL}/api/ipam/ip-addresses/" \
    -H "${AUTH}" -H "${CT}" \
    -d "{
      \"address\": \"10.0.0.${i}/32\",
      \"assigned_object_type\": \"dcim.interface\",
      \"assigned_object_id\": ${MGMT_IF_ID}
    }" | python3 -c "import sys,json; print(json.load(sys.stdin).get('id','FAILED'))")

  # Set as primary IP on the device
  curl -s -X PATCH "${NETBOX_URL}/api/dcim/devices/${DEV_ID}/" \
    -H "${AUTH}" -H "${CT}" \
    -d "{\"primary_ip4\": ${IP_ID}}" > /dev/null

  echo "  primary_ip=10.0.0.${i}"
done

echo ""
echo "=== Done! Verifying ==="
curl -s -H "${AUTH}" \
  "${NETBOX_URL}/api/dcim/devices/?cf_helios_monitor=true" \
  | python3 -c "import sys,json; d=json.load(sys.stdin); print(f\"{d['count']} devices with helios_monitor=true\")"
```

Make it executable and run:
```bash
chmod +x ~/helios-test/netbox/seed-devices.sh
~/helios-test/netbox/seed-devices.sh
```

**Pass criteria:** `5 devices with helios_monitor=true` at the end.

### 5.4 Connect Helios to NetBox

NetBox is running on your host at port 8000. Minikube pods need to reach it:
```bash
# Get your host IP as seen from minikube
HOST_IP=$(minikube ssh -- ip route show default | awk '{print $3}')
echo "Host IP from minikube: ${HOST_IP}"

# Update Helios to point at NetBox
helm upgrade helios ~/git/helios/helm/helios \
  -f ~/git/helios/helm/helios/values-medium.yaml \
  --set integration.netbox.url="http://${HOST_IP}:8000" \
  --set integration.netbox.apiToken="helios-test-token-0123456789abcdef" \
  --reuse-values
```

### 5.5 Verify Target Generator picks up devices
```bash
# Trigger the CronJob manually
kubectl create job --from=cronjob/target-sync test-sync -n helios-integration

# Watch it run
kubectl logs job/test-sync -n helios-integration -f

# Check generated ConfigMaps
kubectl get configmap -n helios-collection | grep targets
kubectl get configmap -n helios-collection -o yaml | grep "spine-"
```
**Pass criteria:** Target Generator creates ConfigMaps containing `spine-1.dc1` through `spine-5.dc1`.

### 5.6 Verify Flow Enricher uses NetBox data
```bash
# Check Flow Enricher logs for NetBox cache refresh
kubectl logs -l app=flow-enricher -n helios-flows --tail=30 | grep -i netbox
```
**Pass criteria:** Logs show `NetBox cache refreshed, devices=5`.

### 5.7 Cleanup (when done testing)
```bash
cd ~/helios-test/netbox
docker compose down -v  # -v removes the postgres volume too
```

---

## Phase 6: Validate Runbook Automation (Day 4-5)

### 6.1 Deploy CRDs and operator
```bash
kubectl get crd runbooks.helios.io
kubectl get crd runbookexecutions.helios.io
kubectl get pods -n helios-automation
```
**Pass criteria:** CRDs exist, operator pod is Running.

### 6.2 Apply a sample runbook
```bash
kubectl apply -f config/runbooks/samples/collect-diagnostics.yaml
kubectl get runbooks -A
```

### 6.3 Create a test execution (dry run)
Without a real network device, verify the operator handles the execution lifecycle:
```bash
kubectl apply -f - <<'EOF'
apiVersion: helios.io/v1alpha1
kind: RunbookExecution
metadata:
  name: test-execution
  namespace: helios-automation
spec:
  runbookRef:
    name: collect-diagnostics
  parameters:
    device: "test-device.example.com"
EOF

# Watch the execution status
kubectl get runbookexecution test-execution -n helios-automation -w
```
**Pass criteria:** Execution transitions through states (Pending → Running → Failed is OK without a real device — the point is the operator is processing it).

---

## Phase 7: Validate Alerting (Day 5)

### 7.1 Verify alert rules loaded
```bash
kubectl port-forward svc/prometheus-operated 9090:9090 -n helios-storage &
curl -s http://localhost:9090/api/v1/rules | python3 -m json.tool | grep alertname | head -20
```
**Pass criteria:** Helios alert rules appear (device_unreachable, interface_error_rate, etc.)

### 7.2 Verify Alertmanager
```bash
kubectl port-forward svc/alertmanager 9093:9093 -n helios-visualization &
curl -s http://localhost:9093/api/v2/alerts | python3 -m json.tool
```

### 7.3 Trigger a test alert
```bash
# Send a test alert to Alertmanager
curl -X POST http://localhost:9093/api/v2/alerts \
  -H 'Content-Type: application/json' \
  -d '[{
    "labels": {"alertname": "TestAlert", "severity": "warning"},
    "annotations": {"summary": "Helios test alert"},
    "startsAt": "'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"
  }]'
```
**Pass criteria:** Alert appears in Alertmanager UI and Grafana Alerts Overview dashboard.

---

## Phase 8: End-to-End Smoke Test (Day 5)

Run through the full data path in one session:

```bash
#!/bin/bash
# helios-smoke-test.sh
set -e

echo "=== 1. Cluster Health ==="
kubectl get nodes
kubectl get pods -A | grep -v Running | grep -v Completed | grep -v NAME && echo "WARN: Some pods not running" || echo "PASS: All pods running"

echo ""
echo "=== 2. Prometheus Targets ==="
PROM=$(kubectl get svc prometheus-operated -n helios-storage -o jsonpath='{.spec.clusterIP}')
TARGETS=$(kubectl exec -it deploy/thanos-query -n helios-storage -- wget -qO- "http://${PROM}:9090/api/v1/targets" 2>/dev/null | grep -c '"health":"up"' || echo 0)
echo "Active targets: ${TARGETS}"
[ "$TARGETS" -gt 0 ] && echo "PASS" || echo "FAIL: No active Prometheus targets"

echo ""
echo "=== 3. Kafka Topics ==="
kubectl get kafkatopic -n helios-flows --no-headers | wc -l
echo "PASS: Kafka topics exist"

echo ""
echo "=== 4. ClickHouse ==="
CH_COUNT=$(kubectl exec chi-helios-clickhouse-0-0-0 -n helios-flows -- clickhouse-client --query "SELECT count() FROM flows_raw" 2>/dev/null || echo "0")
echo "Flow records in ClickHouse: ${CH_COUNT}"

echo ""
echo "=== 5. Grafana ==="
GRAFANA_IP=$(kubectl get svc grafana -n helios-visualization -o jsonpath='{.spec.clusterIP}')
kubectl exec -it deploy/thanos-query -n helios-storage -- wget -qO- "http://${GRAFANA_IP}:3000/api/health" 2>/dev/null && echo "PASS: Grafana healthy" || echo "FAIL: Grafana unreachable"

echo ""
echo "=== 6. Runbook Operator ==="
kubectl get pods -n helios-automation -l app=runbook-operator --no-headers | grep Running && echo "PASS" || echo "FAIL"

echo ""
echo "=== Done ==="
```

---

## Results Template

After running through all phases, fill in:

```
Date: ____
Cluster: minikube (Ryzen 7800X3D, 64GB) / 5x Raspberry Pi 5 (___GB RAM each)
K8s version: ___
Helios version: ___
Values profile: values-medium.yaml / values-small.yaml

PHASE 1 - Operators:          PASS / FAIL (notes: ___)
PHASE 2 - Deploy:             PASS / PARTIAL / FAIL (notes: ___)
PHASE 3 - Metrics Pipeline:   PASS / FAIL (notes: ___)
PHASE 4 - Flow Pipeline:      PASS / FAIL (notes: ___)
PHASE 5 - NetBox Integration: PASS / SKIP / FAIL (notes: ___)
PHASE 6 - Runbook Automation: PASS / FAIL (notes: ___)
PHASE 7 - Alerting:           PASS / FAIL (notes: ___)
PHASE 8 - Smoke Test:         PASS / FAIL (notes: ___)

Issues found:
1. ___
2. ___

Resource observations:
- Peak memory per node: ___
- Disk usage: ___
- Pod scheduling issues: ___
```

---

## Tips

### Minikube on workstation
- **You have headroom.** 32GB allocated to minikube leaves 32GB for your OS. If pods are slow to start, bump `--memory=48g`.
- **Use `minikube dashboard`** for a quick visual overview of all pods and their status.
- **Tunnel for LoadBalancer services:** `minikube tunnel` exposes LoadBalancer-type services on localhost.
- **Disk I/O matters.** If you're on NVMe, no worries. On spinning disk, ClickHouse and Kafka will be slow.
- **Pause/resume:** `minikube stop` / `minikube start` preserves state between testing sessions — no need to redeploy.
- **Debugging:** `minikube ssh` gets you into the node. `crictl pods` and `crictl logs` for container-level debugging.

### Pi cluster
- **Memory is your bottleneck.** Pi 5 has 4GB or 8GB. Kafka, ClickHouse, and Prometheus are the hungriest. If you have 4GB Pis, you may need to run Kafka and ClickHouse on separate nodes.
- **Use k3s over kubeadm.** Lighter control plane, built-in local-path-provisioner.
- **Disable components you're not testing.** Skip MinIO/Thanos if you just want to validate the flow pipeline. Skip Kafka/ClickHouse if you just want metrics.
- **Use `kubectl top pods -A --sort-by=memory`** frequently to catch resource pressure early.
- **SD cards are slow.** Use USB SSD boot for any node running ClickHouse or Kafka — the I/O difference is massive.

### Both environments
- **Test one pipeline at a time.** Get metrics working first (Phase 3), then flows (Phase 4). Don't try to debug everything at once.
- **Check logs early and often:** `kubectl logs -f <pod> -n <namespace>` is your best friend.
- **If a pod is OOMKilled**, check `kubectl describe pod <pod>` for the memory limit, then override in your values file.
