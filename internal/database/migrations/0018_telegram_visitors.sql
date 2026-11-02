CREATE TABLE telegram.visitors (
    chat_id    bigint NOT NULL,
    user_id    bigint NOT NULL,
    chat_type  text NOT NULL,
    chat_title text NOT NULL DEFAULT '',
    username   text NOT NULL DEFAULT '',
    name       text NOT NULL DEFAULT '',
    attempts   bigint NOT NULL DEFAULT 1,
    last_text  text NOT NULL DEFAULT '',
    first_at   timestamptz NOT NULL,
    last_at    timestamptz NOT NULL,
    PRIMARY KEY (chat_id, user_id)
);
CREATE INDEX visitors_last_at ON telegram.visitors (last_at DESC);
COMMENT ON TABLE telegram.visitors IS 'Who tried to use the bot and was refused: one row per person per chat, the newest 500 kept.';
COMMENT ON COLUMN telegram.visitors.user_id IS 'The sender; 0 when Telegram names none, as in a channel.';
COMMENT ON COLUMN telegram.visitors.last_text IS 'The start of their last message, without whatever followed /start.';
