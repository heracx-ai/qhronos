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
    webhook_url TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    schedule TEXT,
    tags TEXT[] NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'active',
    hmac_secret TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE
);

-- Add comments to events table
COMMENT ON COLUMN events.name IS 'Event name';
COMMENT ON COLUMN events.description IS 'Event description';
COMMENT ON COLUMN events.schedule IS 'iCalendar (RFC 5545) recurrence rule';
COMMENT ON COLUMN events.metadata IS 'Event metadata in JSONB format';
COMMENT ON COLUMN events.hmac_secret IS 'Custom HMAC secret for webhook signing. If null, uses default secret.';

-- Occurrences table with retention tracking
CREATE TABLE occurrences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id UUID NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    scheduled_at TIMESTAMP WITH TIME ZONE NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    last_attempt TIMESTAMP WITH TIME ZONE,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Add comment to occurrences table
COMMENT ON COLUMN occurrences.updated_at IS 'Last update timestamp of the occurrence';

-- Create trigger function to update updated_at column
CREATE OR REPLACE FUNCTION update_occurrences_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to call the function before update
CREATE TRIGGER update_occurrences_updated_at_trigger
    BEFORE UPDATE ON occurrences
    FOR EACH ROW
    EXECUTE FUNCTION update_occurrences_updated_at();

-- Webhook delivery attempts with retention period
CREATE TABLE webhook_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    occurrence_id UUID REFERENCES occurrences(id) ON DELETE CASCADE,
    attempt_number INT NOT NULL,
    status_code INT,
    response_body TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT now(),
    archived_at TIMESTAMPTZ,
    CONSTRAINT valid_attempt_period CHECK (
        -- Ensure attempts are not kept beyond retention period
        created_at >= (now() - interval '30 days')
    )
);

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
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    webhook_url TEXT NOT NULL,
    metadata JSONB NOT NULL,
    schedule TEXT,
    tags TEXT[],
    status TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL,
    original_event_id UUID NOT NULL
);

-- Archived occurrences
CREATE TABLE archived_occurrences (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL,
    scheduled_at TIMESTAMPTZ NOT NULL,
    status TEXT,
    last_attempt TIMESTAMPTZ,
    attempt_count INT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL,
    original_occurrence_id UUID NOT NULL
);

-- Archived webhook attempts
CREATE TABLE archived_webhook_attempts (
    id UUID PRIMARY KEY,
    occurrence_id UUID NOT NULL,
    attempt_number INT NOT NULL,
    status_code INT,
    response_body TEXT,
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ NOT NULL,
    original_attempt_id UUID NOT NULL
);

-- Create Indexes

-- Events indexes
CREATE INDEX idx_events_status ON events(status);
CREATE INDEX idx_events_tags ON events USING GIN(tags);
CREATE INDEX idx_events_created_at ON events(created_at);
CREATE INDEX idx_events_schedule ON events(schedule);
CREATE INDEX idx_events_metadata ON events USING GIN(metadata);

-- Occurrences indexes
CREATE INDEX idx_occurrences_event_id ON occurrences(event_id);
CREATE INDEX idx_occurrences_scheduled_at ON occurrences(scheduled_at);
CREATE INDEX idx_occurrences_status ON occurrences(status);
CREATE INDEX idx_occurrences_scheduled_status ON occurrences(scheduled_at) WHERE status = 'scheduled';

-- Webhook attempts indexes
CREATE INDEX idx_webhook_attempts_occurrence_id ON webhook_attempts(occurrence_id);
CREATE INDEX idx_webhook_attempts_status_code ON webhook_attempts(status_code);

-- Analytics indexes
CREATE INDEX idx_analytics_daily_date ON analytics_daily(date);
CREATE INDEX idx_analytics_hourly_timestamp ON analytics_hourly(timestamp);
CREATE INDEX idx_performance_metrics_timestamp ON performance_metrics(timestamp);

-- Data Retention Management Functions

-- Function to archive old data
CREATE OR REPLACE FUNCTION archive_old_data()
RETURNS void AS $$
DECLARE
    retention_policies JSONB;
    webhook_retention INTERVAL;
    event_retention INTERVAL;
    analytics_retention INTERVAL;
BEGIN
    -- Get retention policies
    SELECT value INTO retention_policies
    FROM system_config
    WHERE key = 'retention_policies';

    -- Archive old webhook attempts
    webhook_retention := (retention_policies->'logs'->>'webhook_attempts')::INTERVAL;
    INSERT INTO archived_webhook_attempts
    SELECT 
        id, occurrence_id, attempt_number, status_code, 
        response_body, error_message, started_at, completed_at,
        created_at, now(), id
    FROM webhook_attempts
    WHERE created_at < (now() - webhook_retention);

    DELETE FROM webhook_attempts
    WHERE created_at < (now() - webhook_retention);

    -- Archive old events and occurrences
    event_retention := (retention_policies->'events'->>'max_past_occurrences')::INTERVAL;
    
    -- First archive occurrences
    INSERT INTO archived_occurrences
    SELECT 
        id, event_id, scheduled_at, status, last_attempt,
        attempt_count, created_at, updated_at, now(), id
    FROM occurrences
    WHERE scheduled_at < (now() - event_retention);

    -- Then archive events that have no active occurrences
    INSERT INTO archived_events
    SELECT 
        id, name, description, start_time, webhook_url, metadata,
        schedule, tags, status, created_at, updated_at, now(), id
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