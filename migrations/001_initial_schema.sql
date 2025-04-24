-- Initial Schema: Insert-only occurrences table and all required tables

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Configuration Tables

-- System configuration including retention policies
CREATE TABLE system_config (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT UNIQUE NOT NULL,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ DEFAULT now(),
    updated_by TEXT NOT NULL
);

-- Insert default retention policies
INSERT INTO system_config (key, value, description, updated_by) VALUES
('retention_policies', '{
    "logs": {
        "webhook_attempts": "30d",
        "api_requests": "90d",
        "error_logs": "180d",
        "performance_metrics": "365d"
    },
    "events": {
        "max_future_scheduling": "365d",
        "max_past_occurrences": "30d",
        "archived_events": "5y"
    },
    "analytics": {
        "hourly_metrics": "30d",
        "daily_metrics": "365d",
        "monthly_metrics": "5y"
    }
}', 'Data retention policies in days (d) or years (y)', 'system');

-- Core Tables with Retention Policies

-- Events table with scheduling constraints
CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    description TEXT,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    webhook TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    schedule JSONB,
    tags TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    hmac_secret TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE
);

-- Add comments to events table
COMMENT ON COLUMN events.name IS 'Event name';
COMMENT ON COLUMN events.description IS 'Event description';
COMMENT ON COLUMN events.schedule IS 'JSON schedule format with frequency, interval, and optional constraints';
COMMENT ON COLUMN events.metadata IS 'Event metadata in JSONB format';
COMMENT ON COLUMN events.hmac_secret IS 'Custom HMAC secret for webhook signing. If null, uses default secret.';

-- Occurrences table (insert-only, append-only)

CREATE TABLE occurrences (
    id SERIAL PRIMARY KEY,
    occurrence_id UUID NOT NULL,
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status_code INT,
    response_body TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_occurrences_occurrence_id ON occurrences(occurrence_id);
CREATE INDEX idx_occurrences_event_id ON occurrences(event_id);
CREATE INDEX idx_occurrences_scheduled_at ON occurrences(scheduled_at);
CREATE INDEX idx_occurrences_status ON occurrences(status);

-- Analytics Tables with Retention Periods

-- Daily aggregates with retention
CREATE TABLE analytics_daily (
    date DATE PRIMARY KEY,
    total_events INT NOT NULL DEFAULT 0,
    active_events INT NOT NULL DEFAULT 0,
    total_occurrences INT NOT NULL DEFAULT 0,
    scheduled_occurrences INT NOT NULL DEFAULT 0,
    dispatched_occurrences INT NOT NULL DEFAULT 0,
    failed_occurrences INT NOT NULL DEFAULT 0,
    webhook_attempts INT NOT NULL DEFAULT 0,
    successful_webhooks INT NOT NULL DEFAULT 0,
    failed_webhooks INT NOT NULL DEFAULT 0,
    avg_response_time_ms FLOAT,
    last_updated TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT valid_analytics_period CHECK (
        -- Keep daily analytics for 1 year
        date >= (current_date - interval '365 days')
    )
);

-- Hourly aggregates with retention
CREATE TABLE analytics_hourly (
    timestamp TIMESTAMPTZ PRIMARY KEY,
    event_count INT NOT NULL DEFAULT 0,
    occurrence_count INT NOT NULL DEFAULT 0,
    webhook_success_rate FLOAT,
    avg_processing_time_ms FLOAT,
    last_updated TIMESTAMPTZ DEFAULT now(),
    CONSTRAINT valid_hourly_period CHECK (
        -- Keep hourly analytics for 30 days
        timestamp >= (now() - interval '30 days')
    )
);

-- Performance metrics with retention
CREATE TABLE performance_metrics (
    timestamp TIMESTAMPTZ PRIMARY KEY,
    redis_memory_usage_mb FLOAT,
    redis_ops_per_second FLOAT,
    redis_latency_ms FLOAT,
    postgres_connections INT,
    postgres_queries_per_second FLOAT,
    postgres_avg_query_time_ms FLOAT,
    scheduler_events_per_second FLOAT,
    scheduler_avg_processing_time_ms FLOAT,
    dispatcher_webhooks_per_second FLOAT,
    dispatcher_avg_time_ms FLOAT,
    CONSTRAINT valid_metrics_period CHECK (
        -- Keep performance metrics for 1 year
        timestamp >= (now() - interval '365 days')
    )
);

-- Archive Tables for Long-term Storage

-- Archived events
CREATE TABLE archived_events (
    id SERIAL PRIMARY KEY,
    event_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    webhook TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    schedule JSONB,
    tags TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    hmac_secret TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL
);

-- Archived occurrences
CREATE TABLE archived_occurrences (
    id SERIAL PRIMARY KEY,
    occurrence_id UUID NOT NULL,
    event_id UUID NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    status_code INT,
    response_body TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL
);

-- Create Indexes

-- Events indexes
CREATE INDEX idx_events_status ON events(status);
CREATE INDEX idx_events_tags ON events USING GIN(tags);
CREATE INDEX idx_events_created_at ON events(created_at);
CREATE INDEX idx_events_schedule ON events USING GIN(schedule);
CREATE INDEX idx_events_metadata ON events USING GIN(metadata);

-- Analytics indexes
CREATE INDEX idx_analytics_daily_date ON analytics_daily(date);
CREATE INDEX idx_analytics_hourly_timestamp ON analytics_hourly(timestamp);
CREATE INDEX idx_performance_metrics_timestamp ON performance_metrics(timestamp);

-- Data Retention Management Functions

-- Function to archive old data
CREATE OR REPLACE FUNCTION archive_old_data(event_retention INTERVAL)
RETURNS void AS $$
BEGIN
    -- Archive old occurrences (do not specify SERIAL id)
    INSERT INTO archived_occurrences (
        occurrence_id, event_id, scheduled_at, status, attempt_count,
        timestamp, status_code, response_body, error_message,
        started_at, completed_at, archived_at
    )
    SELECT 
        occurrence_id, event_id, scheduled_at, status, attempt_count,
        timestamp, status_code, response_body, error_message,
        started_at, completed_at, now()
    FROM occurrences
    WHERE scheduled_at < (now() - event_retention);

    -- Archive events that have no active occurrences (do not specify SERIAL id)
    INSERT INTO archived_events (
        event_id, name, description, start_time, webhook, metadata,
        schedule, tags, status, hmac_secret, created_at, updated_at, archived_at
    )
    SELECT 
        id, name, description, start_time, webhook, metadata,
        schedule, tags, status, hmac_secret, created_at, updated_at, now()
    FROM events e
    WHERE NOT EXISTS (
        SELECT 1 FROM occurrences o 
        WHERE o.event_id = e.id 
        AND o.scheduled_at >= (now() - event_retention)
    );

    DELETE FROM occurrences
    WHERE scheduled_at < (now() - event_retention);

    DELETE FROM events e
    WHERE NOT EXISTS (
        SELECT 1 FROM occurrences o 
        WHERE o.event_id = e.id 
        AND o.scheduled_at >= (now() - event_retention)
    );
END;
$$ LANGUAGE plpgsql; 