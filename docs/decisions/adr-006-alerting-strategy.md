#adr #decision #alerting #prometheus

# ADR-006: Alerting Strategy

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #alerting #prometheus #thanos

## Context

Helios requires an alerting system that can:

1. **Low Latency**: Detect and alert on network issues within seconds, not minutes
2. **Global Aggregation**: Create alerts based on metrics across multiple datacenters
3. **High Availability**: Alert delivery must survive component failures
4. **Flexible Routing**: Route alerts to different teams/channels based on criteria
5. **Deduplication**: Prevent alert storms with proper grouping and deduplication
6. **Integration**: Work with existing notification systems (PagerDuty, Slack, email)
7. **Recording Rules**: Pre-compute expensive queries for dashboards

Key considerations:
- Prometheus evaluates alerts locally with low latency
- Thanos Ruler can evaluate alerts against global data but with higher latency
- Some alerts need global context (e.g., "more than 50% of DC devices down")
- Most alerts are device-specific and don't need global aggregation
- Recording rules for dashboards can tolerate higher latency

## Decision

We will use a **split strategy: alerts evaluated in Prometheus, recording rules in Thanos Ruler**.

### Architecture

```
                                         NOTIFICATION LAYER
    +------------------------------------------------------------------------+
    |                                                                        |
    |  +------------------+     +------------------+     +------------------+ |
    |  |    PagerDuty     |     |      Slack       |     |      Email       | |
    |  +------------------+     +------------------+     +------------------+ |
    |            ^                       ^                       ^           |
    +------------|-----------------------|-----------------------|-----------+
                 |                       |                       |
    +------------|-----------------------|-----------------------|------------+
    |            |          ALERTMANAGER CLUSTER                 |            |
    |  +---------+---------+  +---------+---------+  +---------+---------+   |
    |  |  Alertmanager-0   |  |  Alertmanager-1   |  |  Alertmanager-2   |   |
    |  |  (DC1)            |  |  (DC2)            |  |  (DC3)            |   |
    |  +-------------------+  +-------------------+  +-------------------+   |
    |            ^                       ^                       ^           |
    +------------|----+------------------|----+------------------|----+------+
                 |    |                  |    |                  |    |
    +------------|----|-+     +----------|----|-+     +----------|----|-+
    | DC1        |    | |     | DC2      |    | |     | DC3      |    | |
    |  +---------+----v-+     |  +-------+----v-+     |  +-------+----v-+
    |  |                |     |  |                |     |  |                |
    |  |  Prometheus    |     |  |  Prometheus    |     |  |  Prometheus    |
    |  |  (Alerts)      |     |  |  (Alerts)      |     |  |  (Alerts)      |
    |  +----------------+     |  +----------------+     |  +----------------+
    |                         |                         |                    |
    +-------------------------+-------------------------+--------------------+
                                        |
                                        v
    +------------------------------------------------------------------------+
    |                         THANOS RULER (Global)                          |
    |                                                                        |
    |  +------------------------------------------------------------------+  |
    |  |   Recording Rules (pre-computed aggregations)                    |  |
    |  |   Global Alerts (cross-DC correlation)                           |  |
    |  +------------------------------------------------------------------+  |
    |                               |                                        |
    |                               v                                        |
    |  +------------------------------------------------------------------+  |
    |  |                    Thanos Query (Global)                         |  |
    |  +------------------------------------------------------------------+  |
    |                                                                        |
    +------------------------------------------------------------------------+
```

### Alert Evaluation Strategy

| Alert Type | Evaluated By | Latency | Example |
|------------|--------------|---------|---------|
| Device down | Prometheus | ~15s | `up{job="gnmic"} == 0` |
| Interface errors | Prometheus | ~15s | `rate(if_errors[5m]) > 100` |
| High CPU | Prometheus | ~15s | `device_cpu_utilization > 90` |
| BGP session down | Prometheus | ~15s | `bgp_session_state != 6` |
| Multi-DC outage | Thanos Ruler | ~60s | `count(up == 0) BY (dc) > total * 0.5` |
| Global capacity | Thanos Ruler | ~60s | `sum(bandwidth_used) / sum(bandwidth_total)` |

### Prometheus Alert Rules (Per-DC)

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: helios-network-alerts
  namespace: helios-storage
  labels:
    prometheus: helios
    role: alert-rules
spec:
  groups:
    - name: device-availability
      interval: 15s
      rules:
        - alert: DeviceDown
          expr: up{job="gnmic"} == 0
          for: 1m
          labels:
            severity: critical
            category: availability
          annotations:
            summary: "Network device {{ $labels.device }} is down"
            description: "Device {{ $labels.device }} at site {{ $labels.site }} has been unreachable for more than 1 minute."
            runbook_url: "https://runbooks.example.com/device-down"
            dashboard_url: "https://grafana.example.com/d/device-health?var-device={{ $labels.device }}"

        - alert: DeviceUnreachable
          expr: probe_success{job="blackbox", module="icmp"} == 0
          for: 2m
          labels:
            severity: warning
            category: availability
          annotations:
            summary: "Device {{ $labels.device }} not responding to ICMP"
            description: "Device {{ $labels.device }} is not responding to ping probes."

    - name: interface-health
      interval: 15s
      rules:
        - alert: InterfaceDown
          expr: |
            gnmic_interfaces_interface_state_oper_status{oper_status="DOWN"}
            and on(device, interface)
            gnmic_interfaces_interface_state_admin_status{admin_status="UP"}
          for: 1m
          labels:
            severity: warning
            category: interface
          annotations:
            summary: "Interface {{ $labels.interface }} on {{ $labels.device }} is down"
            description: "Interface is administratively up but operationally down."

        - alert: InterfaceHighErrors
          expr: |
            rate(gnmic_interfaces_interface_state_counters_in_errors[5m]) > 100
            or
            rate(gnmic_interfaces_interface_state_counters_out_errors[5m]) > 100
          for: 5m
          labels:
            severity: warning
            category: interface
          annotations:
            summary: "High error rate on {{ $labels.interface }} ({{ $labels.device }})"
            description: "Interface {{ $labels.interface }} on {{ $labels.device }} has >100 errors/sec."

        - alert: InterfaceHighUtilization
          expr: |
            (
              rate(gnmic_interfaces_interface_state_counters_in_octets[5m]) * 8
              / gnmic_interfaces_interface_ethernet_state_port_speed
            ) > 0.9
          for: 10m
          labels:
            severity: warning
            category: capacity
          annotations:
            summary: "Interface {{ $labels.interface }} at >90% utilization"

    - name: system-health
      interval: 30s
      rules:
        - alert: DeviceHighCPU
          expr: gnmic_system_cpus_cpu_state_total_utilization > 90
          for: 5m
          labels:
            severity: warning
            category: system
          annotations:
            summary: "High CPU on {{ $labels.device }} ({{ $value }}%)"

        - alert: DeviceHighMemory
          expr: |
            (gnmic_system_memory_state_used / gnmic_system_memory_state_physical) * 100 > 90
          for: 5m
          labels:
            severity: warning
            category: system
          annotations:
            summary: "High memory on {{ $labels.device }} ({{ $value }}%)"

    - name: bgp-health
      interval: 15s
      rules:
        - alert: BGPSessionDown
          expr: gnmic_bgp_neighbors_neighbor_state_session_state != 6
          for: 1m
          labels:
            severity: critical
            category: routing
          annotations:
            summary: "BGP session down: {{ $labels.device }} -> {{ $labels.neighbor_address }}"
            description: "BGP session is not in ESTABLISHED state."

        - alert: BGPPrefixCountDrop
          expr: |
            (
              gnmic_bgp_neighbors_neighbor_state_prefixes_received
              - gnmic_bgp_neighbors_neighbor_state_prefixes_received offset 10m
            ) / gnmic_bgp_neighbors_neighbor_state_prefixes_received offset 10m < -0.2
          for: 5m
          labels:
            severity: warning
            category: routing
          annotations:
            summary: "BGP prefix count dropped >20% on {{ $labels.device }}"
```

### Thanos Ruler Configuration (Global)

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: thanos-ruler
  namespace: helios-storage
spec:
  replicas: 2  # HA pair
  template:
    spec:
      containers:
      - name: thanos-ruler
        image: quay.io/thanos/thanos:v0.34.0
        args:
          - rule
          - --data-dir=/var/thanos/ruler
          - --rule-file=/etc/thanos/rules/*.yaml
          - --query=thanos-query.helios.svc:9090
          - --alertmanagers.url=http://alertmanager.helios.svc:9093
          - --objstore.config-file=/etc/thanos/objstore.yaml
          - --label=ruler_replica="$(POD_NAME)"
          - --alert.label-drop=ruler_replica
          # Evaluation interval
          - --eval-interval=30s
        volumeMounts:
          - name: rules
            mountPath: /etc/thanos/rules
          - name: objstore-config
            mountPath: /etc/thanos
```

### Thanos Recording and Global Alert Rules

```yaml
# Recording rules for dashboard performance
groups:
  - name: helios-recording-rules
    interval: 1m
    rules:
      # Pre-aggregate interface traffic by site
      - record: site:interface_traffic_bytes:rate5m
        expr: |
          sum by (site, direction) (
            rate(gnmic_interfaces_interface_state_counters_in_octets[5m])
          )

      # Device count by status per site
      - record: site:device_count:by_status
        expr: |
          count by (site, status) (
            up{job="gnmic"} == bool 1
          )

      # BGP session summary per site
      - record: site:bgp_sessions:by_state
        expr: |
          count by (site, session_state) (
            gnmic_bgp_neighbors_neighbor_state_session_state
          )

      # Top interfaces by utilization
      - record: interface:utilization:top10
        expr: |
          topk(10,
            rate(gnmic_interfaces_interface_state_counters_in_octets[5m]) * 8
            / gnmic_interfaces_interface_ethernet_state_port_speed
          )

  - name: helios-global-alerts
    interval: 1m
    rules:
      # Alert if more than 30% of devices in a site are down
      - alert: SiteMajorOutage
        expr: |
          (
            count by (site) (up{job="gnmic"} == 0)
            / count by (site) (up{job="gnmic"})
          ) > 0.3
        for: 2m
        labels:
          severity: critical
          category: infrastructure
        annotations:
          summary: "Major outage at site {{ $labels.site }}"
          description: "More than 30% of devices at site {{ $labels.site }} are unreachable."

      # Alert on cross-DC connectivity issues
      - alert: CrossDCConnectivityIssue
        expr: |
          count(
            probe_success{job="blackbox", module="cross_dc_icmp"} == 0
          ) > 2
        for: 3m
        labels:
          severity: critical
          category: infrastructure
        annotations:
          summary: "Cross-DC connectivity issues detected"
          description: "Multiple cross-DC probes are failing."

      # Global capacity alert
      - alert: GlobalCapacityWarning
        expr: |
          (
            sum(rate(gnmic_interfaces_interface_state_counters_in_octets{role="uplink"}[5m])) * 8
            / sum(gnmic_interfaces_interface_ethernet_state_port_speed{role="uplink"})
          ) > 0.8
        for: 15m
        labels:
          severity: warning
          category: capacity
        annotations:
          summary: "Global uplink capacity at {{ $value | humanizePercentage }}"
```

### Alertmanager Configuration

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: alertmanager-config
  namespace: helios-storage
stringData:
  alertmanager.yaml: |
    global:
      resolve_timeout: 5m
      slack_api_url: '${SLACK_WEBHOOK_URL}'
      pagerduty_url: 'https://events.pagerduty.com/v2/enqueue'

    route:
      receiver: 'default-receiver'
      group_by: ['alertname', 'dc', 'site']
      group_wait: 30s
      group_interval: 5m
      repeat_interval: 4h

      routes:
        # Critical alerts go to PagerDuty immediately
        - receiver: 'pagerduty-critical'
          match:
            severity: critical
          group_wait: 0s
          group_interval: 1m
          repeat_interval: 1h
          continue: true  # Also send to Slack

        # All critical alerts to NOC Slack channel
        - receiver: 'slack-noc'
          match:
            severity: critical
          group_wait: 0s

        # Routing alerts to network team
        - receiver: 'slack-network'
          match:
            category: routing

        # Capacity alerts to capacity planning
        - receiver: 'slack-capacity'
          match:
            category: capacity

        # Infrastructure alerts
        - receiver: 'slack-infrastructure'
          match:
            category: infrastructure

    receivers:
      - name: 'default-receiver'
        slack_configs:
          - channel: '#network-alerts'
            send_resolved: true
            title: '{{ template "slack.title" . }}'
            text: '{{ template "slack.text" . }}'

      - name: 'pagerduty-critical'
        pagerduty_configs:
          - service_key: '${PAGERDUTY_SERVICE_KEY}'
            severity: critical
            description: '{{ template "pagerduty.description" . }}'
            details:
              firing: '{{ template "pagerduty.firing" . }}'

      - name: 'slack-noc'
        slack_configs:
          - channel: '#noc'
            send_resolved: true
            color: '{{ template "slack.color" . }}'
            title: '{{ template "slack.title" . }}'
            text: '{{ template "slack.text" . }}'
            actions:
              - type: button
                text: 'View in Grafana'
                url: '{{ (index .Alerts 0).Annotations.dashboard_url }}'
              - type: button
                text: 'Runbook'
                url: '{{ (index .Alerts 0).Annotations.runbook_url }}'

      - name: 'slack-network'
        slack_configs:
          - channel: '#network-team'
            send_resolved: true

      - name: 'slack-capacity'
        slack_configs:
          - channel: '#capacity-planning'
            send_resolved: true

      - name: 'slack-infrastructure'
        slack_configs:
          - channel: '#infrastructure'
            send_resolved: true

    inhibit_rules:
      # If site is down, suppress individual device alerts
      - source_match:
          alertname: 'SiteMajorOutage'
        target_match_re:
          alertname: 'DeviceDown|InterfaceDown|BGPSessionDown'
        equal: ['site']

      # If device is down, suppress interface/BGP alerts
      - source_match:
          alertname: 'DeviceDown'
        target_match_re:
          alertname: 'InterfaceDown|BGPSessionDown|DeviceHighCPU|DeviceHighMemory'
        equal: ['device']

    templates:
      - '/etc/alertmanager/templates/*.tmpl'
```

### Alertmanager HA Cluster

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Alertmanager
metadata:
  name: helios
  namespace: helios-storage
spec:
  replicas: 3
  # Cluster across DCs via Cilium
  clusterAdvertiseAddress: "$(POD_IP):9094"
  # External URL for links in alerts
  externalUrl: https://alertmanager.example.com
  # Resource allocation
  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 500m
      memory: 512Mi
  # Persistent storage for silences/nflog
  storage:
    volumeClaimTemplate:
      spec:
        storageClassName: fast-ssd
        resources:
          requests:
            storage: 10Gi
  # Pod anti-affinity for HA
  affinity:
    podAntiAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        - labelSelector:
            matchLabels:
              alertmanager: helios
          topologyKey: kubernetes.io/hostname
```

## Consequences

### Positive

1. **Low Latency Alerts**: Device-specific alerts fire within 15-30 seconds
2. **High Availability**: Alertmanager cluster survives node failures
3. **Scalable Recording Rules**: Expensive aggregations pre-computed by Thanos Ruler
4. **Flexible Routing**: Alerts routed to appropriate teams automatically
5. **Deduplication**: Proper inhibition prevents alert storms
6. **Dashboard Performance**: Recording rules speed up common queries
7. **Clear Separation**: Simple alerts local, complex global

### Negative

1. **Complexity**: Two systems evaluating rules (Prometheus + Thanos Ruler)
2. **Consistency**: Same alert could be defined in two places
3. **Higher Latency for Global Alerts**: ~60s evaluation interval vs 15s
4. **Debugging**: May need to check both systems for alert issues

### Neutral

1. **Configuration Split**: Alert rules in PrometheusRule CRs and Thanos ruler config
2. **Storage Overhead**: Recording rules create additional time series
3. **Thanos Ruler Resources**: Needs query capacity for rule evaluation

## Alternatives Considered

### Option 1: Thanos Ruler Only

**Description**: Move all alert evaluation to Thanos Ruler for unified management

**Pros**:
- Single location for all rules
- Easier management
- Consistent global view

**Cons**:
- Higher alert latency (query through Thanos)
- Depends on Thanos Query availability
- Less resilient to network issues
- Not recommended by Thanos documentation for latency-sensitive alerts

**Why Not Selected**: Alert latency too high for critical network alerts. Thanos documentation recommends Prometheus for latency-sensitive alerts.

### Option 2: Prometheus Only (No Thanos Ruler)

**Description**: Run all alerts in local Prometheus instances

**Pros**:
- Lowest latency
- Simplest architecture
- No Thanos Ruler dependency

**Cons**:
- No global aggregation capability
- No cross-DC correlation
- Recording rules must be evaluated per-DC

**Why Not Selected**: Need global alerts for multi-DC scenarios. Recording rules benefit from global data.

### Option 3: Grafana Alerting

**Description**: Use Grafana's built-in alerting instead of Prometheus/Thanos

**Pros**:
- Unified with dashboards
- Visual rule builder
- Multi-datasource alerts

**Cons**:
- Higher latency (queries Thanos)
- Less mature than Alertmanager
- Different routing model
- Vendor lock-in to Grafana

**Why Not Selected**: Prometheus Alertmanager is more mature and better for infrastructure alerting. Grafana Alerting is evolving but not yet feature-complete.

### Option 4: Custom Alerting System

**Description**: Build custom alerting with Kafka-based event streaming

**Pros**:
- Full control over alert pipeline
- Real-time event processing
- Custom correlation logic

**Cons**:
- Significant development effort
- Need to build reliability features
- Reinventing the wheel
- Maintenance burden

**Why Not Selected**: Prometheus + Alertmanager is battle-tested and sufficient. Custom solution not justified.

## Implementation Notes

### Alert Template Examples

```yaml
# /etc/alertmanager/templates/slack.tmpl
{{ define "slack.title" }}
[{{ .Status | toUpper }}] {{ .CommonLabels.alertname }}
{{ end }}

{{ define "slack.text" }}
{{ range .Alerts }}
*Alert:* {{ .Labels.alertname }}
*Severity:* {{ .Labels.severity }}
*Device:* {{ .Labels.device }}
*Site:* {{ .Labels.site }}
*Description:* {{ .Annotations.description }}
{{ if .Annotations.runbook_url }}*Runbook:* {{ .Annotations.runbook_url }}{{ end }}
{{ end }}
{{ end }}

{{ define "slack.color" }}
{{ if eq .Status "firing" }}
{{ if eq .CommonLabels.severity "critical" }}danger{{ else }}warning{{ end }}
{{ else }}good{{ end }}
{{ end }}
```

### Testing Alerts

```yaml
# Test alert rule syntax
promtool check rules alert-rules.yaml

# Unit test alerts
promtool test rules tests.yaml

# Example test file
rule_files:
  - alert-rules.yaml

evaluation_interval: 1m

tests:
  - interval: 1m
    input_series:
      - series: 'up{job="gnmic",device="test-device",site="dc1"}'
        values: '1 1 1 0 0 0 0 0 0 0'

    alert_rule_test:
      - eval_time: 5m
        alertname: DeviceDown
        exp_alerts:
          - exp_labels:
              severity: critical
              device: test-device
              site: dc1
```

### Silence Management

```bash
# Create silence via amtool
amtool silence add alertname=DeviceDown device=core-rtr-01 \
  --comment="Planned maintenance" \
  --duration=4h \
  --author="operator@example.com"

# Query active silences
amtool silence query

# Expire a silence
amtool silence expire <silence-id>
```

## References

- [Prometheus Alerting Rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/)
- [Alertmanager Configuration](https://prometheus.io/docs/alerting/latest/configuration/)
- [Thanos Ruler](https://thanos.io/tip/components/rule.md/)
- [Recording Rules Best Practices](https://prometheus.io/docs/practices/rules/)
- [[adr-001-storage-backend]] - Storage backend decision
- [[adr-005-multi-datacenter]] - Multi-DC architecture
- [[../architecture/architecture]] - Full architecture document
