package trader

import (
	"encoding/json"
	"testing"

	"vergex/internal/database/testdb"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"
)

func TestAnAgentTraderPicksUpAnEditedConfigAndKeepsTheOldOneIfItCannot(t *testing.T) {
	st := NewStores(testdb.Open(t))
	row := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "Mine"})

	saved, err := st.Traders.Get(row.UserID, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	config := kind.DefaultAgent()
	at := &AutoTrader{
		id: row.ID, userID: row.UserID, stores: st, engine: traderagent.NewEngine(config),
		config: AutoTraderConfig{ID: row.ID, Agent: config, ConfigRaw: string(saved.Config)},
	}

	before := at.engine
	if err := at.reloadConfigIfChanged(); err != nil {
		t.Fatal(err)
	}
	if at.engine != before {
		t.Fatal("an unchanged config should not rebuild the engine")
	}

	edited := kind.DefaultAgent()
	edited.RiskControl.MaxPositions = 2
	raw, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Traders.Update(&Trader{ID: row.ID, UserID: row.UserID, Name: row.Name, AgentModelID: row.AgentModelID, AccountID: row.AccountID, MarginMode: MarginCross, Config: raw}); err != nil {
		t.Fatal(err)
	}
	if err := at.reloadConfigIfChanged(); err != nil {
		t.Fatal(err)
	}
	if at.config.Agent.RiskControl.MaxPositions != 2 || at.engine.GetConfig().RiskControl.MaxPositions != 2 {
		t.Fatalf("not picked up: %+v", at.config.Agent.RiskControl)
	}

	if err := st.Traders.Update(&Trader{ID: row.ID, UserID: row.UserID, Name: row.Name, AgentModelID: row.AgentModelID, AccountID: row.AccountID, MarginMode: MarginCross, Config: []byte(`{"coin_source":{"source_type":"nonsense"}}`)}); err != nil {
		t.Fatal(err)
	}
	if err := at.reloadConfigIfChanged(); err == nil {
		t.Fatal("a config that cannot run should be reported")
	}
	if at.config.Agent.RiskControl.MaxPositions != 2 {
		t.Fatalf("a config that cannot run replaced the working one: %+v", at.config.Agent.RiskControl)
	}
}
