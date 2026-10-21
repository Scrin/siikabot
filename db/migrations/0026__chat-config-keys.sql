BEGIN;

-- Each chat setting goes by one name everywhere: in !chat config and !chat set, in the logs, and in
-- this table. Three columns went by names of their own, which nothing else used.
--
-- max_tokens read as a limit on the context as easily as on an answer, and max_web_content_size as a
-- limit on how much of a page is read, where it is how much of a page one get_web_content call
-- returns, a longer page coming in parts. The rows keep their values, so the history of each setting
-- stays as it was. The checks are renamed with their columns, as 0024 did.
ALTER TABLE chat_config RENAME COLUMN llm_model TO model;
ALTER TABLE chat_config RENAME CONSTRAINT chat_config_llm_model_check TO chat_config_model_check;
ALTER TABLE chat_config RENAME COLUMN max_tokens TO max_response_tokens;
ALTER TABLE chat_config RENAME CONSTRAINT chat_config_max_tokens_check TO chat_config_max_response_tokens_check;
ALTER TABLE chat_config RENAME COLUMN max_web_content_size TO web_part_bytes;
ALTER TABLE chat_config RENAME CONSTRAINT chat_config_max_web_content_size_check TO chat_config_web_part_bytes_check;

COMMIT;
