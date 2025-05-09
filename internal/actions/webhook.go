package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/services"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func NewWebhookExecutor(hmacService *services.HMACService, httpClient HTTPClient) ActionExecutor {
	return func(ctx context.Context, event *models.Event, params json.RawMessage) error {
		var whParams models.WebhookActionParams
		if err := json.Unmarshal(params, &whParams); err != nil {
			return fmt.Errorf("failed to unmarshal webhook params: %w", err)
		}
		if whParams.URL == "" {
			return fmt.Errorf("webhook URL is empty in action params")
		}

		payload := map[string]interface{}{
			"event_id":     event.ID,
			"name":         event.Name,
			"description":  event.Description,
			"scheduled_at": event.StartTime,
			"metadata":     event.Metadata,
			"tags":         event.Tags,
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshaling webhook payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "POST", whParams.URL, bytes.NewBuffer(jsonPayload))
		if err != nil {
			return fmt.Errorf("error creating webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		if hmacService != nil {
			secret := event.HMACSecret
			if secret == nil || *secret == "" {
				sig := hmacService.SignPayload(jsonPayload, "")
				req.Header.Set("X-Qhronos-Signature", sig)
			} else {
				sig := hmacService.SignPayload(jsonPayload, *secret)
				req.Header.Set("X-Qhronos-Signature", sig)
			}
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("received non-2xx status code: %d", resp.StatusCode)
		}
		return nil
	}
}
