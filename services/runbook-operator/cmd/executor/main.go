package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	heliosv1alpha1 "github.com/rhwendt/helios/services/runbook-operator/api/v1alpha1"
	"github.com/rhwendt/helios/services/runbook-operator/pkg/audit"
	gnmiclient "github.com/rhwendt/helios/services/runbook-operator/pkg/gnmic"
	"github.com/rhwendt/helios/services/runbook-operator/pkg/template"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	executionName := os.Getenv("EXECUTION_NAME")
	executionNamespace := os.Getenv("EXECUTION_NAMESPACE")
	if executionName == "" || executionNamespace == "" {
		log.Error("EXECUTION_NAME and EXECUTION_NAMESPACE must be set")
		os.Exit(1)
	}

	exitCode, err := run(log, executionName, executionNamespace)
	if err != nil {
		log.Error("execution failed", "error", err)
	}
	os.Exit(exitCode)
}

func run(log *slog.Logger, executionName, executionNamespace string) (int, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	// Set up Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		return 1, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	k8sClient, err := client.New(config, client.Options{})
	if err != nil {
		return 1, fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Fetch execution
	var execution heliosv1alpha1.RunbookExecution
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: executionName, Namespace: executionNamespace}, &execution); err != nil {
		return 1, fmt.Errorf("failed to get execution: %w", err)
	}

	// Fetch referenced runbook
	rbNS := execution.Spec.RunbookRef.Namespace
	if rbNS == "" {
		rbNS = executionNamespace
	}
	var runbook heliosv1alpha1.Runbook
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: execution.Spec.RunbookRef.Name, Namespace: rbNS}, &runbook); err != nil {
		return 1, fmt.Errorf("failed to get runbook: %w", err)
	}

	auditLogger := audit.NewLogger(log)
	tmplEngine := template.NewEngine()

	// Build parameters map
	params := make(map[string]interface{})
	if execution.Spec.Parameters != nil {
		params = execution.Spec.Parameters
	}

	// Execute steps sequentially
	steps := runbook.Spec.Steps
	stepStatuses := make([]heliosv1alpha1.ExecutionStepStatus, len(steps))

	for i, step := range steps {
		stepStatuses[i] = heliosv1alpha1.ExecutionStepStatus{
			Name:   step.Name,
			Status: heliosv1alpha1.StepPending,
		}
	}

	exitCode := 0
	for i, step := range steps {
		now := metav1.Now()
		stepStatuses[i].Status = heliosv1alpha1.StepRunning
		stepStatuses[i].StartTime = &now

		auditLogger.LogStepStart(ctx, executionName, executionNamespace, runbook.Spec.Name, step.Name, execution.Spec.TriggeredBy)

		// Check condition
		if step.Condition != "" {
			result, err := tmplEngine.Render(step.Condition, params)
			if err != nil {
				log.Warn("condition evaluation failed", "step", step.Name, "error", err)
			}
			if result == "false" || result == "" {
				completionTime := metav1.Now()
				stepStatuses[i].Status = heliosv1alpha1.StepSkipped
				stepStatuses[i].CompletionTime = &completionTime
				stepStatuses[i].Output = "Condition not met, skipped"
				continue
			}
		}

		// Execute step
		output, err := executeStep(ctx, log, step, params, tmplEngine, execution.Spec.DryRun)

		completionTime := metav1.Now()
		stepStatuses[i].CompletionTime = &completionTime

		if err != nil {
			stepStatuses[i].Status = heliosv1alpha1.StepFailed
			stepStatuses[i].Error = err.Error()
			auditLogger.LogStepFailed(ctx, executionName, executionNamespace, runbook.Spec.Name, step.Name, execution.Spec.TriggeredBy, err.Error())

			if !step.ContinueOnError {
				exitCode = 1
				break
			}
		} else {
			stepStatuses[i].Status = heliosv1alpha1.StepCompleted
			stepStatuses[i].Output = output
			auditLogger.LogStepComplete(ctx, executionName, executionNamespace, runbook.Spec.Name, step.Name, execution.Spec.TriggeredBy, output)
		}

		// Update execution status with step progress
		execution.Status.Steps = stepStatuses
		if updateErr := k8sClient.Status().Update(ctx, &execution); updateErr != nil {
			log.Error("failed to update execution status", "error", updateErr)
		}
	}

	// Mark remaining steps as skipped if we exited early
	for i := range stepStatuses {
		if stepStatuses[i].Status == heliosv1alpha1.StepPending {
			stepStatuses[i].Status = heliosv1alpha1.StepSkipped
		}
	}

	execution.Status.Steps = stepStatuses
	if err := k8sClient.Status().Update(ctx, &execution); err != nil {
		log.Error("failed to update final execution status", "error", err)
	}

	return exitCode, nil
}

func executeStep(ctx context.Context, log *slog.Logger, step heliosv1alpha1.RunbookStep, params map[string]interface{}, tmplEngine *template.Engine, dryRun bool) (string, error) {
	switch step.Action {
	case heliosv1alpha1.ActionGNMISet:
		return executeGNMISet(ctx, log, step, params, tmplEngine, dryRun)
	case heliosv1alpha1.ActionGNMIGet:
		return executeGNMIGet(ctx, log, step, params, tmplEngine)
	case heliosv1alpha1.ActionWait:
		return executeWait(ctx, step)
	case heliosv1alpha1.ActionNotify:
		return "notification sent", nil
	case heliosv1alpha1.ActionCondition:
		return "condition evaluated", nil
	default:
		return "", fmt.Errorf("unsupported action: %s", step.Action)
	}
}

func executeGNMISet(ctx context.Context, log *slog.Logger, step heliosv1alpha1.RunbookStep, params map[string]interface{}, tmplEngine *template.Engine, dryRun bool) (string, error) {
	config, err := tmplEngine.RenderConfig(step.Config, params)
	if err != nil {
		return "", fmt.Errorf("failed to render config: %w", err)
	}

	target, _ := config["target"].(string)
	if target == "" {
		return "", fmt.Errorf("gNMI target not specified in step config")
	}

	if dryRun {
		configJSON, err := json.Marshal(config)
		if err != nil {
			return "", fmt.Errorf("failed to marshal config for dry run: %w", err)
		}
		return fmt.Sprintf("[DRY RUN] Would execute gNMI Set on %s: %s", target, string(configJSON)), nil
	}

	client := gnmiclient.NewClient(target, "", "", log)
	if err := client.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to %s: %w", target, err)
	}
	defer func() { _ = client.Close() }()

	path, _ := config["path"].(string)
	value := config["value"]

	_, err = client.Set(ctx, []gnmiclient.SetRequest{
		{Operation: gnmiclient.SetUpdate, Path: path, Value: value},
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("gNMI Set completed on %s path %s", target, path), nil
}

func executeGNMIGet(ctx context.Context, log *slog.Logger, step heliosv1alpha1.RunbookStep, params map[string]interface{}, tmplEngine *template.Engine) (string, error) {
	config, err := tmplEngine.RenderConfig(step.Config, params)
	if err != nil {
		return "", fmt.Errorf("failed to render config: %w", err)
	}

	target, _ := config["target"].(string)
	if target == "" {
		return "", fmt.Errorf("gNMI target not specified in step config")
	}

	client := gnmiclient.NewClient(target, "", "", log)
	if err := client.Connect(ctx); err != nil {
		return "", fmt.Errorf("failed to connect to %s: %w", target, err)
	}
	defer func() { _ = client.Close() }()

	path, _ := config["path"].(string)
	resp, err := client.Get(ctx, []string{path})
	if err != nil {
		return "", err
	}

	respJSON, _ := json.Marshal(resp)
	return string(respJSON), nil
}

func executeWait(ctx context.Context, step heliosv1alpha1.RunbookStep) (string, error) {
	durationStr, _ := step.Config["duration"].(string)
	if durationStr == "" {
		durationStr = step.Timeout
	}
	if durationStr == "" {
		durationStr = "10s"
	}

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		return "", fmt.Errorf("invalid wait duration %q: %w", durationStr, err)
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(duration):
		return fmt.Sprintf("waited %s", duration), nil
	}
}
