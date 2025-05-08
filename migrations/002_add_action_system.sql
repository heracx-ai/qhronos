-- Add action system to events and archived_events

-- Add action column to events table
ALTER TABLE events
ADD COLUMN IF NOT EXISTS action JSONB;

-- Add action column to archived_events table
ALTER TABLE archived_events
ADD COLUMN IF NOT EXISTS action JSONB;

-- Update existing events to populate the action column
UPDATE events
SET action = CASE
    WHEN webhook LIKE 'q:%' THEN jsonb_build_object('type', 'websocket', 'params', jsonb_build_object('client_name', substring(webhook from 3)))
    ELSE jsonb_build_object('type', 'webhook', 'params', jsonb_build_object('url', webhook))
END
WHERE action IS NULL; -- Only update if not already populated

-- Update existing archived_events to populate the action column
UPDATE archived_events
SET action = CASE
    WHEN webhook LIKE 'q:%' THEN jsonb_build_object('type', 'websocket', 'params', jsonb_build_object('client_name', substring(webhook from 3)))
    ELSE jsonb_build_object('type', 'webhook', 'params', jsonb_build_object('url', webhook))
END
WHERE action IS NULL; -- Only update if not already populated

-- Add comments for the new column if desired (optional)
COMMENT ON COLUMN events.action IS 'Stores the generalized action (type and params) for the event';
COMMENT ON COLUMN archived_events.action IS 'Stores the generalized action (type and params) for the archived event'; 