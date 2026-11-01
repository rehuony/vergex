CREATE TABLE telegram.bot (
    id             smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    bot_token      text NOT NULL DEFAULT '',
    chat_id        bigint NOT NULL DEFAULT 0,
    username       text NOT NULL DEFAULT '',
    bound_at       timestamptz,
    agent_model_id uuid REFERENCES agent.models(id) ON DELETE SET NULL,
    language       text NOT NULL DEFAULT '',
    muted_alerts   text[] NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE telegram.bot IS 'The Telegram bot and the chat bound to it; a single row.';
COMMENT ON COLUMN telegram.bot.bot_token IS 'Encrypted with DATA_ENCRYPTION_KEY.';
COMMENT ON COLUMN telegram.bot.chat_id IS 'The bound chat; 0 means none.';
COMMENT ON COLUMN telegram.bot.agent_model_id IS 'The model for Telegram replies; NULL uses the first enabled one.';
COMMENT ON COLUMN telegram.bot.muted_alerts IS 'Alert kinds turned off. Anything not listed is on, so a new kind starts on.';

CREATE TABLE telegram.alert_marks (
    key text PRIMARY KEY,
    at  timestamptz NOT NULL
);
COMMENT ON TABLE telegram.alert_marks IS 'What the alert watcher has already told the user, so a restart repeats nothing.';
