package controllers

import (
	"context"
	"fmt"
	"log/slog"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	heliosv1alpha1 "github.com/rhwendt/helios/services/runbook-operator/api/v1alpha1"
)

// RunbookReconciler reconciles a Runbook object.
type RunbookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    *slog.Logger
}

// +kubebuilder:rbac:groups=helios.io,resources=runbooks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helios.io,resources=runbooks/status,verbs=get;update;patch

func (r *RunbookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.With("runbook", req.NamespacedName)

	var runbook heliosv1alpha1.Runbook
	if err := r.Get(ctx, req.NamespacedName, &runbook); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Validate runbook schema
	if err := r.validateRunbook(&runbook); err != nil {
		log.Error("runbook validation failed", "error", err)
		meta.SetStatusCondition(&runbook.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "ValidationFailed",
			Message:            err.Error(),
			LastTransitionTime: metav1.Now(),
		})
		if updateErr := r.Status().Update(ctx, &runbook); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Set Ready condition
	meta.SetStatusCondition(&runbook.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Valid",
		Message:            "Runbook schema is valid",
		LastTransitionTime: metav1.Now(),
	})
	if err := r.Status().Update(ctx, &runbook); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("runbook reconciled successfully", "category", runbook.Spec.Category)
	return ctrl.Result{}, nil
}

func (r *RunbookReconciler) validateRunbook(rb *heliosv1alpha1.Runbook) error {
	if rb.Spec.Name == "" {
		return fmt.Errorf("runbook name is required")
	}
	if len(rb.Spec.Steps) == 0 {
		return fmt.Errorf("runbook must have at least one step")
	}
	if rb.Spec.RequiresApproval && len(rb.Spec.Approvers) == 0 {
		return fmt.Errorf("approvers required when requiresApproval is true")
	}
	allowedActions := map[heliosv1alpha1.StepAction]bool{
		heliosv1alpha1.ActionGNMISet:       true,
		heliosv1alpha1.ActionGNMIGet:       true,
		heliosv1alpha1.ActionGNMISubscribe: true,
		heliosv1alpha1.ActionWait:          true,
		heliosv1alpha1.ActionNotify:        true,
		heliosv1alpha1.ActionCondition:     true,
	}
	for i, step := range rb.Spec.Steps {
		if step.Name == "" {
			return fmt.Errorf("step %d: name is required", i)
		}
		if step.Action == "" {
			return fmt.Errorf("step %d: action is required", i)
		}
		if !allowedActions[step.Action] {
			return fmt.Errorf("step %d: action %q is not allowed", i, step.Action)
		}
	}
	return nil
}

func (r *RunbookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&heliosv1alpha1.Runbook{}).
		Complete(r)
}
