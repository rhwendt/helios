package audit

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// EventType defines the type of audit event.
type EventType string

const (
	EventExecutionCreated  EventType = "ExecutionCreated"
	EventExecutionStarted  EventType = "ExecutionStarted"
	EventStepStarted       EventType = "StepStarted"
	EventStepCompleted     EventType = "StepCompleted"
	EventStepFailed        EventType = "StepFailed"
	EventApprovalRequested EventType = "ApprovalRequested"
	EventApprovalGranted   EventType = "ApprovalGranted"
	EventApprovalDenied    EventType = "ApprovalDenied"
	EventRollbackStarted   EventType = "RollbackStarted"
	EventRollbackCompleted EventType = "RollbackCompleted"
	EventExecutionCompleted EventType = "ExecutionCompleted"
	EventExecutionFailed   EventType = "ExecutionFailed"
)

// AuditEvent represents a single audit log entry.
type AuditEvent struct {
	Timestamp     time.Time         `json:"timestamp"`
	EventType     EventType         `json:"eventType"`
	ExecutionName string            `json:"executionName"`
	Namespace     string            `json:"namespace"`
	RunbookName   string            `json:"runbookName"`
	StepName      string            `json:"stepName,omitempty"`
	TriggeredBy   string            `json:"triggeredBy"`
	Message       string            `json:"message"`
	Details       map[string]string `json:"details,omitempty"`
}

// Logger provides structured audit logging for runbook executions.
type Logger struct {
	log *slog.Logger
}

// NewLogger creates a new audit Logger.
func NewLogger(log *slog.Logger) *Logger {
	return &Logger{
		log: log.With("component", "audit"),
	}
}

// LogEvent records an audit event to structured logging.
func (l *Logger) LogEvent(_ context.Context, event AuditEvent) {
	event.Timestamp = time.Now()

	attrs := []slog.Attr{
		slog.String("event_type", string(event.EventType)),
		slog.String("execution", fmt.Sprintf("%s/%s", event.Namespace, event.ExecutionName)),
		slog.String("runbook", event.RunbookName),
		slog.String("triggered_by", event.TriggeredBy),
		slog.String("message", event.Message),
	}

	if event.StepName != "" {
		attrs = append(attrs, slog.String("step", event.StepName))
	}

	for k, v := range event.Details {
		attrs = append(attrs, slog.String(k, v))
	}

	l.log.LogAttrs(context.Background(), slog.LevelInfo, "audit_event", attrs...)
}

// LogStepStart logs the start of a step execution.
func (l *Logger) LogStepStart(ctx context.Context, execName, ns, runbook, step, triggeredBy string) {
	l.LogEvent(ctx, AuditEvent{
		EventType:     EventStepStarted,
		ExecutionName: execName,
		Namespace:     ns,
		RunbookName:   runbook,
		StepName:      step,
		TriggeredBy:   triggeredBy,
		Message:       fmt.Sprintf("Step %q started", step),
	})
}

// LogStepComplete logs the completion of a step.
func (l *Logger) LogStepComplete(ctx context.Context, execName, ns, runbook, step, triggeredBy, output string) {
	l.LogEvent(ctx, AuditEvent{
		EventType:     EventStepCompleted,
		ExecutionName: execName,
		Namespace:     ns,
		RunbookName:   runbook,
		StepName:      step,
		TriggeredBy:   triggeredBy,
		Message:       fmt.Sprintf("Step %q completed", step),
		Details:       map[string]string{"output": output},
	})
}

// LogStepFailed logs a step failure.
func (l *Logger) LogStepFailed(ctx context.Context, execName, ns, runbook, step, triggeredBy, errMsg string) {
	l.LogEvent(ctx, AuditEvent{
		EventType:     EventStepFailed,
		ExecutionName: execName,
		Namespace:     ns,
		RunbookName:   runbook,
		StepName:      step,
		TriggeredBy:   triggeredBy,
		Message:       fmt.Sprintf("Step %q failed: %s", step, errMsg),
		Details:       map[string]string{"error": errMsg},
	})
}
