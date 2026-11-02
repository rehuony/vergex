ALTER TABLE account.accounts ADD COLUMN testnet boolean NOT NULL DEFAULT false;
COMMENT ON COLUMN account.accounts.testnet IS 'Trades on the venue''s test environment, with test money. Set when created and never changes, so one account''s ledger never mixes the two.';
