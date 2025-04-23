package handlers

import (
	"net/http"
	"strconv"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid occurrence ID (must be integer)"})
		return
	}

	occurrence, err := h.occurrenceRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get occurrence"})
		return
	}

	if occurrence == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Occurrence not found"})
		return
	}

	c.JSON(http.StatusOK, occurrence)
}

func (h *OccurrenceHandler) ListOccurrencesByTags(c *gin.Context) {
	tags := c.QueryArray("tags")
	if len(tags) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tags parameter is required"})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 50
	}

	filter := models.OccurrenceFilter{
		Tags:  tags,
		Page:  page,
		Limit: limit,
	}

	occurrences, total, err := h.occurrenceRepo.ListByTags(c.Request.Context(), filter)
	if err != nil {
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

	c.JSON(http.StatusOK, response)
}

func (h *OccurrenceHandler) ListOccurrencesByEvent(c *gin.Context) {
	eventID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid event ID"})
		return
	}

	// Check if event exists
	event, err := h.eventRepo.GetByID(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get event"})
		return
	}
	if event == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Event not found"})
		return
	}

	occurrences, err := h.occurrenceRepo.ListByEventID(c.Request.Context(), eventID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list occurrences"})
		return
	}

	c.JSON(http.StatusOK, occurrences)
}
