package config

import (
	"testing"
	"time"
)

func TestParseFlexibleDuration(t *testing.T) {
	tests := []struct {
		input   string
		expect  time.Duration
		wantErr bool
	}{
		{"1h", time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"2d", 48 * time.Hour, false},
		{"1w", 168 * time.Hour, false},
		{"0d", 0, false},
		{"0w", 0, false},
		{"5x", 0, true}, // unsupported unit
		{"", 0, true},   // empty string
	}

	for _, tt := range tests {
		dur, err := ParseFlexibleDuration(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("input %q: expected error=%v, got %v", tt.input, tt.wantErr, err)
		}
		if err == nil && dur != tt.expect {
			t.Errorf("input %q: expected %v, got %v", tt.input, tt.expect, dur)
		}
	}
}

func TestParseRetentionDurations(t *testing.T) {
	cfg := &Config{
		Retention: RetentionConfig{
			Events:          "30d",
			Occurrences:     "7d",
			CleanupInterval: "1h",
		},
	}
	durations, err := cfg.ParseRetentionDurations()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if durations.Events != 30*24*time.Hour {
		t.Errorf("expected Events=720h, got %v", durations.Events)
	}
	if durations.Occurrences != 7*24*time.Hour {
		t.Errorf("expected Occurrences=168h, got %v", durations.Occurrences)
	}
	if durations.CleanupInterval != time.Hour {
		t.Errorf("expected CleanupInterval=1h, got %v", durations.CleanupInterval)
	}

	// Test invalid config
	cfgBad := &Config{
		Retention: RetentionConfig{
			Events:          "bad",
			Occurrences:     "7d",
			CleanupInterval: "1h",
		},
	}
	_, err = cfgBad.ParseRetentionDurations()
	if err == nil {
		t.Error("expected error for invalid Events duration, got nil")
	}
}
