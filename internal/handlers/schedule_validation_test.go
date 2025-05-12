package handlers

import (
	"testing"

	"github.com/feedloop/qhronos/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestIsValidSchedule(t *testing.T) {
	valid := []*models.ScheduleConfig{
		{Frequency: "minutely", Interval: 1},
		{Frequency: "hourly", Interval: 1},
		{Frequency: "daily", Interval: 1},
		{Frequency: "weekly", Interval: 1, ByDay: []string{"MO", "WE"}},
		{Frequency: "monthly", Interval: 1, ByMonthDay: []int{1, 15}},
		{Frequency: "yearly", Interval: 1, ByMonth: []int{1, 12}},
	}
	for _, sched := range valid {
		assert.True(t, isValidSchedule(sched), "should be valid: %+v", sched)
	}

	invalid := []*models.ScheduleConfig{
		{Frequency: "secondly", Interval: 1},
		{Frequency: "foo", Interval: 1},
		{Frequency: "minutely", Interval: 0},
		{Frequency: "weekly", Interval: 1, ByDay: []string{"XX"}},
		{Frequency: "monthly", Interval: 1, ByMonthDay: []int{0, 32}},
		{Frequency: "yearly", Interval: 1, ByMonth: []int{0, 13}},
	}
	for _, sched := range invalid {
		assert.False(t, isValidSchedule(sched), "should be invalid: %+v", sched)
	}
}
