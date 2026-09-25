BEGIN;

-- Memories are scoped to where they were saved. A memory saved in a group room is used in that
-- room and in DMs; one saved in a DM only in DMs, so something said in private never surfaces in
-- front of a group.
--
-- room_id is the group room a memory was saved in, and NULL means it was saved in a DM. Existing
-- memories get NULL, which makes them DM-only: they were saved without any notion of scope, so
-- keeping them private is the safe reading. Unlike the chat history they are kept, since people
-- save them on purpose.
ALTER TABLE user_memory ADD COLUMN room_id TEXT;

COMMIT;
