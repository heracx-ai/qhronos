package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/feedloop/qhronos/internal/models"
)

func NewAPICallExecutor(httpClient HTTPClient) ActionExecutor {
	return func(ctx context.Context, event *models.Event, params json.RawMessage) error {
		var apiParams models.ApicallActionParams
		if err := json.Unmarshal(params, &apiParams); err != nil {
			return fmt.Errorf("failed to unmarshal apicall params: %w", err)
		}
		if apiParams.URL == "" || apiParams.Method == "" {
			return fmt.Errorf("apicall action requires both url and method")
		}
		var bodyReader *bytes.Reader
		if len(apiParams.Body) > 0 {
			bodyReader = bytes.NewReader(apiParams.Body)
		} else {
			bodyReader = bytes.NewReader([]byte{})
		}
		req, err := http.NewRequestWithContext(ctx, apiParams.Method, apiParams.URL, bodyReader)
		if err != nil {
			return fmt.Errorf("failed to build apicall request: %w", err)
		}
		for k, v := range apiParams.Headers {
			req.Header.Set(k, v)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("apicall request failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("apicall returned non-2xx status: %d", resp.StatusCode)
		}
		return nil
	}
}
