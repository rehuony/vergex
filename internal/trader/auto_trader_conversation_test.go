package trader

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"vergex/internal/account"
	"vergex/internal/agent"
	"vergex/internal/database/testdb"
	"vergex/internal/market"
	traderagent "vergex/internal/trader/agent"
	"vergex/internal/trader/kind"
)

// fakeAccount is an empty account on an imaginary exchange. Embedding the interface
// makes any other call panic, so a cycle that tried to trade would fail the test.
type fakeAccount struct{ account.Trading }

func (fakeAccount) GetBalance() (map[string]any, error) {
	return map[string]any{"totalWalletBalance": 1000.0, "availableBalance": 1000.0, "totalEquity": 1000.0}, nil
}

func (fakeAccount) GetPositions() ([]map[string]any, error) { return nil, nil }

// A real client against a local server, so the whole path is what a cycle really uses.
func cycleModel(t *testing.T, status int, onCall func()) (agent.Client, *string) {
	t.Helper()
	var mu sync.Mutex
	sent := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if onCall != nil {
			onCall()
		}
		var body struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		for _, message := range body.Messages {
			sent += message.Role + " "
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{
				"content":           `<reasoning>from the reply</reasoning><decision>[{"symbol":"BTCUSDT","action":"wait","reasoning":"nothing to do"}]</decision>`,
				"reasoning_content": "the model's own thinking",
			}, "finish_reason": "stop"}}})
		}
	}))
	t.Cleanup(server.Close)
	client := agent.NewClient(agent.WithHTTPClient(server.Client()))
	client.SetAPIKey("test-key", server.URL, "test-model")
	agent.TagUsage(client, agent.FeatureTrader, "")
	return client, &sent
}

func cycleTrader(t *testing.T, client agent.Client) (*AutoTrader, *Stores) {
	t.Helper()
	st := NewStores(testdb.Open(t))
	trader := testdb.AddTrader(t, st.DB, &testdb.Trader{Name: "test"})
	// No candles and no way to reach an exchange for them: the prompts go out without.
	market.SetLiveSource(&fakeMarks{marks: map[string]float64{}})

	config := kind.DefaultAgent()
	config.CoinSource.SourceType = "static"
	config.CoinSource.StaticCoins = []string{"BTCUSDT"}
	return &AutoTrader{
		id: trader.ID, name: trader.Name, userID: trader.UserID, isRunning: true, stores: st,
		trader: fakeAccount{}, agentClient: client, initialBalance: 1000,
		engine:                traderagent.NewEngine(config),
		positionFirstSeenTime: map[string]int64{},
		peakPnLCache:          map[string]float64{},
	}, st
}

// waitForMessages polls: messages are stored in the background.
func waitForMessages(t *testing.T, st *Stores, conversationID string, want int) []agent.ConversationMessage {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		messages, err := st.Conversations.Messages(conversationID)
		if err != nil {
			t.Fatal(err)
		}
		if len(messages) >= want {
			return messages
		}
		if time.Now().After(deadline) {
			t.Fatalf("messages = %d, want %d", len(messages), want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func lastConversation(t *testing.T, st *Stores, at *AutoTrader) *agent.Conversation {
	t.Helper()
	conversation, err := st.Conversations.Latest(at.userID, agent.FeatureTrader, at.id)
	if err != nil {
		t.Fatalf("conversation: %v", err)
	}
	return conversation
}

func TestACycleKeepsWhatWasSaidWithItsThinking(t *testing.T) {
	client, sent := cycleModel(t, http.StatusOK, nil)
	at, st := cycleTrader(t, client)

	var reported string
	agent.CallCallback = func(report agent.CallReport) { reported = report.ConversationID }
	t.Cleanup(func() { agent.CallCallback = nil })

	if err := at.runCycle(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(*sent) != "system user" {
		t.Fatalf("the model was sent %q", *sent)
	}
	conversation := lastConversation(t, st, at)
	if reported != conversation.ID {
		t.Fatalf("the call reported conversation %q, want %q", reported, conversation.ID)
	}
	messages := waitForMessages(t, st, conversation.ID, 3)
	if len(messages) != 3 || messages[0].Role != agent.RoleSystem || messages[1].Role != agent.RoleUser || messages[2].Role != agent.RoleAssistant {
		t.Fatalf("messages = %+v", messages)
	}
	if messages[0].Content == "" || messages[1].Content == "" {
		t.Fatalf("the prompts were not kept: %+v", messages[:2])
	}
	if !strings.Contains(messages[2].Content, "from the reply") || messages[2].Thinking != "the model's own thinking" {
		t.Fatalf("reply = %+v", messages[2])
	}
}

func TestACycleWhoseModelFailedKeepsWhatWasSent(t *testing.T) {
	client, _ := cycleModel(t, http.StatusBadRequest, nil)
	at, st := cycleTrader(t, client)

	if err := at.runCycle(); err == nil {
		t.Fatal("a failed model call is still reported")
	}
	messages := waitForMessages(t, st, lastConversation(t, st, at).ID, 2)
	if len(messages) != 2 || messages[1].Role != agent.RoleUser {
		t.Fatalf("messages = %+v", messages)
	}
}

// savedCycle is the cycle's own record, which reaching the end of a cycle leaves.
func savedCycle(t *testing.T, st *Stores, at *AutoTrader) *DecisionRecord {
	t.Helper()
	records, err := st.Decisions.GetSummaries(at.id, 1, 0)
	if err != nil || len(records) != 1 {
		t.Fatalf("records = %d, err %v", len(records), err)
	}
	return records[0]
}

func TestAConversationThatCannotBeSavedDoesNotStopACycle(t *testing.T) {
	client, sent := cycleModel(t, http.StatusOK, nil)
	at, st := cycleTrader(t, client)
	if err := st.DB.Exec(`ALTER TABLE agent.conversations RENAME TO conversations_gone`).Error; err != nil {
		t.Fatal(err)
	}

	if err := at.runCycle(); err != nil {
		t.Fatalf("the cycle stopped: %v", err)
	}
	if *sent == "" {
		t.Fatal("the model was not called")
	}
	record := savedCycle(t, st, at)
	if !record.Success || len(record.Decisions) != 1 || record.Decisions[0].Action != "wait" {
		t.Fatalf("record = %+v", record)
	}
}

// holdTable keeps a table locked until the returned func runs, as a database stuck on
// it would.
func holdTable(t *testing.T, st *Stores, table string) func() {
	t.Helper()
	tx := st.DB.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := tx.Exec(`LOCK TABLE ` + table + ` IN ACCESS EXCLUSIVE MODE`).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(func() { tx.Rollback() }) }
	t.Cleanup(release)
	return release
}

func TestAStuckConversationDoesNotHoldACycleUp(t *testing.T) {
	client, _ := cycleModel(t, http.StatusOK, nil)
	at, st := cycleTrader(t, client)
	release := holdTable(t, st, "agent.messages")

	started := time.Now()
	if err := at.runCycle(); err != nil {
		t.Fatalf("the cycle stopped: %v", err)
	}
	if took := time.Since(started); took >= conversationWait {
		t.Fatalf("the cycle waited %v for its conversation", took)
	}
	if record := savedCycle(t, st, at); !record.Success {
		t.Fatalf("record = %+v", record)
	}

	// Past its wait the write gives up and says so; nothing else comes of it.
	time.Sleep(conversationWait + 500*time.Millisecond)
	release()
	if messages, err := st.Conversations.Messages(lastConversation(t, st, at).ID); err != nil || len(messages) != 0 {
		t.Fatalf("messages = %+v, err %v", messages, err)
	}
}

func TestAConversationThatCannotOpenWaitsOnlySoLong(t *testing.T) {
	// The cycle's own record needs the table too, so the lock lasts until the model is
	// called: what is measured is the wait for the conversation.
	var release func()
	called := time.Time{}
	client, _ := cycleModel(t, http.StatusOK, func() {
		called = time.Now()
		release()
	})
	at, st := cycleTrader(t, client)
	release = holdTable(t, st, "agent.conversations")

	started := time.Now()
	if err := at.runCycle(); err != nil {
		t.Fatalf("the cycle stopped: %v", err)
	}
	if called.IsZero() {
		t.Fatal("the model was not called")
	}
	if waited := called.Sub(started); waited > conversationWait+2*time.Second {
		t.Fatalf("the model waited %v, past the %v it may", waited, conversationWait)
	}
	record := savedCycle(t, st, at)
	if !record.Success || record.ConversationID != "" {
		t.Fatalf("record = %+v", record)
	}
}

func TestACycleThatCannotBeSavedDoesNotHoldTheLoop(t *testing.T) {
	client, sent := cycleModel(t, http.StatusOK, nil)
	at, st := cycleTrader(t, client)
	holdTable(t, st, "trader.cycles")

	started := time.Now()
	if err := at.runCycle(); err != nil {
		t.Fatalf("the cycle stopped: %v", err)
	}
	// One second is the pause after a decision is carried out.
	if took := time.Since(started); took > cycleSaveWait+3*time.Second {
		t.Fatalf("the cycle waited %v, past the %v it may", took, cycleSaveWait)
	}
	if *sent == "" {
		t.Fatal("the model was not called")
	}
}
