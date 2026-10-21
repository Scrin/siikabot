BEGIN;

-- The images a user turn showed the model, kept so that the turn shows them again for as long as it
-- is in the context window: the message's own image, and those of the messages it replies to or
-- links to. They go with the turn, and the image of a message that is redacted goes at once.
--
-- They are stored as prepared for the model: scaled down and re-encoded, without the original's
-- metadata. A turn shows its images in the order its text refers to them, which the turn itself
-- gives, so there is no order to keep here.
--
-- The history is not emptied for this. A turn stored before has no images here, which is what a
-- turn whose image couldn't be downloaded looks like as well, and it says so the same way.
CREATE TABLE chat_images (
    history_id BIGINT NOT NULL REFERENCES chat_history (id) ON DELETE CASCADE,
    event_id TEXT NOT NULL,             -- the message the image is from: the turn's own, or one it refers to
    mime_type TEXT NOT NULL CHECK (mime_type IN ('image/jpeg', 'image/png')),
    width INTEGER NOT NULL CHECK (width > 0),
    height INTEGER NOT NULL CHECK (height > 0),
    data BYTEA NOT NULL,
    PRIMARY KEY (history_id, event_id)
);

-- Redactions look images up by the message they are from
CREATE INDEX chat_images_event_idx ON chat_images (event_id);

-- Whether a turn's message had an image told which model answered it. There is one model now, and
-- with images staying in the context, it no longer says what a turn cost either. Nothing reads it.
ALTER TABLE chat_usage DROP COLUMN has_image;

COMMIT;
