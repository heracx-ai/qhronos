package scheduler

import (
	"context"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"bytes"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/feedloop/qhronos/internal/repository"
	"github.com/feedloop/qhronos/internal/services"
	"github.com/feedloop/qhronos/internal/testutils"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	resp := args.Get(0)
	if resp == nil {
		return nil, args.Error(1)
	}
	return resp.(*http.Response), args.Error(1)
}

func TestDispatcher(t *testing.T) {
	ctx := context.Background()
	db := testutils.TestDB(t)
	eventRepo := repository.NewEventRepository(db)
	occurrenceRepo := repository.NewOccurrenceRepository(db)
	hmacService := services.NewHMACService("test-secret")
	mockHTTP := new(MockHTTPClient)

	dispatcher := NewDispatcher(eventRepo, occurrenceRepo, hmacService)
	dispatcher.SetHTTPClient(mockHTTP)

	// Add cleanup function
	cleanup := func() {
		_, err := db.ExecContext(ctx, "TRUNCATE TABLE occurrences CASCADE")
		require.NoError(t, err)
		_, err = db.ExecContext(ctx, "TRUNCATE TABLE events CASCADE")
		require.NoError(t, err)
		// Reset mock expectations
		mockHTTP = new(MockHTTPClient)
		dispatcher.SetHTTPClient(mockHTTP)
	}

	t.Run("successful dispatch", func(t *testing.T) {
		cleanup()

		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		// Create test occurrence (simulate scheduling in Redis only)
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: "",
			StartedAt:    time.Time{},
			CompletedAt:  time.Time{},
		}

		// Setup expectations
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBuffer([]byte{})),
		}, nil)

		// Run dispatch
		err = dispatcher.DispatchWebhook(ctx, occurrence, event)

		// Verify
		assert.NoError(t, err)
		mockHTTP.AssertExpectations(t)

		// Verify occurrence log in Postgres (append-only)
		logged, err := occurrenceRepo.GetLatestByOccurrenceID(ctx, occurrence.OccurrenceID)
		require.NoError(t, err)
		require.NotNil(t, logged)
		assert.Equal(t, models.OccurrenceStatusCompleted, logged.Status)
	})

	t.Run("event not found", func(t *testing.T) {
		cleanup()
		// Create test occurrence with non-existent event
		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      uuid.New(),
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: "",
			StartedAt:    time.Time{},
			CompletedAt:  time.Time{},
		}

		// Create a dummy event for the test
		event := &models.Event{
			ID:          occurrence.EventID,
			Name:        "Non-existent Event",
			Description: "This event doesn't exist in the database",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		// Run dispatch
		err := dispatcher.DispatchWebhook(ctx, occurrence, event)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "event not found")
	})

	t.Run("webhook request failure", func(t *testing.T) {
		cleanup()
		// Create test event
		event := &models.Event{
			ID:          uuid.New(),
			Name:        "Test Event",
			Description: "Test Description",
			StartTime:   time.Now(),
			WebhookURL:  "http://example.com/webhook",
			Status:      models.EventStatusActive,
			Metadata:    []byte(`{"key": "value"}`),
			Tags:        pq.StringArray{"test"},
			CreatedAt:   time.Now(),
		}

		err := eventRepo.Create(ctx, event)
		require.NoError(t, err)

		occurrence := &models.Occurrence{
			OccurrenceID: uuid.New(),
			EventID:      event.ID,
			ScheduledAt:  time.Now(),
			Status:       models.OccurrenceStatusPending,
			Timestamp:    time.Now(),
			StatusCode:   0,
			ResponseBody: "",
			ErrorMessage: "",
			StartedAt:    time.Time{},
			CompletedAt:  time.Time{},
		}

		err = occurrenceRepo.Create(ctx, occurrence)
		require.NoError(t, err)

		// Setup expectations
		mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("connection failed"))

		// Run dispatch
		err = dispatcher.DispatchWebhook(ctx, occurrence, event)

		// Verify
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "webhook request failed")
		mockHTTP.AssertExpectations(t)

		// Verify occurrence status
		updatedOccurrence, err := occurrenceRepo.GetLatestByOccurrenceID(ctx, occurrence.OccurrenceID)
		require.NoError(t, err)
		assert.Equal(t, models.OccurrenceStatusFailed, updatedOccurrence.Status)
	})
}
