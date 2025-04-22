package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/lib/pq"
)

type EventHandler struct {
	repo *repository.EventRepository
}

func NewEventHandler(repo *repository.EventRepository) *EventHandler {
	return &EventHandler{repo: repo}
}

func (h *EventHandler) CreateEvent(c *gin.Context) {
	var req models.CreateEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required fields
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.WebhookURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "webhook_url is required"})
		return
	}
	if req.StartTime.IsZero() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time is required"})
		return
	}
	if len(req.Metadata) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "metadata is required"})
		return
	}

	// Validate schedule format if provided
	if req.Schedule != nil && !isValidSchedule(*req.Schedule) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule format"})
		return
	}

	event := &models.Event{
		Name:        req.Name,
		Description: req.Description,
		StartTime:   req.StartTime,
		WebhookURL:  req.WebhookURL,
		Metadata:    req.Metadata,
		Schedule:    req.Schedule,
		Tags:        pq.StringArray(req.Tags),
	}

	if err := h.repo.Create(c, event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
		if err == models.ErrEventNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if req.WebhookURL != nil {
		event.WebhookURL = *req.WebhookURL
	}
	if req.Metadata != nil {
		event.Metadata = req.Metadata
	}
	if req.Schedule != nil {
		if !isValidSchedule(*req.Schedule) {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tags parameter is required"})
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

func isValidSchedule(schedule string) bool {
	// Basic validation: check if the schedule string contains required parts
	parts := strings.Split(schedule, ";")
	if len(parts) < 1 {
		return false
	}

	hasFreq := false
	for _, part := range parts {
		if strings.HasPrefix(part, "FREQ=") {
			hasFreq = true
			freq := strings.TrimPrefix(part, "FREQ=")
			switch freq {
			case "YEARLY", "MONTHLY", "WEEKLY", "DAILY", "HOURLY", "MINUTELY", "SECONDLY":
				continue
			default:
				return false
			}
		}
	}

	return hasFreq
} 