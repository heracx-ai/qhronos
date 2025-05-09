package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/feedloop/qhronos/internal/models"
)

type ClientNotifier interface {
	DispatchToClient(clientID string, payload []byte) error
}

func NewWebsocketExecutor(clientNotifier ClientNotifier) ActionExecutor {
	return func(ctx context.Context, event *models.Event, params json.RawMessage) error {
		var wsParams models.WebsocketActionParams
		if err := json.Unmarshal(params, &wsParams); err != nil {
			return fmt.Errorf("failed to unmarshal websocket params: %w", err)
		}
		if wsParams.ClientName == "" {
			return fmt.Errorf("client_name is empty in websocket action params")
		}

		payload := map[string]interface{}{
			"event_id":     event.ID,
			"name":         event.Name,
			"description":  event.Description,
			"scheduled_at": event.StartTime,
			"metadata":     event.Metadata,
			"tags":         event.Tags,
			"type":         "event",
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshaling websocket payload: %w", err)
		}
		return clientNotifier.DispatchToClient(wsParams.ClientName, jsonPayload)
	}
}
