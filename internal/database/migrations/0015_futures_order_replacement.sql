ALTER TABLE futures.run_orders ADD COLUMN replaces text NOT NULL DEFAULT '';
COMMENT ON COLUMN futures.run_orders.replaces IS 'On a stop or target whose price was changed after approval, the client_id of the order it takes over from. Empty otherwise.';
