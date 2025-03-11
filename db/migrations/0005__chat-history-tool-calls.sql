BEGIN;

-- Add message_type column to chat_history table
ALTER TABLE chat_history ADD COLUMN message_type TEXT NOT NULL DEFAULT 'text';

-- Add tool_call_id column to chat_history table (can be NULL for regular messages)
ALTER TABLE chat_history ADD COLUMN tool_call_id TEXT;

-- Add tool_name column to chat_history table (can be NULL for regular messages)
ALTER TABLE chat_history ADD COLUMN tool_name TEXT;

-- Add index for faster lookups by tool_call_id
CREATE INDEX chat_history_tool_call_idx ON chat_history (tool_call_id);

COMMIT; 
