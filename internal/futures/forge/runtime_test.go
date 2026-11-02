package forge

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"vergex/internal/database/testdb"
)

func sample(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/geometric_pyramid.star")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func sampleOutput(t *testing.T) json.RawMessage {
	t.Helper()
	return json.RawMessage(`{"ideas":[{"name":"Geometric weighted ladder","side":"long","entry":"100000","stop":"99000","target":"104000","rationale":"Conditional pullback ladder with quantity weights 1:2:4; no fill or profit guarantee","execution":{"kind":"ladder","levels":[{"price":"100000","weight":"0.142857"},{"price":"99900","weight":"0.285714"},{"price":"99800","weight":"0.571429"}]}}]}`)
}
func definition(t *testing.T) Definition {
	return Definition{ID: "geometric_pyramid", Description: "finite allocation", Source: sample(t), Tests: []TestCase{{Name: "geometric weights", Args: map[string]any{"gap_bps": float64(10)}, Expected: sampleOutput(t)}, {Name: "invalid gap", Args: map[string]any{"gap_bps": float64(0)}, ExpectError: true}}}
}
func TestWorkerAndImmutableRegistry(t *testing.T) {
	db := testdb.Open(t)
	session := uuid.NewString()
	if err := db.Exec(`INSERT INTO agent.conversations (id, user_id, feature) VALUES (?, ?, 'futures')`, session, testdb.AddUser(t, db)).Error; err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(db)
	a, err := r.Develop(context.Background(), session, definition(t))
	if err != nil || a.Status != "passed" {
		t.Fatalf("develop: %+v %v", a, err)
	}
	a.Tests[0].Args["gap_bps"] = 999.
	safe, _ := r.Get(session, a.Hash)
	if safe.Tests[0].Args["gap_bps"] != float64(10) {
		t.Fatal("returned development package mutated registry")
	}
	again, err := r.Develop(context.Background(), session, definition(t))
	if err != nil || again.Hash != a.Hash {
		t.Fatalf("same definition became a new version: %+v %v", again, err)
	}
	r = NewRegistry(db)
	out, err := r.Run(context.Background(), session, a.Hash, fixtureMarket(), map[string]any{"gap_bps": float64(10)})
	if err != nil || len(out.Ideas) != 1 {
		t.Fatalf("run after reopening: %+v %v", out, err)
	}
	if _, err = r.Run(context.Background(), uuid.NewString(), a.Hash, fixtureMarket(), nil); err == nil {
		t.Fatal("cross-session use")
	}
	if items, err := r.List("not-a-conversation"); err != nil || len(items) != 0 {
		t.Fatalf("list of a malformed session: %+v %v", items, err)
	}
	d := definition(t)
	d.Tests[0].Expected = json.RawMessage(`{"ideas":[]}`)
	failed, err := r.Develop(context.Background(), session, d)
	if err != nil || failed.Status != "failed" || failed.Enabled {
		t.Fatalf("failed test enabled: %+v %v", failed, err)
	}
	if _, err = r.Run(context.Background(), session, failed.Hash, fixtureMarket(), map[string]any{"gap_bps": float64(10)}); err == nil {
		t.Fatal("failed version ran")
	}
	if items, err := r.List(session); err != nil || len(items) != 2 || items[0].Hash != failed.Hash {
		t.Fatalf("list, newest first: %+v %v", items, err)
	}
	if err = db.Exec(`UPDATE futures.calculators SET body = replace(body, 'finite allocation', 'tampered')`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = r.List(session); err == nil {
		t.Fatal("tampered package accepted")
	}
	if err = db.Exec(`DELETE FROM agent.conversations WHERE id = ?`, session).Error; err != nil {
		t.Fatal(err)
	}
	if items, err := r.List(session); err != nil || len(items) != 0 {
		t.Fatalf("calculators outlived their conversation: %+v %v", items, err)
	}
}
func TestDialectDeniesUnsafeOrUnboundedOperations(t *testing.T) {
	for _, source := range []string{`load("/etc/passwd", "secret")`, "def plan(m,a):\n    return open('/etc/passwd')", "def plan(m,a):\n    return 'x' * 1000000000", "def plan(m,a):\n    return [1] + [2]", "def plan(m,a):\n    return a.get('key')", "def plan(m,a):\n    return print('secret')", "def plan(m,a):\n    return {\"ideas\":[],\"orders\":[1]}"} {
		t.Run(source, func(t *testing.T) {
			if _, err := Run(context.Background(), Invocation{Source: source, Market: fixtureMarket(), Args: map[string]any{}}); err == nil {
				t.Fatal("unsafe code accepted")
			}
		})
	}
}
func TestStepBudgetCancellationAndContainerBound(t *testing.T) {
	for _, source := range []string{"def plan(m,a):\n    for i in range(64):\n        for j in range(64):\n            for k in range(64):\n                x = i - j\n    return {\"ideas\":[]}", "def plan(m,a):\n    x=[]\n    for i in range(64):\n        for j in range(64):\n            append(x,i)\n    return {\"ideas\":[]}", "def plan(m,a):\n    return range(1000000)"} {
		start := time.Now()
		if _, err := Run(context.Background(), Invocation{Source: source, Market: fixtureMarket(), Args: map[string]any{}}); err == nil {
			t.Fatal("unbounded program passed")
		}
		if time.Since(start) > 3*time.Second {
			t.Fatal("resource bound failed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Run(ctx, Invocation{Source: sample(t), Market: fixtureMarket(), Args: map[string]any{"gap_bps": 10.}}); err == nil {
		t.Fatal("cancelled worker ran")
	}
}
