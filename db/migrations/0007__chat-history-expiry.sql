BEGIN;

-- Add expiry column to chat_history table
ALTER TABLE chat_history ADD COLUMN expiry TIMESTAMPTZ;

COMMIT; 
