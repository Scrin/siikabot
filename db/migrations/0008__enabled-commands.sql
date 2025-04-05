BEGIN;

ALTER TABLE room_config ADD COLUMN enabled_commands TEXT[] DEFAULT '{}';

COMMIT; 
