package api

import (
	"errors"
	"vergex/internal/account"
)

// Keep account inspection independent of autonomous-trader loading. A finite
// planning lease deliberately prevents that loader from creating a trader.
// This interface cannot submit, cancel, or change account settings.
type accountReader interface {
	GetPositions() ([]map[string]interface{}, error)
	GetOpenOrders(string) ([]account.OpenOrder, error)
}

func (s *Server) accountReadConfig(user, traderID string, accountIDs ...string) (*account.Account, error) {
	if user == "" {
		return nil, errors.New("Authentication is required")
	}
	if len(accountIDs) > 0 && accountIDs[0] != "" {
		if traderID != "" {
			bound, err := s.accountReadConfig(user, traderID)
			if err != nil || bound.ID != accountIDs[0] {
				return nil, errors.New("Trader does not belong to the selected account")
			}
		}
		ex, err := s.stores.Accounts.GetByID(user, accountIDs[0])
		if err != nil {
			return nil, errors.New("Trading account is unavailable")
		}
		return ex, nil
	}
	owned, err := s.stores.Traders.List(user)
	if err != nil {
		return nil, errors.New("Could not verify account ownership")
	}
	for _, t := range owned {
		if traderID != "" && t.ID != traderID {
			continue
		}
		ex, err := s.stores.Accounts.GetByID(user, t.AccountID)
		if err != nil {
			return nil, errors.New("Trading account is unavailable")
		}
		return ex, nil
	}
	return nil, errors.New("Trader does not belong to this account")
}

func (s *Server) accountReadClient(user, traderID string, accountIDs ...string) (accountReader, error) {
	ex, err := s.accountReadConfig(user, traderID, accountIDs...)
	if err != nil {
		return nil, err
	}
	return buildAccountProbe(ex, user)
}
