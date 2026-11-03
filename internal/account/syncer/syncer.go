// Package syncer keeps the ledger up to date with what happened on each account
// something can be trading on: one loop per account, however many traders use it.
package syncer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
	"vergex/internal/account"

	"vergex/internal/logger"
	"vergex/internal/market/instrument"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"

	"gorm.io/gorm"
)

const (
	syncEvery = 30 * time.Second
	scanEvery = 15 * time.Second
	// stayFor keeps an account's loop going once nothing holds it open any more, so a
	// trade that just ended still gets its last fills.
	stayFor = 10 * time.Minute
	// firstRead is how far back an account with no known trades starts.
	firstRead = 24 * time.Hour
	// settleFor is how long an order is followed up. One still unsettled after this is
	// more than the exchange will say anything useful about.
	settleFor = 7 * 24 * time.Hour
	// readOverlap reads a little before the last known trade: reading one twice is
	// harmless, and a slow exchange can report trades out of order.
	readOverlap = 10 * time.Minute
)

// Source reads an exchange account.
type Source interface {
	FillsSince(account.FillCursor) ([]account.Fill, error)
	GetOpenOrders(symbol string) ([]account.OpenOrder, error)
	GetOrderStatus(symbol, orderID string) (map[string]interface{}, error)
}

// Service runs the loops.
type Service struct {
	db       *gorm.DB
	store    *account.Store
	ledger   *account.Ledger
	features Features
	open     func(context.Context, *account.Account) (Source, error)
	now      func() time.Time

	mu       sync.Mutex
	accounts map[string]*target
	failing  map[string]Failure
}

// Features is what only the features using accounts know. The ledger names them by
// source and source_id and imports none of them, so they are told to it here.
type Features struct {
	// ActiveAccounts lists the accounts a feature is running on now.
	ActiveAccounts func() ([]string, error)
	// Owner is who gets an order that nothing claims on an account no one has reserved.
	Owner func(accountID string) (account.Owner, error)
}

type target struct {
	mu         sync.Mutex
	source     Source
	stamp      time.Time
	stop       chan struct{} // nil when no loop is running
	lastActive time.Time
	// readFrom is where the next read starts. It only moves on once a whole read has
	// worked, because one symbol failing must not let the others carry the start past
	// trades it still has to report. Zero until the first read since starting.
	readFrom time.Time
}

// Failure says how long an account has been failing to read, and why.
type Failure struct {
	Since  time.Time
	Reason string
}

// New creates a Service that reads the exchanges themselves.
func New(db *gorm.DB, features Features) *Service {
	return &Service{
		db: db, store: account.NewStore(db), ledger: account.NewLedger(db), features: features,
		open: openExchange, now: time.Now, accounts: make(map[string]*target), failing: make(map[string]Failure),
	}
}

// openExchange connects to an account without touching its settings, which the
// constructors for traders do.
func openExchange(ctx context.Context, ex *account.Account) (Source, error) {
	switch ex.Venue {
	case "binance":
		return binance.NewReadOnlyAccount(ctx, string(ex.APIKey), string(ex.SecretKey), ex.Testnet)
	case "okx":
		return okx.NewReadOnlyAccount(string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase), ex.Testnet), nil
	default:
		return nil, fmt.Errorf("order sync is not available for exchange type %s", ex.Venue)
	}
}

// Start begins and ends loops as accounts become active and stop being so, until ctx ends.
func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scanEvery)
		defer ticker.Stop()
		for {
			s.scan()
			select {
			case <-ctx.Done():
				s.stopAll()
				return
			case <-ticker.C:
			}
		}
	}()
}

// scan starts a loop for each active account and ends those that have been quiet.
func (s *Service) scan() {
	ids, err := s.activeAccounts()
	if err != nil {
		logger.Warnf("Account sync: list active accounts: %v", err)
		return
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range ids {
		state := s.state(id)
		state.lastActive = now
		if state.stop == nil {
			state.stop = make(chan struct{})
			s.startLoop(id, state.stop)
		}
	}
	for _, state := range s.accounts {
		if state.stop != nil && now.Sub(state.lastActive) > stayFor {
			close(state.stop)
			state.stop = nil
		}
	}
}

// activeAccounts lists the accounts something can be trading on: what the ledger knows of
// and what a feature is running on. An account can appear twice.
func (s *Service) activeAccounts() ([]string, error) {
	ids, err := s.store.ActiveIDs()
	if err != nil {
		return nil, err
	}
	running, err := s.features.ActiveAccounts()
	if err != nil {
		return nil, err
	}
	return append(ids, running...), nil
}

func (s *Service) startLoop(id string, stop <-chan struct{}) {
	sync := func() error { return s.Sync(id) }
	go func() {
		if err := sync(); err != nil {
			logger.Infof("⚠️ Account %s first order sync failed: %v", id, err)
		}
	}()
	runLoop(stop, syncEvery, "Account "+id, sync)
}

func (s *Service) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, state := range s.accounts {
		if state.stop != nil {
			close(state.stop)
			state.stop = nil
		}
	}
}

// state must be called with s.mu held.
func (s *Service) state(id string) *target {
	state := s.accounts[id]
	if state == nil {
		state = &target{}
		s.accounts[id] = state
	}
	return state
}

// noteResult remembers whether an account's read worked, for Failing.
func (s *Service) noteResult(accountID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		delete(s.failing, accountID)
		return
	}
	failure, ok := s.failing[accountID]
	if !ok {
		failure.Since = s.now()
	}
	failure.Reason = err.Error()
	s.failing[accountID] = failure
}

// Failing lists the accounts whose last read failed. It does not wait for a read under way.
func (s *Service) Failing() map[string]Failure {
	s.mu.Lock()
	defer s.mu.Unlock()
	failing := make(map[string]Failure, len(s.failing))
	for id, failure := range s.failing {
		failing[id] = failure
	}
	return failing
}

// Sync reads an account's new trades into the ledger now. Each account syncs one at a
// time, so a caller that needs the result waits for one already under way.
func (s *Service) Sync(accountID string) (err error) {
	s.mu.Lock()
	state := s.state(accountID)
	s.mu.Unlock()
	state.mu.Lock()
	defer state.mu.Unlock()
	defer func() { s.noteResult(accountID, err) }()

	ex, err := s.store.Get(accountID)
	if err != nil {
		return fmt.Errorf("load account: %w", err)
	}
	// A saved change can mean new keys.
	if state.source == nil || !state.stamp.Equal(ex.UpdatedAt) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		state.source, err = s.open(ctx, ex)
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		state.stamp = ex.UpdatedAt
	}

	started := s.now()
	cursor, err := s.cursor(accountID, state)
	if err != nil {
		return err
	}
	// A symbol that could not be read does not hold the others up.
	fills, readErr := state.source.FillsSince(cursor)
	bookErr := s.book(ex, fills)
	if readErr == nil && bookErr == nil {
		state.readFrom = started.Add(-readOverlap)
	}
	return errors.Join(readErr, bookErr, s.settle(ex.ID, state.source))
}

// settle finds out how orders the ledger has as working ended, because a cancel or a
// rejection leaves no fill to say so. It only asks about symbols that have one, and only
// about an order that is no longer listed as open.
func (s *Service) settle(accountID string, source Source) error {
	orders, err := s.ledger.WorkingOrders(accountID, s.now().Add(-settleFor))
	if err != nil || len(orders) == 0 {
		return err
	}
	var failures []error
	var open map[string]bool
	symbol := ""
	for _, order := range orders {
		if order.Symbol != symbol || open == nil {
			symbol = order.Symbol
			listed, err := source.GetOpenOrders(symbol)
			if err != nil {
				failures = append(failures, fmt.Errorf("read open orders of %s: %w", symbol, err))
				open = nil
				continue
			}
			open = make(map[string]bool, len(listed))
			for _, item := range listed {
				open[item.OrderID] = true
			}
		}
		if open[order.VenueOrderID] {
			continue
		}
		state, err := source.GetOrderStatus(symbol, order.VenueOrderID)
		if err != nil {
			failures = append(failures, fmt.Errorf("read order %s: %w", order.VenueOrderID, err))
			continue
		}
		status, _ := state["status"].(string)
		switch status {
		case account.OrderFilled, account.OrderCanceled, account.OrderExpired, account.OrderRejected:
			if err := s.ledger.SettleOrder(order.ID, status); err != nil {
				failures = append(failures, fmt.Errorf("settle order %d: %w", order.ID, err))
			}
		}
	}
	return errors.Join(failures...)
}

func (s *Service) cursor(accountID string, state *target) (account.FillCursor, error) {
	ledger := s.ledger
	if state.readFrom.IsZero() {
		// However long this was not running, pick up from the last trade it knows of.
		last, err := ledger.LastFillTime(accountID)
		if err != nil {
			return account.FillCursor{}, fmt.Errorf("read last fill time: %w", err)
		}
		if last.IsZero() {
			state.readFrom = s.now().Add(-firstRead)
		} else {
			state.readFrom = last.Add(-readOverlap)
		}
	}
	openSymbols, err := ledger.OpenSymbols(accountID)
	if err != nil {
		return account.FillCursor{}, fmt.Errorf("read open symbols: %w", err)
	}
	return account.FillCursor{Since: state.readFrom, OpenSymbols: openSymbols}, nil
}

// book records fills oldest first. It stops at the first it cannot record, because the
// next read picks up after the latest one recorded.
func (s *Service) book(ex *account.Account, fills []account.Fill) error {
	if len(fills) == 0 {
		return nil
	}
	sort.SliceStable(fills, func(i, j int) bool { return fills[i].Time.Before(fills[j].Time) })
	ledger := s.ledger
	owner, err := s.unclaimedOwner(ex.ID)
	if err != nil {
		return fmt.Errorf("find the owner of unknown orders: %w", err)
	}
	for _, fill := range fills {
		if fill.Unbookable != "" {
			isNew, err := ledger.RecordSkippedFill(&account.SkippedFill{
				AccountID: ex.ID, NativeSymbol: fill.NativeSymbol, VenueTradeID: fill.TradeID, Detail: fill.Unbookable, ExecutedAt: fill.Time,
			})
			if err != nil {
				return fmt.Errorf("record skipped trade %s: %w", fill.TradeID, err)
			}
			if isNew {
				logger.Warnf("⚠️ Account %s: trade %s on %s cannot be booked: %s", ex.ID, fill.TradeID, fill.NativeSymbol, fill.Unbookable)
			}
			continue
		}
		instrumentID, err := instrument.Ensure(s.db, ex.Venue, instrument.KindPerpetual, fill.Symbol)
		if err != nil {
			return err
		}
		recorded, err := ledger.RecordTrade(account.Trade{
			AccountID:       ex.ID,
			InstrumentID:    instrumentID,
			VenueTradeID:    fill.TradeID,
			VenueOrderID:    fill.OrderID,
			Side:            fill.Side,
			PositionSide:    fill.PositionSide,
			Price:           fill.Price,
			Quantity:        fill.Quantity,
			QuoteQuantity:   fill.QuoteQuantity,
			Commission:      fill.Commission,
			CommissionAsset: fill.CommissionAsset,
			RealizedPnL:     fill.RealizedPnL,
			Maker:           fill.IsMaker,
			ExecutedAt:      fill.Time,
		}, owner)
		if err != nil {
			return fmt.Errorf("record trade %s: %w", fill.TradeID, err)
		}
		if recorded {
			logger.Infof("  ✅ Synced trade: %s %s %s qty=%s price=%s", fill.TradeID, fill.Symbol, fill.Side, fill.Quantity, fill.Price)
			if err := ledger.ClearSkippedFill(ex.ID, fill.NativeSymbol, fill.TradeID); err != nil {
				logger.Warnf("Account sync: clear skipped trade %s: %v", fill.TradeID, err)
			}
		}
	}
	return nil
}

// unclaimedOwner is who gets an order that nothing claims: whoever has reserved the
// account, or else a feature running on it, or else no one.
func (s *Service) unclaimedOwner(accountID string) (account.Owner, error) {
	owner, err := s.ledger.LeaseOwner(accountID)
	if err != nil || owner.Source != "" {
		return owner, err
	}
	return s.features.Owner(accountID)
}
