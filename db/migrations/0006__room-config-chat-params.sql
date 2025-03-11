BEGIN;

-- Add columns for max history messages and max tool iterations to room_config table
ALTER TABLE room_config 
    ADD COLUMN chat_max_history_messages INTEGER,
    ADD COLUMN chat_max_tool_iterations INTEGER;

COMMIT; 
