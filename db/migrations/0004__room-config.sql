BEGIN;

-- Table for storing room-specific configuration
CREATE TABLE room_config (
    room_id TEXT PRIMARY KEY,
    chat_llm_model_text TEXT,
    chat_llm_model_image TEXT
);

COMMIT; 
