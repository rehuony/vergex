package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"vergex/internal/account"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"vergex/internal/futures"
	"vergex/internal/futures/run"
	"vergex/internal/market/instrument"
	"vergex/internal/venue/binance"
	"vergex/internal/venue/okx"
)

type accountCloseRequest struct {
	AccountID string `json:"account_id"`
	RequestID string `json:"request_id"`
	Symbol    string `json:"symbol"`
	Side      string `json:"side"`
	Quantity  string `json:"quantity"`
	Entry     string `json:"entry_price"`
}

type accountCloser interface {
	GetPositions() ([]map[string]interface{}, error)
	RouteClientID(string) (string, error)
	SubmitOrder(context.Context, account.Order) (account.Order, error)
	LookupOrder(context.Context, account.Order) (account.Order, error)
}

type closeNotSubmittedError struct{ error }

func writeAccountCloseError(c *gin.Context, err error) {
	state := "unconfirmed"
	var notSent closeNotSubmittedError
	if errors.As(err, &notSent) {
		state = "not_submitted"
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "close_state": state})
}

func (s *Server) handleFuturesAccountClose(c *gin.Context) {
	var in accountCloseRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil {
		writeAccountCloseError(c, closeNotSubmittedError{errors.New("Invalid position close request; no close was sent")})
		return
	}
	p, err := s.futuresService()
	if err != nil {
		writeAccountCloseError(c, closeNotSubmittedError{err})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	user := c.GetString("user_id")
	order, err := s.closeFuturesAccount(ctx, user, in, func(ex *account.Account) (accountCloser, error) {
		if ex.Venue == "okx" {
			return okx.NewReadOnlyAccount(string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase), ex.Testnet), nil
		}
		return binance.NewReadOnlyAccount(ctx, string(ex.APIKey), string(ex.SecretKey), ex.Testnet)
	})
	if err != nil {
		writeAccountCloseError(c, err)
		return
	}
	warning := ""
	if order.Status == "FILLED" {
		runs, readErr := p.controller.List(user)
		if readErr != nil {
			warning = "Execution records could not be refreshed"
		}
		for _, run := range runs {
			if run.Status == "completed" || run.Status == "cancelled" || run.Status == "expired" || run.AccountID != in.AccountID || run.Plan.Snapshot.Symbol != in.Symbol || run.Plan.Idea.Side != in.Side {
				continue
			}
			if _, err := p.controller.FinishManualClose(ctx, user, run.ID); err != nil {
				warning = strings.TrimSpace(warning + " " + err.Error())
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"warning": warning, "order": order})
}

func (s *Server) closeFuturesAccount(ctx context.Context, user string, in accountCloseRequest, factory func(*account.Account) (accountCloser, error)) (out account.Order, err error) {
	// Only a definite pre-submission failure may release the UI's close latch.
	// Existing durable intents and ambiguous submissions must stay protected.
	mayHaveSubmitted := false
	defer func() {
		if err != nil && !mayHaveSubmitted {
			err = closeNotSubmittedError{err}
		}
	}()
	id, err := uuid.Parse(in.RequestID)
	if err != nil || id == uuid.Nil || !instrument.ValidSymbol(in.Symbol) || (in.Side != "long" && in.Side != "short") || !account.Positive(account.Decimal(in.Quantity)) || !account.ValidOrderPrice(account.Order{Role: "entry", Type: "market", ReduceOnly: true, Price: account.Decimal(in.Entry)}) {
		return out, errors.New("Review a valid position and quantity before closing")
	}
	in.RequestID = id.String()
	ex, err := s.accountReadConfig(user, "", in.AccountID)
	if err != nil {
		return out, err
	}
	if in.AccountID == "" || (ex.Venue != "binance" && ex.Venue != "okx") {
		return out, errors.New("Select a Binance or OKX USDT futures account")
	}
	key := ex.Identity
	unlock := account.Lock(ex.ID)
	defer unlock()
	traders, err := s.stores.Traders.ListByAccountID(user, ex.ID)
	if err != nil {
		return out, errors.New("Could not verify trader state")
	}
	for _, trader := range traders {
		running := trader.Running
		if s.traderManager != nil {
			if runtime, e := s.traderManager.GetTrader(trader.ID); e == nil {
				active, _ := runtime.GetStatus()["running"].(bool)
				running = running || active
			}
		}
		if running {
			return out, errors.New("Stop the trader on this account before closing manually")
		}
	}
	if err := run.CheckCloseEntries(s.stores.DB, key, in.Symbol); err != nil {
		return out, err
	}
	client, err := factory(ex)
	if err != nil {
		return out, err
	}
	closes := s.stores.AccountCloses
	previous, err := closes.Get(in.RequestID)
	if err == nil {
		mayHaveSubmitted = true
		if previous.UserID != user || previous.AccountID != in.AccountID || previous.Symbol != in.Symbol || previous.Side != in.Side || previous.Quantity != in.Quantity || previous.Entry != in.Entry {
			return out, errors.New("Close request identity mismatch")
		}
		return s.reconcileAccountClose(ctx, ex, client, previous)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return out, errors.New("Could not verify the earlier close request")
	}
	pending, err := closes.Unconfirmed(key, in.Symbol, in.Side)
	if err != nil {
		return out, err
	}
	for i := range pending {
		mayHaveSubmitted = true
		if _, err := s.reconcileAccountClose(ctx, ex, client, &pending[i]); err != nil {
			return out, err
		}
	}
	positions, err := client.GetPositions()
	if err != nil {
		return out, errors.New("Could not verify the current exchange position; no close was sent")
	}
	quantity, err := reviewedCloseQuantity(positions, in)
	if err != nil {
		return out, err
	}
	side := "short"
	if in.Side == "short" {
		side = "long"
	}
	out = account.Order{ClientID: strings.ReplaceAll(id.String(), "-", ""), Symbol: in.Symbol, Role: "entry", Type: "market", Side: side, Price: account.Decimal(in.Entry), Quantity: account.Decimal(quantity), ReduceOnly: true, Status: "submitting", Filled: "0"}
	out.VenueClientID, err = client.RouteClientID(out.ClientID)
	if err != nil {
		return out, err
	}
	intent := futures.AccountClose{ID: in.RequestID, UserID: user, AccountID: in.AccountID, AccountKey: key, Symbol: in.Symbol, Side: in.Side, Quantity: in.Quantity, Entry: in.Entry, Status: futures.AccountCloseUnconfirmed, Order: out, UpdatedAt: time.Now().UTC()}
	if err := closes.Create(&intent); err != nil {
		return out, errors.New("Could not record close intent; no order was sent")
	}
	mayHaveSubmitted = true
	out, err = client.SubmitOrder(ctx, out)
	if err != nil {
		return out, errors.New("The exchange close is unconfirmed. Check the position and exchange order history before retrying; no automatic repeat was sent")
	}
	if err := s.saveAccountClose(ex, &intent, out); err != nil {
		return out, err
	}
	return out, nil
}

func reviewedCloseQuantity(positions []map[string]interface{}, in accountCloseRequest) (string, error) {
	maximum, _ := strconv.ParseFloat(in.Quantity, 64)
	entry, _ := strconv.ParseFloat(in.Entry, 64)
	for _, p := range positions {
		if p["symbol"] != in.Symbol || p["side"] != in.Side {
			continue
		}
		qty, ok := p["positionAmt"].(float64)
		price, priceOK := p["entryPrice"].(float64)
		qty = math.Abs(qty)
		if !ok || !priceOK || math.IsNaN(qty) || math.IsInf(qty, 0) || qty <= 0 || qty > maximum || math.IsNaN(price) || math.Abs(price-entry) > math.Max(1, entry)*1e-12 {
			return "", errors.New("Position changed. Refresh and review its current size before closing")
		}
		return strconv.FormatFloat(qty, 'f', -1, 64), nil
	}
	return "", errors.New("Position is no longer open; no close was sent")
}

func (s *Server) saveAccountClose(ex *account.Account, intent *futures.AccountClose, order account.Order) error {
	known := intent.Order.OrderID != ""
	intent.Order, intent.UpdatedAt = order, time.Now().UTC()
	if run.OrderEnded(order.Status) {
		intent.Status = futures.AccountCloseConfirmed
	}
	if err := s.stores.AccountCloses.Save(intent); err != nil {
		return errors.New("Close receipt could not be saved; check the exchange before another close")
	}
	if !known {
		run.RecordManualClose(s.stores.DB, ex.Venue, ex.ID, intent.ID, intent.Side, order)
	}
	return nil
}

func (s *Server) reconcileAccountClose(ctx context.Context, ex *account.Account, client accountCloser, intent *futures.AccountClose) (account.Order, error) {
	if intent.Status == futures.AccountCloseConfirmed {
		return intent.Order, nil
	}
	result, err := client.LookupOrder(ctx, intent.Order)
	if err != nil || !run.OrderEnded(result.Status) {
		return intent.Order, errors.New("An earlier close is still unconfirmed. Check exchange order history; it will not be sent again")
	}
	return result, s.saveAccountClose(ex, intent, result)
}
