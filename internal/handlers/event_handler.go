package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"go.uber.org/zap"
	"gorm.io/datatypes"
)

type EventHandler struct {
	repo     *repository.EventRepository
	expander *scheduler.Expander
}

func NewEventHandler(repo *repository.EventRepository, expander *scheduler.Expander) *EventHandler {
	return &EventHandler{repo: repo, expander: expander}
}

// deriveActionFromRequest determines the Action and webhook string based on request fields.
// It prioritizes reqAction. If reqAction is nil, it derives from reqWebhook.
// Returns the derived Action, the canonical webhook string, and an error if validation fails.
func deriveActionFromRequest(reqWebhook *string, reqAction *models.Action) (*models.Action, string, error) {
	var finalAction *models.Action
	var finalWebhookString string
	var err error

	if reqAction != nil {
		finalAction = reqAction
		// Validate provided action
		if finalAction.Type == "" {
			return nil, "", fmt.Errorf("action type is required when action object is provided")
		}
		if finalAction.Params == nil || len(finalAction.Params) == 0 {
			return nil, "", fmt.Errorf("action params are required when action object is provided")
		}

		switch finalAction.Type {
		case models.ActionTypeWebhook:
			var whParams models.WebhookActionParams
			if err := json.Unmarshal(finalAction.Params, &whParams); err != nil {
				return nil, "", fmt.Errorf("invalid params for webhook action: %w", err)
			}
			if whParams.URL == "" {
				return nil, "", fmt.Errorf("URL is required for webhook action params")
			}
			finalWebhookString = whParams.URL
		case models.ActionTypeWebsocket:
			var wsParams models.WebsocketActionParams
			if err := json.Unmarshal(finalAction.Params, &wsParams); err != nil {
				return nil, "", fmt.Errorf("invalid params for websocket action: %w", err)
			}
			if wsParams.ClientName == "" {
				return nil, "", fmt.Errorf("client_name is required for websocket action params")
			}
			finalWebhookString = "q:" + wsParams.ClientName
		default:
			return nil, "", fmt.Errorf("unknown action type: %s", finalAction.Type)
		}
	} else if reqWebhook != nil && *reqWebhook != "" {
		finalWebhookString = *reqWebhook
		var paramsBytes []byte
		var actionType models.ActionType

		if len(finalWebhookString) > 2 && finalWebhookString[:2] == "q:" {
			actionType = models.ActionTypeWebsocket
			wsParams := models.WebsocketActionParams{ClientName: finalWebhookString[2:]}
			paramsBytes, err = json.Marshal(wsParams)
		} else {
			actionType = models.ActionTypeWebhook
			whParams := models.WebhookActionParams{URL: finalWebhookString}
			paramsBytes, err = json.Marshal(whParams)
		}
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal derived action params: %w", err)
		}
		finalAction = &models.Action{
			Type:   actionType,
			Params: paramsBytes,
		}
	} else {
		// Neither Action nor a valid Webhook string was provided.
		return nil, "", fmt.Errorf("either action object or a non-empty webhook string is required")
	}

	return finalAction, finalWebhookString, nil
}

func (h *EventHandler) CreateEvent(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	var req models.EventCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Invalid event creation request format", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format: " + err.Error()})
		return
	}

	// Validate required fields (name and start_time are validated by binding)
	if req.Name == "" { // Redundant if binding:required works, but good for clarity
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	// startTime is validated by binding:required

	derivedAction, webhookString, err := deriveActionFromRequest(req.Webhook, req.Action)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Default metadata to empty object if not provided
	// req.Metadata is json.RawMessage from EventCreateRequest
	var metadataToStore datatypes.JSON
	if len(req.Metadata) > 0 {
		metadataToStore = datatypes.JSON(req.Metadata)
	} else {
		metadataToStore = datatypes.JSON("{}")
	}

	// Default tags to empty array if not provided
	if req.Tags == nil {
		req.Tags = []string{}
	}

	// Validate schedule format if provided
	if req.Schedule != nil && !isValidSchedule(req.Schedule) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule format"})
		return
	}

	event := &models.Event{
		Name:        req.Name,
		Description: "",
		StartTime:   req.StartTime,
		Webhook:     webhookString, // Use the derived webhook string
		Action:      derivedAction, // Use the derived action
		Metadata:    metadataToStore,
		Schedule:    req.Schedule,
		Tags:        pq.StringArray(req.Tags),
		HMACSecret:  req.HMACSecret,
	}

	if req.Description != nil {
		event.Description = *req.Description
	}

	if err := h.repo.Create(c, event); err != nil {
		logger.Error("Failed to create event", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logger.Info("Event created", zap.String("event_id", event.ID.String()))

	// Immediate expansion if within window
	now := time.Now().UTC()
	graceStart := now.Add(-h.expander.GracePeriod())
	lookAheadEnd := now.Add(h.expander.LookAheadDuration())
	if event.StartTime.After(graceStart) && event.StartTime.Before(lookAheadEnd) {
		go func() {
			if event.Schedule == nil {
				_ = h.expander.ExpandNonRecurringEvent(context.Background(), event)
			} else {
				_ = h.expander.ExpandRecurringEvent(context.Background(), event)
			}
		}()
	}

	c.JSON(http.StatusCreated, event.ToEventResponse()) // Use ToEventResponse
}

func (h *EventHandler) GetEvent(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	event, err := h.repo.GetByID(c, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	c.JSON(http.StatusOK, event)
}

func (h *EventHandler) UpdateEvent(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID format"})
		return
	}

	var req models.EventUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Invalid event update request format", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format: " + err.Error()})
		return
	}

	event, err := h.repo.GetByID(c, id)
	if err != nil {
		// Assuming GetByID returns sql.ErrNoRows which is converted to nil by repo
		// or a specific error like models.ErrEventNotFound
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch event for update"})
		return
	}
	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}

	// Apply updates from request
	if req.Name != nil {
		event.Name = *req.Name
	}
	if req.Description != nil {
		event.Description = *req.Description
	}
	if req.StartTime != nil {
		event.StartTime = *req.StartTime
	}

	// Handle Action and Webhook update logic
	// Priority: req.Action > req.Webhook > existing value
	if req.Action != nil {
		derivedAction, derivedWebhookString, deriveErr := deriveActionFromRequest(nil, req.Action)
		if deriveErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action in update: " + deriveErr.Error()})
			return
		}
		event.Action = derivedAction
		event.Webhook = derivedWebhookString
	} else if req.Webhook != nil { // Only consider req.Webhook if req.Action was not provided
		derivedAction, derivedWebhookString, deriveErr := deriveActionFromRequest(req.Webhook, nil)
		if deriveErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid webhook string in update: " + deriveErr.Error()})
			return
		}
		event.Action = derivedAction
		event.Webhook = derivedWebhookString
	} // If neither is provided, event.Action and event.Webhook remain as fetched

	if req.Metadata != nil { // req.Metadata is json.RawMessage
		event.Metadata = datatypes.JSON(req.Metadata)
	}

	if req.Schedule != nil {
		if !isValidSchedule(req.Schedule) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule format"})
			return
		}
		event.Schedule = req.Schedule
	}
	if req.Tags != nil {
		event.Tags = pq.StringArray(req.Tags)
	}
	if req.Status != nil {
		event.Status = *req.Status
	}
	if req.HMACSecret != nil {
		event.HMACSecret = req.HMACSecret
	}

	if err := h.repo.Update(c, event); err != nil {
		logger.Error("Failed to update event", zap.String("event_id", idStr), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update event"})
		return
	}

	// Remove all scheduled occurrences for this event from Redis and re-expand
	// This is important if start_time or schedule changed
	logger.Info("Event updated, re-expanding occurrences", zap.String("event_id", event.ID.String()))
	err = h.repo.RemoveEventOccurrencesFromRedis(c, event.ID)
	if err != nil {
		logger.Warn("Failed to remove old scheduled occurrences from Redis after update", zap.String("event_id", event.ID.String()), zap.Error(err))
	}
	go func() {
		// Use a new context for background task
		bgCtx := context.Background()
		if event.Schedule == nil {
			err := h.expander.ExpandNonRecurringEvent(bgCtx, event)
			if err != nil {
				logger.Error("Error re-expanding non-recurring event after update", zap.String("event_id", event.ID.String()), zap.Error(err))
			}
		} else {
			err := h.expander.ExpandRecurringEvent(bgCtx, event)
			if err != nil {
				logger.Error("Error re-expanding recurring event after update", zap.String("event_id", event.ID.String()), zap.Error(err))
			}
		}
	}()

	c.JSON(http.StatusOK, event.ToEventResponse()) // Use ToEventResponse
}

func (h *EventHandler) DeleteEvent(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	if err := h.repo.Delete(c, id); err != nil {
		if err == models.ErrEventNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *EventHandler) ListEventsByTags(c *gin.Context) {
	tagsParam := c.Query("tags")
	if tagsParam == "" {
		// No tags param: return all events
		events, err := h.repo.List(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, events)
		return
	}
	tags := strings.Split(tagsParam, ",")
	events, err := h.repo.ListByTags(c, tags)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

func isValidSchedule(schedule *models.ScheduleConfig) bool {
	if schedule == nil {
		return false
	}

	// Validate frequency
	switch schedule.Frequency {
	case "daily", "weekly", "monthly", "yearly":
		// Valid frequency
	default:
		return false
	}

	// Validate interval
	if schedule.Interval < 1 {
		return false
	}

	// Validate by_day for weekly frequency
	if schedule.Frequency == "weekly" && len(schedule.ByDay) > 0 {
		validDays := map[string]bool{
			"MO": true, "TU": true, "WE": true,
			"TH": true, "FR": true, "SA": true, "SU": true,
		}
		for _, day := range schedule.ByDay {
			if !validDays[day] {
				return false
			}
		}
	}

	// Validate by_month_day for monthly frequency
	if schedule.Frequency == "monthly" && len(schedule.ByMonthDay) > 0 {
		for _, day := range schedule.ByMonthDay {
			if day < 1 || day > 31 {
				return false
			}
		}
	}

	// Validate by_month for yearly frequency
	if schedule.Frequency == "yearly" && len(schedule.ByMonth) > 0 {
		for _, month := range schedule.ByMonth {
			if month < 1 || month > 12 {
				return false
			}
		}
	}

	// Validate count if provided
	if schedule.Count != nil && *schedule.Count < 1 {
		return false
	}

	// Validate until if provided
	if schedule.Until != nil {
		_, err := time.Parse(time.RFC3339, *schedule.Until)
		if err != nil {
			return false
		}
	}

	return true
}
