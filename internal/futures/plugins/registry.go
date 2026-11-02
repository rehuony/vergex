// Package plugins hosts trusted, read-only data components. It never receives
// trade storage, approval handles, credentials or an executable shell.
package plugins

import (
	"context"
	"errors"
	"math/big"
	"regexp"
	"sort"
	"time"
	"vergex/internal/market/instrument"
)

type Request struct {
	Venue  string `json:"venue"`
	Symbol string `json:"symbol"`
}
type Metric struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
	Unit  string `json:"unit"`
}
type Result struct {
	Plugin     string    `json:"plugin"`
	Version    string    `json:"version"`
	Venue      string    `json:"venue"`
	Symbol     string    `json:"symbol"`
	Source     string    `json:"source"`
	Observed   time.Time `json:"observed"`
	SourceTime time.Time `json:"source_time,omitempty"`
	Metrics    []Metric  `json:"metrics"`
	Notice     string    `json:"notice"`
	Mode       string    `json:"mode"`
}
type Manifest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Capability  string `json:"capability"`
	Enabled     bool   `json:"enabled"`
}
type Definition struct {
	Manifest Manifest
	Read     func(context.Context, Request) (Result, error)
}
type Source interface {
	PluginData(context.Context, string, Request) (Result, error)
}
type Registry struct {
	definitions map[string]Definition
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var decimal = regexp.MustCompile(`^-?[0-9]{1,30}(\.[0-9]{1,20})?$`)

func Open(definitions ...Definition) (*Registry, error) {
	if len(definitions) > 8 {
		return nil, errors.New("at most eight data plugins may be registered")
	}
	r := &Registry{definitions: map[string]Definition{}}
	for _, d := range definitions {
		if !identifier.MatchString(d.Manifest.ID) || d.Manifest.Capability != "public.read" || d.Manifest.Version == "" || len(d.Manifest.Version) > 30 || len(d.Manifest.Name) > 100 || len(d.Manifest.Description) > 1200 || d.Read == nil {
			return nil, errors.New("invalid read-only plugin definition")
		}
		if _, ok := r.definitions[d.Manifest.ID]; ok {
			return nil, errors.New("duplicate plugin ID")
		}
		r.definitions[d.Manifest.ID] = d
	}
	return r, nil
}
func Builtins(source Source) []Definition {
	defs := []Definition{}
	for _, m := range []Manifest{{ID: "funding", Name: "Funding rate", Version: "1.0.0", Description: "Public funding and mark prices. Settlement conventions vary by venue; not automatically included in this trade risk estimate.", Capability: "public.read", Enabled: true}, {ID: "open_interest", Name: "Open interest", Version: "1.0.0", Description: "Current open interest in venue-native units. One snapshot does not establish a directional increase.", Capability: "public.read", Enabled: true}} {
		id := m.ID
		defs = append(defs, Definition{Manifest: m, Read: func(ctx context.Context, q Request) (Result, error) {
			if source == nil {
				return Result{}, errors.New("This market source does not provide the requested plugin")
			}
			return source.PluginData(ctx, id, q)
		}})
	}
	return defs
}
func (r *Registry) List() []Manifest {
	items := []Manifest{}
	for _, d := range r.definitions {
		items = append(items, d.Manifest)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}
func (r *Registry) Read(ctx context.Context, id string, q Request) (Result, error) {
	if !instrument.ValidSymbol(q.Symbol) || (q.Venue != "binance" && q.Venue != "okx") {
		return Result{}, errors.New("invalid plugin workspace")
	}
	d, ok := r.definitions[id]
	if !ok || !d.Manifest.Enabled {
		return Result{}, errors.New("plugin unavailable or disabled")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := d.Read(ctx, q)
	if err != nil {
		return Result{}, err
	}
	if err = ctx.Err(); err != nil {
		return Result{}, err
	}
	if out.Venue != q.Venue || out.Symbol != q.Symbol || len(out.Metrics) == 0 || len(out.Metrics) > 8 || out.Source == "" || len(out.Source) > 300 || len(out.Notice) > 1200 || (out.Mode != "public" && out.Mode != "demo" && out.Mode != "paid") {
		return Result{}, errors.New("invalid canonical plugin result")
	}
	now := time.Now()
	if out.Observed.IsZero() || now.Sub(out.Observed) > 30*time.Second || out.Observed.After(now.Add(time.Second)) {
		return Result{}, errors.New("stale plugin receipt")
	}
	if !out.SourceTime.IsZero() && (now.Sub(out.SourceTime) > 60*time.Second || out.SourceTime.After(now.Add(10*time.Second))) {
		return Result{}, errors.New("stale plugin source timestamp")
	}
	seen := map[string]bool{}
	for _, m := range out.Metrics {
		n, valid := new(big.Rat).SetString(m.Value)
		if !identifier.MatchString(m.Key) || seen[m.Key] || !decimal.MatchString(m.Value) || !valid || len(m.Label) > 100 || len(m.Unit) > 30 || (m.Key == "open_interest" && n.Sign() < 0) {
			return Result{}, errors.New("invalid plugin metric")
		}
		seen[m.Key] = true
		if m.Key == "mark_price" && n.Sign() <= 0 {
			return Result{}, errors.New("invalid plugin mark price")
		}
	}
	out.Plugin = id
	out.Version = d.Manifest.Version
	return out, nil
}
