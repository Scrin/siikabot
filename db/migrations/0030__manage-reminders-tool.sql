BEGIN;

-- create_reminder became the create action of the manage_reminders tool, which also lists and
-- deletes reminders. Stored turns call the old tool, so every room starts over, exactly as if
-- !chat reset had been run everywhere, rather than the code handling calls to a tool that no longer
-- exists. The tables that hang off chat_history go with it, since PostgreSQL won't truncate a table
-- others reference on its own. Reminders, memories and chat_usage are kept.
TRUNCATE chat_history, chat_references, chat_images, chat_context_anchors;

COMMIT;
