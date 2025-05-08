package handlers

import (
	"context"
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

func (h *EventHandler) CreateEvent(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	var req models.CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("Invalid event creation request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required fields
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Webhook == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "webhook is required"})
		return
	}
	if req.StartTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time is required"})
		return
	}
	// Default metadata to empty object if not provided
	if len(req.Metadata) == 0 {
		emptyJSON := datatypes.JSON([]byte("{}"))
		req.Metadata = emptyJSON
	}
	// Default tags to empty array if not provided
	if req.Tags == nil {
		req.Tags = []string{}
	}

	// Validate start_time
	if req.StartTime.Before(time.Now().UTC()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Start time must be in the future"})
		return
	}

	// Validate schedule format if provided
	if req.Schedule != nil && !isValidSchedule(req.Schedule) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule format"})
		return
	}

	event := &models.Event{
		Name:        req.Name,
		Description: req.Description,
		StartTime:   req.StartTime,
		Webhook:     req.Webhook,
		Metadata:    req.Metadata,
		Schedule:    req.Schedule,
		Tags:        pq.StringArray(req.Tags),
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

	c.JSON(http.StatusCreated, event)
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
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event ID"})
		return
	}

	var req models.UpdateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	event, err := h.repo.GetByID(c, id)
	if err != nil {
		if err == models.ErrEventNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		event.Name = *req.Name
	}
	if req.Description != nil {
		event.Description = *req.Description
	}
	if req.StartTime != nil {
		event.StartTime = *req.StartTime
	}
	if req.Webhook != nil {
		event.Webhook = *req.Webhook
	}
	if req.Metadata != nil {
		event.Metadata = req.Metadata
	}
	if req.StartTime != nil {
		oldStartTime := event.StartTime
		if oldStartTime != *req.StartTime && req.StartTime.Before(time.Now().UTC()) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Start time must be in the future"})
		}
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
		event.Status = models.EventStatus(*req.Status)
	}

	if err := h.repo.Update(c, event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Remove all scheduled occurrences for this event from Redis
	err = h.repo.RemoveEventOccurrencesFromRedis(c, event.ID)
	if err != nil {
		// Log but do not fail the request
		zap.L().Warn("Failed to remove old scheduled occurrences from Redis", zap.String("event_id", event.ID.String()), zap.Error(err))
	}

	// Re-expand the event to schedule new occurrences
	go func() {
		if event.Schedule == nil {
			_ = h.expander.ExpandNonRecurringEvent(context.Background(), event)
		} else {
			_ = h.expander.ExpandRecurringEvent(context.Background(), event)
		}
	}()

	c.JSON(http.StatusOK, event)
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
	case "minutely", "hourly", "daily", "weekly", "monthly", "yearly":
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
