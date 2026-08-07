BEGIN;

-- Per-room cost controls.
--
-- chat_max_tokens caps the length of a single response. Responses were previously unbounded, so a
-- runaway generation was billed in full.
--
-- chat_image_detail selects the image fidelity requested from the provider. The default of 'low'
-- costs a flat, small number of tokens per image, where the provider default tiles the image and
-- can run into thousands. 'auto' omits the field entirely, leaving the provider to decide.
ALTER TABLE room_config
    ADD COLUMN chat_max_tokens INTEGER,
    ADD COLUMN chat_image_detail TEXT;

COMMIT;
