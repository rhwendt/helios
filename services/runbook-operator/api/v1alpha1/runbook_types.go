package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RunbookCategory defines the category of a runbook.
type RunbookCategory string

const (
	CategoryInterface  RunbookCategory = "interface"
	CategoryBGP        RunbookCategory = "bgp"
	CategorySystem     RunbookCategory = "system"
	CategorySecurity   RunbookCategory = "security"
	CategoryDiagnostic RunbookCategory = "diagnostic"
	CategoryCustom     RunbookCategory = "custom"
)

// RiskLevel defines the risk level of a runbook.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// StepAction defines the action type for a runbook step.
type StepAction string

const (
	ActionGNMISet       StepAction = "gnmi_set"
	ActionGNMIGet       StepAction = "gnmi_get"
	ActionGNMISubscribe StepAction = "gnmi_subscribe"
	ActionWait          StepAction = "wait"
	ActionNotify        StepAction = "notify"
	ActionCondition     StepAction = "condition"
	ActionScript        StepAction = "script"
)

// RunbookSpec defines the desired state of Runbook.
type RunbookSpec struct {
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	Category         RunbookCategory `json:"category"`
	RiskLevel        RiskLevel       `json:"riskLevel"`
	RequiresApproval bool            `json:"requiresApproval,omitempty"`
	Approvers        []Approver      `json:"approvers,omitempty"`
	ApprovalTimeout  string          `json:"approvalTimeout,omitempty"`
	AllowedRoles     []string        `json:"allowedRoles,omitempty"`
	Cooldown         string          `json:"cooldown,omitempty"`
	Parameters       []Parameter     `json:"parameters,omitempty"`
	Steps            []RunbookStep   `json:"steps"`
	Rollback         []RunbookStep   `json:"rollback,omitempty"`
}

// Approver defines an approver for a runbook.
type Approver struct {
	Type string `json:"type"` // "user" or "group"
	Name string `json:"name"`
}

// Parameter defines a parameter for a runbook.
type Parameter struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"` // string, integer, boolean, device, interface, select
	Required    bool        `json:"required,omitempty"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
	Validation  string      `json:"validation,omitempty"`
	Options     []string    `json:"options,omitempty"`
}

// RunbookStep defines a single step in a runbook.
type RunbookStep struct {
	Name            string                 `json:"name"`
	Action          StepAction             `json:"action"`
	Timeout         string                 `json:"timeout,omitempty"`
	ContinueOnError bool                   `json:"continueOnError,omitempty"`
	Condition       string                 `json:"condition,omitempty"`
	Config          map[string]interface{} `json:"config,omitempty"`
}

// RunbookStatus defines the observed state of Runbook.
type RunbookStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Category",type=string,JSONPath=`.spec.category`
// +kubebuilder:printcolumn:name="Risk",type=string,JSONPath=`.spec.riskLevel`
// +kubebuilder:printcolumn:name="Approval",type=boolean,JSONPath=`.spec.requiresApproval`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Runbook is the Schema for the runbooks API.
type Runbook struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RunbookSpec   `json:"spec,omitempty"`
	Status RunbookStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RunbookList contains a list of Runbook.
type RunbookList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Runbook `json:"items"`
}
