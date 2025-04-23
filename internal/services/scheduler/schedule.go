package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	// Default maximum look-ahead period for unbounded rules
	defaultMaxLookAhead = 365 * 24 * time.Hour
	// Maximum occurrences to prevent infinite loops
	maxOccurrences = 1000
)

// Frequency represents the recurrence frequency
type Frequency string

const (
	FrequencyDaily   Frequency = "daily"
	FrequencyWeekly  Frequency = "weekly"
	FrequencyMonthly Frequency = "monthly"
	FrequencyYearly  Frequency = "yearly"
)

// DayOfWeek represents a day of the week
type DayOfWeek string

const (
	DayMonday    DayOfWeek = "MO"
	DayTuesday   DayOfWeek = "TU"
	DayWednesday DayOfWeek = "WE"
	DayThursday  DayOfWeek = "TH"
	DayFriday    DayOfWeek = "FR"
	DaySaturday  DayOfWeek = "SA"
	DaySunday    DayOfWeek = "SU"
)

// Schedule represents a JSON-based recurrence schedule
type Schedule struct {
	Frequency    Frequency   `json:"frequency"`
	Interval     int         `json:"interval"`
	DaysOfWeek   []DayOfWeek `json:"days_of_week,omitempty"`
	DaysOfMonth  []int       `json:"days_of_month,omitempty"`
	Months       []int       `json:"months,omitempty"`
	Count        *int        `json:"count,omitempty"`
	Until        *time.Time  `json:"until,omitempty"`
}

// ParseSchedule parses a JSON schedule string and returns a Schedule instance
func ParseSchedule(scheduleStr string) (*Schedule, error) {
	log.Printf("Parsing Schedule: %s", scheduleStr)
	if scheduleStr == "" {
		log.Printf("Empty Schedule provided")
		return nil, fmt.Errorf("empty schedule")
	}

	var schedule Schedule
	if err := json.Unmarshal([]byte(scheduleStr), &schedule); err != nil {
		log.Printf("Error parsing Schedule: %v", err)
		return nil, fmt.Errorf("invalid schedule format: %w", err)
	}

	if err := schedule.Validate(); err != nil {
		return nil, err
	}

	log.Printf("Successfully parsed Schedule: %s", scheduleStr)
	return &schedule, nil
}

// Validate checks if the schedule is valid
func (s *Schedule) Validate() error {
	// Validate frequency
	switch s.Frequency {
	case FrequencyDaily, FrequencyWeekly, FrequencyMonthly, FrequencyYearly:
	default:
		return fmt.Errorf("invalid frequency: %s", s.Frequency)
	}

	// Validate interval
	if s.Interval < 1 {
		return fmt.Errorf("interval must be positive")
	}

	// Validate days of week
	if len(s.DaysOfWeek) > 0 {
		validDays := map[DayOfWeek]bool{
			DayMonday: true, DayTuesday: true, DayWednesday: true,
			DayThursday: true, DayFriday: true, DaySaturday: true, DaySunday: true,
		}
		for _, day := range s.DaysOfWeek {
			if !validDays[day] {
				return fmt.Errorf("invalid day of week: %s", day)
			}
		}
	}

	// Validate days of month
	for _, day := range s.DaysOfMonth {
		if day < 1 || day > 31 {
			return fmt.Errorf("invalid day of month: %d", day)
		}
	}

	// Validate months
	for _, month := range s.Months {
		if month < 1 || month > 12 {
			return fmt.Errorf("invalid month: %d", month)
		}
	}

	// Validate count
	if s.Count != nil && *s.Count < 1 {
		return fmt.Errorf("count must be positive")
	}

	return nil
}

// GetNextOccurrences calculates the next occurrences based on the schedule
func (s *Schedule) GetNextOccurrences(startTime time.Time, endTime time.Time) ([]time.Time, error) {
	log.Printf("Getting next occurrences from %v to %v with schedule: %+v", 
		startTime.UTC(), endTime.UTC(), s)

	// Ensure times are in UTC
	startTimeUTC := startTime.UTC()
	endTimeUTC := endTime.UTC()

	occurrences := make([]time.Time, 0)
	current := startTimeUTC
	count := 0

	for current.Before(endTimeUTC) && count < maxOccurrences {
		if s.Count != nil && count >= *s.Count {
			break
		}

		if s.Until != nil && current.After(*s.Until) {
			break
		}

		if s.isValidOccurrence(current) {
			occurrences = append(occurrences, current)
			count++
		}

		// Move to next interval
		switch s.Frequency {
		case FrequencyDaily:
			current = current.AddDate(0, 0, s.Interval)
		case FrequencyWeekly:
			current = current.AddDate(0, 0, 7*s.Interval)
		case FrequencyMonthly:
			current = current.AddDate(0, s.Interval, 0)
		case FrequencyYearly:
			current = current.AddDate(s.Interval, 0, 0)
		}
	}

	log.Printf("Found %d occurrences between %v and %v", len(occurrences), startTimeUTC, endTimeUTC)
	return occurrences, nil
}

// isValidOccurrence checks if a given time is valid according to the schedule rules
func (s *Schedule) isValidOccurrence(t time.Time) bool {
	// Check days of week
	if len(s.DaysOfWeek) > 0 {
		dayOfWeek := t.Weekday()
		validDay := false
		for _, day := range s.DaysOfWeek {
			if day == DayOfWeek(dayOfWeek.String()[:2]) {
				validDay = true
				break
			}
		}
		if !validDay {
			return false
		}
	}

	// Check days of month
	if len(s.DaysOfMonth) > 0 {
		dayOfMonth := t.Day()
		validDay := false
		for _, day := range s.DaysOfMonth {
			if day == dayOfMonth {
				validDay = true
				break
			}
		}
		if !validDay {
			return false
		}
	}

	// Check months
	if len(s.Months) > 0 {
		month := int(t.Month())
		validMonth := false
		for _, m := range s.Months {
			if m == month {
				validMonth = true
				break
			}
		}
		if !validMonth {
			return false
		}
	}

	return true
} 