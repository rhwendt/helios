package controllers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	heliosv1alpha1 "github.com/rhwendt/helios/services/runbook-operator/api/v1alpha1"
)

// RunbookExecutionReconciler reconciles a RunbookExecution object.
type RunbookExecutionReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           *slog.Logger
	ExecutorImage string
}

// +kubebuilder:rbac:groups=helios.io,resources=runbookexecutions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helios.io,resources=runbookexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *RunbookExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.With("execution", req.NamespacedName)

	var execution heliosv1alpha1.RunbookExecution
	if err := r.Get(ctx, req.NamespacedName, &execution); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// State machine reconciliation
	switch execution.Status.Phase {
	case "", heliosv1alpha1.PhasePending:
		return r.handlePending(ctx, log, &execution)
	case heliosv1alpha1.PhasePendingApproval:
		return r.handlePendingApproval(ctx, log, &execution)
	case heliosv1alpha1.PhaseApproved:
		return r.handleApproved(ctx, log, &execution)
	case heliosv1alpha1.PhaseRunning:
		return r.handleRunning(ctx, log, &execution)
	case heliosv1alpha1.PhaseFailed:
		return r.handleFailed(ctx, log, &execution)
	case heliosv1alpha1.PhaseRollingBack:
		return r.handleRollingBack(ctx, log, &execution)
	case heliosv1alpha1.PhaseCompleted, heliosv1alpha1.PhaseCancelled,
		heliosv1alpha1.PhaseTimedOut, heliosv1alpha1.PhaseRolledBack:
		// Terminal states, no further reconciliation needed
		return ctrl.Result{}, nil
	default:
		log.Warn("unknown phase", "phase", execution.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *RunbookExecutionReconciler) handlePending(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	// Fetch the referenced Runbook
	runbook, err := r.getRunbook(ctx, exec)
	if err != nil {
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseFailed, fmt.Sprintf("failed to get runbook: %v", err))
	}

	// Check if runbook requires approval
	if runbook.Spec.RequiresApproval {
		log.Info("runbook requires approval, transitioning to PendingApproval")
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhasePendingApproval, "Awaiting approval")
	}

	// No approval needed, transition to Running
	now := metav1.Now()
	exec.Status.StartTime = &now
	return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseRunning, "Starting execution")
}

func (r *RunbookExecutionReconciler) handlePendingApproval(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	// Check if approved (approvedBy field set externally)
	if exec.Status.ApprovedBy != "" {
		// Validate ApprovedBy is in the runbook's Approvers list
		runbook, err := r.getRunbook(ctx, exec)
		if err != nil {
			return ctrl.Result{}, err
		}
		approverValid := false
		for _, approver := range runbook.Spec.Approvers {
			if approver.Name == exec.Status.ApprovedBy {
				approverValid = true
				break
			}
		}
		if !approverValid {
			log.Warn("approval rejected: approver not in allowed list", "approvedBy", exec.Status.ApprovedBy)
			return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseFailed,
				fmt.Sprintf("approver %q is not in the runbook's approved approvers list", exec.Status.ApprovedBy))
		}

		log.Info("execution approved", "approvedBy", exec.Status.ApprovedBy)
		now := metav1.Now()
		exec.Status.StartTime = &now
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseApproved, "Approved, starting execution")
	}

	// Check approval timeout
	runbook, err := r.getRunbook(ctx, exec)
	if err != nil {
		return ctrl.Result{}, err
	}
	timeout, _ := time.ParseDuration(runbook.Spec.ApprovalTimeout)
	if timeout == 0 {
		timeout = time.Hour
	}
	if time.Since(exec.CreationTimestamp.Time) > timeout {
		log.Warn("approval timeout exceeded")
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseTimedOut, "Approval timeout exceeded")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) handleApproved(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	now := metav1.Now()
	exec.Status.StartTime = &now
	return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseRunning, "Starting execution")
}

func (r *RunbookExecutionReconciler) handleRunning(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	// Check if executor Job exists
	jobName := fmt.Sprintf("%s-executor", exec.Name)
	var job batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: exec.Namespace}, &job)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// Create executor Job
		log.Info("creating executor job", "jobName", jobName)
		if err := r.createExecutorJob(ctx, exec, jobName); err != nil {
			return ctrl.Result{}, err
		}
		exec.Status.JobName = jobName
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, exec)
	}

	// Check Job completion
	if job.Status.Succeeded > 0 {
		now := metav1.Now()
		exec.Status.CompletionTime = &now
		if exec.Status.StartTime != nil {
			exec.Status.Duration = now.Sub(exec.Status.StartTime.Time).Round(time.Second).String()
		}
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseCompleted, "Execution completed successfully")
	}
	if job.Status.Failed > 0 {
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseFailed, "Executor job failed")
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) handleFailed(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	runbook, err := r.getRunbook(ctx, exec)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If runbook has rollback steps, initiate rollback
	if len(runbook.Spec.Rollback) > 0 {
		log.Info("initiating rollback")
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseRollingBack, "Initiating rollback")
	}

	// No rollback defined, stay in Failed
	now := metav1.Now()
	exec.Status.CompletionTime = &now
	if exec.Status.StartTime != nil {
		exec.Status.Duration = now.Sub(exec.Status.StartTime.Time).Round(time.Second).String()
	}
	return ctrl.Result{}, r.Status().Update(ctx, exec)
}

func (r *RunbookExecutionReconciler) handleRollingBack(ctx context.Context, log *slog.Logger, exec *heliosv1alpha1.RunbookExecution) (ctrl.Result, error) {
	// Check if rollback Job exists
	jobName := fmt.Sprintf("%s-rollback", exec.Name)
	var job batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: exec.Namespace}, &job)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		log.Info("creating rollback job", "jobName", jobName)
		if err := r.createExecutorJob(ctx, exec, jobName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if job.Status.Succeeded > 0 {
		now := metav1.Now()
		exec.Status.CompletionTime = &now
		if exec.Status.StartTime != nil {
			exec.Status.Duration = now.Sub(exec.Status.StartTime.Time).Round(time.Second).String()
		}
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseRolledBack, "Rollback completed")
	}
	if job.Status.Failed > 0 {
		return ctrl.Result{}, r.setPhase(ctx, exec, heliosv1alpha1.PhaseFailed, "Rollback failed")
	}

	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *RunbookExecutionReconciler) getRunbook(ctx context.Context, exec *heliosv1alpha1.RunbookExecution) (*heliosv1alpha1.Runbook, error) {
	ns := exec.Spec.RunbookRef.Namespace
	if ns == "" {
		ns = exec.Namespace
	}
	var runbook heliosv1alpha1.Runbook
	if err := r.Get(ctx, types.NamespacedName{Name: exec.Spec.RunbookRef.Name, Namespace: ns}, &runbook); err != nil {
		return nil, err
	}
	return &runbook, nil
}

func (r *RunbookExecutionReconciler) setPhase(ctx context.Context, exec *heliosv1alpha1.RunbookExecution, phase heliosv1alpha1.ExecutionPhase, message string) error {
	exec.Status.Phase = phase
	exec.Status.Message = message
	meta.SetStatusCondition(&exec.Status.Conditions, metav1.Condition{
		Type:               string(phase),
		Status:             metav1.ConditionTrue,
		Reason:             string(phase),
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
	return r.Status().Update(ctx, exec)
}

func (r *RunbookExecutionReconciler) createExecutorJob(ctx context.Context, exec *heliosv1alpha1.RunbookExecution, jobName string) error {
	backoffLimit := int32(0)
	runAsNonRoot := true
	readOnlyRootFS := true
	allowPrivEsc := false
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: exec.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "runbook-executor",
				"app.kubernetes.io/instance":  exec.Name,
				"app.kubernetes.io/component": "automation",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: exec.APIVersion,
					Kind:       exec.Kind,
					Name:       exec.Name,
					UID:        exec.UID,
				},
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "executor",
							Image: r.ExecutorImage,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivEsc,
								ReadOnlyRootFilesystem:   &readOnlyRootFS,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "EXECUTION_NAME",
									Value: exec.Name,
								},
								{
									Name:  "EXECUTION_NAMESPACE",
									Value: exec.Namespace,
								},
							},
						},
					},
				},
			},
		},
	}
	return r.Create(ctx, job)
}

func (r *RunbookExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&heliosv1alpha1.RunbookExecution{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}
