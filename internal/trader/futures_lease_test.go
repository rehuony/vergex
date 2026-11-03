package trader

import (
	"testing"
	"vergex/internal/account"
	"vergex/internal/database/testdb"
)

func TestFuturesLeaseBlocksConstructorAndRestoration(t *testing.T) {
	st := NewStores(testdb.Open(t))
	db := st.DB
	user := testdb.AddUser(t, st.DB)
	ex := account.Account{UserID: user, Venue: "binance", APIKey: "owner-key"}
	if e := db.Create(&ex).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Create(&account.Lease{AccountID: ex.ID, Source: account.SourceFutures, SourceID: "locked"}).Error; e != nil {
		t.Fatal(e)
	}
	if _, e := NewAutoTrader(AutoTraderConfig{AccountID: ex.ID}, st, user); e == nil {
		t.Fatal("constructor escaped unresolved lease")
	}
	at := &AutoTrader{stores: st, userID: user, accountID: ex.ID}
	if e := at.Run(); e == nil {
		t.Fatal("restoration escaped unresolved lease")
	}
	if at.isRunning {
		t.Fatal("marked runtime active on rejected launch")
	}
}
