#adr #decision #dashboards #grafana

# ADR-007: Dashboard Provisioning Strategy

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #dashboards #grafana #gitops

## Context

Helios requires a strategy for managing Grafana dashboards that:

1. **Version Control**: All dashboards should be tracked in Git with change history
2. **Review Process**: Dashboard changes should go through PR review
3. **Validation**: Dashboards should be validated before deployment
4. **Multiple Sources**: Support core Helios dashboards and user customizations
5. **Consistency**: Ensure dashboards work across environments
6. **Rollback**: Ability to revert to previous dashboard versions
7. **Collaboration**: Multiple team members can contribute dashboards

Current challenges:
- Grafana UI allows direct editing (bypassing version control)
- JSON dashboard files are hard to review in PRs
- No validation of dashboard syntax before deployment
- Dashboard sprawl without organization

## Decision

We will use **Git-based dashboard management with ConfigMap provisioning**.

### Architecture

```
                               GIT REPOSITORY
    +------------------------------------------------------------------+
    |                                                                  |
    |  helios-config/                                                  |
    |  +-----------------------------------------------------------+   |
    |  | dashboards/                                               |   |
    |  |   +-- core/              # Helios core dashboards         |   |
    |  |   |   +-- overview.json                                   |   |
    |  |   |   +-- device-health.json                              |   |
    |  |   |   +-- interface-analytics.json                        |   |
    |  |   +-- vendor/            # Vendor-specific                |   |
    |  |   |   +-- arista/                                         |   |
    |  |   |   +-- cisco/                                          |   |
    |  |   |   +-- juniper/                                        |   |
    |  |   +-- custom/            # User customizations            |   |
    |  |       +-- my-dashboard.json                               |   |
    |  +-----------------------------------------------------------+   |
    |                                                                  |
    +--------------------------------+----------------------------+----+
                                     |                            |
                                     v                            v
    +--------------------------------+-+    +---------------------+----+
    |           CI PIPELINE            |    |      CD PIPELINE          |
    |                                  |    |                           |
    | +------------------------------+ |    | +----------------------+  |
    | | 1. Validate JSON syntax      | |    | | 1. Generate ConfigMaps|  |
    | | 2. Check required fields     | |    | | 2. Apply to cluster  |  |
    | | 3. Validate PromQL queries   | |    | | 3. Grafana reloads   |  |
    | | 4. Check variable references | |    | +----------------------+  |
    | +------------------------------+ |    |                           |
    |                                  |    |                           |
    +----------------------------------+    +---------------------------+
                                                         |
                                                         v
    +------------------------------------------------------------------------+
    |                          KUBERNETES                                    |
    |                                                                        |
    |  +-------------------+  +-------------------+  +-------------------+   |
    |  | ConfigMap:        |  | ConfigMap:        |  | ConfigMap:        |   |
    |  | grafana-dash-core |  | grafana-dash-arista| | grafana-dash-custom|  |
    |  +--------+----------+  +--------+----------+  +--------+----------+   |
    |           |                      |                      |              |
    |           +----------------------+----------------------+              |
    |                                  |                                     |
    |                                  v                                     |
    |  +---------------------------------------------------------------+     |
    |  |                         GRAFANA                               |     |
    |  |                                                               |     |
    |  |  +-------------------+  +----------------------------------+  |     |
    |  |  | Provisioning      |  | Dashboard Folders                |  |     |
    |  |  | Sidecar           |  |  +-- Network Overview            |  |     |
    |  |  | (watches CMs)     +->|  +-- Vendor Specific             |  |     |
    |  |  +-------------------+  |  +-- Custom                      |  |     |
    |  |                         +----------------------------------+  |     |
    |  +---------------------------------------------------------------+     |
    |                                                                        |
    +------------------------------------------------------------------------+
```

### Repository Structure

```
helios-dashboards/
+-- README.md
+-- .github/
|   +-- workflows/
|       +-- validate.yaml        # PR validation
|       +-- deploy.yaml          # Deploy on merge
+-- dashboards/
|   +-- core/                    # Core Helios dashboards
|   |   +-- _folder.json         # Folder metadata
|   |   +-- overview.json
|   |   +-- device-health.json
|   |   +-- interface-analytics.json
|   |   +-- bgp-status.json
|   |   +-- alerts-dashboard.json
|   |   +-- site-overview.json
|   +-- vendor/
|   |   +-- arista/
|   |   |   +-- _folder.json
|   |   |   +-- arista-switch.json
|   |   |   +-- arista-mlag.json
|   |   +-- cisco/
|   |   |   +-- _folder.json
|   |   |   +-- cisco-router.json
|   |   +-- juniper/
|   |       +-- _folder.json
|   |       +-- juniper-mx.json
|   +-- custom/                  # User dashboards
|       +-- _folder.json
|       +-- .gitkeep
+-- provisioning/
|   +-- dashboards.yaml          # Grafana provisioning config
+-- scripts/
    +-- validate.py              # Dashboard validation
    +-- generate-configmaps.sh   # ConfigMap generation
```

### Dashboard JSON Structure

```json
{
  "__inputs": [],
  "__requires": [
    {
      "type": "grafana",
      "id": "grafana",
      "name": "Grafana",
      "version": "10.0.0"
    },
    {
      "type": "datasource",
      "id": "prometheus",
      "name": "Prometheus"
    }
  ],
  "annotations": {
    "list": []
  },
  "description": "Overview of all network devices",
  "editable": false,
  "fiscalYearStartMonth": 0,
  "graphTooltip": 0,
  "id": null,
  "links": [],
  "liveNow": false,
  "panels": [],
  "schemaVersion": 38,
  "tags": ["helios", "network", "overview"],
  "templating": {
    "list": [
      {
        "name": "datasource",
        "type": "datasource",
        "query": "prometheus"
      },
      {
        "name": "site",
        "type": "query",
        "datasource": "${datasource}",
        "query": "label_values(up{job=\"gnmic\"}, site)"
      }
    ]
  },
  "time": {
    "from": "now-1h",
    "to": "now"
  },
  "title": "Network Overview",
  "uid": "helios-overview",
  "version": 1
}
```

### CI Pipeline - Validation

```yaml
# .github/workflows/validate.yaml
name: Validate Dashboards

on:
  pull_request:
    paths:
      - 'dashboards/**/*.json'

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: '3.11'

      - name: Install dependencies
        run: pip install jsonschema promql-parser

      - name: Validate JSON syntax
        run: |
          find dashboards -name "*.json" -exec python -m json.tool {} \; > /dev/null

      - name: Validate dashboard schema
        run: python scripts/validate.py dashboards/

      - name: Check for duplicate UIDs
        run: |
          find dashboards -name "*.json" -exec jq -r '.uid' {} \; | sort | uniq -d | \
          while read uid; do
            echo "ERROR: Duplicate UID found: $uid"
            exit 1
          done

      - name: Validate PromQL queries
        run: python scripts/validate_promql.py dashboards/

  preview:
    runs-on: ubuntu-latest
    needs: validate
    steps:
      - uses: actions/checkout@v4

      - name: Generate preview links
        run: |
          echo "## Dashboard Changes" >> $GITHUB_STEP_SUMMARY
          for f in $(git diff --name-only origin/main...HEAD -- 'dashboards/**/*.json'); do
            title=$(jq -r '.title' "$f")
            echo "- $title ($f)" >> $GITHUB_STEP_SUMMARY
          done
```

### Dashboard Validation Script

```python
#!/usr/bin/env python3
"""Dashboard validation script."""

import json
import sys
from pathlib import Path
from jsonschema import validate, ValidationError

# Minimal Grafana dashboard schema
DASHBOARD_SCHEMA = {
    "type": "object",
    "required": ["title", "uid", "panels", "schemaVersion"],
    "properties": {
        "title": {"type": "string", "minLength": 1},
        "uid": {"type": "string", "pattern": "^[a-zA-Z0-9_-]+$"},
        "panels": {"type": "array"},
        "schemaVersion": {"type": "integer", "minimum": 30},
        "tags": {"type": "array", "items": {"type": "string"}},
        "editable": {"type": "boolean"},
        "templating": {
            "type": "object",
            "properties": {
                "list": {"type": "array"}
            }
        }
    }
}

# Required standard variables
REQUIRED_VARIABLES = ["datasource"]

# Required tags for Helios dashboards
REQUIRED_TAGS = ["helios"]

def validate_dashboard(filepath: Path) -> list[str]:
    """Validate a single dashboard file."""
    errors = []

    try:
        with open(filepath) as f:
            dashboard = json.load(f)
    except json.JSONDecodeError as e:
        return [f"Invalid JSON: {e}"]

    # Schema validation
    try:
        validate(dashboard, DASHBOARD_SCHEMA)
    except ValidationError as e:
        errors.append(f"Schema error: {e.message}")

    # Check required variables
    variables = dashboard.get("templating", {}).get("list", [])
    var_names = {v.get("name") for v in variables}
    for required in REQUIRED_VARIABLES:
        if required not in var_names:
            errors.append(f"Missing required variable: {required}")

    # Check required tags
    tags = set(dashboard.get("tags", []))
    for required in REQUIRED_TAGS:
        if required not in tags:
            errors.append(f"Missing required tag: {required}")

    # Check for hardcoded datasource references
    dashboard_str = json.dumps(dashboard)
    if '"datasource": "Prometheus"' in dashboard_str:
        errors.append("Hardcoded datasource 'Prometheus' found. Use variable ${datasource}")

    # Check panels have titles
    for i, panel in enumerate(dashboard.get("panels", [])):
        if panel.get("type") not in ["row"] and not panel.get("title"):
            errors.append(f"Panel {i} missing title")

    return errors

def main():
    dashboard_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("dashboards")

    all_errors = []
    for filepath in dashboard_dir.rglob("*.json"):
        if filepath.name.startswith("_"):
            continue  # Skip folder metadata files

        errors = validate_dashboard(filepath)
        if errors:
            all_errors.append((filepath, errors))

    if all_errors:
        print("Dashboard validation failed:")
        for filepath, errors in all_errors:
            print(f"\n{filepath}:")
            for error in errors:
                print(f"  - {error}")
        sys.exit(1)
    else:
        print("All dashboards validated successfully")
        sys.exit(0)

if __name__ == "__main__":
    main()
```

### CD Pipeline - Deployment

```yaml
# .github/workflows/deploy.yaml
name: Deploy Dashboards

on:
  push:
    branches: [main]
    paths:
      - 'dashboards/**/*.json'

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Generate ConfigMaps
        run: ./scripts/generate-configmaps.sh

      - name: Configure kubectl
        uses: azure/k8s-set-context@v3
        with:
          kubeconfig: ${{ secrets.KUBECONFIG }}

      - name: Apply ConfigMaps
        run: kubectl apply -f generated/configmaps/

      - name: Notify Slack
        uses: slackapi/slack-github-action@v1
        with:
          channel-id: '#helios-deploys'
          slack-message: |
            Dashboard deployment completed
            Commit: ${{ github.sha }}
            Author: ${{ github.actor }}
```

### ConfigMap Generation

```bash
#!/bin/bash
# scripts/generate-configmaps.sh

OUTPUT_DIR="generated/configmaps"
NAMESPACE="helios-visualization"

mkdir -p "$OUTPUT_DIR"

# Generate ConfigMap for each dashboard folder
for folder in dashboards/*/; do
    folder_name=$(basename "$folder")

    # Skip if no JSON files
    if ! ls "$folder"/*.json 1> /dev/null 2>&1; then
        continue
    fi

    cat > "$OUTPUT_DIR/grafana-dashboards-${folder_name}.yaml" <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-dashboards-${folder_name}
  namespace: ${NAMESPACE}
  labels:
    app.kubernetes.io/part-of: helios
    grafana_dashboard: "1"
  annotations:
    grafana_folder: "${folder_name^}"
data:
EOF

    # Add each dashboard as a data key
    for dashboard in "$folder"/*.json; do
        if [[ $(basename "$dashboard") == _* ]]; then
            continue  # Skip metadata files
        fi

        filename=$(basename "$dashboard")
        echo "  ${filename}: |" >> "$OUTPUT_DIR/grafana-dashboards-${folder_name}.yaml"
        sed 's/^/    /' "$dashboard" >> "$OUTPUT_DIR/grafana-dashboards-${folder_name}.yaml"
    done
done

echo "Generated ConfigMaps in $OUTPUT_DIR"
```

### Grafana Provisioning Configuration

```yaml
# Grafana deployment with sidecar
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: helios-visualization
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: grafana
        image: grafana/grafana:10.2.0
        env:
        - name: GF_PATHS_PROVISIONING
          value: /etc/grafana/provisioning
        - name: GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH
          value: /var/lib/grafana/dashboards/core/overview.json
        # Disable UI editing to enforce Git workflow
        - name: GF_DASHBOARDS_VERSIONS_TO_KEEP
          value: "20"
        volumeMounts:
        - name: grafana-provisioning
          mountPath: /etc/grafana/provisioning
        - name: dashboards-core
          mountPath: /var/lib/grafana/dashboards/core
        - name: dashboards-vendor
          mountPath: /var/lib/grafana/dashboards/vendor
        - name: dashboards-custom
          mountPath: /var/lib/grafana/dashboards/custom

      - name: dashboard-sidecar
        image: kiwigrid/k8s-sidecar:1.25.0
        env:
        - name: LABEL
          value: "grafana_dashboard"
        - name: LABEL_VALUE
          value: "1"
        - name: FOLDER
          value: /var/lib/grafana/dashboards
        - name: FOLDER_ANNOTATION
          value: "grafana_folder"
        - name: NAMESPACE
          value: helios-visualization
        - name: RESOURCE
          value: configmap
        volumeMounts:
        - name: dashboards-core
          mountPath: /var/lib/grafana/dashboards/core
        - name: dashboards-vendor
          mountPath: /var/lib/grafana/dashboards/vendor
        - name: dashboards-custom
          mountPath: /var/lib/grafana/dashboards/custom

      volumes:
      - name: grafana-provisioning
        configMap:
          name: grafana-provisioning
      - name: dashboards-core
        emptyDir: {}
      - name: dashboards-vendor
        emptyDir: {}
      - name: dashboards-custom
        emptyDir: {}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-provisioning
  namespace: helios-visualization
data:
  dashboards.yaml: |
    apiVersion: 1
    providers:
      - name: 'core'
        folder: 'Network Overview'
        type: file
        disableDeletion: true
        editable: false
        options:
          path: /var/lib/grafana/dashboards/core

      - name: 'vendor'
        folder: 'Vendor Specific'
        type: file
        disableDeletion: true
        editable: false
        options:
          path: /var/lib/grafana/dashboards/vendor

      - name: 'custom'
        folder: 'Custom'
        type: file
        disableDeletion: false
        editable: true
        options:
          path: /var/lib/grafana/dashboards/custom

  datasources.yaml: |
    apiVersion: 1
    datasources:
      - name: Thanos
        type: prometheus
        url: http://thanos-query.helios-storage.svc:9090
        isDefault: true
        jsonData:
          timeInterval: "15s"
```

### Dashboard Folder Organization

| Folder | Purpose | Editable | Delete-able |
|--------|---------|----------|-------------|
| Network Overview | Core Helios dashboards | No | No |
| Vendor Specific | Vendor/platform dashboards | No | No |
| Custom | User-created dashboards | Yes | Yes |

## Consequences

### Positive

1. **Version Control**: All dashboards tracked in Git with full history
2. **Review Process**: PRs enable code review for dashboard changes
3. **Validation**: CI catches errors before deployment
4. **GitOps**: Dashboards deployed through standard GitOps pipeline
5. **Consistency**: Same dashboards across all environments
6. **Rollback**: Easy revert via Git
7. **Collaboration**: Multiple contributors with PR workflow
8. **Organization**: Clear folder structure with metadata

### Negative

1. **Learning Curve**: Users must use Git instead of Grafana UI
2. **Two-Step Process**: Edit JSON, push to Git, wait for deploy
3. **JSON Complexity**: Dashboard JSON is verbose and hard to read
4. **Merge Conflicts**: Dashboard JSON can have complex diffs
5. **No Live Preview**: Must deploy to see changes

### Neutral

1. **ConfigMap Limits**: ~1MB limit per ConfigMap (split if needed)
2. **Sidecar Reload**: ~30s delay for changes to appear
3. **Export Workflow**: Users can design in staging, export JSON

## Alternatives Considered

### Option 1: Grafana Database (UI-Only)

**Description**: Use Grafana's built-in SQLite/PostgreSQL for dashboard storage

**Pros**:
- Native Grafana experience
- Immediate preview
- No external tooling
- Built-in versioning (limited)

**Cons**:
- No Git history
- No PR review process
- Hard to sync across environments
- Database backup required
- UI sprawl without governance

**Why Not Selected**: Lacks version control and review process. Critical for enterprise governance.

### Option 2: Grafana Operator CRDs

**Description**: Use GrafanaDashboard CRDs from the Grafana Operator

**Pros**:
- Kubernetes-native
- Reconciliation loop
- Status reporting
- Direct API integration

**Cons**:
- Operator adds complexity
- Less mature than ConfigMap approach
- CRD versioning challenges
- Larger YAML than ConfigMaps

**Why Not Selected**: Grafana Operator is evolving but ConfigMaps are simpler and more proven. Consider for v2 when operator matures.

### Option 3: Grafonnet / Jsonnet

**Description**: Generate dashboards from Jsonnet code

**Pros**:
- Programmable dashboards
- Reusable components
- Less verbose than JSON
- Type-safe-ish

**Cons**:
- Learning curve for Jsonnet
- Build step required
- Harder to debug
- Not standard Grafana format

**Why Not Selected**: Adds complexity without clear benefit for our use case. Consider for advanced dashboard templating later.

### Option 4: Terraform grafana Provider

**Description**: Manage dashboards via Terraform

**Pros**:
- Infrastructure-as-code
- State management
- Drift detection
- Works with existing Terraform workflows

**Cons**:
- Terraform dependency
- State file management
- Slower than ConfigMaps
- Dashboard JSON embedded in HCL

**Why Not Selected**: Adds Terraform dependency. ConfigMaps are simpler for Kubernetes-native deployments.

## Implementation Notes

### Exporting Dashboards from Grafana UI

For users who prefer the Grafana UI workflow:

```bash
# Export dashboard JSON from Grafana
curl -H "Authorization: Bearer ${GRAFANA_TOKEN}" \
  "https://grafana.example.com/api/dashboards/uid/my-dashboard" \
  | jq '.dashboard' > my-dashboard.json

# Clean up exported JSON for Git
python scripts/clean-export.py my-dashboard.json
```

### Clean Export Script

```python
#!/usr/bin/env python3
"""Clean exported Grafana dashboard for Git storage."""

import json
import sys

def clean_dashboard(filepath: str):
    with open(filepath) as f:
        dashboard = json.load(f)

    # Remove auto-generated fields
    dashboard.pop("id", None)
    dashboard.pop("version", None)

    # Set editable based on folder
    if "custom" in filepath:
        dashboard["editable"] = True
    else:
        dashboard["editable"] = False

    # Ensure required tags
    tags = set(dashboard.get("tags", []))
    tags.add("helios")
    dashboard["tags"] = sorted(list(tags))

    # Reset version
    dashboard["version"] = 1

    with open(filepath, "w") as f:
        json.dump(dashboard, f, indent=2)

    print(f"Cleaned {filepath}")

if __name__ == "__main__":
    for filepath in sys.argv[1:]:
        clean_dashboard(filepath)
```

### Preventing UI Edits

Configure Grafana to discourage UI editing:

```yaml
# grafana.ini via ConfigMap
[dashboards]
# Minimum interval between dashboard refreshes
min_refresh_interval = 10s

[users]
# Viewers cannot edit dashboards
viewers_can_edit = false

[auth.anonymous]
enabled = false
```

### Core Dashboard List

| Dashboard | UID | Description |
|-----------|-----|-------------|
| Network Overview | helios-overview | Fleet-wide status |
| Device Health | helios-device | Per-device metrics |
| Interface Analytics | helios-interface | Interface deep-dive |
| BGP Status | helios-bgp | Routing health |
| Site Overview | helios-site | Per-site aggregation |
| Alerts Dashboard | helios-alerts | Alert management |
| Top Talkers | helios-top | Bandwidth analysis |
| SLA Report | helios-sla | Availability report |

## References

- [Grafana Provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)
- [Grafana Dashboard JSON Model](https://grafana.com/docs/grafana/latest/dashboards/json-model/)
- [k8s-sidecar](https://github.com/kiwigrid/k8s-sidecar)
- [Grafana Operator](https://github.com/grafana-operator/grafana-operator)
- [[adr-003-target-discovery]] - Target discovery strategy
- [[adr-006-alerting-strategy]] - Alerting strategy
- [[../architecture/architecture]] - Full architecture document
