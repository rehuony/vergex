-- A model can be deleted once no trader in use runs on it: an archived trader gives it up,
-- which its CHECK allows, and one in use still holds it, since that CHECK refuses the NULL.
ALTER TABLE trader.traders DROP CONSTRAINT traders_agent_model_id_fkey;
ALTER TABLE trader.traders ADD CONSTRAINT traders_agent_model_id_fkey
    FOREIGN KEY (agent_model_id) REFERENCES agent.models(id) ON DELETE SET NULL;
