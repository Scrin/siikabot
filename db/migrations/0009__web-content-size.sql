BEGIN;

-- Add column for max web content size to room_config table
ALTER TABLE room_config ADD COLUMN chat_max_web_content_size INTEGER;

COMMIT; 
