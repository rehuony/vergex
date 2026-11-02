package binance

import (
	"context"
	"vergex/internal/account"

	spot "github.com/adshao/go-binance/v2"
)

// Permissions reads what the API key may do. Binance answers on its wallet API rather than
// the futures one, and its test environment has no such endpoint, so there it says nothing.
func (b *Account) Permissions(ctx context.Context) (*account.Permissions, error) {
	if b.testnet {
		return nil, nil
	}
	client := spot.NewClient(b.client.APIKey, b.client.SecretKey)
	client.HTTPClient = b.client.HTTPClient
	client.TimeOffset = b.client.TimeOffset
	return keyPermissions(ctx, client)
}

// keyPermissions asks a client for its key's permissions. Trading here means futures, the
// only market vergex trades on Binance.
func keyPermissions(ctx context.Context, client *spot.Client) (*account.Permissions, error) {
	key, err := client.NewGetAPIKeyPermission().Do(ctx)
	if err != nil {
		return nil, binanceReadError("read the API key's permissions", err)
	}
	return &account.Permissions{
		Read:         key.EnableReading,
		Trade:        key.EnableFutures,
		Withdraw:     key.EnableWithdrawals,
		IPRestricted: key.IPRestrict,
	}, nil
}
