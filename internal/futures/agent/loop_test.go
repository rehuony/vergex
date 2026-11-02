package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"vergex/internal/futures"
	"vergex/internal/futures/plan"
)

type loopFixture struct {
	reads int
	chats int
}

func TestModelPlanResultsDoNotRepeatOrderBooksOrHistory(t *testing.T) {
	draft := plan.Plan{ID: "draft", Quantity: "0.1", Risk: "10", Reward: "15", RewardRisk: "1.5", Idea: plan.Idea{Name: "Entry", Entry: "100", Stop: "90", Target: "115"}, Snapshot: plan.Snapshot{Venue: "okx", Symbol: "BTC-USDT-SWAP", QuoteAsset: "USDT", Bid: "99", Ask: "100"}, Orders: []plan.OrderSpec{{Index: 1, Price: "100", Quantity: "0.1"}}}
	result := map[string]any{"plans": []plan.Plan{draft}, "rejected": []string{"TWAP: below minimum quantity"}, "approval_required": true}
	compact := modelToolResult("propose_plans", result).(map[string]any)
	raw, err := json.Marshal(compact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Plans    []map[string]any
		Rejected []string
		Approval bool `json:"approval_required"`
	}
	if err = json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	p := decoded.Plans[0]
	if p["snapshot"] != nil || p["analysis"] != nil || p["reward_risk"] != "1.5" || p["idea"] == nil || p["orders"] == nil || !decoded.Approval || len(decoded.Rejected) != 1 {
		t.Fatal("lost decision data or repeated full plan", string(raw))
	}
	if result["plans"].([]plan.Plan)[0].Snapshot.Bid != "99" {
		t.Fatal("persisted or UI plan was mutated")
	}
}

func (f *loopFixture) Snapshot(context.Context, string, string) (plan.Snapshot, error) {
	return plan.Snapshot{}, nil
}
func (f *loopFixture) Candles(_ context.Context, q futures.CandleQuery) ([]futures.Candle, error) {
	f.reads++
	return []futures.Candle{{Time: time.Now().Add(-time.Hour).Unix(), Open: "100", High: "101", Low: "99", Close: "100", Volume: "1"}}, nil
}
func (f *loopFixture) Chat(_ context.Context, _ []Message, tools []Tool) (Message, error) {
	f.chats++
	if len(tools) == 0 {
		return Message{Role: "assistant", Content: "No validated plan was saved. No trade."}, nil
	}
	c := ToolCall{ID: fmt.Sprint("call_", f.chats), Type: "function"}
	c.Function.Name = "get_candles"
	if f.chats%2 == 0 {
		c.Function.Arguments = `{ "limit": 1, "interval":"1h" }`
	} else {
		c.Function.Arguments = `{"interval":"1h","limit":1}`
	}
	return Message{Role: "assistant", ToolCalls: []ToolCall{c}}, nil
}
func TestRepeatedReadLoopTerminatesWithOutcome(t *testing.T) {
	f := &loopFixture{}
	engine := Engine{Market: f, Charts: f, Model: f, Limits: plan.Constraints{Equity: "1000", MaxRisk: "10", MaxNotional: "100", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"}}
	events := []string{}
	err := engine.Run(context.Background(), Input{Message: "plan a trade", Venue: "binance", Symbol: "BTCUSDT", Interval: "1h"}, func(e Event) error { events = append(events, e.Type); return nil })
	if err != nil {
		t.Fatal(err)
	}
	// Six context reads (including pagination checks), then one additional raw request; duplicate requests
	// with reordered JSON must not fetch again or consume all eight model rounds.
	if f.reads != 7 || f.chats != 4 {
		t.Fatalf("unexpected loop: reads=%d chats=%d", f.reads, f.chats)
	}
	if len(events) < 2 || events[len(events)-1] != "outcome" {
		t.Fatal("missing terminal explanation", events)
	}
}

// reasoningModel runs out of length reasoning for as many rounds as it is told to.
type reasoningModel struct {
	loopFixture
	ranOut int
	calls  int
	last   []Message
}

func (m *reasoningModel) Chat(_ context.Context, messages []Message, _ []Tool) (Message, error) {
	m.calls++
	m.last = messages
	if m.calls <= m.ranOut {
		return Message{}, errReplyRanOut
	}
	return Message{Role: "assistant", Content: "No trade."}, nil
}

func TestAReplyLostToReasoningIsTriedOnceMoreThenTheTurnCloses(t *testing.T) {
	run := func(ranOut int) (*reasoningModel, outcome, error) {
		m := &reasoningModel{ranOut: ranOut}
		engine := Engine{Market: m, Charts: m, Model: m, Limits: plan.Constraints{Equity: "1000", MaxRisk: "10", MaxNotional: "100", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"}}
		var closed outcome
		err := engine.Run(context.Background(), Input{Message: "plan a trade", Venue: "binance", Symbol: "BTCUSDT", Interval: "1h", Language: "en"}, func(e Event) error {
			if e.Type == "outcome" {
				closed = e.Data.(outcome)
			}
			return nil
		})
		return m, closed, err
	}
	m, closed, err := run(1)
	if err != nil || m.calls != 2 || closed.StopReason != "completed" {
		t.Fatal(err, m.calls, closed)
	}
	if !strings.Contains(m.last[len(m.last)-1].Content, "Reason briefly") {
		t.Fatal("the model was not asked to reason less")
	}
	m, closed, err = run(2)
	if err != nil || m.calls != 2 || closed.StopReason != "model_unavailable" {
		t.Fatal(err, m.calls, closed)
	}
}

type stalledModel struct{}

func (stalledModel) Chat(ctx context.Context, _ []Message, _ []Tool) (Message, error) {
	<-ctx.Done()
	return Message{}, ctx.Err()
}
func TestStalledModelRespectsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := chatWithProgress(ctx, stalledModel{}, nil, nil, func(Event) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

type promptModel struct {
	loopFixture
	system string
}

func (m *promptModel) Chat(_ context.Context, messages []Message, _ []Tool) (Message, error) {
	m.system = messages[0].Content
	return Message{Role: "assistant", Content: "No trade."}, nil
}

// A turn that ends without the model's own words still closes in the interface's language,
// and asking the model to close it says the same.
func TestATurnClosesInTheInterfacesLanguage(t *testing.T) {
	closing := func(model ChatPlanner, explain bool) string {
		t.Helper()
		said := ""
		engine := Engine{Model: model}
		err := engine.finishTurn(context.Background(), nil, outcome{StopReason: "budget"}, "zh-CN", func(event Event) error {
			if event.Type == "assistant" {
				said = event.Data.(map[string]string)["message"]
			}
			return nil
		}, explain)
		if err != nil {
			t.Fatal(err)
		}
		return said
	}
	if said := closing(nil, false); !strings.Contains(said, "没有计划通过校验") || !strings.Contains(said, "额度已用完") {
		t.Errorf("closing message = %q", said)
	}
	asked := &lastPromptModel{}
	closing(asked, true)
	if strings.Contains(asked.prompt, "in English") || !strings.Contains(asked.prompt, "Write in Simplified Chinese") {
		t.Errorf("the closing prompt = %q", asked.prompt)
	}
}

type lastPromptModel struct{ prompt string }

func (m *lastPromptModel) Chat(_ context.Context, messages []Message, _ []Tool) (Message, error) {
	m.prompt = messages[len(messages)-1].Content
	return Message{Role: "assistant", Content: "没有交易。"}, nil
}

func TestPlansAreWrittenInTheInterfacesLanguage(t *testing.T) {
	system := func(language string) string {
		t.Helper()
		m := &promptModel{}
		engine := Engine{Market: m, Charts: m, Model: m, Limits: plan.Constraints{Equity: "1000", MaxRisk: "10", MaxNotional: "100", MinRewardRisk: "2", FeeBPS: "10", SlippageBPS: "10"}}
		input := Input{Message: "plan a trade", Language: language, Venue: "binance", Symbol: "BTCUSDT", Interval: "1h"}
		if err := input.Validate(); err != nil {
			t.Fatal(err)
		}
		if err := engine.Run(context.Background(), input, func(Event) error { return nil }); err != nil {
			t.Fatal(err)
		}
		return m.system
	}
	chinese := system("zh-CN")
	if !strings.Contains(chinese, "Simplified Chinese") {
		t.Errorf("no language in the prompt: %s", chinese[len(chinese)-200:])
	}
	for _, forced := range []string{"Use English", "English words"} {
		if strings.Contains(chinese, forced) {
			t.Errorf("the prompt still forces English: %q", forced)
		}
	}
	if own := system(""); !strings.Contains(own, "language of the user's latest message") || strings.Contains(own, "Simplified Chinese") {
		t.Error("a request without a language should follow the user's words alone")
	}
	if err := (Input{Message: "x", Language: "fr", Venue: "binance", Symbol: "BTCUSDT", Interval: "1h"}).Validate(); err == nil {
		t.Error("an unknown language was accepted")
	}
}

func TestALongLabelIsCutToFitTheChart(t *testing.T) {
	if got := shortLabel("Resistance 81950"); got != "Resistance 81950" {
		t.Fatalf("a short label changed: %q", got)
	}
	got := shortLabel(strings.Repeat("阻力", 60))
	if runes := []rune(got); len(runes) != maxLabelRunes || !strings.HasSuffix(got, "…") {
		t.Fatalf("label = %q", got)
	}
}

// streamPlanner writes its reply in pieces, then returns it whole.
type streamPlanner struct{ pieces []string }

func (p *streamPlanner) Chat(context.Context, []Message, []Tool) (Message, error) {
	return Message{}, errors.New("a planner that can stream should be asked to")
}

func (p *streamPlanner) ChatStream(_ context.Context, _ []Message, _ []Tool, onText func(string)) (Message, error) {
	for _, piece := range p.pieces {
		onText(piece)
		time.Sleep(textEvery / 2)
	}
	return Message{Role: "assistant", Content: strings.Join(p.pieces, "")}, nil
}

func TestAReplyReachesThePageAsItIsWrittenAndInOrder(t *testing.T) {
	planner := &streamPlanner{pieces: []string{"one ", "two ", "three ", "four ", "five"}}
	var events []Event
	answer, err := chatWithProgress(context.Background(), planner, nil, nil, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil || answer.Content != "one two three four five" {
		t.Fatalf("answer=%+v err=%v", answer, err)
	}
	var seen strings.Builder
	deltas := 0
	for _, event := range events {
		if event.Type == "assistant_delta" {
			deltas++
			seen.WriteString(event.Data.(map[string]string)["text"])
		}
	}
	// Everything arrives, the last piece before the call returns, and in fewer events than
	// pieces only when they came fast enough to be worth joining.
	if seen.String() != answer.Content || deltas == 0 || deltas > len(planner.pieces) {
		t.Fatalf("seen=%q in %d events", seen.String(), deltas)
	}
}

func TestAPlannerThatCannotStreamStillAnswers(t *testing.T) {
	var events []Event
	answer, err := chatWithProgress(context.Background(), plainPlanner{}, nil, nil, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil || answer.Content != "whole" {
		t.Fatalf("answer=%+v err=%v", answer, err)
	}
	for _, event := range events {
		if event.Type == "assistant_delta" {
			t.Fatal("no text should go out for a planner that cannot stream")
		}
	}
}

type plainPlanner struct{}

func (plainPlanner) Chat(context.Context, []Message, []Tool) (Message, error) {
	return Message{Role: "assistant", Content: "whole"}, nil
}
