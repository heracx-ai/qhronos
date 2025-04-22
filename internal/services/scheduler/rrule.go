package scheduler

import (
	"time"

	"github.com/teambition/rrule-go"
)

// ParseRRule parses an iCalendar recurrence rule string and returns an RRule instance
func ParseRRule(rruleStr string) (*rrule.RRule, error) {
	return rrule.StrToRRule(rruleStr)
}

// GetNextOccurrences calculates the next n occurrences based on the recurrence rule
func GetNextOccurrences(rruleStr string, startTime time.Time, count int) ([]time.Time, error) {
	rule, err := ParseRRule(rruleStr)
	if err != nil {
		return nil, err
	}

	// Set the start time for the rule
	rule.DTStart(startTime)

	// Get the next occurrences
	return rule.All(), nil
}

// ValidateRRule validates if a string is a valid iCalendar recurrence rule
func ValidateRRule(rruleStr string) error {
	_, err := ParseRRule(rruleStr)
	return err
}

// GetOccurrencesBetween calculates all occurrences between two dates
func GetOccurrencesBetween(rruleStr string, startTime, endTime time.Time) ([]time.Time, error) {
	rule, err := ParseRRule(rruleStr)
	if err != nil {
		return nil, err
	}

	// Set the start time for the rule
	rule.DTStart(startTime)

	// Get all occurrences between start and end time
	return rule.Between(startTime, endTime, true), nil
} 