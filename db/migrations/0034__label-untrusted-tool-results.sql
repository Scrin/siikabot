BEGIN;

-- dns_lookup and whois_lookup now label what they return as content from the web, get_web_content puts a
-- page's title and byline under that label, and a page it refuses to fetch shows the URL as the model wrote
-- it. Stored turns hold results in the old forms, so every room starts over, exactly as if !chat reset had
-- been run everywhere. The tables that hang off chat_history go with it, since PostgreSQL won't truncate a
-- table others reference on its own. Reminders, memories, the bot's memories of rooms, room_memory_progress
-- and chat_usage are kept.
TRUNCATE chat_history, chat_references, chat_images, chat_context_anchors;

COMMIT;
