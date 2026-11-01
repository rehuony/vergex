package live

import (
	"context"
	"errors"
	"vergex/internal/trading"
)

// TraderBroker adapts the finite scheduler to Vergex's shared exchange trader.
// It owns no credentials, exchange SDK, order conversion or submission logic.
type TraderBroker struct{ trading.Executor }

func (b TraderBroker) Account(ctx context.Context, s string) (Account, error) {
	return b.AccountSnapshot(ctx, s)
}
func (b TraderBroker) Prepare(ctx context.Context, s string) error { return b.PrepareExecution(ctx, s) }
func (b TraderBroker) Place(ctx context.Context, o Order) (Order, error) {
	return b.SubmitOrder(ctx, o)
}
func (b TraderBroker) Lookup(ctx context.Context, o Order) (Order, error) {
	return b.LookupOrder(ctx, o)
}
func (b TraderBroker) Cancel(ctx context.Context, o Order) error {
	return b.CancelExecutionOrder(ctx, o)
}
func (b TraderBroker) RouteClientID(id string) (string, error) {
	if r, ok := b.Executor.(ClientIDRouter); ok {
		return r.RouteClientID(id)
	}
	return "", nil
}

var _ Broker = TraderBroker{}

func (b TraderBroker) PrepareLeverage(ctx context.Context, symbol string, leverage int) error {
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
