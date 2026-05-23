#adr #decision #runbook #automation

# ADR-004: Runbook Automation Framework

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #automation #runbook

## Context

Helios requires an automation framework for executing network operations (runbooks) triggered by alerts or manual operator actions. The system must:

1. **Integrate with gNMI**: Execute gNMI Set operations for configuration changes
2. **Audit Trail**: Complete logging of who executed what, when, and the results
3. **RBAC**: Role-based access control for different operation types
4. **Approval Workflow**: Support for requiring approval on dangerous operations
5. **Kubernetes Native**: Fit into the existing Kubernetes deployment model
6. **Extensible**: Support custom scripts and integrations beyond gNMI
7. **Version Control**: Runbook definitions stored in Git

Key use cases:
- Interface bounce (disable/enable) to clear errors
- Clear BGP sessions
- Collect diagnostic information during incidents
- Apply emergency configuration changes
- Automated remediation for specific alert conditions

Constraints:
- Must work with existing gnmic deployment for gNMI operations
- Must integrate with existing RBAC (likely OIDC/Dex)
- Runbooks should be defined declaratively, not imperatively

## Decision

We will implement a **Custom Kubernetes Operator with CRDs** for runbook automation.

### Architecture

```
                                    +-------------------+
                                    |   Grafana UI      |
                                    |   (Trigger)       |
                                    +---------+---------+
                                              |
+-------------------+                         v
|   Alert Rule      | ----+         +-------------------+
|   (Auto-trigger)  |     |         |   Runbook API     |
+-------------------+     +-------> |   (Admission)     |
                                    +---------+---------+
                                              |
                                              v
+-------------------+               +-------------------+
|   Runbook CRD     |               | RunbookExecution  |
|   (Definition)    | <------------ |   CRD (Instance)  |
+-------------------+               +---------+---------+
                                              |
                                              v
                                    +-------------------+
                                    |  Runbook Operator |
                                    |   (Controller)    |
                                    +---------+---------+
                                              |
              +------------------+------------+------------+
              |                  |                         |
              v                  v                         v
    +-------------------+  +-------------------+  +-------------------+
    |  Execution Job    |  |  Approval Check   |  |   Audit Logger    |
    |  (K8s Job)        |  |  (Slack/Teams)    |  |  (Elasticsearch)  |
    +-------------------+  +-------------------+  +-------------------+
              |
              v
    +-------------------+
    |   gnmic Client    |
    |   (gNMI Set/Get)  |
    +-------------------+
              |
              v
    +-------------------+
    |  Network Device   |
    +-------------------+
```

### Custom Resource Definitions

#### Runbook CRD (Definition)

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: runbooks.helios.io
spec:
  group: helios.io
  names:
    kind: Runbook
    listKind: RunbookList
    plural: runbooks
    singular: runbook
    shortNames: [rb]
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          required: [spec]
          properties:
            spec:
              type: object
              required: [name, description, steps]
              properties:
                name:
                  type: string
                  description: Human-readable runbook name
                description:
                  type: string
                  description: Detailed description of what this runbook does
                category:
                  type: string
                  enum: [interface, bgp, system, security, diagnostic, custom]
                  default: custom
                riskLevel:
                  type: string
                  enum: [low, medium, high, critical]
                  default: medium
                requiresApproval:
                  type: boolean
                  default: false
                approvers:
                  type: array
                  items:
                    type: object
                    properties:
                      type:
                        type: string
                        enum: [user, group]
                      name:
                        type: string
                approvalTimeout:
                  type: string
                  default: "1h"
                allowedRoles:
                  type: array
                  items:
                    type: string
                  default: ["network-admin"]
                cooldown:
                  type: string
                  description: Minimum time between executions on same target
                  default: "0s"
                parameters:
                  type: array
                  items:
                    type: object
                    required: [name, type]
                    properties:
                      name:
                        type: string
                      type:
                        type: string
                        enum: [string, integer, boolean, device, interface, select]
                      required:
                        type: boolean
                        default: true
                      default:
                        x-kubernetes-preserve-unknown-fields: true
                      description:
                        type: string
                      validation:
                        type: string
                        description: Regex pattern for validation
                      options:
                        type: array
                        items:
                          type: string
                        description: Options for select type
                steps:
                  type: array
                  minItems: 1
                  items:
                    type: object
                    required: [name, action]
                    properties:
                      name:
                        type: string
                      action:
                        type: string
                        enum: [gnmi_set, gnmi_get, gnmi_subscribe, script, wait, notify, condition]
                      continueOnError:
                        type: boolean
                        default: false
                      timeout:
                        type: string
                        default: "30s"
                      config:
                        type: object
                        x-kubernetes-preserve-unknown-fields: true
      additionalPrinterColumns:
        - name: Category
          type: string
          jsonPath: .spec.category
        - name: Risk
          type: string
          jsonPath: .spec.riskLevel
        - name: Approval
          type: boolean
          jsonPath: .spec.requiresApproval
```

#### RunbookExecution CRD (Instance)

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: runbookexecutions.helios.io
spec:
  group: helios.io
  names:
    kind: RunbookExecution
    listKind: RunbookExecutionList
    plural: runbookexecutions
    singular: runbookexecution
    shortNames: [rbe, exec]
  scope: Namespaced
  versions:
    - name: v1alpha1
      served: true
      storage: true
      subresources:
        status: {}
      schema:
        openAPIV3Schema:
          type: object
          required: [spec]
          properties:
            spec:
              type: object
              required: [runbookRef, parameters]
              properties:
                runbookRef:
                  type: object
                  required: [name]
                  properties:
                    name:
                      type: string
                    namespace:
                      type: string
                parameters:
                  type: object
                  x-kubernetes-preserve-unknown-fields: true
                triggeredBy:
                  type: string
                  description: User or system that triggered execution
                triggerSource:
                  type: string
                  enum: [manual, alert, scheduled, api]
                  default: manual
                alertRef:
                  type: string
                  description: Reference to triggering alert (if alert-triggered)
                dryRun:
                  type: boolean
                  default: false
            status:
              type: object
              properties:
                phase:
                  type: string
                  enum: [Pending, PendingApproval, Approved, Running, Completed, Failed, Cancelled, TimedOut]
                startTime:
                  type: string
                  format: date-time
                completionTime:
                  type: string
                  format: date-time
                approvedBy:
                  type: string
                approvedAt:
                  type: string
                  format: date-time
                message:
                  type: string
                steps:
                  type: array
                  items:
                    type: object
                    properties:
                      name:
                        type: string
                      status:
                        type: string
                        enum: [Pending, Running, Completed, Failed, Skipped]
                      startTime:
                        type: string
                        format: date-time
                      completionTime:
                        type: string
                        format: date-time
                      output:
                        type: string
                      error:
                        type: string
      additionalPrinterColumns:
        - name: Runbook
          type: string
          jsonPath: .spec.runbookRef.name
        - name: Phase
          type: string
          jsonPath: .status.phase
        - name: Triggered By
          type: string
          jsonPath: .spec.triggeredBy
        - name: Age
          type: date
          jsonPath: .metadata.creationTimestamp
```

### Example Runbooks

#### Interface Bounce

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: interface-bounce
  namespace: helios-automation
  labels:
    helios.io/category: interface
spec:
  name: Bounce Interface
  description: |
    Administratively disables and re-enables an interface.
    Useful for clearing interface errors or resetting port state.

    WARNING: This will cause traffic disruption on the interface.
  category: interface
  riskLevel: medium
  requiresApproval: false
  allowedRoles:
    - network-engineer
    - network-admin
  cooldown: "5m"

  parameters:
    - name: device
      type: device
      required: true
      description: Target device hostname (must exist in NetBox)
    - name: interface
      type: interface
      required: true
      description: Interface to bounce (e.g., Ethernet1/1)
    - name: wait_seconds
      type: integer
      required: false
      default: 5
      description: Seconds to wait before re-enabling (1-60)
      validation: "^([1-9]|[1-5][0-9]|60)$"

  steps:
    - name: Verify Interface Exists
      action: gnmi_get
      timeout: 10s
      config:
        path: /interfaces/interface[name={{interface}}]/state/admin-status
        target: "{{device}}"
        failIfNotFound: true

    - name: Disable Interface
      action: gnmi_set
      timeout: 15s
      config:
        target: "{{device}}"
        updates:
          - path: /interfaces/interface[name={{interface}}]/config/enabled
            value: false
            encoding: json_ietf

    - name: Wait for Link Down
      action: wait
      config:
        duration: "{{wait_seconds}}s"

    - name: Enable Interface
      action: gnmi_set
      timeout: 15s
      config:
        target: "{{device}}"
        updates:
          - path: /interfaces/interface[name={{interface}}]/config/enabled
            value: true
            encoding: json_ietf

    - name: Verify Interface Up
      action: gnmi_get
      timeout: 60s
      config:
        target: "{{device}}"
        path: /interfaces/interface[name={{interface}}]/state/oper-status
        retryUntil: "UP"
        retryInterval: 5s

    - name: Notify Completion
      action: notify
      config:
        channel: slack
        webhook: "${SLACK_WEBHOOK_URL}"
        message: |
          :white_check_mark: Interface bounced successfully
          *Device*: {{device}}
          *Interface*: {{interface}}
          *Executed by*: {{.triggeredBy}}
```

#### Clear BGP Session (Requires Approval)

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: clear-bgp-neighbor
  namespace: helios-automation
spec:
  name: Clear BGP Neighbor Session
  description: |
    Clears a specific BGP neighbor session, causing it to reset and re-establish.
    This will cause temporary traffic disruption for routes learned from this peer.
  category: bgp
  riskLevel: high
  requiresApproval: true
  approvers:
    - type: group
      name: network-admin
    - type: user
      name: oncall@example.com
  approvalTimeout: 30m
  allowedRoles:
    - network-engineer
    - network-admin
  cooldown: "15m"

  parameters:
    - name: device
      type: device
      required: true
    - name: neighbor_ip
      type: string
      required: true
      description: BGP neighbor IP address
      validation: "^(?:[0-9]{1,3}\\.){3}[0-9]{1,3}$"
    - name: reason
      type: string
      required: true
      description: Reason for clearing session (for audit log)

  steps:
    - name: Get Current BGP State
      action: gnmi_get
      config:
        target: "{{device}}"
        path: /network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=BGP]/bgp/neighbors/neighbor[neighbor-address={{neighbor_ip}}]/state

    - name: Clear BGP Session
      action: script
      timeout: 30s
      config:
        image: helios/network-scripts:v1.0.0
        script: clear_bgp.py
        args:
          - --device={{device}}
          - --neighbor={{neighbor_ip}}
          - --reason="{{reason}}"
        env:
          - name: GNMI_USERNAME
            valueFrom:
              secretKeyRef:
                name: gnmi-credentials
                key: username
          - name: GNMI_PASSWORD
            valueFrom:
              secretKeyRef:
                name: gnmi-credentials
                key: password

    - name: Wait for Session Re-establishment
      action: wait
      config:
        duration: 30s

    - name: Verify BGP Session Established
      action: gnmi_get
      timeout: 120s
      config:
        target: "{{device}}"
        path: /network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=BGP]/bgp/neighbors/neighbor[neighbor-address={{neighbor_ip}}]/state/session-state
        retryUntil: "ESTABLISHED"
        retryInterval: 10s

    - name: Notify Completion
      action: notify
      config:
        channel: slack
        message: |
          :arrows_counterclockwise: BGP session cleared and re-established
          *Device*: {{device}}
          *Neighbor*: {{neighbor_ip}}
          *Reason*: {{reason}}
          *Executed by*: {{.triggeredBy}}
          *Approved by*: {{.status.approvedBy}}
```

### Operator Implementation (Go)

```go
// controllers/runbookexecution_controller.go
package controllers

import (
    "context"
    "fmt"
    "time"

    batchv1 "k8s.io/api/batch/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    heliosv1alpha1 "github.com/helios/runbook-operator/api/v1alpha1"
)

type RunbookExecutionReconciler struct {
    client.Client
    Scheme       *runtime.Scheme
    AuditLogger  AuditLogger
    Notifier     Notifier
}

func (r *RunbookExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := log.FromContext(ctx)

    // Fetch the RunbookExecution instance
    var execution heliosv1alpha1.RunbookExecution
    if err := r.Get(ctx, req.NamespacedName, &execution); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Fetch the referenced Runbook
    var runbook heliosv1alpha1.Runbook
    runbookKey := client.ObjectKey{
        Name:      execution.Spec.RunbookRef.Name,
        Namespace: execution.Spec.RunbookRef.Namespace,
    }
    if runbookKey.Namespace == "" {
        runbookKey.Namespace = execution.Namespace
    }
    if err := r.Get(ctx, runbookKey, &runbook); err != nil {
        return r.markFailed(ctx, &execution, fmt.Errorf("runbook not found: %v", err))
    }

    // State machine
    switch execution.Status.Phase {
    case "":
        // New execution - validate and initialize
        if err := r.validateExecution(&execution, &runbook); err != nil {
            return r.markFailed(ctx, &execution, err)
        }
        return r.initializeExecution(ctx, &execution, &runbook)

    case "Pending":
        // Check if approval is required
        if runbook.Spec.RequiresApproval {
            return r.requestApproval(ctx, &execution, &runbook)
        }
        return r.startExecution(ctx, &execution, &runbook)

    case "PendingApproval":
        // Check if approved or timed out
        return r.checkApproval(ctx, &execution, &runbook)

    case "Approved":
        return r.startExecution(ctx, &execution, &runbook)

    case "Running":
        // Check job status
        return r.checkProgress(ctx, &execution)
    }

    return ctrl.Result{}, nil
}

func (r *RunbookExecutionReconciler) initializeExecution(
    ctx context.Context,
    exec *heliosv1alpha1.RunbookExecution,
    runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
    exec.Status.Phase = "Pending"
    exec.Status.Steps = make([]heliosv1alpha1.StepStatus, len(runbook.Spec.Steps))
    for i, step := range runbook.Spec.Steps {
        exec.Status.Steps[i] = heliosv1alpha1.StepStatus{
            Name:   step.Name,
            Status: "Pending",
        }
    }

    // Audit log
    r.AuditLogger.Log(AuditEvent{
        Type:      "runbook.triggered",
        Execution: exec.Name,
        Runbook:   runbook.Name,
        User:      exec.Spec.TriggeredBy,
        Timestamp: time.Now(),
    })

    return ctrl.Result{Requeue: true}, r.Status().Update(ctx, exec)
}

func (r *RunbookExecutionReconciler) requestApproval(
    ctx context.Context,
    exec *heliosv1alpha1.RunbookExecution,
    runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
    exec.Status.Phase = "PendingApproval"

    // Send approval request notification
    r.Notifier.SendApprovalRequest(NotificationRequest{
        Execution:  exec.Name,
        Runbook:    runbook.Spec.Name,
        Parameters: exec.Spec.Parameters,
        TriggeredBy: exec.Spec.TriggeredBy,
        Approvers:  runbook.Spec.Approvers,
        Timeout:    runbook.Spec.ApprovalTimeout,
    })

    // Parse timeout duration
    timeout, _ := time.ParseDuration(runbook.Spec.ApprovalTimeout)

    return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, exec)
}

func (r *RunbookExecutionReconciler) startExecution(
    ctx context.Context,
    exec *heliosv1alpha1.RunbookExecution,
    runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
    // Create execution Job
    job := r.buildExecutionJob(exec, runbook)
    if err := r.Create(ctx, job); err != nil {
        return r.markFailed(ctx, exec, err)
    }

    exec.Status.Phase = "Running"
    exec.Status.StartTime = &metav1.Time{Time: time.Now()}

    r.AuditLogger.Log(AuditEvent{
        Type:      "runbook.started",
        Execution: exec.Name,
        Runbook:   runbook.Name,
        User:      exec.Spec.TriggeredBy,
        Timestamp: time.Now(),
    })

    return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, exec)
}

func (r *RunbookExecutionReconciler) buildExecutionJob(
    exec *heliosv1alpha1.RunbookExecution,
    runbook *heliosv1alpha1.Runbook,
) *batchv1.Job {
    return &batchv1.Job{
        ObjectMeta: metav1.ObjectMeta{
            Name:      exec.Name,
            Namespace: exec.Namespace,
            Labels: map[string]string{
                "helios.io/runbook-execution": exec.Name,
                "helios.io/runbook":           runbook.Name,
            },
            OwnerReferences: []metav1.OwnerReference{
                *metav1.NewControllerRef(exec, heliosv1alpha1.GroupVersion.WithKind("RunbookExecution")),
            },
        },
        Spec: batchv1.JobSpec{
            BackoffLimit: int32Ptr(0), // No retries
            Template: corev1.PodTemplateSpec{
                Spec: corev1.PodSpec{
                    ServiceAccountName: "runbook-executor",
                    Containers: []corev1.Container{{
                        Name:  "executor",
                        Image: "helios/runbook-executor:v1.0.0",
                        Args: []string{
                            "--execution", exec.Name,
                            "--namespace", exec.Namespace,
                        },
                        EnvFrom: []corev1.EnvFromSource{{
                            SecretRef: &corev1.SecretEnvSource{
                                LocalObjectReference: corev1.LocalObjectReference{
                                    Name: "gnmi-credentials",
                                },
                            },
                        }},
                    }},
                    RestartPolicy: corev1.RestartPolicyNever,
                },
            },
        },
    }
}
```

## Consequences

### Positive

1. **Kubernetes Native**: Uses standard K8s patterns (CRDs, Controllers, Jobs)
2. **GitOps Compatible**: Runbook definitions are YAML, stored in Git, deployed via ArgoCD
3. **Complete Audit Trail**: Every execution is a CR with full history
4. **RBAC Integration**: Leverages K8s RBAC and admission webhooks
5. **Approval Workflow**: Built-in support for multi-approver workflows
6. **Extensible**: Support for gNMI, scripts, and custom actions
7. **Declarative**: Runbooks define desired outcome, not implementation details
8. **Isolation**: Each execution runs in its own Job/Pod
9. **Visibility**: kubectl and Grafana can query execution status

### Negative

1. **Development Effort**: Custom operator requires Go development
2. **Learning Curve**: Team needs to learn CRD patterns
3. **Complexity**: More moving parts than simple script execution
4. **Testing**: CRDs and controllers require integration testing

### Neutral

1. **Job Overhead**: Each execution creates a Pod (acceptable for network operations)
2. **Namespace Scope**: Runbooks are namespaced, need cross-namespace RBAC for sharing

## Alternatives Considered

### Option 1: Ansible AWX / Automation Controller

**Description**: Use Red Hat's AWX (upstream of Ansible Automation Platform) for runbook execution

**Pros**:
- Mature, production-proven
- Excellent GUI for operators
- Built-in credential management
- Large community and module library
- Supports approval workflows

**Cons**:
- Not Kubernetes-native (runs as separate system)
- Limited gNMI support (no native modules)
- Ansible playbook format different from K8s manifests
- Separate RBAC system from Kubernetes
- Resource-heavy for simple operations

**Why Not Selected**: AWX is excellent but introduces a separate system. For a K8s-native platform like Helios, a custom operator provides tighter integration. AWX can be reconsidered if Ansible expertise is already present.

### Option 2: Rundeck

**Description**: Open-source runbook automation and job scheduler

**Pros**:
- Mature and stable
- Good web UI
- Built-in approval workflows
- Extensive plugin ecosystem

**Cons**:
- Not Kubernetes-native
- Java-based (different tech stack)
- Separate authentication system
- No native gNMI support
- Requires persistent storage for state

**Why Not Selected**: Rundeck is another non-K8s-native solution. Adds operational complexity without clear benefits over custom operator.

### Option 3: StackStorm

**Description**: Event-driven automation platform with rules engine

**Pros**:
- Powerful event-driven automation
- Extensive integration library (packs)
- Complex workflow support
- OpenStack project heritage

**Cons**:
- Complex architecture (multiple components)
- Steep learning curve
- Resource-intensive
- Not Kubernetes-native
- Overkill for targeted runbook use case

**Why Not Selected**: StackStorm is powerful but significantly more complex than needed. Better suited for organizations needing complex event-driven automation across many systems.

### Option 4: Simple CronJobs / Scripts

**Description**: Deploy runbooks as CronJobs or simple scripts triggered via API

**Pros**:
- Simple to implement
- Uses existing K8s primitives
- Low barrier to entry

**Cons**:
- No approval workflow
- Limited audit trail
- No parameter validation
- Manual RBAC implementation
- No execution status tracking
- Doesn't scale well

**Why Not Selected**: Too simplistic for production use. Lacks essential features like approval workflows and proper audit trails.

## Implementation Notes

### Approval Webhook Handler

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: runbook-approval-webhook
  namespace: helios-automation
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: webhook
        image: helios/approval-webhook:v1.0.0
        ports:
        - containerPort: 8443
        env:
        - name: SLACK_SIGNING_SECRET
          valueFrom:
            secretKeyRef:
              name: slack-credentials
              key: signing-secret
```

### Slack Approval Message Format

```json
{
  "blocks": [
    {
      "type": "header",
      "text": {
        "type": "plain_text",
        "text": "Runbook Approval Required"
      }
    },
    {
      "type": "section",
      "fields": [
        {"type": "mrkdwn", "text": "*Runbook:*\nClear BGP Neighbor"},
        {"type": "mrkdwn", "text": "*Risk Level:*\nHigh"},
        {"type": "mrkdwn", "text": "*Device:*\ncore-rtr-01"},
        {"type": "mrkdwn", "text": "*Requested By:*\njsmith@example.com"}
      ]
    },
    {
      "type": "actions",
      "elements": [
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "Approve"},
          "style": "primary",
          "action_id": "approve",
          "value": "exec-20260109-143022"
        },
        {
          "type": "button",
          "text": {"type": "plain_text", "text": "Reject"},
          "style": "danger",
          "action_id": "reject",
          "value": "exec-20260109-143022"
        }
      ]
    }
  ]
}
```

### Operator RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: runbook-operator
rules:
  - apiGroups: ["helios.io"]
    resources: ["runbooks", "runbookexecutions"]
    verbs: ["*"]
  - apiGroups: ["helios.io"]
    resources: ["runbookexecutions/status"]
    verbs: ["update", "patch"]
  - apiGroups: ["batch"]
    resources: ["jobs"]
    verbs: ["create", "get", "list", "watch", "delete"]
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
    resourceNames: ["gnmi-credentials"]
```

## References

- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Kubebuilder Book](https://book.kubebuilder.io/)
- [gnmic Set Operations](https://gnmic.openconfig.net/cmd/set/)
- [[adr-002-gnmic-deployment]] - gnmic deployment model
- [[adr-003-target-discovery]] - Target discovery strategy
- [[../architecture/architecture]] - Full architecture document
