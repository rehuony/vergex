package trader

import (
	"testing"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestPlanningLeaseBlocksConstructorAndRestoration(t *testing.T) {
	st := storetest.New(t)
	db := st.GormDB()
	user := storetest.Account(t, st)
	ex := store.Exchange{UserID: user, ExchangeType: "binance", APIKey: "owner-key"}
	if e := db.Create(&ex).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Create(&store.AccountLease{AccountID: ex.ID, Holder: store.LeaseHolderFrontier, HolderID: "locked"}).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := NewAutoTrader(AutoTraderConfig{ExchangeID: ex.ID}, st, user); e == nil {
		t.Fatal("constructor escaped unresolved lease")
	}
	at := &AutoTrader{store: st, userID: user, exchangeID: ex.ID}
	if e := at.Run(); e == nil {
		t.Fatal("restoration escaped unresolved lease")
	}
	if at.isRunning {
		t.Fatal("marked runtime active on rejected launch")
	}
}
