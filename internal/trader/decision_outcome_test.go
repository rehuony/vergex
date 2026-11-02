package trader

import (
	"errors"
	"testing"
	"vergex/internal/account"
	traderagent "vergex/internal/trader/agent"
)

type rejectedAuthorizationTrader struct{ account.Trading }

func (*rejectedAuthorizationTrader) CheckTradingAuthorization() error {
	return errors.New("agent mismatch")
}

func TestCycleChecksAuthorizationBeforeFetchingDataOrCallingAgent(t *testing.T) {
	// Nil data/model dependencies would panic if the paid context path ran.
	at := &AutoTrader{isRunning: true, trader: &rejectedAuthorizationTrader{}}
	if err := at.runCycle(); err == nil {
		t.Fatal("expected authorization block")
	}
	status := at.GetStatus()
	if status["trading_blocked"] != true || status["trading_error"] != "agent mismatch" {
		t.Fatalf("missing runtime block: %v", status)
	}
}

func TestGridDecisionRecordsWhatBecameOfIt(t *testing.T) {
	d := &traderagent.Decision{Action: "place_buy_limit", Symbol: "BTCUSDT", Quantity: 0.2}
	if got := gridDecisionAction(d, nil); !got.Success || got.Error != "" || got.outcome() != OutcomeExecuted {
		t.Errorf("placed = %+v", got)
	}
	if got := gridDecisionAction(d, errors.New("rejected")); got.Success || got.Blocked || got.Error != "rejected" || got.outcome() != OutcomeFailed {
		t.Errorf("failed = %+v", got)
	}
	if got := gridDecisionAction(d, riskBlock{"over the limit"}); got.Success || !got.Blocked || got.outcome() != OutcomeBlocked {
		t.Errorf("blocked = %+v", got)
	}
}

func TestDecisionExecutionOutcome(t *testing.T) {
	for _, tc := range []struct {
		name          string
		actions       []DecisionAction
		initial, want bool
	}{
		{"hold only", []DecisionAction{{Action: "hold", Success: true}}, true, true},
		{"all failed with hold", []DecisionAction{{Action: "open_long"}, {Action: "close_long"}, {Action: "hold", Success: true}}, true, false},
		{"partial failure", []DecisionAction{{Action: "open_long", Success: true}, {Action: "close_long"}}, true, false},
		{"filled", []DecisionAction{{Action: "open_long", Success: true}}, true, true},
		{"preserve upstream fault", nil, false, false},
		{"a rule stopping it is no fault", []DecisionAction{{Action: "open_long", Blocked: true}, {Action: "close_long", Success: true}}, true, true},
		{"a failure beside a block still is", []DecisionAction{{Action: "open_long", Blocked: true}, {Action: "close_long"}}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := &DecisionRecord{Success: tc.initial, Decisions: tc.actions}
			summarizeDecisionExecution(record)
			if record.Success != tc.want {
				t.Fatalf("record=%+v", record)
			}
			if tc.initial && !tc.want && record.ErrorMessage == "" {
				t.Fatal("missing execution failure summary")
			}
		})
	}
}
