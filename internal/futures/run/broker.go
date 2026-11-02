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
func (b ExecutorBroker) RouteClientID(id string) (string, error) {
	if r, ok := b.Executor.(ClientIDRouter); ok {
		return r.RouteClientID(id)
	}
	return "", nil
}

var _ Broker = ExecutorBroker{}

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
