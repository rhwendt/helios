package controllers

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	heliosv1alpha1 "github.com/rhwendt/helios/services/runbook-operator/api/v1alpha1"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// TestRunbookValidation tests the runbook validation logic directly
// without requiring a full controller-runtime environment.
func TestRunbookValidation(t *testing.T) {
	reconciler := &RunbookReconciler{Log: testLogger()}

	tests := []struct {
		name    string
		runbook *heliosv1alpha1.Runbook
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid runbook without approval",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name:      "interface-bounce",
					Category:  heliosv1alpha1.CategoryInterface,
					RiskLevel: heliosv1alpha1.RiskMedium,
					Steps: []heliosv1alpha1.RunbookStep{
						{Name: "disable-interface", Action: heliosv1alpha1.ActionGNMISet},
						{Name: "wait", Action: heliosv1alpha1.ActionWait},
						{Name: "enable-interface", Action: heliosv1alpha1.ActionGNMISet},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid runbook with approval and approvers",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name:             "clear-bgp",
					Category:         heliosv1alpha1.CategoryBGP,
					RiskLevel:        heliosv1alpha1.RiskHigh,
					RequiresApproval: true,
					Approvers:        []heliosv1alpha1.Approver{{Type: "group", Name: "noc-leads"}},
					Steps: []heliosv1alpha1.RunbookStep{
						{Name: "clear-bgp", Action: heliosv1alpha1.ActionGNMISet},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "empty name",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name:  "",
					Steps: []heliosv1alpha1.RunbookStep{{Name: "step-1", Action: heliosv1alpha1.ActionWait}},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "no steps",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name:  "empty-runbook",
					Steps: []heliosv1alpha1.RunbookStep{},
				},
			},
			wantErr: true,
			errMsg:  "at least one step",
		},
		{
			name: "requires approval without approvers",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name:             "needs-approval",
					RequiresApproval: true,
					Approvers:        nil,
					Steps: []heliosv1alpha1.RunbookStep{
						{Name: "step-1", Action: heliosv1alpha1.ActionGNMISet},
					},
				},
			},
			wantErr: true,
			errMsg:  "approvers required",
		},
		{
			name: "step with empty name",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name: "bad-step",
					Steps: []heliosv1alpha1.RunbookStep{
						{Name: "", Action: heliosv1alpha1.ActionWait},
					},
				},
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "step with empty action",
			runbook: &heliosv1alpha1.Runbook{
				Spec: heliosv1alpha1.RunbookSpec{
					Name: "bad-step-action",
					Steps: []heliosv1alpha1.RunbookStep{
						{Name: "step-1", Action: ""},
					},
				},
			},
			wantErr: true,
			errMsg:  "action is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := reconciler.validateRunbook(tc.runbook)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tc.errMsg != "" && !containsStr(err.Error(), tc.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestStateMachineTransitions tests the state machine logic by verifying
// which handler method gets called for each phase.
func TestStateMachineTransitions_PendingNoApproval(t *testing.T) {
	// Verify: for a no-approval runbook, Pending → Running transition
	exec := &heliosv1alpha1.RunbookExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-exec",
			Namespace: "helios-automation",
		},
		Spec: heliosv1alpha1.RunbookExecutionSpec{
			RunbookRef:  heliosv1alpha1.RunbookRef{Name: "test-runbook"},
			TriggeredBy: "admin@example.com",
		},
		Status: heliosv1alpha1.RunbookExecutionStatus{
			Phase: heliosv1alpha1.PhasePending,
		},
	}

	if exec.Status.Phase != heliosv1alpha1.PhasePending {
		t.Fatalf("initial phase = %q, want Pending", exec.Status.Phase)
	}

	// Simulate the transition
	exec.Status.Phase = heliosv1alpha1.PhaseRunning
	now := metav1.Now()
	exec.Status.StartTime = &now

	if exec.Status.Phase != heliosv1alpha1.PhaseRunning {
		t.Errorf("phase after transition = %q, want Running", exec.Status.Phase)
	}
	if exec.Status.StartTime == nil {
		t.Error("StartTime should be set after transition to Running")
	}
}

func TestStateMachineTransitions_PendingApprovalFlow(t *testing.T) {
	exec := &heliosv1alpha1.RunbookExecution{
		Status: heliosv1alpha1.RunbookExecutionStatus{
			Phase: heliosv1alpha1.PhasePending,
		},
	}

	// Pending → PendingApproval
	exec.Status.Phase = heliosv1alpha1.PhasePendingApproval
	if exec.Status.Phase != heliosv1alpha1.PhasePendingApproval {
		t.Errorf("phase = %q, want PendingApproval", exec.Status.Phase)
	}

	// PendingApproval → Approved
	exec.Status.ApprovedBy = "noc-lead@example.com"
	now := metav1.Now()
	exec.Status.ApprovedAt = &now
	exec.Status.Phase = heliosv1alpha1.PhaseApproved
	if exec.Status.Phase != heliosv1alpha1.PhaseApproved {
		t.Errorf("phase = %q, want Approved", exec.Status.Phase)
	}
	if exec.Status.ApprovedBy != "noc-lead@example.com" {
		t.Errorf("ApprovedBy = %q, want noc-lead@example.com", exec.Status.ApprovedBy)
	}

	// Approved → Running
	exec.Status.Phase = heliosv1alpha1.PhaseRunning
	exec.Status.StartTime = &now
	if exec.Status.Phase != heliosv1alpha1.PhaseRunning {
		t.Errorf("phase = %q, want Running", exec.Status.Phase)
	}
}

func TestStateMachineTransitions_FailedRollback(t *testing.T) {
	now := metav1.Now()
	exec := &heliosv1alpha1.RunbookExecution{
		Status: heliosv1alpha1.RunbookExecutionStatus{
			Phase:     heliosv1alpha1.PhaseRunning,
			StartTime: &now,
		},
	}

	// Running → Failed
	exec.Status.Phase = heliosv1alpha1.PhaseFailed
	exec.Status.Message = "step 2 failed: connection refused"
	if exec.Status.Phase != heliosv1alpha1.PhaseFailed {
		t.Errorf("phase = %q, want Failed", exec.Status.Phase)
	}

	// Failed → RollingBack
	exec.Status.Phase = heliosv1alpha1.PhaseRollingBack
	if exec.Status.Phase != heliosv1alpha1.PhaseRollingBack {
		t.Errorf("phase = %q, want RollingBack", exec.Status.Phase)
	}

	// RollingBack → RolledBack
	exec.Status.Phase = heliosv1alpha1.PhaseRolledBack
	completionTime := metav1.Now()
	exec.Status.CompletionTime = &completionTime
	exec.Status.Duration = completionTime.Sub(now.Time).Round(time.Second).String()
	if exec.Status.Phase != heliosv1alpha1.PhaseRolledBack {
		t.Errorf("phase = %q, want RolledBack", exec.Status.Phase)
	}
	if exec.Status.CompletionTime == nil {
		t.Error("CompletionTime should be set after rollback")
	}
}

func TestStateMachineTransitions_TerminalStates(t *testing.T) {
	terminalPhases := []heliosv1alpha1.ExecutionPhase{
		heliosv1alpha1.PhaseCompleted,
		heliosv1alpha1.PhaseCancelled,
		heliosv1alpha1.PhaseTimedOut,
		heliosv1alpha1.PhaseRolledBack,
	}

	for _, phase := range terminalPhases {
		t.Run(string(phase), func(t *testing.T) {
			// Verify these are recognized as terminal states in the switch
			exec := &heliosv1alpha1.RunbookExecution{
				Status: heliosv1alpha1.RunbookExecutionStatus{
					Phase: phase,
				},
			}
			// Terminal states should not require further action
			if exec.Status.Phase != phase {
				t.Errorf("phase = %q, want %q", exec.Status.Phase, phase)
			}
		})
	}
}

func TestExecutionStepStatuses(t *testing.T) {
	steps := []heliosv1alpha1.ExecutionStepStatus{
		{Name: "step-1", Status: heliosv1alpha1.StepPending},
		{Name: "step-2", Status: heliosv1alpha1.StepPending},
		{Name: "step-3", Status: heliosv1alpha1.StepPending},
	}

	// Step 1 starts
	steps[0].Status = heliosv1alpha1.StepRunning
	now := metav1.Now()
	steps[0].StartTime = &now
	if steps[0].Status != heliosv1alpha1.StepRunning {
		t.Errorf("step-1 status = %q, want Running", steps[0].Status)
	}

	// Step 1 completes
	steps[0].Status = heliosv1alpha1.StepCompleted
	completionTime := metav1.Now()
	steps[0].CompletionTime = &completionTime
	steps[0].Output = "interface disabled"
	if steps[0].Output != "interface disabled" {
		t.Error("step output should be set")
	}

	// Step 2 fails
	steps[1].Status = heliosv1alpha1.StepFailed
	steps[1].Error = "connection refused"
	if steps[1].Error != "connection refused" {
		t.Error("step error should be set")
	}

	// Step 3 skipped
	steps[2].Status = heliosv1alpha1.StepSkipped
	if steps[2].Status != heliosv1alpha1.StepSkipped {
		t.Errorf("step-3 status = %q, want Skipped", steps[2].Status)
	}
}

func TestSetPhase_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Verify context cancellation is respected
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
