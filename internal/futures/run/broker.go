package run

import (
	"context"
	"errors"
	"vergex/internal/account"
)

// ExecutorBroker adapts the finite scheduler to Vergex's shared exchange trader.
// It owns no credentials, exchange SDK, order conversion or submission logic.
type ExecutorBroker struct{ account.Executor }

func (b ExecutorBroker) Account(ctx context.Context, s string) (Snapshot, error) {
	return b.AccountSnapshot(ctx, s)
}
func (b ExecutorBroker) Prepare(ctx context.Context, s string) error {
	return b.PrepareExecution(ctx, s)
}
func (b ExecutorBroker) Place(ctx context.Context, o Order) (Order, error) {
	return b.SubmitOrder(ctx, o)
}
func (b ExecutorBroker) Lookup(ctx context.Context, o Order) (Order, error) {
	return b.LookupOrder(ctx, o)
}
func (b ExecutorBroker) Cancel(ctx context.Context, o Order) error {
	return b.CancelExecutionOrder(ctx, o)
}

// CancelOpen cancels an order that belongs to no run, known by what the exchange lists.
func (b ExecutorBroker) CancelOpen(ctx context.Context, symbol, orderID string, conditional bool) error {
	canceller, ok := b.Executor.(account.OrderCanceller)
	if !ok {
		return errors.New("this account cannot cancel orders from Vergex")
	}
	return canceller.CancelOpenOrder(ctx, symbol, orderID, conditional)
}

func (b ExecutorBroker) RouteClientID(id string) (string, error) {
	if r, ok := b.Executor.(ClientIDRouter); ok {
		return r.RouteClientID(id)
	}
	return "", nil
}

var _ Broker = ExecutorBroker{}

// Testnet says whether the account underneath is on a test environment.
func (b ExecutorBroker) Testnet() bool {
	t, ok := b.Executor.(interface{ Testnet() bool })
	return ok && t.Testnet()
}

func (b ExecutorBroker) PrepareLeverage(ctx context.Context, symbol string, leverage int) error {
	if t, ok := b.Executor.(interface {
		PrepareExecutionLeverage(context.Context, string, int) error
	}); ok {
		return t.PrepareExecutionLeverage(ctx, symbol, leverage)
	}
	if leverage == 1 {
		return b.Prepare(ctx, symbol)
	}
	return errors.New("account does not support the approved leverage")
}
