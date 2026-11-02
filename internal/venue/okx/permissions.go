package okx

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"vergex/internal/account"
)

// Permissions reads what the API key may do from the account's config, on real and demo
// trading alike. A key with no IP bound to it reports none.
func (t *Account) Permissions(ctx context.Context) (*account.Permissions, error) {
	data, err := t.doRequestContext(ctx, "GET", okxAccountConfigPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get account config: %w", err)
	}
	var configs []struct {
		Perm string `json:"perm"`
		IP   string `json:"ip"`
	}
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("failed to parse account config: %w", err)
	}
	if len(configs) == 0 || configs[0].Perm == "" {
		return nil, nil
	}
	granted := strings.Split(configs[0].Perm, ",")
	return &account.Permissions{
		Read:         slices.Contains(granted, "read_only"),
		Trade:        slices.Contains(granted, "trade"),
		Withdraw:     slices.Contains(granted, "withdraw"),
		IPRestricted: configs[0].IP != "",
	}, nil
}
