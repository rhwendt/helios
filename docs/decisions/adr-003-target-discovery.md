#adr #decision #discovery #netbox

# ADR-003: Target Discovery Strategy

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #discovery #netbox

## Context

Helios requires a mechanism to discover and configure monitoring targets (network devices) across the infrastructure. The system must:

1. **Scale**: Support 50 to 8,000+ network devices across multiple datacenters
2. **Automate**: Minimize manual configuration when devices are added/removed
3. **Integrate**: Work with existing inventory systems (NetBox)
4. **Latency**: Acceptable delay for new devices to appear in monitoring
5. **Reliability**: Continue working if the source of truth is temporarily unavailable
6. **GitOps**: Fit into GitOps workflows with version control

Key challenges:
- Devices are already tracked in NetBox with relevant metadata (site, role, vendor, platform)
- gnmic and Prometheus require different target formats
- Target configuration must include device-specific labels for filtering
- Changes to device inventory should propagate automatically

## Decision

We will use **NetBox as the Source of Truth with CronJob-based sync** to generate target ConfigMaps.

### Architecture

```
+------------------+     +---------------------+     +--------------------+
|                  |     |                     |     |                    |
|     NetBox       | --> |   Target Generator  | --> |    ConfigMaps      |
|   (Device API)   |     |     (CronJob)       |     |                    |
|                  |     |   Every 5 minutes   |     | - gnmic-targets    |
+------------------+     +----------+----------+     | - snmp-targets     |
                                    |                | - blackbox-targets |
                                    |                +--------------------+
                                    |                         |
                                    v                         v
                         +-------------------+      +-------------------+
                         |  Git Repository   |      |   Prometheus SD   |
                         |  (audit trail)    |      |   (file_sd_config)|
                         +-------------------+      +-------------------+
```

### Sync Process

```
1. CronJob runs every 5 minutes
2. Query NetBox API for active devices with monitoring enabled
3. Transform device data into target format (gnmic, SNMP, Blackbox)
4. Generate ConfigMaps with appropriate labels
5. Apply ConfigMaps to Kubernetes
6. gnmic and exporters reload targets automatically
7. Prometheus discovers new targets via file_sd_config
```

### CronJob Definition

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: netbox-target-sync
  namespace: helios-integration
spec:
  schedule: "*/5 * * * *"  # Every 5 minutes
  concurrencyPolicy: Forbid  # Don't run overlapping jobs
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 3
  jobTemplate:
    spec:
      activeDeadlineSeconds: 240  # 4 minute timeout
      backoffLimit: 2
      template:
        spec:
          serviceAccountName: target-generator
          containers:
          - name: target-generator
            image: helios/target-generator:v1.0.0
            env:
            - name: NETBOX_URL
              valueFrom:
                secretKeyRef:
                  name: netbox-credentials
                  key: url
            - name: NETBOX_TOKEN
              valueFrom:
                secretKeyRef:
                  name: netbox-credentials
                  key: token
            - name: NAMESPACE
              value: helios-collection
            resources:
              requests:
                cpu: 100m
                memory: 128Mi
              limits:
                cpu: 500m
                memory: 512Mi
          restartPolicy: OnFailure
```

### Target Generator Logic

```python
#!/usr/bin/env python3
"""NetBox to Helios Target Generator"""

import os
import json
import yaml
import pynetbox
from kubernetes import client, config

class TargetGenerator:
    def __init__(self):
        self.nb = pynetbox.api(
            url=os.environ['NETBOX_URL'],
            token=os.environ['NETBOX_TOKEN']
        )
        config.load_incluster_config()
        self.k8s = client.CoreV1Api()
        self.namespace = os.environ.get('NAMESPACE', 'helios-collection')

    def get_device_labels(self, device) -> dict:
        """Extract standard labels from NetBox device."""
        return {
            'device': device.name,
            'site': device.site.slug if device.site else 'unknown',
            'region': device.site.region.slug if device.site and device.site.region else 'unknown',
            'role': device.device_role.slug if device.device_role else 'unknown',
            'vendor': device.device_type.manufacturer.slug if device.device_type else 'unknown',
            'platform': device.platform.slug if device.platform else 'unknown',
            'tier': device.custom_fields.get('monitoring_tier', 'standard'),
        }

    def get_gnmic_targets(self) -> dict:
        """Generate gnmic targets from NetBox devices."""
        targets = {}

        devices = self.nb.dcim.devices.filter(
            status='active',
            cf_gnmi_enabled=True
        )

        for device in devices:
            if not device.primary_ip:
                continue

            ip = str(device.primary_ip.address).split('/')[0]
            port = device.custom_fields.get('gnmi_port', 6030)

            targets[device.name] = {
                'address': f"{ip}:{port}",
                'username': '${GNMI_USERNAME}',
                'password': '${GNMI_PASSWORD}',
                'skip-verify': True,
                'labels': self.get_device_labels(device),
                'subscriptions': self._get_subscriptions(device),
            }

        return {'targets': targets}

    def _get_subscriptions(self, device) -> list:
        """Get subscription list based on device profile."""
        profile = device.custom_fields.get('telemetry_profile', 'default')

        subscriptions = {
            'minimal': ['interfaces'],
            'default': ['interfaces', 'system'],
            'detailed': ['interfaces', 'system', 'bgp', 'lldp'],
            'custom': device.custom_fields.get('custom_subscriptions', ['interfaces']),
        }

        return subscriptions.get(profile, ['interfaces'])

    def get_snmp_targets(self) -> list:
        """Generate SNMP targets for Prometheus static config."""
        targets = []

        devices = self.nb.dcim.devices.filter(
            status='active',
            cf_snmp_enabled=True
        )

        for device in devices:
            if not device.primary_ip:
                continue

            ip = str(device.primary_ip.address).split('/')[0]
            labels = self.get_device_labels(device)
            labels['__param_module'] = device.custom_fields.get('snmp_module', 'if_mib')

            targets.append({
                'targets': [ip],
                'labels': labels,
            })

        return targets

    def get_blackbox_targets(self) -> list:
        """Generate Blackbox probe targets."""
        targets = []

        devices = self.nb.dcim.devices.filter(
            status='active',
            cf_blackbox_probes__n=''  # Has blackbox probes defined
        )

        for device in devices:
            if not device.primary_ip:
                continue

            ip = str(device.primary_ip.address).split('/')[0]
            probes = device.custom_fields.get('blackbox_probes', ['icmp'])
            labels = self.get_device_labels(device)

            for probe in probes:
                probe_labels = labels.copy()
                probe_labels['__param_module'] = probe

                targets.append({
                    'targets': [ip],
                    'labels': probe_labels,
                })

        return targets

    def update_configmaps(self):
        """Update Kubernetes ConfigMaps with generated targets."""
        # Generate all targets
        gnmic_targets = self.get_gnmic_targets()
        snmp_targets = self.get_snmp_targets()
        blackbox_targets = self.get_blackbox_targets()

        # Update gnmic targets ConfigMap
        self._update_configmap(
            name='gnmic-targets',
            data={'targets.yaml': yaml.dump(gnmic_targets, default_flow_style=False)}
        )

        # Update SNMP targets ConfigMap (for file_sd_config)
        self._update_configmap(
            name='snmp-targets',
            data={'targets.json': json.dumps(snmp_targets, indent=2)}
        )

        # Update Blackbox targets ConfigMap
        self._update_configmap(
            name='blackbox-targets',
            data={'targets.json': json.dumps(blackbox_targets, indent=2)}
        )

        print(f"Updated targets: gnmic={len(gnmic_targets.get('targets', {}))}, "
              f"snmp={len(snmp_targets)}, blackbox={len(blackbox_targets)}")

    def _update_configmap(self, name: str, data: dict):
        """Create or update a ConfigMap."""
        configmap = client.V1ConfigMap(
            metadata=client.V1ObjectMeta(
                name=name,
                namespace=self.namespace,
                labels={
                    'app.kubernetes.io/part-of': 'helios',
                    'app.kubernetes.io/component': 'target-discovery',
                }
            ),
            data=data
        )

        try:
            self.k8s.patch_namespaced_config_map(
                name=name,
                namespace=self.namespace,
                body=configmap
            )
        except client.exceptions.ApiException as e:
            if e.status == 404:
                self.k8s.create_namespaced_config_map(
                    namespace=self.namespace,
                    body=configmap
                )
            else:
                raise


if __name__ == '__main__':
    generator = TargetGenerator()
    generator.update_configmaps()
```

### NetBox Custom Fields Required

```yaml
custom_fields:
  - name: gnmi_enabled
    type: boolean
    object_types: [dcim.device]
    label: "gNMI Enabled"
    description: "Device supports gNMI streaming telemetry"
    default: false

  - name: gnmi_port
    type: integer
    object_types: [dcim.device]
    label: "gNMI Port"
    description: "gNMI port (default 6030)"
    default: 6030

  - name: snmp_enabled
    type: boolean
    object_types: [dcim.device]
    label: "SNMP Enabled"
    description: "Device supports SNMP polling"
    default: false

  - name: snmp_module
    type: text
    object_types: [dcim.device]
    label: "SNMP Module"
    description: "SNMP exporter module name"
    default: "if_mib"

  - name: blackbox_probes
    type: multiselect
    object_types: [dcim.device]
    label: "Blackbox Probes"
    description: "Synthetic probes to run"
    choices: [icmp, tcp_22, tcp_443, http_443]

  - name: telemetry_profile
    type: select
    object_types: [dcim.device]
    label: "Telemetry Profile"
    description: "OpenConfig subscription profile"
    choices: [minimal, default, detailed, custom]
    default: "default"

  - name: monitoring_tier
    type: select
    object_types: [dcim.device]
    label: "Monitoring Tier"
    description: "Monitoring priority level"
    choices: [critical, standard, best-effort]
    default: "standard"
```

## Consequences

### Positive

1. **Single Source of Truth**: NetBox already contains device inventory, no duplication
2. **Automatic Updates**: New devices appear in monitoring within 5 minutes
3. **Rich Metadata**: NetBox provides site, role, vendor, platform labels automatically
4. **GitOps Compatible**: ConfigMaps can be version controlled and audited
5. **Simple Architecture**: CronJob is easy to understand, deploy, and debug
6. **Graceful Degradation**: Existing targets continue working if NetBox is down
7. **Existing Integration**: NetBox is already used for IPAM and device inventory
8. **Extensible**: Easy to add new target types (e.g., new exporters)

### Negative

1. **Sync Delay**: Up to 5 minutes for new devices to appear in monitoring
2. **ConfigMap Size Limit**: ~1MB limit may be reached at very large scale (8000+ devices)
3. **NetBox Dependency**: If NetBox is permanently unavailable, no updates occur
4. **No Real-Time Events**: Misses immediate device additions/removals

### Neutral

1. **CronJob Overhead**: Runs every 5 minutes regardless of changes
2. **API Rate Limiting**: May need to consider NetBox API limits at large scale

## Alternatives Considered

### Option 1: File-Based Static Configuration

**Description**: Maintain target lists in YAML files, deploy via GitOps

**Pros**:
- Simple, no external dependencies
- Full version control
- Works offline

**Cons**:
- Manual updates required for every device change
- Duplicate data (devices already in NetBox)
- Error-prone and time-consuming
- Doesn't scale

**Why Not Selected**: Manual maintenance doesn't scale and duplicates existing NetBox data.

### Option 2: Prometheus HTTP Service Discovery

**Description**: Implement HTTP SD endpoint that queries NetBox directly

**Pros**:
- Native Prometheus integration
- Real-time updates (configurable refresh)
- No ConfigMap size limits

**Cons**:
- Requires custom HTTP SD server
- More complex than CronJob
- Prometheus-specific (gnmic needs different solution)
- Single point of failure during queries

**Why Not Selected**: Requires maintaining an additional service and doesn't work for gnmic. Consider as enhancement for Prometheus-only targets in future.

### Option 3: Kubernetes Operator with NetBox Watch

**Description**: Build operator that watches NetBox events and updates immediately

**Pros**:
- Real-time updates
- Event-driven (no polling)
- Kubernetes-native

**Cons**:
- Significant development effort
- NetBox webhook configuration required
- More complex failure modes
- Overkill for most use cases

**Why Not Selected**: 5-minute delay is acceptable for network monitoring. Operator complexity not justified unless real-time discovery is required.

### Option 4: Nautobot Instead of NetBox

**Description**: Use Nautobot (NetBox fork) as source of truth

**Pros**:
- Active development
- Better plugin ecosystem
- GraphQL API

**Cons**:
- Migration effort from existing NetBox
- Different API (code changes required)
- Smaller community

**Why Not Selected**: NetBox is already deployed and working. No compelling reason to migrate.

## Implementation Notes

### RBAC for Target Generator

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: target-generator
  namespace: helios-integration
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: target-generator
  namespace: helios-collection
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "create", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: target-generator
  namespace: helios-collection
subjects:
  - kind: ServiceAccount
    name: target-generator
    namespace: helios-integration
roleRef:
  kind: Role
  name: target-generator
  apiGroup: rbac.authorization.k8s.io
```

### Prometheus file_sd_config Integration

```yaml
# prometheus.yaml
scrape_configs:
  - job_name: 'snmp'
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/snmp-targets.json
        refresh_interval: 1m
    metrics_path: /snmp
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: snmp-exporter:9116

  - job_name: 'blackbox'
    file_sd_configs:
      - files:
          - /etc/prometheus/targets/blackbox-targets.json
        refresh_interval: 1m
    metrics_path: /probe
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: blackbox-exporter:9115
```

### Handling ConfigMap Size Limits

For very large deployments (8000+ devices), split targets across multiple ConfigMaps:

```python
def _split_targets(self, targets: list, max_per_configmap: int = 2000) -> dict:
    """Split targets into multiple ConfigMaps if needed."""
    chunks = {}
    for i in range(0, len(targets), max_per_configmap):
        chunk_num = i // max_per_configmap
        chunk_name = f"targets-{chunk_num:03d}.json"
        chunks[chunk_name] = targets[i:i + max_per_configmap]
    return chunks
```

## References

- [NetBox API Documentation](https://docs.netbox.dev/en/stable/integrations/rest-api/)
- [gnmic Target Configuration](https://gnmic.openconfig.net/user_guide/targets/)
- [Prometheus file_sd_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#file_sd_config)
- [[adr-001-storage-backend]] - Storage backend decision
- [[adr-002-gnmic-deployment]] - gnmic deployment model
- [[../architecture/architecture]] - Full architecture document
