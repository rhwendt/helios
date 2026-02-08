package approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// NotificationType defines the notification channel type.
type NotificationType string

const (
	NotifySlack    NotificationType = "slack"
	NotifyTeams    NotificationType = "teams"
	NotifyWebhook  NotificationType = "webhook"
)

// ApprovalRequest represents a pending approval request.
type ApprovalRequest struct {
	ExecutionName string
	Namespace     string
	RunbookName   string
	TriggeredBy   string
	RiskLevel     string
	Approvers     []string
}

// Approver dispatches approval notifications and checks approval status.
type Approver struct {
	webhookURL string
	notifyType NotificationType
	httpClient *http.Client
	log        *slog.Logger
}

// NewApprover creates a new Approver.
func NewApprover(webhookURL string, notifyType NotificationType, log *slog.Logger) *Approver {
	return &Approver{
		webhookURL: webhookURL,
		notifyType: notifyType,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		log:        log,
	}
}

// SendApprovalNotification sends a notification requesting approval.
func (a *Approver) SendApprovalNotification(ctx context.Context, req ApprovalRequest) error {
	var payload []byte
	var err error

	switch a.notifyType {
	case NotifySlack:
		payload, err = a.buildSlackPayload(req)
	case NotifyTeams:
		payload, err = a.buildTeamsPayload(req)
	default:
		payload, err = a.buildGenericPayload(req)
	}
	if err != nil {
		return fmt.Errorf("failed to build notification payload: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("notification webhook returned status %d", resp.StatusCode)
	}

	a.log.Info("approval notification sent", "execution", req.ExecutionName, "type", a.notifyType)
	return nil
}

func (a *Approver) buildSlackPayload(req ApprovalRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"text": fmt.Sprintf("Runbook approval requested for *%s*", req.RunbookName),
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Runbook Approval Request*\n\n*Runbook:* %s\n*Execution:* %s/%s\n*Triggered by:* %s\n*Risk Level:* %s",
						req.RunbookName, req.Namespace, req.ExecutionName, req.TriggeredBy, req.RiskLevel),
				},
			},
		},
	}
	return json.Marshal(payload)
}

func (a *Approver) buildTeamsPayload(req ApprovalRequest) ([]byte, error) {
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"summary":    fmt.Sprintf("Runbook approval: %s", req.RunbookName),
		"themeColor": "FF9800",
		"title":      "Runbook Approval Request",
		"sections": []map[string]interface{}{
			{
				"facts": []map[string]string{
					{"name": "Runbook", "value": req.RunbookName},
					{"name": "Execution", "value": fmt.Sprintf("%s/%s", req.Namespace, req.ExecutionName)},
					{"name": "Triggered by", "value": req.TriggeredBy},
					{"name": "Risk Level", "value": req.RiskLevel},
				},
			},
		},
	}
	return json.Marshal(payload)
}

func (a *Approver) buildGenericPayload(req ApprovalRequest) ([]byte, error) {
	return json.Marshal(req)
}
