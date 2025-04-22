package models

import "errors"

var (
	ErrEventNotFound      = errors.New("event not found")
	ErrOccurrenceNotFound = errors.New("occurrence not found")
) 