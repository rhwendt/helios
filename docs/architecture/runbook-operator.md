#architecture #helios #runbook #operator #kubernetes

# Helios Runbook Operator - Detailed Design

**Version**: 1.0
**Date**: 2026-01-10
**Status**: Draft

---

## Table of Contents

1. [Overview](#overview)
2. [Project Structure](#project-structure)
3. [CRD Specifications](#crd-specifications)
4. [Controller Design](#controller-design)
5. [Executor Design](#executor-design)
6. [Approval Workflow](#approval-workflow)
7. [Observability](#observability)
8. [Security Model](#security-model)
9. [Example Runbooks](#example-runbooks)
10. [Sequence Diagrams](#sequence-diagrams)

---

## Overview

The Helios Runbook Operator enables network engineers to define and execute remediation actions on network devices through a Kubernetes-native interface.

### Design Goals

1. **Declarative** - Runbooks are YAML manifests, stored in Git
2. **Auditable** - Every execution is tracked with full history
3. **Safe** - Approval workflows for dangerous operations
4. **Extensible** - Support for gnmic, scripts, and custom actions
5. **Observable** - Prometheus metrics and Grafana dashboards

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                     helios-automation namespace                  │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────────┐     ┌──────────────────┐                  │
│  │ Runbook Operator │────▶│   Executor Job   │                  │
│  │   (Controller)   │     │   (gnmic-based)  │                  │
│  └────────┬─────────┘     └────────┬─────────┘                  │
│           │                        │                             │
│           ▼                        ▼                             │
│  ┌──────────────────┐     ┌──────────────────┐                  │
│  │  Approval        │     │  Network Device  │                  │
│  │  Webhook         │     │  (gNMI target)   │                  │
│  └──────────────────┘     └──────────────────┘                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Project Structure

```
helios-runbook-operator/
├── Makefile
├── Dockerfile
├── go.mod
├── go.sum
├── main.go
│
├── api/
│   └── v1alpha1/
│       ├── groupversion_info.go
│       ├── runbook_types.go
│       ├── runbookexecution_types.go
│       └── zz_generated.deepcopy.go
│
├── config/
│   ├── crd/
│   │   └── bases/
│   │       ├── helios.io_runbooks.yaml
│   │       └── helios.io_runbookexecutions.yaml
│   ├── rbac/
│   │   ├── role.yaml
│   │   ├── role_binding.yaml
│   │   └── service_account.yaml
│   ├── manager/
│   │   └── manager.yaml
│   └── samples/
│       ├── interface-bounce.yaml
│       └── clear-bgp-neighbor.yaml
│
├── controllers/
│   ├── runbook_controller.go
│   ├── runbookexecution_controller.go
│   ├── executor.go
│   └── suite_test.go
│
├── pkg/
│   ├── gnmic/
│   │   ├── client.go
│   │   ├── set.go
│   │   ├── get.go
│   │   └── subscribe.go
│   ├── template/
│   │   ├── engine.go
│   │   └── functions.go
│   ├── approval/
│   │   ├── slack.go
│   │   ├── teams.go
│   │   └── webhook.go
│   └── audit/
│       ├── logger.go
│       └── elasticsearch.go
│
├── cmd/
│   └── executor/
│       └── main.go           # Executor container entrypoint
│
└── helm/
    └── runbook-operator/
        ├── Chart.yaml
        ├── values.yaml
        └── templates/
            ├── deployment.yaml
            ├── crds.yaml
            ├── rbac.yaml
            └── service.yaml
```

### Makefile Targets

```makefile
# Build
build: generate fmt vet
	go build -o bin/manager main.go

build-executor:
	go build -o bin/executor cmd/executor/main.go

# Docker
docker-build:
	docker build -t helios/runbook-operator:$(VERSION) .
	docker build -t helios/runbook-executor:$(VERSION) -f Dockerfile.executor .

# Code generation
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

manifests: controller-gen
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Install/Deploy
install: manifests
	kubectl apply -k config/crd

deploy: manifests
	kubectl apply -k config/default

# Test
test:
	go test ./... -coverprofile cover.out

test-e2e:
	go test ./test/e2e/... -v
```

---

## CRD Specifications

### Runbook CRD

```go
// api/v1alpha1/runbook_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunbookSpec defines a runbook template
type RunbookSpec struct {
	// Name is the human-readable runbook name
	Name string `json:"name"`

	// Description explains what this runbook does
	Description string `json:"description"`

	// Category groups related runbooks
	// +kubebuilder:validation:Enum=interface;bgp;system;security;diagnostic;custom
	// +kubebuilder:default=custom
	Category string `json:"category,omitempty"`

	// RiskLevel indicates the potential impact
	// +kubebuilder:validation:Enum=low;medium;high;critical
	// +kubebuilder:default=medium
	RiskLevel string `json:"riskLevel,omitempty"`

	// RequiresApproval indicates if execution needs approval
	// +kubebuilder:default=false
	RequiresApproval bool `json:"requiresApproval,omitempty"`

	// Approvers defines who can approve executions
	Approvers []Approver `json:"approvers,omitempty"`

	// ApprovalTimeout is how long to wait for approval
	// +kubebuilder:default="1h"
	ApprovalTimeout string `json:"approvalTimeout,omitempty"`

	// AllowedRoles defines who can execute this runbook
	// +kubebuilder:default={"network-admin"}
	AllowedRoles []string `json:"allowedRoles,omitempty"`

	// Cooldown is minimum time between executions on same target
	// +kubebuilder:default="0s"
	Cooldown string `json:"cooldown,omitempty"`

	// Parameters defines input parameters
	Parameters []Parameter `json:"parameters,omitempty"`

	// Steps defines the execution steps
	// +kubebuilder:validation:MinItems=1
	Steps []Step `json:"steps"`

	// Rollback defines optional rollback steps
	Rollback []Step `json:"rollback,omitempty"`
}

// Parameter defines an input parameter
type Parameter struct {
	// Name is the parameter name (used in templates)
	Name string `json:"name"`

	// Type is the parameter type
	// +kubebuilder:validation:Enum=string;integer;boolean;device;interface;select
	Type string `json:"type"`

	// Required indicates if parameter must be provided
	// +kubebuilder:default=true
	Required bool `json:"required,omitempty"`

	// Default is the default value if not provided
	// +kubebuilder:pruning:PreserveUnknownFields
	Default *apiextensionsv1.JSON `json:"default,omitempty"`

	// Description explains the parameter
	Description string `json:"description,omitempty"`

	// Validation is a regex pattern for validation
	Validation string `json:"validation,omitempty"`

	// Options are valid values for select type
	Options []string `json:"options,omitempty"`
}

// Step defines a single execution step
type Step struct {
	// Name is the step name (for display)
	Name string `json:"name"`

	// Action is the step type
	// +kubebuilder:validation:Enum=gnmi_set;gnmi_get;gnmi_subscribe;wait;notify;condition;script
	Action string `json:"action"`

	// Timeout for this step
	// +kubebuilder:default="30s"
	Timeout string `json:"timeout,omitempty"`

	// ContinueOnError allows execution to continue if step fails
	// +kubebuilder:default=false
	ContinueOnError bool `json:"continueOnError,omitempty"`

	// Config is action-specific configuration
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *apiextensionsv1.JSON `json:"config,omitempty"`
}

// Approver defines who can approve
type Approver struct {
	// Type is user or group
	// +kubebuilder:validation:Enum=user;group
	Type string `json:"type"`

	// Name is the user email or group name
	Name string `json:"name"`
}

// Runbook is the Schema for runbooks
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Category",type=string,JSONPath=`.spec.category`
// +kubebuilder:printcolumn:name="Risk",type=string,JSONPath=`.spec.riskLevel`
// +kubebuilder:printcolumn:name="Approval",type=boolean,JSONPath=`.spec.requiresApproval`
type Runbook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RunbookSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
type RunbookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Runbook `json:"items"`
}
```

### RunbookExecution CRD

```go
// api/v1alpha1/runbookexecution_types.go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunbookExecutionSpec defines the desired state
type RunbookExecutionSpec struct {
	// RunbookRef references the runbook to execute
	RunbookRef RunbookReference `json:"runbookRef"`

	// Parameters are the input values
	// +kubebuilder:pruning:PreserveUnknownFields
	Parameters map[string]apiextensionsv1.JSON `json:"parameters"`

	// TriggeredBy identifies who triggered this
	TriggeredBy string `json:"triggeredBy,omitempty"`

	// TriggerSource indicates how it was triggered
	// +kubebuilder:validation:Enum=manual;alert;scheduled;api
	// +kubebuilder:default=manual
	TriggerSource string `json:"triggerSource,omitempty"`

	// AlertRef is reference to triggering alert (if alert-triggered)
	AlertRef string `json:"alertRef,omitempty"`

	// DryRun simulates execution without making changes
	// +kubebuilder:default=false
	DryRun bool `json:"dryRun,omitempty"`
}

// RunbookReference identifies a runbook
type RunbookReference struct {
	// Name of the runbook
	Name string `json:"name"`

	// Namespace of the runbook (defaults to execution namespace)
	Namespace string `json:"namespace,omitempty"`
}

// RunbookExecutionStatus defines the observed state
type RunbookExecutionStatus struct {
	// Phase is the current execution phase
	// +kubebuilder:validation:Enum=Pending;PendingApproval;Approved;Running;Completed;Failed;Cancelled;TimedOut;RollingBack;RolledBack
	Phase string `json:"phase,omitempty"`

	// StartTime is when execution started
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when execution finished
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// ApprovedBy is who approved the execution
	ApprovedBy string `json:"approvedBy,omitempty"`

	// ApprovedAt is when it was approved
	ApprovedAt *metav1.Time `json:"approvedAt,omitempty"`

	// Message provides additional status details
	Message string `json:"message,omitempty"`

	// Steps tracks status of each step
	Steps []StepStatus `json:"steps,omitempty"`

	// JobName is the name of the executor Job
	JobName string `json:"jobName,omitempty"`
}

// StepStatus tracks a single step
type StepStatus struct {
	// Name of the step
	Name string `json:"name"`

	// Status of the step
	// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed;Skipped
	Status string `json:"status"`

	// StartTime of this step
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime of this step
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Output captured from the step
	Output string `json:"output,omitempty"`

	// Error message if step failed
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Runbook",type=string,JSONPath=`.spec.runbookRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Triggered By",type=string,JSONPath=`.spec.triggeredBy`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type RunbookExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunbookExecutionSpec   `json:"spec,omitempty"`
	Status RunbookExecutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RunbookExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RunbookExecution `json:"items"`
}
```

---

## Controller Design

### State Machine

```
                    ┌─────────────────┐
                    │    (created)    │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
             ┌──────│     Pending     │──────┐
             │      └─────────────────┘      │
             │ (approval                     │ (no approval
             │  required)                    │  required)
             ▼                               ▼
    ┌─────────────────┐             ┌─────────────────┐
    │ PendingApproval │             │                 │
    └────────┬────────┘             │                 │
             │                      │                 │
     ┌───────┼───────┐              │                 │
     │       │       │              │                 │
     ▼       ▼       ▼              │                 │
┌────────┐ ┌────────┐ ┌──────────┐  │                 │
│Approved│ │Rejected│ │ TimedOut │  │                 │
└───┬────┘ └────────┘ └──────────┘  │                 │
    │                               │                 │
    └───────────────┬───────────────┘                 │
                    ▼                                 │
           ┌─────────────────┐                        │
           │     Running     │◀───────────────────────┘
           └────────┬────────┘
                    │
           ┌────────┴────────┐
           ▼                 ▼
  ┌─────────────────┐  ┌─────────────────┐
  │    Completed    │  │     Failed      │
  └─────────────────┘  └────────┬────────┘
                                │
                       (has rollback?)
                                │
                       ┌────────┴────────┐
                       ▼                 ▼
              ┌─────────────────┐  ┌──────────┐
              │   RollingBack   │  │  (done)  │
              └────────┬────────┘  └──────────┘
                       │
              ┌────────┴────────┐
              ▼                 ▼
     ┌─────────────────┐  ┌─────────────────┐
     │   RolledBack    │  │  RollbackFailed │
     └─────────────────┘  └─────────────────┘
```

### Controller Implementation

```go
// controllers/runbookexecution_controller.go
package controllers

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	heliosv1alpha1 "github.com/helios/runbook-operator/api/v1alpha1"
	"github.com/helios/runbook-operator/pkg/approval"
	"github.com/helios/runbook-operator/pkg/audit"
)

const (
	finalizerName = "runbookexecution.helios.io/finalizer"
)

type RunbookExecutionReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	AuditLogger  *audit.Logger
	Approver     approval.Approver
	ExecutorImage string
}

// +kubebuilder:rbac:groups=helios.io,resources=runbookexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helios.io,resources=runbookexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helios.io,resources=runbookexecutions/finalizers,verbs=update
// +kubebuilder:rbac:groups=helios.io,resources=runbooks,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

func (r *RunbookExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the execution
	var exec heliosv1alpha1.RunbookExecution
	if err := r.Get(ctx, req.NamespacedName, &exec); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Fetch referenced runbook
	runbook, err := r.getRunbook(ctx, &exec)
	if err != nil {
		return r.setFailed(ctx, &exec, fmt.Sprintf("runbook not found: %v", err))
	}

	// Handle finalizer for cleanup
	if exec.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, &exec)
	}

	// Add finalizer if missing
	if !controllerutil.ContainsFinalizer(&exec, finalizerName) {
		controllerutil.AddFinalizer(&exec, finalizerName)
		if err := r.Update(ctx, &exec); err != nil {
			return ctrl.Result{}, err
		}
	}

	// State machine
	switch exec.Status.Phase {
	case "":
		return r.handleNew(ctx, &exec, runbook)
	case "Pending":
		return r.handlePending(ctx, &exec, runbook)
	case "PendingApproval":
		return r.handlePendingApproval(ctx, &exec, runbook)
	case "Approved":
		return r.handleApproved(ctx, &exec, runbook)
	case "Running":
		return r.handleRunning(ctx, &exec, runbook)
	case "RollingBack":
		return r.handleRollingBack(ctx, &exec, runbook)
	default:
		// Terminal states: Completed, Failed, Cancelled, TimedOut, RolledBack
		return ctrl.Result{}, nil
	}
}

func (r *RunbookExecutionReconciler) handleNew(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Initializing new execution")

	// Validate parameters
	if err := r.validateParameters(exec, runbook); err != nil {
		return r.setFailed(ctx, exec, fmt.Sprintf("validation failed: %v", err))
	}

	// Check cooldown
	if err := r.checkCooldown(ctx, exec, runbook); err != nil {
		return r.setFailed(ctx, exec, fmt.Sprintf("cooldown active: %v", err))
	}

	// Initialize step status
	exec.Status.Steps = make([]heliosv1alpha1.StepStatus, len(runbook.Spec.Steps))
	for i, step := range runbook.Spec.Steps {
		exec.Status.Steps[i] = heliosv1alpha1.StepStatus{
			Name:   step.Name,
			Status: "Pending",
		}
	}

	exec.Status.Phase = "Pending"

	// Audit log
	r.AuditLogger.Log(audit.Event{
		Type:       "runbook.triggered",
		Execution:  exec.Name,
		Namespace:  exec.Namespace,
		Runbook:    runbook.Name,
		User:       exec.Spec.TriggeredBy,
		Parameters: exec.Spec.Parameters,
		Timestamp:  time.Now(),
	})

	if err := r.Status().Update(ctx, exec); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *RunbookExecutionReconciler) handlePending(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	if runbook.Spec.RequiresApproval {
		log.Info("Requesting approval")

		// Send approval request
		if err := r.Approver.RequestApproval(ctx, approval.Request{
			ExecutionName:      exec.Name,
			ExecutionNamespace: exec.Namespace,
			RunbookName:        runbook.Spec.Name,
			RiskLevel:          runbook.Spec.RiskLevel,
			TriggeredBy:        exec.Spec.TriggeredBy,
			Parameters:         exec.Spec.Parameters,
			Approvers:          runbook.Spec.Approvers,
			Timeout:            runbook.Spec.ApprovalTimeout,
		}); err != nil {
			log.Error(err, "Failed to send approval request")
		}

		exec.Status.Phase = "PendingApproval"
		if err := r.Status().Update(ctx, exec); err != nil {
			return ctrl.Result{}, err
		}

		// Check approval status periodically
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// No approval needed, proceed to execution
	return r.startExecution(ctx, exec, runbook)
}

func (r *RunbookExecutionReconciler) handlePendingApproval(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Check if approved (via annotation set by webhook)
	if approved, approver := r.checkApprovalStatus(exec); approved {
		log.Info("Execution approved", "approver", approver)

		exec.Status.Phase = "Approved"
		exec.Status.ApprovedBy = approver
		now := metav1.Now()
		exec.Status.ApprovedAt = &now

		r.AuditLogger.Log(audit.Event{
			Type:      "runbook.approved",
			Execution: exec.Name,
			Namespace: exec.Namespace,
			Runbook:   runbook.Name,
			User:      approver,
			Timestamp: time.Now(),
		})

		if err := r.Status().Update(ctx, exec); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Check for rejection
	if rejected, rejecter := r.checkRejectionStatus(exec); rejected {
		log.Info("Execution rejected", "rejecter", rejecter)
		return r.setFailed(ctx, exec, fmt.Sprintf("rejected by %s", rejecter))
	}

	// Check timeout
	timeout, _ := time.ParseDuration(runbook.Spec.ApprovalTimeout)
	if time.Since(exec.CreationTimestamp.Time) > timeout {
		log.Info("Approval timeout")
		exec.Status.Phase = "TimedOut"
		exec.Status.Message = "Approval timeout exceeded"
		if err := r.Status().Update(ctx, exec); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Continue waiting
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) handleApproved(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	return r.startExecution(ctx, exec, runbook)
}

func (r *RunbookExecutionReconciler) startExecution(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Starting execution")

	// Create executor Job
	job := r.buildExecutorJob(exec, runbook)
	if err := ctrl.SetControllerReference(exec, job, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, job); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return r.setFailed(ctx, exec, fmt.Sprintf("failed to create job: %v", err))
		}
	}

	exec.Status.Phase = "Running"
	exec.Status.JobName = job.Name
	now := metav1.Now()
	exec.Status.StartTime = &now

	r.AuditLogger.Log(audit.Event{
		Type:      "runbook.started",
		Execution: exec.Name,
		Namespace: exec.Namespace,
		Runbook:   runbook.Name,
		User:      exec.Spec.TriggeredBy,
		Timestamp: time.Now(),
	})

	if err := r.Status().Update(ctx, exec); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) handleRunning(
	ctx context.Context,
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get the Job
	var job batchv1.Job
	jobKey := client.ObjectKey{
		Name:      exec.Status.JobName,
		Namespace: exec.Namespace,
	}
	if err := r.Get(ctx, jobKey, &job); err != nil {
		if apierrors.IsNotFound(err) {
			return r.setFailed(ctx, exec, "executor job not found")
		}
		return ctrl.Result{}, err
	}

	// Check Job status
	if job.Status.Succeeded > 0 {
		log.Info("Execution completed successfully")
		exec.Status.Phase = "Completed"
		now := metav1.Now()
		exec.Status.CompletionTime = &now

		r.AuditLogger.Log(audit.Event{
			Type:      "runbook.completed",
			Execution: exec.Name,
			Namespace: exec.Namespace,
			Runbook:   runbook.Name,
			User:      exec.Spec.TriggeredBy,
			Timestamp: time.Now(),
		})

		if err := r.Status().Update(ctx, exec); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if job.Status.Failed > 0 {
		log.Info("Execution failed")

		// Check if rollback is defined
		if len(runbook.Spec.Rollback) > 0 {
			exec.Status.Phase = "RollingBack"
			exec.Status.Message = "Execution failed, initiating rollback"
		} else {
			exec.Status.Phase = "Failed"
			exec.Status.Message = "Execution failed"
			now := metav1.Now()
			exec.Status.CompletionTime = &now
		}

		r.AuditLogger.Log(audit.Event{
			Type:      "runbook.failed",
			Execution: exec.Name,
			Namespace: exec.Namespace,
			Runbook:   runbook.Name,
			User:      exec.Spec.TriggeredBy,
			Timestamp: time.Now(),
		})

		if err := r.Status().Update(ctx, exec); err != nil {
			return ctrl.Result{}, err
		}

		if exec.Status.Phase == "RollingBack" {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, nil
	}

	// Still running
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) buildExecutorJob(
	exec *heliosv1alpha1.RunbookExecution,
	runbook *heliosv1alpha1.Runbook,
) *batchv1.Job {
	backoffLimit := int32(0)
	ttlSeconds := int32(3600) // Clean up after 1 hour

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-executor", exec.Name),
			Namespace: exec.Namespace,
			Labels: map[string]string{
				"helios.io/runbook-execution": exec.Name,
				"helios.io/runbook":           runbook.Name,
				"app.kubernetes.io/component": "executor",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttlSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"helios.io/runbook-execution": exec.Name,
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "runbook-executor",
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "executor",
							Image: r.ExecutorImage,
							Args: []string{
								"--execution-name", exec.Name,
								"--execution-namespace", exec.Namespace,
							},
							Env: []corev1.EnvVar{
								{
									Name: "GNMI_USERNAME",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "gnmi-credentials",
											},
											Key: "username",
										},
									},
								},
								{
									Name: "GNMI_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "gnmi-credentials",
											},
											Key: "password",
										},
									},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *RunbookExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&heliosv1alpha1.RunbookExecution{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
```

---

## Executor Design

The executor runs as a Kubernetes Job and executes runbook steps using gnmic.

### Executor Main

```go
// cmd/executor/main.go
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/helios/runbook-operator/pkg/executor"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func main() {
	var executionName string
	var executionNamespace string

	flag.StringVar(&executionName, "execution-name", "", "RunbookExecution name")
	flag.StringVar(&executionNamespace, "execution-namespace", "", "RunbookExecution namespace")
	flag.Parse()

	log := zap.New()
	ctx := context.Background()

	// Get in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "Failed to get cluster config")
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Error(err, "Failed to create clientset")
		os.Exit(1)
	}

	// Create executor
	exec := executor.New(executor.Config{
		ExecutionName:      executionName,
		ExecutionNamespace: executionNamespace,
		Clientset:          clientset,
		GNMIUsername:       os.Getenv("GNMI_USERNAME"),
		GNMIPassword:       os.Getenv("GNMI_PASSWORD"),
		Log:                log,
	})

	// Run execution
	if err := exec.Run(ctx); err != nil {
		log.Error(err, "Execution failed")
		os.Exit(1)
	}

	log.Info("Execution completed successfully")
}
```

### Executor Package

```go
// pkg/executor/executor.go
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	heliosv1alpha1 "github.com/helios/runbook-operator/api/v1alpha1"
	"github.com/helios/runbook-operator/pkg/gnmic"
	"github.com/helios/runbook-operator/pkg/template"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Executor struct {
	config      Config
	gnmiClient  *gnmic.Client
	templateEng *template.Engine
	heliosClient client.Client
}

type Config struct {
	ExecutionName      string
	ExecutionNamespace string
	Clientset          *kubernetes.Clientset
	GNMIUsername       string
	GNMIPassword       string
	Log                logr.Logger
}

func New(cfg Config) *Executor {
	return &Executor{
		config:      cfg,
		gnmiClient:  gnmic.NewClient(cfg.GNMIUsername, cfg.GNMIPassword),
		templateEng: template.NewEngine(),
	}
}

func (e *Executor) Run(ctx context.Context) error {
	log := e.config.Log

	// Fetch execution and runbook
	exec, runbook, err := e.fetchResources(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch resources: %w", err)
	}

	// Build template context
	templateCtx := e.buildTemplateContext(exec, runbook)

	// Execute each step
	for i, step := range runbook.Spec.Steps {
		log.Info("Executing step", "name", step.Name, "action", step.Action)

		// Update step status to Running
		if err := e.updateStepStatus(ctx, exec, i, "Running", "", ""); err != nil {
			log.Error(err, "Failed to update step status")
		}

		startTime := time.Now()

		// Execute the step
		output, err := e.executeStep(ctx, step, templateCtx)

		duration := time.Since(startTime)

		if err != nil {
			log.Error(err, "Step failed", "name", step.Name, "duration", duration)

			if err := e.updateStepStatus(ctx, exec, i, "Failed", output, err.Error()); err != nil {
				log.Error(err, "Failed to update step status")
			}

			if !step.ContinueOnError {
				return fmt.Errorf("step %q failed: %w", step.Name, err)
			}
		} else {
			log.Info("Step completed", "name", step.Name, "duration", duration)

			if err := e.updateStepStatus(ctx, exec, i, "Completed", output, ""); err != nil {
				log.Error(err, "Failed to update step status")
			}
		}
	}

	return nil
}

func (e *Executor) executeStep(ctx context.Context, step heliosv1alpha1.Step, tmplCtx map[string]interface{}) (string, error) {
	// Parse timeout
	timeout, _ := time.ParseDuration(step.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch step.Action {
	case "gnmi_set":
		return e.executeGNMISet(ctx, step.Config, tmplCtx)
	case "gnmi_get":
		return e.executeGNMIGet(ctx, step.Config, tmplCtx)
	case "gnmi_subscribe":
		return e.executeGNMISubscribe(ctx, step.Config, tmplCtx)
	case "wait":
		return e.executeWait(ctx, step.Config, tmplCtx)
	case "notify":
		return e.executeNotify(ctx, step.Config, tmplCtx)
	case "condition":
		return e.executeCondition(ctx, step.Config, tmplCtx)
	case "script":
		return e.executeScript(ctx, step.Config, tmplCtx)
	default:
		return "", fmt.Errorf("unknown action: %s", step.Action)
	}
}

func (e *Executor) executeGNMISet(ctx context.Context, config *apiextensionsv1.JSON, tmplCtx map[string]interface{}) (string, error) {
	var cfg GNMISetConfig
	if err := json.Unmarshal(config.Raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	// Render templates in target and paths
	target, err := e.templateEng.Render(cfg.Target, tmplCtx)
	if err != nil {
		return "", fmt.Errorf("failed to render target: %w", err)
	}

	for i, update := range cfg.Updates {
		path, err := e.templateEng.Render(update.Path, tmplCtx)
		if err != nil {
			return "", fmt.Errorf("failed to render path: %w", err)
		}
		cfg.Updates[i].Path = path
	}

	// Execute gNMI Set
	result, err := e.gnmiClient.Set(ctx, target, cfg.Updates)
	if err != nil {
		return "", fmt.Errorf("gNMI Set failed: %w", err)
	}

	return fmt.Sprintf("Set completed: %v", result), nil
}

func (e *Executor) executeGNMIGet(ctx context.Context, config *apiextensionsv1.JSON, tmplCtx map[string]interface{}) (string, error) {
	var cfg GNMIGetConfig
	if err := json.Unmarshal(config.Raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	// Render templates
	target, _ := e.templateEng.Render(cfg.Target, tmplCtx)
	path, _ := e.templateEng.Render(cfg.Path, tmplCtx)

	// If retryUntil is set, poll until condition met
	if cfg.RetryUntil != "" {
		retryInterval, _ := time.ParseDuration(cfg.RetryInterval)
		if retryInterval == 0 {
			retryInterval = 5 * time.Second
		}

		for {
			result, err := e.gnmiClient.Get(ctx, target, path)
			if err != nil {
				return "", err
			}

			if result == cfg.RetryUntil {
				return fmt.Sprintf("Condition met: %s = %s", path, result), nil
			}

			select {
			case <-ctx.Done():
				return "", fmt.Errorf("timeout waiting for %s = %s (current: %s)", path, cfg.RetryUntil, result)
			case <-time.After(retryInterval):
				continue
			}
		}
	}

	result, err := e.gnmiClient.Get(ctx, target, path)
	if err != nil {
		return "", err
	}

	if cfg.FailIfNotFound && result == "" {
		return "", fmt.Errorf("path not found: %s", path)
	}

	return fmt.Sprintf("Get result: %s", result), nil
}

func (e *Executor) executeWait(ctx context.Context, config *apiextensionsv1.JSON, tmplCtx map[string]interface{}) (string, error) {
	var cfg WaitConfig
	if err := json.Unmarshal(config.Raw, &cfg); err != nil {
		return "", fmt.Errorf("invalid config: %w", err)
	}

	// Render duration template
	durationStr, _ := e.templateEng.Render(cfg.Duration, tmplCtx)
	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return "", fmt.Errorf("invalid duration: %w", err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(duration):
		return fmt.Sprintf("Waited %s", duration), nil
	}
}

// Config structs for each action type
type GNMISetConfig struct {
	Target  string       `json:"target"`
	Updates []GNMIUpdate `json:"updates"`
}

type GNMIUpdate struct {
	Path     string      `json:"path"`
	Value    interface{} `json:"value"`
	Encoding string      `json:"encoding"`
}

type GNMIGetConfig struct {
	Target         string `json:"target"`
	Path           string `json:"path"`
	FailIfNotFound bool   `json:"failIfNotFound"`
	RetryUntil     string `json:"retryUntil"`
	RetryInterval  string `json:"retryInterval"`
}

type WaitConfig struct {
	Duration string `json:"duration"`
}
```

### gnmic Client Wrapper

```go
// pkg/gnmic/client.go
package gnmic

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type Client struct {
	username string
	password string
}

func NewClient(username, password string) *Client {
	return &Client{
		username: username,
		password: password,
	}
}

func (c *Client) connect(ctx context.Context, target string) (gnmi.GNMIClient, *grpc.ClientConn, error) {
	// Parse target (hostname:port)
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	creds := credentials.NewTLS(tlsConfig)

	conn, err := grpc.DialContext(ctx, target,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(&loginCreds{
			username: c.username,
			password: c.password,
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to %s: %w", target, err)
	}

	return gnmi.NewGNMIClient(conn), conn, nil
}

func (c *Client) Set(ctx context.Context, target string, updates []Update) (string, error) {
	client, conn, err := c.connect(ctx, target)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	// Build SetRequest
	req := &gnmi.SetRequest{
		Update: make([]*gnmi.Update, len(updates)),
	}

	for i, u := range updates {
		path, err := parsePath(u.Path)
		if err != nil {
			return "", fmt.Errorf("invalid path %q: %w", u.Path, err)
		}

		val, err := encodeValue(u.Value, u.Encoding)
		if err != nil {
			return "", fmt.Errorf("failed to encode value: %w", err)
		}

		req.Update[i] = &gnmi.Update{
			Path: path,
			Val:  val,
		}
	}

	resp, err := client.Set(ctx, req)
	if err != nil {
		return "", fmt.Errorf("Set RPC failed: %w", err)
	}

	return fmt.Sprintf("timestamp: %d, response: %v", resp.Timestamp, resp.Response), nil
}

func (c *Client) Get(ctx context.Context, target, pathStr string) (string, error) {
	client, conn, err := c.connect(ctx, target)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	path, err := parsePath(pathStr)
	if err != nil {
		return "", fmt.Errorf("invalid path %q: %w", pathStr, err)
	}

	req := &gnmi.GetRequest{
		Path:     []*gnmi.Path{path},
		Encoding: gnmi.Encoding_JSON_IETF,
	}

	resp, err := client.Get(ctx, req)
	if err != nil {
		return "", fmt.Errorf("Get RPC failed: %w", err)
	}

	if len(resp.Notification) == 0 || len(resp.Notification[0].Update) == 0 {
		return "", nil
	}

	val := resp.Notification[0].Update[0].Val
	return extractValue(val), nil
}

type Update struct {
	Path     string
	Value    interface{}
	Encoding string
}

type loginCreds struct {
	username string
	password string
}

func (c *loginCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"username": c.username,
		"password": c.password,
	}, nil
}

func (c *loginCreds) RequireTransportSecurity() bool {
	return true
}
```

---

## Observability

### Prometheus Metrics

```go
// pkg/metrics/metrics.go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ExecutionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "helios_runbook_executions_total",
			Help: "Total number of runbook executions",
		},
		[]string{"runbook", "status"},
	)

	ExecutionDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "helios_runbook_execution_duration_seconds",
			Help:    "Duration of runbook executions",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600},
		},
		[]string{"runbook"},
	)

	StepDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "helios_runbook_step_duration_seconds",
			Help:    "Duration of individual steps",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
		},
		[]string{"runbook", "step", "action"},
	)

	PendingApprovals = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "helios_runbook_pending_approvals",
			Help: "Number of executions pending approval",
		},
	)

	ActiveExecutions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "helios_runbook_active_executions",
			Help: "Number of currently running executions",
		},
	)
)

func init() {
	metrics.Registry.MustRegister(
		ExecutionsTotal,
		ExecutionDuration,
		StepDuration,
		PendingApprovals,
		ActiveExecutions,
	)
}
```

### Grafana Dashboard

```json
{
  "title": "Helios Runbook Operations",
  "panels": [
    {
      "title": "Executions by Status",
      "type": "stat",
      "targets": [
        {
          "expr": "sum(increase(helios_runbook_executions_total[24h])) by (status)",
          "legendFormat": "{{status}}"
        }
      ]
    },
    {
      "title": "Execution Duration",
      "type": "timeseries",
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(helios_runbook_execution_duration_seconds_bucket[5m])) by (le, runbook))",
          "legendFormat": "{{runbook}} p95"
        }
      ]
    },
    {
      "title": "Pending Approvals",
      "type": "stat",
      "targets": [
        {
          "expr": "helios_runbook_pending_approvals",
          "legendFormat": "Pending"
        }
      ]
    },
    {
      "title": "Recent Executions",
      "type": "table",
      "targets": [
        {
          "expr": "helios_runbook_executions_total",
          "format": "table"
        }
      ]
    }
  ]
}
```

---

## Example Runbooks

### Interface Bounce

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: interface-bounce
  namespace: helios-automation
spec:
  name: Bounce Interface
  description: |
    Administratively disables and re-enables an interface.
    Useful for clearing interface errors or resetting port state.
  category: interface
  riskLevel: medium
  requiresApproval: false
  allowedRoles: [network-engineer, network-admin]
  cooldown: "5m"

  parameters:
    - name: device
      type: device
      required: true
      description: Target device hostname
    - name: interface
      type: interface
      required: true
      description: Interface to bounce (e.g., Ethernet1/1)
    - name: wait_seconds
      type: integer
      required: false
      default: 5
      description: Seconds to wait (1-60)
      validation: "^([1-9]|[1-5][0-9]|60)$"

  steps:
    - name: Verify Interface Exists
      action: gnmi_get
      timeout: 10s
      config:
        target: "{{device}}:6030"
        path: "/interfaces/interface[name={{interface}}]/state/admin-status"
        failIfNotFound: true

    - name: Disable Interface
      action: gnmi_set
      timeout: 15s
      config:
        target: "{{device}}:6030"
        updates:
          - path: "/interfaces/interface[name={{interface}}]/config/enabled"
            value: false
            encoding: json_ietf

    - name: Wait
      action: wait
      config:
        duration: "{{wait_seconds}}s"

    - name: Enable Interface
      action: gnmi_set
      timeout: 15s
      config:
        target: "{{device}}:6030"
        updates:
          - path: "/interfaces/interface[name={{interface}}]/config/enabled"
            value: true
            encoding: json_ietf

    - name: Verify Interface Up
      action: gnmi_get
      timeout: 60s
      config:
        target: "{{device}}:6030"
        path: "/interfaces/interface[name={{interface}}]/state/oper-status"
        retryUntil: "UP"
        retryInterval: "5s"

    - name: Notify Completion
      action: notify
      config:
        channel: slack
        message: |
          :white_check_mark: Interface bounced
          *Device*: {{device}}
          *Interface*: {{interface}}

  rollback:
    - name: Ensure Interface Enabled
      action: gnmi_set
      config:
        target: "{{device}}:6030"
        updates:
          - path: "/interfaces/interface[name={{interface}}]/config/enabled"
            value: true
            encoding: json_ietf
```

### Collect Diagnostics

```yaml
apiVersion: helios.io/v1alpha1
kind: Runbook
metadata:
  name: collect-diagnostics
  namespace: helios-automation
spec:
  name: Collect Device Diagnostics
  description: Gather diagnostic information from a device for troubleshooting
  category: diagnostic
  riskLevel: low
  requiresApproval: false
  allowedRoles: [network-viewer, network-engineer, network-admin]

  parameters:
    - name: device
      type: device
      required: true
    - name: include_routing
      type: boolean
      default: true

  steps:
    - name: Get System Info
      action: gnmi_get
      config:
        target: "{{device}}:6030"
        path: "/system/state"

    - name: Get Interface Counters
      action: gnmi_get
      config:
        target: "{{device}}:6030"
        path: "/interfaces/interface/state/counters"

    - name: Get BGP Summary
      action: gnmi_get
      config:
        target: "{{device}}:6030"
        path: "/network-instances/network-instance[name=default]/protocols/protocol[identifier=BGP][name=BGP]/bgp/neighbors"
```

---

## Sequence Diagrams

### Normal Execution (No Approval)

```
User          kubectl         Operator         Job           Device
  |              |               |              |               |
  |--create RBE->|               |              |               |
  |              |--watch------->|              |               |
  |              |               |--validate--->|              |
  |              |               |   (self)     |               |
  |              |               |              |               |
  |              |               |--create Job->|               |
  |              |               |              |--gNMI Set---->|
  |              |               |              |<----OK--------|
  |              |               |              |--gNMI Get---->|
  |              |               |              |<---result-----|
  |              |               |<-Job done----|               |
  |              |               |--update----->|               |
  |              |               |   status     |               |
  |<--Completed--|               |              |               |
```

### Execution with Approval

```
User      kubectl    Operator     Slack      Approver    Job       Device
  |          |          |           |           |          |          |
  |--create->|          |           |           |          |          |
  |          |--watch-->|           |           |          |          |
  |          |          |--msg----->|           |          |          |
  |          |          |           |--notify-->|          |          |
  |          |          |           |           |          |          |
  |          |          |           |<-approve--|          |          |
  |          |          |<--webhook-|           |          |          |
  |          |          |           |           |          |          |
  |          |          |--create---------------+--------->|          |
  |          |          |           |           |          |--gNMI--->|
  |          |          |           |           |          |<--OK-----|
  |          |          |<--done----|-----------|----------|          |
  |<-done----|          |           |           |          |          |
```

---

## Related Documents

- [[adr-004-runbook-automation]] - ADR for runbook automation
- [[architecture]] - Main architecture document
- [[adr-002-gnmic-deployment]] - gnmic deployment model
