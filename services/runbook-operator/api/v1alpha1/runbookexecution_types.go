package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ExecutionPhase defines the phase of a RunbookExecution.
type ExecutionPhase string

const (
	PhasePending         ExecutionPhase = "Pending"
	PhasePendingApproval ExecutionPhase = "PendingApproval"
	PhaseApproved        ExecutionPhase = "Approved"
	PhaseRunning         ExecutionPhase = "Running"
	PhaseCompleted       ExecutionPhase = "Completed"
	PhaseFailed          ExecutionPhase = "Failed"
	PhaseCancelled       ExecutionPhase = "Cancelled"
	PhaseTimedOut        ExecutionPhase = "TimedOut"
	PhaseRollingBack     ExecutionPhase = "RollingBack"
	PhaseRolledBack      ExecutionPhase = "RolledBack"
)

// TriggerSource defines the source of a RunbookExecution trigger.
type TriggerSource string

const (
	TriggerManual    TriggerSource = "manual"
	TriggerAlert     TriggerSource = "alert"
	TriggerScheduled TriggerSource = "scheduled"
	TriggerAPI       TriggerSource = "api"
)

// StepStatus defines the status of a step.
type StepStatus string

const (
	StepPending   StepStatus = "Pending"
	StepRunning   StepStatus = "Running"
	StepCompleted StepStatus = "Completed"
	StepFailed    StepStatus = "Failed"
	StepSkipped   StepStatus = "Skipped"
)

// RunbookExecutionSpec defines the desired state of RunbookExecution.
type RunbookExecutionSpec struct {
	RunbookRef    RunbookRef             `json:"runbookRef"`
	Parameters    map[string]interface{} `json:"parameters,omitempty"`
	TriggeredBy   string                 `json:"triggeredBy"`
	TriggerSource TriggerSource          `json:"triggerSource,omitempty"`
	AlertRef      string                 `json:"alertRef,omitempty"`
	DryRun        bool                   `json:"dryRun,omitempty"`
}

// RunbookRef references a Runbook.
type RunbookRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// RunbookExecutionStatus defines the observed state of RunbookExecution.
type RunbookExecutionStatus struct {
	Phase          ExecutionPhase       `json:"phase,omitempty"`
	StartTime      *metav1.Time         `json:"startTime,omitempty"`
	CompletionTime *metav1.Time         `json:"completionTime,omitempty"`
	Duration       string               `json:"duration,omitempty"`
	ApprovedBy     string               `json:"approvedBy,omitempty"`
	ApprovedAt     *metav1.Time         `json:"approvedAt,omitempty"`
	Message        string               `json:"message,omitempty"`
	Steps          []ExecutionStepStatus `json:"steps,omitempty"`
	JobName        string               `json:"jobName,omitempty"`
	Conditions     []metav1.Condition   `json:"conditions,omitempty"`
}

// ExecutionStepStatus defines the status of a single execution step.
type ExecutionStepStatus struct {
	Name           string     `json:"name"`
	Status         StepStatus `json:"status"`
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	Output         string     `json:"output,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Runbook",type=string,JSONPath=`.spec.runbookRef.name`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Triggered By",type=string,JSONPath=`.spec.triggeredBy`
// +kubebuilder:printcolumn:name="Source",type=string,JSONPath=`.spec.triggerSource`
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.status.duration`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// RunbookExecution is the Schema for the runbookexecutions API.
type RunbookExecution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunbookExecutionSpec   `json:"spec,omitempty"`
	Status RunbookExecutionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RunbookExecutionList contains a list of RunbookExecution.
type RunbookExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RunbookExecution `json:"items"`
}
