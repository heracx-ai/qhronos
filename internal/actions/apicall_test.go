package actions

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	resp, _ := args.Get(0).(*http.Response)
	return resp, args.Error(1)
}

func TestAPICallExecutor_Success(t *testing.T) {
	mockHTTP := new(MockHTTPClient)
	executor := NewAPICallExecutor(mockHTTP)
	params := models.ApicallActionParams{
		Method:  "POST",
		URL:     "https://api.example.com/endpoint",
		Headers: map[string]string{"Authorization": "Bearer token", "Content-Type": "application/json"},
		Body:    json.RawMessage(`{"foo":"bar"}`),
	}
	paramsBytes, _ := json.Marshal(params)
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 200,
		Body:       ioutil.NopCloser(nil),
	}, nil)
	event := &models.Event{ID: [16]byte{1}, Name: "API Call Event"}
	err := executor(context.Background(), event, paramsBytes)
	assert.NoError(t, err)
	mockHTTP.AssertCalled(t, "Do", mock.AnythingOfType("*http.Request"))
}

func TestAPICallExecutor_Non2xx(t *testing.T) {
	mockHTTP := new(MockHTTPClient)
	executor := NewAPICallExecutor(mockHTTP)
	params := models.ApicallActionParams{
		Method:  "POST",
		URL:     "https://api.example.com/endpoint",
		Headers: map[string]string{},
		Body:    json.RawMessage(`{"foo":"bar"}`),
	}
	paramsBytes, _ := json.Marshal(params)
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
		StatusCode: 500,
		Body:       ioutil.NopCloser(nil),
	}, nil)
	event := &models.Event{ID: [16]byte{1}, Name: "API Call Event"}
	err := executor(context.Background(), event, paramsBytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "non-2xx")
}

func TestAPICallExecutor_NetworkError(t *testing.T) {
	mockHTTP := new(MockHTTPClient)
	executor := NewAPICallExecutor(mockHTTP)
	params := models.ApicallActionParams{
		Method:  "POST",
		URL:     "https://api.example.com/endpoint",
		Headers: map[string]string{},
		Body:    json.RawMessage(`{"foo":"bar"}`),
	}
	paramsBytes, _ := json.Marshal(params)
	mockHTTP.On("Do", mock.AnythingOfType("*http.Request")).Return((*http.Response)(nil), errors.New("network error"))
	event := &models.Event{ID: [16]byte{1}, Name: "API Call Event"}
	err := executor(context.Background(), event, paramsBytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "network error")
}

func TestAPICallExecutor_MissingParams(t *testing.T) {
	mockHTTP := new(MockHTTPClient)
	executor := NewAPICallExecutor(mockHTTP)
	params := models.ApicallActionParams{
		Method:  "",
		URL:     "",
		Headers: map[string]string{},
		Body:    json.RawMessage(`{"foo":"bar"}`),
	}
	paramsBytes, _ := json.Marshal(params)
	event := &models.Event{ID: [16]byte{1}, Name: "API Call Event"}
	err := executor(context.Background(), event, paramsBytes)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires both url and method")
}
