package forge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TestCase struct {
	Name        string          `json:"name"`
	Args        map[string]any  `json:"args"`
	Expected    json.RawMessage `json:"expected,omitempty"`
	ExpectError bool            `json:"expect_error,omitempty"`
}
type Definition struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Source      string     `json:"source"`
	Tests       []TestCase `json:"tests"`
}
type Report struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}
type Artifact struct {
	Definition
	SessionID string    `json:"session_id"`
	Hash      string    `json:"hash"`
	Status    string    `json:"status"`
	Enabled   bool      `json:"enabled"`
	Created   time.Time `json:"created"`
	Reports   []Report  `json:"reports"`
}
type Registry struct {
	db *gorm.DB
}

// calculator is a row of futures.calculators, with the Artifact as JSON in Body.
type calculator struct {
	Hash           string    `gorm:"column:hash;primaryKey"`
	ConversationID string    `gorm:"column:conversation_id"`
	Body           string    `gorm:"column:body"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (calculator) TableName() string { return "futures.calculators" }

const maxVersions = 64

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

func digest(session string, d Definition) string {
	b, _ := json.Marshal(struct {
		Session    string
		Definition Definition
	}{session, d})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func NewRegistry(db *gorm.DB) *Registry {
	return &Registry{db: db}
}

// artifact decodes a stored version and checks it still matches its hash.
func (c calculator) artifact() (Artifact, error) {
	var a Artifact
	if err := json.Unmarshal([]byte(c.Body), &a); err != nil {
		return Artifact{}, errors.New("Invalid entry plugin storage")
	}
	if c.Hash != a.Hash || c.Hash != digest(a.SessionID, a.Definition) || !idPattern.MatchString(a.ID) || (a.Status != "passed" && a.Status != "failed") {
		return Artifact{}, errors.New("Entry plugin version verification failed")
	}
	return a, nil
}
func fixtureMarket() map[string]any {
	return map[string]any{"venue": "binance", "symbol": "BTCUSDT", "bid": "99999.9", "ask": "100000.1", "mid": "100000", "tick": "0.1"}
}
func (r *Registry) Develop(ctx context.Context, session string, d Definition) (Artifact, error) {
	if session == "" || len(session) > 80 || !idPattern.MatchString(d.ID) || len(d.Description) == 0 || len(d.Description) > 500 || len(d.Tests) < 2 || len(d.Tests) > 6 {
		return Artifact{}, errors.New("Entry plugin requires an ID, description, current session and 2-6 tests")
	}
	if len(d.Source) > MaxSource {
		return Artifact{}, errors.New("Plugin source exceeds 16 KiB")
	}
	raw, marshalErr := json.Marshal(d)
	if marshalErr != nil {
		return Artifact{}, errors.New("Invalid plugin test JSON")
	}
	if len(raw) > 60000 {
		return Artifact{}, errors.New("Plugin package exceeds limits")
	}
	var detached Definition
	if err := json.Unmarshal(raw, &detached); err != nil {
		return Artifact{}, err
	}
	d = detached
	hash := digest(session, d)
	var existing []calculator
	if err := r.db.Where("hash = ?", hash).Limit(1).Find(&existing).Error; err != nil {
		return Artifact{}, err
	}
	if len(existing) == 1 {
		return existing[0].artifact()
	}
	var stored int64
	if err := r.db.Model(&calculator{}).Count(&stored).Error; err != nil {
		return Artifact{}, err
	}
	if stored >= maxVersions {
		return Artifact{}, errors.New("At most 64 entry plugin versions can be stored")
	}
	a := Artifact{Definition: d, SessionID: session, Hash: hash, Status: "failed", Created: time.Now().UTC(), Reports: []Report{}}
	if err := ValidateSource(d.Source); err != nil {
		a.Reports = append(a.Reports, Report{Name: "Source boundary check", Error: err.Error()})
	} else {
		successes := 0
		all := true
		for _, test := range d.Tests {
			report := Report{Name: test.Name}
			if test.Name == "" || len(test.Name) > 80 {
				report.Error = "Invalid test name"
			} else {
				out, err := Run(ctx, Invocation{Source: d.Source, Market: fixtureMarket(), Args: test.Args})
				if test.ExpectError {
					report.Passed = err != nil
					if err == nil {
						report.Error = "Expected rejection, but execution succeeded"
					}
				} else if err != nil {
					report.Error = err.Error()
				} else {
					expected, err := decodeOutput(test.Expected)
					if err != nil {
						report.Error = "Expected output must be valid idea JSON"
					} else {
						actual, _ := json.Marshal(out)
						want, _ := json.Marshal(expected)
						report.Passed = string(actual) == string(want)
						if !report.Passed {
							report.Error = "Actual output differs from expectation: " + string(actual)
						} else {
							successes++
						}
					}
				}
			}
			if !report.Passed {
				all = false
			}
			a.Reports = append(a.Reports, report)
			if ctx.Err() != nil {
				return a, ctx.Err()
			}
		}
		if all && successes > 0 {
			a.Status = "passed"
			a.Enabled = true
		} else if successes == 0 {
			a.Reports = append(a.Reports, Report{Name: "At least one successful output test", Error: "Tests cannot cover only error paths"})
		}
	}
	body, err := json.Marshal(a)
	if err != nil {
		return a, err
	}
	row := calculator{Hash: hash, ConversationID: session, Body: string(body), CreatedAt: a.Created}
	if err := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return a, err
	}
	return a, nil
}

// List returns a session's versions, newest first. A session is a conversation, so
// anything that can't be a conversation's ID has none.
func (r *Registry) List(session string) ([]Artifact, error) {
	out := []Artifact{}
	if uuid.Validate(session) != nil {
		return out, nil
	}
	var rows []calculator
	if err := r.db.Where("conversation_id = ?", session).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		a, err := row.artifact()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
func (r *Registry) Get(session, hash string) (Artifact, error) {
	items, err := r.List(session)
	if err != nil {
		return Artifact{}, err
	}
	for _, a := range items {
		if a.Hash == hash {
			return a, nil
		}
	}
	return Artifact{}, errors.New("This session does not own this plugin version")
}
func (r *Registry) Run(ctx context.Context, session, hash string, market, args map[string]any) (Output, error) {
	a, err := r.Get(session, hash)
	if err != nil {
		return Output{}, err
	}
	if a.Status != "passed" || !a.Enabled {
		return Output{}, errors.New("Plugin has not passed tests or is disabled")
	}
	out, err := Run(ctx, Invocation{Source: a.Source, Market: market, Args: args})
	if err != nil {
		return out, fmt.Errorf("Plugin %s: %w", a.ID, err)
	}
	return out, nil
}
