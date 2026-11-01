package api

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"vergex/internal/trading"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"vergex/internal/exchange/binance"
	"vergex/internal/exchange/okx"
	"vergex/internal/planning/instrument"
	"vergex/internal/planning/live"
	"vergex/internal/store"
)

type accountCloseRequest struct {
	ExchangeID string `json:"exchange_id"`
	RequestID  string `json:"request_id"`
	Symbol     string `json:"symbol"`
	Side       string `json:"side"`
	Quantity   string `json:"quantity"`
	Entry      string `json:"entry_price"`
}

type accountCloser interface {
	GetPositions() ([]map[string]interface{}, error)
	RouteClientID(string) (string, error)
	SubmitOrder(context.Context, trading.Order) (trading.Order, error)
	LookupOrder(context.Context, trading.Order) (trading.Order, error)
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

func (s *Server) handlePlanningAccountClose(c *gin.Context) {
	var in accountCloseRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&in) != nil {
		writeAccountCloseError(c, closeNotSubmittedError{errors.New("Invalid position close request; no close was sent")})
		return
	}
	p, err := s.planningService()
	if err != nil {
		writeAccountCloseError(c, closeNotSubmittedError{err})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	user := c.GetString("user_id")
	order, err := s.closeFuturesAccount(ctx, user, in, func(ex *store.Exchange) (accountCloser, error) {
		if ex.ExchangeType == "okx" {
			return okx.NewAccountTrader(string(ex.APIKey), string(ex.SecretKey), string(ex.Passphrase)), nil
		}
		return binance.NewAccountTrader(ctx, string(ex.APIKey), string(ex.SecretKey))
	})
	if err != nil {
		writeAccountCloseError(c, err)
		return
	}
	message := "Close order accepted. Verify the refreshed position and remaining exchange orders."
	if order.Status == "FILLED" {
		runs, readErr := p.live.List(user)
		if readErr != nil {
			message += " Execution records could not be refreshed."
		}
		for _, run := range runs {
			if run.State == "completed" || run.State == "cancelled" || run.State == "expired" || run.ExchangeID != in.ExchangeID || run.Plan.Snapshot.Symbol != in.Symbol || run.Plan.Idea.Side != in.Side {
				continue
			}
			if _, err := p.live.FinishManualClose(ctx, user, run.ID); err != nil {
				message += " " + err.Error()
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "order": order})
}

func (s *Server) closeFuturesAccount(ctx context.Context, user string, in accountCloseRequest, factory func(*store.Exchange) (accountCloser, error)) (out trading.Order, err error) {
	// Only a definite pre-submission failure may release the UI's close latch.
	// Existing durable intents and ambiguous submissions must stay protected.
	mayHaveSubmitted := false
	defer func() {
		if err != nil && !mayHaveSubmitted {
			err = closeNotSubmittedError{err}
		}
	}()
	id, err := uuid.Parse(in.RequestID)
	if err != nil || id == uuid.Nil || !instrument.ValidSymbol(in.Symbol) || (in.Side != "long" && in.Side != "short") || !trading.Positive(trading.Decimal(in.Quantity)) || !trading.ValidOrderPrice(trading.Order{Role: "entry", Type: "market", ReduceOnly: true, Price: trading.Decimal(in.Entry)}) {
		return out, errors.New("Review a valid position and quantity before closing")
	}
	in.RequestID = id.String()
	ex, err := s.accountReadConfig(user, "", in.ExchangeID)
	if err != nil {
		return out, err
	}
	if in.ExchangeID == "" || (ex.ExchangeType != "binance" && ex.ExchangeType != "okx") {
		return out, errors.New("Select a Binance or OKX USDT futures account")
	}
	key := ex.Identity
	unlock := store.LockAccount(ex.ID)
	defer unlock()
	traders, err := s.store.Trader().ListByExchangeID(user, ex.ID)
	if err != nil {
		return out, errors.New("Could not verify trader state")
	}
	for _, trader := range traders {
		running := trader.IsRunning
		if s.traderManager != nil {
			if runtime, e := s.traderManager.GetTrader(trader.ID); e == nil {
				active, _ := runtime.GetStatus()["is_running"].(bool)
				running = running || active
			}
		}
		if running {
			return out, errors.New("Stop the trader on this account before closing manually")
		}
	}
	if err := live.CheckCloseEntries(s.store, key, in.Symbol); err != nil {
		return out, err
	}
	client, err := factory(ex)
	if err != nil {
		return out, err
	}
	closes := s.store.AccountClose()
	previous, err := closes.Get(in.RequestID)
	if err == nil {
		mayHaveSubmitted = true
		if previous.UserID != user || previous.ExchangeID != in.ExchangeID || previous.Symbol != in.Symbol || previous.Side != in.Side || previous.Quantity != in.Quantity || previous.Entry != in.Entry {
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
	out = trading.Order{ClientID: strings.ReplaceAll(id.String(), "-", ""), Symbol: in.Symbol, Role: "entry", Type: "market", Side: side, Price: trading.Decimal(in.Entry), Quantity: trading.Decimal(quantity), ReduceOnly: true, Status: "submitting", Filled: "0"}
	out.VenueClientID, err = client.RouteClientID(out.ClientID)
	if err != nil {
		return out, err
	}
	intent := store.AccountClose{ID: in.RequestID, UserID: user, ExchangeID: in.ExchangeID, AccountKey: key, Symbol: in.Symbol, Side: in.Side, Quantity: in.Quantity, Entry: in.Entry, State: store.AccountCloseUnconfirmed, Order: out, UpdatedAt: time.Now().UTC()}
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

func (s *Server) saveAccountClose(ex *store.Exchange, intent *store.AccountClose, order trading.Order) error {
	known := intent.Order.OrderID != ""
	intent.Order, intent.UpdatedAt = order, time.Now().UTC()
	if live.OrderEnded(order.Status) {
		intent.State = store.AccountCloseConfirmed
	}
	if err := s.store.AccountClose().Save(intent); err != nil {
		return errors.New("Close receipt could not be saved; check the exchange before another close")
	}
	if !known {
		live.RecordManualClose(s.store, ex.ExchangeType, ex.ID, intent.ID, intent.Side, order)
	}
	return nil
}

func (s *Server) reconcileAccountClose(ctx context.Context, ex *store.Exchange, client accountCloser, intent *store.AccountClose) (trading.Order, error) {
	if intent.State == store.AccountCloseConfirmed {
		return intent.Order, nil
	}
	result, err := client.LookupOrder(ctx, intent.Order)
	if err != nil || !live.OrderEnded(result.Status) {
		return intent.Order, errors.New("An earlier close is still unconfirmed. Check exchange order history; it will not be sent again")
	}
	return result, s.saveAccountClose(ex, intent, result)
}
