# Helios Runbook Library

Pre-built runbooks for common network operations.

## Quick Start

```bash
# Deploy all sample runbooks
kubectl apply -f samples/

# List available runbooks
kubectl get runbooks -n helios-automation

# Execute a runbook
cat <<EOF | kubectl apply -f -
apiVersion: helios.io/v1alpha1
kind: RunbookExecution
metadata:
  generateName: interface-bounce-
  namespace: helios-automation
spec:
  runbookRef:
    name: interface-bounce
  parameters:
    device: core-rtr-01
    interface: Ethernet1/1
    wait_seconds: 5
  triggeredBy: $(whoami)@example.com
EOF

# Watch execution progress
kubectl get runbookexecutions -n helios-automation -w
```

## Available Runbooks

### Core Operations

| Runbook | Risk | Approval | Description |
|---------|------|----------|-------------|
| `interface-bounce` | Medium | No | Disable/enable interface to clear errors |
| `drain-interface` | Medium | Yes | Gracefully drain traffic before maintenance |
| `clear-bgp-neighbor` | High | Yes | Reset a BGP session |
| `collect-diagnostics` | Low | No | Gather device state for troubleshooting |
| `backup-config` | Low | No | Export config to object storage |

### Maintenance Workflows

| Runbook | Risk | Approval | Description |
|---------|------|----------|-------------|
| `device-maintenance-mode` | High | Yes | Full pre-maintenance preparation |
| `exit-maintenance-mode` | Medium | No | Restore device to normal operation |

### Vendor-Specific

| Runbook | Vendor | Description |
|---------|--------|-------------|
| `arista-mlag-check` | Arista | Validate MLAG health |
| `cisco-reload-in` | Cisco | Schedule device reload |
| `paloalto-commit` | Palo Alto | Commit firewall configuration |

## Runbook Structure

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: example-runbook
  namespace: helios-automation
  labels:
    helios.io/category: interface    # Category for organization
    helios.io/vendor: multi-vendor   # Vendor compatibility
spec:
  name: Human Readable Name
  description: |
    Detailed description of what this runbook does,
    including use cases and impact.

  category: interface                 # interface|bgp|system|security|diagnostic|custom
  riskLevel: medium                   # low|medium|high|critical
  requiresApproval: false             # Require human approval before execution
  approvers:                          # Who can approve (if required)
    - type: group
      name: network-admin
  approvalTimeout: "1h"               # How long to wait for approval
  allowedRoles:                       # RBAC: who can execute
    - network-engineer
    - network-admin
  cooldown: "5m"                      # Minimum time between executions on same target

  parameters:                         # Input parameters
    - name: device
      type: device                    # string|integer|boolean|device|interface|select
      required: true
      description: Target device

  steps:                              # Execution steps
    - name: Step Name
      action: gnmi_set                # gnmi_set|gnmi_get|wait|notify|script|condition
      timeout: 30s
      config:
        target: "{{device}}:6030"
        updates:
          - path: "/path/to/leaf"
            value: true

  rollback:                           # Optional: steps to run on failure
    - name: Undo Changes
      action: gnmi_set
      config: ...
```

## Step Actions

### gnmi_set
Configure device via gNMI Set RPC.

```yaml
- name: Disable Interface
  action: gnmi_set
  config:
    target: "{{device}}:6030"
    updates:
      - path: "/interfaces/interface[name={{interface}}]/config/enabled"
        value: false
        encoding: json_ietf
```

### gnmi_get
Read device state. Can retry until condition is met.

```yaml
- name: Wait for Interface Up
  action: gnmi_get
  timeout: 60s
  config:
    target: "{{device}}:6030"
    path: "/interfaces/interface[name={{interface}}]/state/oper-status"
    retryUntil: "UP"
    retryInterval: "5s"
```

### wait
Simple delay between steps.

```yaml
- name: Wait for Convergence
  action: wait
  config:
    duration: "30s"
```

### notify
Send notification via Slack, Teams, or webhook.

```yaml
- name: Notify Team
  action: notify
  config:
    channel: slack
    message: |
      :white_check_mark: Operation complete
      *Device:* {{device}}
```

### script
Run custom script in container.

```yaml
- name: Custom Operation
  action: script
  config:
    image: helios/custom-scripts:v1.0.0
    script: my_script.py
    args:
      - --device={{device}}
    env:
      - name: API_KEY
        valueFrom:
          secretKeyRef:
            name: my-secret
            key: api_key
```

### condition
Conditional branching (skip step if condition not met).

```yaml
- name: Only If Routing Enabled
  action: gnmi_get
  condition: "{{include_routing}} == true"
  config:
    path: "/network-instances/..."
```

## Parameter Types

| Type | Description | Example |
|------|-------------|---------|
| `string` | Free-form text | `"any text"` |
| `integer` | Whole number | `30` |
| `boolean` | True/false | `true` |
| `device` | Device from NetBox (validated) | `"core-rtr-01"` |
| `interface` | Interface name | `"Ethernet1/1"` |
| `select` | Choice from options | `"ospf"` or `"isis"` |

## Template Variables

Runbooks use Go templates. Available variables:

| Variable | Description |
|----------|-------------|
| `{{param_name}}` | Parameter value |
| `{{.triggeredBy}}` | User who triggered execution |
| `{{.startTime}}` | Execution start time |
| `{{.status.approvedBy}}` | User who approved (if approval required) |
| `{{.status.duration}}` | Total execution duration |
| `{{steps['Step Name'].output}}` | Output from a previous step |

## Creating Custom Runbooks

1. Copy a sample as a starting point
2. Modify metadata and parameters
3. Define your steps
4. Test with `dryRun: true`
5. Submit PR to this repository

```bash
# Test in dry-run mode
cat <<EOF | kubectl apply -f -
apiVersion: helios.io/v1alpha1
kind: RunbookExecution
metadata:
  generateName: test-
  namespace: helios-automation
spec:
  runbookRef:
    name: my-new-runbook
  parameters:
    device: test-device
  dryRun: true
  triggeredBy: developer@example.com
EOF
```

## Directory Structure

```
runbooks/
├── README.md           # This file
├── samples/            # Pre-built runbooks
│   ├── interface-bounce.yaml
│   ├── clear-bgp-neighbor.yaml
│   ├── drain-interface.yaml
│   ├── collect-diagnostics.yaml
│   ├── backup-config.yaml
│   ├── device-maintenance-mode.yaml
│   ├── exit-maintenance-mode.yaml
│   ├── arista-mlag-check.yaml
│   ├── cisco-reload-in.yaml
│   └── paloalto-commit.yaml
└── custom/             # Your custom runbooks (gitignored or separate repo)
```

## Related Documentation

- [Runbook Operator Architecture](../architecture/runbook-operator.md)
- [ADR-004: Runbook Automation](../decisions/adr-004-runbook-automation.md)
- [Helios Overview](../overview.md)
