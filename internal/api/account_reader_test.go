package api

import (
	"testing"
	"vergex/internal/store"
	"vergex/internal/store/storetest"
)

func TestAccountInspectionDoesNotNeedAutopilotOrModel(t *testing.T) {
	st := storetest.New(t)
	db := st.GormDB()
	alice, bob := storetest.Account(t, st), storetest.OtherAccount(t, st)
	const (
		aliceAccount      = "00000000-0000-4000-8000-0000000000b1"
		bobAccount        = "00000000-0000-4000-8000-0000000000b2"
		standaloneAccount = "00000000-0000-4000-8000-0000000000b3"
	)
	for _, value := range []any{
		&store.Exchange{ID: aliceAccount, UserID: alice, ExchangeType: "binance"},
		&store.Exchange{ID: bobAccount, UserID: bob, ExchangeType: "binance"},
		&store.Exchange{ID: standaloneAccount, UserID: alice, ExchangeType: "binance"},
	} {
		if err := db.Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	const (
		aliceTrader  = "00000000-0000-4000-8000-0000000000a1"
		bobTrader    = "00000000-0000-4000-8000-0000000000a2"
		badReference = "00000000-0000-4000-8000-0000000000a3"
	)
	storetest.AddTrader(t, st, &store.Trader{ID: aliceTrader, UserID: alice, ExchangeID: aliceAccount})
	storetest.AddTrader(t, st, &store.Trader{ID: bobTrader, UserID: bob, ExchangeID: bobAccount})
	// Even a corrupt cross-user exchange reference cannot grant access.
	storetest.AddTrader(t, st, &store.Trader{ID: badReference, UserID: alice, ExchangeID: bobAccount})
	s := &Server{store: st} // No autonomous trader manager, model or strategy.
	standalone, err := s.accountReadConfig(alice, "", standaloneAccount)
	if err != nil || standalone.ID != standaloneAccount {
		t.Fatalf("saved account required an Autopilot: %v %v", standalone, err)
	}
	for _, tc := range [][3]string{{alice, "", bobAccount}, {"", "", standaloneAccount}, {alice, aliceTrader, standaloneAccount}, {alice, "", "unknown"}} {
		if _, err := s.accountReadConfig(tc[0], tc[1], tc[2]); err == nil {
			t.Fatalf("direct account ownership bypass: %v", tc)
		}
	}
	ex, err := s.accountReadConfig(alice, aliceTrader)
	if err != nil || ex.ID != aliceAccount {
		t.Fatalf("own account: %v %v", ex, err)
	}
	for _, tc := range [][2]string{{alice, bobTrader}, {alice, badReference}, {"", aliceTrader}, {alice, "unknown"}} {
		if _, err := s.accountReadConfig(tc[0], tc[1]); err == nil {
			t.Fatalf("ownership bypass: %v", tc)
		}
	}
}
