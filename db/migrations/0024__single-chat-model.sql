BEGIN;

-- The chat uses one model for everything, which has to take images as well as text, so that an
-- image stays in the conversation after the turn it was sent in. The history of the image model
-- setting goes with its column.
--
-- No row is added: the model in effect is the text model of the latest row, as it was.
ALTER TABLE chat_config RENAME COLUMN text_model TO llm_model;
ALTER TABLE chat_config RENAME CONSTRAINT chat_config_text_model_check TO chat_config_llm_model_check;
ALTER TABLE chat_config DROP COLUMN image_model;

COMMIT;
