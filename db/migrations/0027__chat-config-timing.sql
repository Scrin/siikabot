BEGIN;

-- How long a turn may take, and each model call in it, become chat settings. They were constants in
-- the code: a turn had 3 minutes, and a model call three attempts of 45 seconds each.
--
-- The existing rows get those constants, since they are what was in effect when each row was the
-- configuration, and the defaults then go: like every other column, a new row has to give a value.
--
-- A model call with every attempt it may get has to fit in the turn, or its retries could never run.
-- The code checks that with the waits between attempts as well, which this check leaves out, so the
-- check here is only a backstop for rows written by hand.
ALTER TABLE chat_config
    ADD COLUMN turn_timeout INTERVAL NOT NULL DEFAULT '3 minutes' CHECK (turn_timeout > INTERVAL '0'),
    ADD COLUMN model_timeout INTERVAL NOT NULL DEFAULT '45 seconds' CHECK (model_timeout > INTERVAL '0'),
    ADD COLUMN model_attempts INTEGER NOT NULL DEFAULT 3 CHECK (model_attempts > 0),
    ADD CONSTRAINT chat_config_model_call_fits_turn CHECK (model_timeout * model_attempts < turn_timeout);

ALTER TABLE chat_config
    ALTER COLUMN turn_timeout DROP DEFAULT,
    ALTER COLUMN model_timeout DROP DEFAULT,
    ALTER COLUMN model_attempts DROP DEFAULT;

COMMIT;
