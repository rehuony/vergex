package run

import (
	"context"
	"testing"
	"time"
	"vergex/internal/account"
	"vergex/internal/futures"
	"vergex/internal/futures/plan"
)

func TestZeroFreeMarginAllowsDraftsButNeverApproval(t *testing.T) {
	for _, kind := range []string{"market", "limit", "ladder", "twap"} {
		t.Run(kind, func(t *testing.T) {
			controller, broker, market, base := setup(t, kind)
			snapshot := Snapshot{Equity: "259.47", Available: "0", FeeBPS: "5", Position: "0.01624", OpenOrders: 2}
			limits, err := DraftAccountLimits(snapshot, plan.Constraints{Leverage: 5, MinRewardRisk: "0", FeeBPS: "10", SlippageBPS: "10"})
			if err != nil || !limits.PlanningOnly || limits.AvailableMargin != "0" || limits.Equity != snapshot.Equity {
				t.Fatalf("zero free margin blocked planning or invented funds: %+v %v", limits, err)
			}
			preview, err := plan.Compile(market.book, limits, base.Idea, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			drafts := futures.PlanStore{DB: controller.DB, UserID: testUser, AccountID: testAccount, Venue: "binance"}
			if err = drafts.Add([]plan.Plan{preview}); err != nil {
				t.Fatal(err)
			}
			// Even after funding, this exact reference-size plan needs a fresh review.
			if err = CheckAccountCapacity(preview, broker.account); err == nil {
				t.Fatal("equity preview treated as a funded order")
			}
			if _, err = controller.Approve(context.Background(), testUser, preview.ID, preview.Hash, "APPROVE LIVE"); err == nil {
				t.Fatal("equity preview acquired live approval")
			}
			var leases int64
			controller.DB.Model(&account.Lease{}).Count(&leases)
			if leases != 0 || len(broker.placed) != 0 {
				t.Fatal("planning reserved the account or placed orders")
			}
			if _, err = AccountLimits(snapshot, limits); err == nil {
				t.Fatal("order review allowed without free margin")
			}
			snapshot.Available, snapshot.Position, snapshot.OpenOrders = "200", "0", 0
			funded, err := AccountLimits(snapshot, limits)
			if err != nil || funded.PlanningOnly || cmp(funded.AvailableMargin, "200") != 0 {
				t.Fatalf("fresh review did not use actual free margin: %+v %v", funded, err)
			}
		})
	}
}

func TestDraftSizingDoesNotMaskInvalidAccountData(t *testing.T) {
	base := plan.Constraints{Leverage: 5, MinRewardRisk: "0", FeeBPS: "10", SlippageBPS: "10"}
	for _, a := range []Snapshot{
		{Equity: "259", Available: "", FeeBPS: "5"},
		{Equity: "259", Available: "unknown", FeeBPS: "5"},
		{Equity: "0", Available: "0", FeeBPS: "5"},
		{Equity: "259", Available: "0", FeeBPS: ""},
	} {
		if _, err := DraftAccountLimits(a, base); err == nil {
			t.Fatalf("invalid account became a preview: %+v", a)
		}
	}
	for _, available := range []plan.Decimal{"0", "-2", "30"} {
		c, err := DraftAccountLimits(Snapshot{Equity: "259", Available: available, FeeBPS: "5"}, base)
		if err != nil || c.PlanningOnly != (available != "30") {
			t.Fatalf("unexpected planning mode: %+v %v", c, err)
		}
	}
}

func TestAccountSizingUsesFreeMarginAndLeverage(t *testing.T) {
	a := Snapshot{Equity: "100", Available: "40", FeeBPS: "12", Position: "0"}
	c, e := AccountLimits(a, plan.Constraints{Leverage: 5, MaxRiskPct: "0", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"})
	if e != nil {
		t.Fatal(e)
	}
	if cmp(c.MaxNotional, "195.694716242661") != 0 || cmp(c.MaxRisk, "100") != 0 || c.FeeBPS != "12" || cmp(c.AvailableMargin, "40") != 0 {
		t.Fatalf("incorrect automatic limits: %+v", c)
	}
	for _, leverage := range []int{-1, 0, 21} {
		c.Leverage = leverage
		if _, e = AccountLimits(a, c); e == nil {
			t.Fatal("invalid leverage allowed")
		}
	}
	c.Leverage = 5
	for _, position := range []plan.Decimal{"0.01", "-0.02", "0"} {
		a.Position, a.OpenOrders = position, 2
		got, err := AccountLimits(a, c)
		if err != nil {
			t.Fatalf("existing exposure blocked a draft: %v", err)
		}
		if got.MaxNotional != c.MaxNotional || got.MaxRisk != c.MaxRisk {
			t.Fatal("existing exposure increased the free-margin planning budget")
		}
	}
}

func TestAutoSizingAllocatesMarginAcrossEntryStructures(t *testing.T) {
	for _, kind := range []string{"market", "limit", "ladder", "twap"} {
		t.Run(kind, func(t *testing.T) {
			_, b, m, base := setup(t, kind)
			b.account.Equity, b.account.Available = "283.22", "283.22"
			limits, err := AccountLimits(b.account, plan.Constraints{Leverage: 5, MaxRiskPct: "0", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"})
			if err != nil {
				t.Fatal(err)
			}
			p, err := plan.Compile(m.book, limits, base.Idea, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if cmp(p.Quantity, "0.013") < 0 || cmp(p.Risk, "2.8322") <= 0 || p.SizingLimit != "budget" {
				t.Fatalf("with no limit on the loss, margin sizing was still capped: %+v", p)
			}
			if err := CheckAccountCapacity(p, b.account); err != nil {
				t.Fatal(err)
			}
			m.book.AskSize = "0.001"
			m.book.Asks = []plan.BookLevel{{Price: "100000", Quantity: "0.001"}, {Price: "100001", Quantity: "1"}}
			limited, err := plan.Compile(m.book, limits, base.Idea, time.Now().UTC())
			if err != nil || limited.SizingLimit != "budget" || limited.Quantity != p.Quantity {
				t.Fatalf("top quote silently changed margin allocation: %+v %v", limited, err)
			}
			if err = plan.CheckBook(limited, m.book, time.Now().UTC()); err != nil {
				t.Fatal("multi-level depth was not used", err)
			}
			if err = limited.Verify(time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAccountMarginSizingReproducesReportedSmallPosition(t *testing.T) {
	_, _, m, base := setup(t, "market")
	now := time.Now().UTC()
	m.book.Bid, m.book.Ask = "81401", "81402"
	m.book.AskSize = "0.0379"
	base.Idea.Entry, base.Idea.Stop, base.Idea.Target = "81402", "80700", "84100"
	a := Snapshot{Equity: "283", Available: "283", FeeBPS: "10"}
	c, err := AccountLimits(a, plan.Constraints{Leverage: 5, MaxRiskPct: "0", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"})
	if err != nil {
		t.Fatal(err)
	}
	byHand := c
	byHand.SizingPolicy = ""
	old, err := plan.Compile(m.book, byHand, base.Idea, now)
	if err != nil || old.SizingLimit != "liquidity" || cmp(old.Quantity, "0.00379") != 0 {
		t.Fatalf("failed to reproduce reported cap: %+v %v", old, err)
	}
	if err = old.Verify(now); err != nil {
		t.Fatal("saved plan changed", err)
	}
	p, err := plan.Compile(m.book, c, base.Idea, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.SizingLimit != "budget" || cmp(p.Quantity, "0.0165") < 0 || cmp(p.Quantity, "0.018") > 0 {
		t.Fatalf("283 USDC at 5x did not allocate the margin budget: %+v", p)
	}
	if err = CheckAccountCapacity(p, a); err != nil {
		t.Fatal(err)
	}
	t.Logf("by-hand quantity=%s; margin-sized quantity=%s; entry estimate=%s; max notional=%s", old.Quantity, p.Quantity, p.Orders[0].Price, c.MaxNotional)
}

func TestCapacityChecksActualOrdersInsteadOfUnusedBudget(t *testing.T) {
	p := plan.Plan{Constraints: plan.Constraints{Equity: "10000", MaxNotional: "1000", FeeBPS: "10", SlippageBPS: "10", Leverage: 5}, Risk: "1", Orders: []plan.OrderSpec{{Price: "100", Quantity: "1"}, {Price: "100", Quantity: "1"}}}
	a := Snapshot{Equity: "50", Available: "41"}
	if e := CheckAccountCapacity(p, a); e != nil {
		t.Fatal(e)
	}
	a.Available = "40"
	if e := CheckAccountCapacity(p, a); e == nil {
		t.Fatal("missing fee reserve accepted")
	}
	a.Available = "41"
	p.Constraints.Leverage = 1
	if e := CheckAccountCapacity(p, a); e == nil {
		t.Fatal("1x capacity exceeded")
	}
}

func TestDefaultLeverageAndBounds(t *testing.T) {
	c := plan.Constraints{Equity: "100", MaxRisk: "1", MaxNotional: "500", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"}
	if c.EffectiveLeverage() != 1 || c.Validate() == nil {
		t.Fatal("omitted leverage is not 1x")
	}
	c.Leverage = 5
	if e := c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.Leverage = 21
	if c.Validate() == nil {
		t.Fatal("leverage bound bypassed")
	}
}

func TestAccountLimitsCapWhatAStopMayLose(t *testing.T) {
	snapshot := Snapshot{Equity: "5000", Available: "4000", FeeBPS: "5", Position: "0"}
	base := plan.Constraints{Leverage: 5, MinRewardRisk: "0", FeeBPS: "10", SlippageBPS: "10"}
	for _, tt := range []struct {
		pct, wantPct, wantRisk string
	}{
		{"", DefaultMaxRiskPct, "50"},
		{"2.5", "2.5", "125"},
		{"0", "0", "5000"},
	} {
		c := base
		c.MaxRiskPct = plan.Decimal(tt.pct)
		limits, err := AccountLimits(snapshot, c)
		if err != nil {
			t.Fatal(err)
		}
		if string(limits.MaxRiskPct) != tt.wantPct || cmp(limits.MaxRisk, plan.Decimal(tt.wantRisk)) != 0 {
			t.Fatalf("pct %q: %+v", tt.pct, limits)
		}
	}
	c := base
	c.MaxRiskPct = "150"
	if _, err := AccountLimits(snapshot, c); err == nil {
		t.Fatal("a share over 100 passed")
	}
}
