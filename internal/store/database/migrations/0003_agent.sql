CREATE TABLE agent.models (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    name       text NOT NULL,
    provider   text NOT NULL,
    enabled    boolean NOT NULL DEFAULT false,
    api_key    text NOT NULL DEFAULT '',
    base_url   text NOT NULL DEFAULT '',
    model_name text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE agent.models IS 'The models a user can call, with their own API keys.';
COMMENT ON COLUMN agent.models.api_key IS 'Encrypted with DATA_ENCRYPTION_KEY.';
COMMENT ON COLUMN agent.models.base_url IS 'Custom endpoint; empty uses the provider''s own.';
COMMENT ON COLUMN agent.models.model_name IS 'Custom model; empty uses the provider''s default.';

CREATE TABLE agent.conversations (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES core.users(id) ON DELETE CASCADE,
    feature        text NOT NULL CHECK (feature IN ('trader', 'frontier', 'telegram')),
    source_id      text,
    agent_model_id uuid REFERENCES agent.models(id) ON DELETE SET NULL,
    title          text,
    context        jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_recent ON agent.conversations (user_id, feature, updated_at DESC);
COMMENT ON TABLE agent.conversations IS 'One exchange with a model: a Frontier session, a Telegram chat, a trader cycle.';
COMMENT ON COLUMN agent.conversations.source_id IS 'What it belongs to within the feature, such as a Telegram chat id. Not a foreign key, so features stay apart.';
COMMENT ON COLUMN agent.conversations.context IS 'What the feature keeps about it, such as the symbol and account.';

CREATE TABLE agent.messages (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    conversation_id uuid NOT NULL REFERENCES agent.conversations(id) ON DELETE CASCADE,
    seq             integer NOT NULL,
    role            text NOT NULL CHECK (role IN ('system', 'context', 'user', 'assistant', 'tool')),
    content         text NOT NULL DEFAULT '',
    thinking        text,
    tool_calls      jsonb,
    tool_call_id    text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (conversation_id, seq)
);
COMMENT ON TABLE agent.messages IS 'Everything said in a conversation, in the order it happened.';
COMMENT ON COLUMN agent.messages.role IS 'context is data sent with a turn that the user did not type, kept apart from their words.';
COMMENT ON COLUMN agent.messages.thinking IS 'The chain of thought, when the provider returns one.';

CREATE TABLE agent.calls (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feature              text NOT NULL CHECK (feature IN ('trader', 'frontier', 'telegram', 'test')),
    source_id            text,
    conversation_id      uuid REFERENCES agent.conversations(id) ON DELETE SET NULL,
    provider             text NOT NULL,
    model                text NOT NULL,
    host                 text NOT NULL DEFAULT '',
    prompt_tokens        integer NOT NULL DEFAULT 0,
    cached_prompt_tokens integer NOT NULL DEFAULT 0,
    completion_tokens    integer NOT NULL DEFAULT 0,
    cost_usd             numeric NOT NULL DEFAULT 0,
    unpriced             boolean NOT NULL DEFAULT false,
    duration_ms          integer NOT NULL DEFAULT 0,
    status               text NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'failed')),
    error                text,
    finish_reason        text,
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX calls_created_at ON agent.calls (created_at);
CREATE INDEX calls_source ON agent.calls (feature, source_id, created_at);
COMMENT ON TABLE agent.calls IS 'Each request to a model, failed ones included: its tokens, cost and time.';
COMMENT ON COLUMN agent.calls.prompt_tokens IS 'Includes cached_prompt_tokens.';
COMMENT ON COLUMN agent.calls.cost_usd IS 'Estimated from models.dev prices.';
COMMENT ON COLUMN agent.calls.unpriced IS 'The model has no known price, so cost_usd is 0 and only the tokens count.';
