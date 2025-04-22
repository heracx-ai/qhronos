-- Enable pg_cron extension for scheduled tasks
CREATE EXTENSION IF NOT EXISTS pg_cron;

-- Create a scheduled job to run the archiving function
SELECT cron.schedule('0 2 * * *', 'SELECT archive_old_data()'); 