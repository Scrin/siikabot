BEGIN;

-- What it takes to find out afterwards why the bot remembers what it does about a room.
--
-- Each memory records the trace of the pass that formed it and of the one that last revised it. The
-- trace ID also finds the pass's logs, which are kept far longer than the traces. NULL means it wasn't
-- recorded: the memories formed before this have none, and neither does a pass run without tracing.
ALTER TABLE room_memory
    ADD COLUMN created_trace_id TEXT,
    ADD COLUMN updated_trace_id TEXT;

-- Each room records how its latest pass went, such as unchanged or request_failed, and the pass's
-- trace, so the dashboard can say why a room's memories are as they are. NULL means no pass has
-- recorded it yet.
ALTER TABLE room_memory_progress
    ADD COLUMN last_outcome TEXT,
    ADD COLUMN last_trace_id TEXT;

COMMIT;
