package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// ActionType defines the type of action to be performed.
type ActionType string

const (
	ActionTypeWebhook   ActionType = "webhook"
	ActionTypeWebsocket ActionType = "websocket"
	ActionTypeAPICall   ActionType = "apicall"
	// ActionTypeUnknown represents an unsupported or undefined action type.
	ActionTypeUnknown ActionType = "unknown"
)

// ActionParams is an interface for action-specific parameters.
// We use json.RawMessage for Params in the Action struct to allow flexible
// marshalling/unmarshalling of type-specific parameter structs.
type ActionParams interface{}

// WebhookActionParams contains parameters specific to webhook actions.
type WebhookActionParams struct {
	URL string `json:"url"`
	// Future enhancements:
	// Method  string            `json:"method,omitempty"`
	// Headers map[string]string `json:"headers,omitempty"`
	// Body    string            `json:"body,omitempty"` // Template support could be added
}

// WebsocketActionParams contains parameters specific to websocket actions.
// This refers to dispatching to a client connected to Qhronos's own WebSocket server.
type WebsocketActionParams struct {
	ClientName string `json:"client_name"`
}

// ApicallActionParams defines parameters for the apicall action type
type ApicallActionParams struct {
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	URL     string            `json:"url"`
}

// Action represents a generic action to be performed for an event.
type Action struct {
	Type   ActionType      `json:"type"`
	Params json.RawMessage `json:"params"` // Holds WebhookActionParams or WebsocketActionParams as JSON
}

// Value implements the driver.Valuer interface for database serialization.
func (a *Action) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}
	return json.Marshal(a)
}

// Scan implements the sql.Scanner interface for database deserialization.
func (a *Action) Scan(value interface{}) error {
	if value == nil {
		*a = Action{}
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("type assertion to []byte failed for Action scan")
	}
	return json.Unmarshal(b, a)
}

// GetWebhookParams attempts to unmarshal the action's params into WebhookActionParams.
func (a *Action) GetWebhookParams() (*WebhookActionParams, error) {
	if a.Type != ActionTypeWebhook {
		return nil, fmt.Errorf("action type is not '%s'", ActionTypeWebhook)
	}
	var params WebhookActionParams
	if err := json.Unmarshal(a.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook params: %w", err)
	}
	return &params, nil
}

// GetWebsocketParams attempts to unmarshal the action's params into WebsocketActionParams.
func (a *Action) GetWebsocketParams() (*WebsocketActionParams, error) {
	if a.Type != ActionTypeWebsocket {
		return nil, fmt.Errorf("action type is not '%s'", ActionTypeWebsocket)
	}
	var params WebsocketActionParams
	if err := json.Unmarshal(a.Params, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal websocket params: %w", err)
	}
	return &params, nil
}
