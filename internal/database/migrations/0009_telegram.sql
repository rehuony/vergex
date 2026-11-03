CREATE TABLE telegram.bot (
    id             smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    bot_token      text NOT NULL DEFAULT '',
    chat_id        bigint NOT NULL DEFAULT 0,
    username       text NOT NULL DEFAULT '',
    bound_at       timestamptz,
    agent_model_id uuid REFERENCES agent.models (id) ON DELETE SET NULL,
    language       text NOT NULL DEFAULT '',
    muted_alerts   text[] NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE telegram.bot IS 'The Telegram bot and the chat bound to it; a single row.';
COMMENT ON COLUMN telegram.bot.bot_token IS 'Encrypted with DATA_ENCRYPTION_KEY.';
COMMENT ON COLUMN telegram.bot.chat_id IS 'The bound chat; 0 means none.';
COMMENT ON COLUMN telegram.bot.username IS 'The bound user''s @username, or empty when they have none.';
COMMENT ON COLUMN telegram.bot.agent_model_id IS 'The model for Telegram replies; NULL uses the first enabled one.';
COMMENT ON COLUMN telegram.bot.muted_alerts IS 'Alert kinds turned off. Anything not listed is on, so a new kind starts on.';

CREATE TABLE telegram.alert_marks (
    name      text PRIMARY KEY,
    marked_at timestamptz NOT NULL
);
COMMENT ON TABLE telegram.alert_marks IS 'What the alert watcher has already told the user, so a restart repeats nothing: a time for a stream of events, or that a standing problem was reported.';

CREATE TABLE telegram.visitors (
    chat_id    bigint NOT NULL,
    sender_id  bigint NOT NULL,
    chat_type  text NOT NULL,
    chat_title text NOT NULL DEFAULT '',
    username   text NOT NULL DEFAULT '',
    name       text NOT NULL DEFAULT '',
    attempts   integer NOT NULL DEFAULT 1,
    last_text  text NOT NULL DEFAULT '',
    first_at   timestamptz NOT NULL,
    last_at    timestamptz NOT NULL,
    PRIMARY KEY (chat_id, sender_id)
);
CREATE INDEX visitors_last_at ON telegram.visitors (last_at DESC);
COMMENT ON TABLE telegram.visitors IS 'Who tried to use the bot and was refused: one row per person per chat, the newest 500 kept.';
COMMENT ON COLUMN telegram.visitors.sender_id IS 'The Telegram user who sent it; 0 when Telegram names none, as in a channel.';
COMMENT ON COLUMN telegram.visitors.last_text IS 'The start of their last message, without whatever followed /start.';
