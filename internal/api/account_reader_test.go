package api

import (
	"testing"
	"vergex/internal/account"
	"vergex/internal/database/testdb"
)

func TestAccountInspectionDoesNotNeedAutopilotOrModel(t *testing.T) {
	st := NewStores(testdb.Open(t))
	db := st.DB
	alice, bob := testdb.AddUser(t, st.DB), testdb.AddOtherUser(t, st.DB)
	const (
		aliceAccount      = "00000000-0000-4000-8000-0000000000b1"
		bobAccount        = "00000000-0000-4000-8000-0000000000b2"
		standaloneAccount = "00000000-0000-4000-8000-0000000000b3"
	)
	for _, value := range []any{
		&account.Account{ID: aliceAccount, UserID: alice, Venue: "binance"},
		&account.Account{ID: bobAccount, UserID: bob, Venue: "binance"},
		&account.Account{ID: standaloneAccount, UserID: alice, Venue: "binance"},
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
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: aliceTrader, UserID: alice, AccountID: aliceAccount})
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: bobTrader, UserID: bob, AccountID: bobAccount})
	// Even a corrupt cross-user exchange reference cannot grant access.
	testdb.AddTrader(t, st.DB, &testdb.Trader{ID: badReference, UserID: alice, AccountID: bobAccount})
	s := &Server{stores: st} // No autonomous trader manager, model or strategy.
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
