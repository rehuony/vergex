package trader

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
	"vergex/internal/planning"
	"vergex/internal/planning/live"
	"vergex/internal/store"
)

func TestPlanningLeaseBlocksConstructorAndRestoration(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "guard.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if e = db.AutoMigrate(&store.Exchange{}, &live.Lease{}); e != nil {
		t.Fatal(e)
	}
	ex := store.Exchange{ID: "ex", UserID: "alice", ExchangeType: "binance", APIKey: "owner-key", Enabled: true}
	if e = db.Create(&ex).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&live.Lease{AccountKey: planning.AccountIdentity("binance", "owner-key"), RunID: "locked", UserID: "alice", ExchangeID: "ex"}).Error; e != nil {
		t.Fatal(e)
	}
	st, e := store.NewFromGorm(db)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = NewAutoTrader(AutoTraderConfig{ExchangeID: "ex"}, st, "alice"); e == nil {
		t.Fatal("constructor escaped unresolved lease")
	}
	at := &AutoTrader{store: st, userID: "alice", exchangeID: "ex"}
	if e = at.Run(); e == nil {
		t.Fatal("restoration escaped unresolved lease")
	}
	if at.isRunning {
		t.Fatal("marked runtime active on rejected launch")
	}
}
