CREATE TABLE core.users (
    id            uuid PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    display_name  text NOT NULL DEFAULT 'Admin',
    avatar        text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE core.users IS 'The account. The first one registered is the only one.';
COMMENT ON COLUMN core.users.avatar IS 'A small image as a data URL; empty shows the initial.';

CREATE TABLE core.settings (
    module     text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
COMMENT ON TABLE core.settings IS 'One row of settings per module: market, forecast, frontier.';

COMMENT ON TABLE core.schema_migrations IS 'Which of these migration files have been applied, and the checksum of each when it was.';
