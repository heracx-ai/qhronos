package handlers

import (
	"net/http"
	"strconv"

	"github.com/feedloop/qhronos/internal/middleware"
	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type OccurrenceHandler struct {
	eventRepo      *repository.EventRepository
	occurrenceRepo *repository.OccurrenceRepository
}

func NewOccurrenceHandler(eventRepo *repository.EventRepository, occurrenceRepo *repository.OccurrenceRepository) *OccurrenceHandler {
	return &OccurrenceHandler{
		eventRepo:      eventRepo,
		occurrenceRepo: occurrenceRepo,
	}
}

func (h *OccurrenceHandler) GetOccurrence(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		logger.Warn("Invalid occurrence ID", zap.String("id", idStr), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid occurrence ID (must be integer)"})
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		logger.Error("Failed to get occurrence", zap.Int("id", id), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get occurrence"})
		return
	}

	if occurrence == nil {
		logger.Info("Occurrence not found", zap.Int("id", id))
		c.JSON(http.StatusNotFound, gin.H{"error": "Occurrence not found"})
		return
	}
	logger.Info("Occurrence retrieved", zap.Int("id", id))
	c.JSON(http.StatusOK, occurrence)
}

func (h *OccurrenceHandler) ListOccurrencesByTags(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	tags := c.QueryArray("tags")
	if len(tags) == 0 {
		logger.Warn("Tags parameter is required for listing occurrences")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tags parameter is required"})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		logger.Warn("Invalid page parameter", zap.String("page", c.DefaultQuery("page", "1")), zap.Error(err))
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		logger.Warn("Invalid limit parameter", zap.String("limit", c.DefaultQuery("limit", "50")), zap.Error(err))
		limit = 50
	}

	filter := models.OccurrenceFilter{
		Tags:  tags,
		Page:  page,
		Limit: limit,
	}

	occurrences, total, err := h.occurrenceRepo.ListByTags(c.Request.Context(), filter)
	if err != nil {
		logger.Error("Failed to list occurrences by tags", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list occurrences"})
		return
	}

	if occurrences == nil {
		occurrences = []models.Occurrence{}
	}

	response := models.PaginatedResponse{
		Data: occurrences,
		Pagination: struct {
			Page  int `json:"page"`
			Limit int `json:"limit"`
			Total int `json:"total"`
		}{
			Page:  page,
			Limit: limit,
			Total: total,
		},
	}

	logger.Info("Listed occurrences by tags", zap.Strings("tags", tags), zap.Int("total", total))
	c.JSON(http.StatusOK, response)
}

func (h *OccurrenceHandler) ListOccurrencesByEvent(c *gin.Context) {
	logger := c.MustGet(middleware.LoggerKey).(*zap.Logger)
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		logger.Warn("Invalid event ID for listing occurrences", zap.String("id", c.Param("id")), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Check if event exists
	event, err := h.eventRepo.GetByID(c.Request.Context(), eventID)
	if err != nil {
		logger.Error("Failed to get event for listing occurrences", zap.String("event_id", eventID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get event"})
		return
	}
	if event == nil {
		logger.Info("Event not found for listing occurrences", zap.String("event_id", eventID.String()))
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	occurrences, err := h.occurrenceRepo.ListByEventID(c.Request.Context(), eventID)
	if err != nil {
		logger.Error("Failed to list occurrences by event", zap.String("event_id", eventID.String()), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list occurrences"})
		return
	}

	logger.Info("Listed occurrences by event", zap.String("event_id", eventID.String()), zap.Int("count", len(occurrences)))
	c.JSON(http.StatusOK, occurrences)
}
