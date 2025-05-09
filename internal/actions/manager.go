package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/feedloop/qhronos/internal/models"
)

type ActionExecutor func(ctx context.Context, event *models.Event, params json.RawMessage) error

type ActionsManager struct {
	executors map[models.ActionType]ActionExecutor
}

func NewActionsManager() *ActionsManager {
	return &ActionsManager{
		executors: make(map[models.ActionType]ActionExecutor),
	}
}

func (am *ActionsManager) Register(actionType models.ActionType, executor ActionExecutor) {
	am.executors[actionType] = executor
}

func (am *ActionsManager) Execute(ctx context.Context, event *models.Event) error {
	if event.Action == nil {
		return errors.New("event action is nil")
	}
	exec, ok := am.executors[event.Action.Type]
	if !ok {
		return fmt.Errorf("no executor registered for action type: %s", event.Action.Type)
	}
	return exec(ctx, event, event.Action.Params)
}
