package binance

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/adshao/go-binance/v2/common"
	"github.com/adshao/go-binance/v2/futures"
	"math/big"
	"net/http"
	"time"
	"vergex/internal/account"
	"vergex/internal/service"
)

// RouteClientID retains all 128 bits of the durable ID behind the system's
// client order prefix. Raw URL base64 needs 22 characters, so the complete
// tagged ID fits the existing 32-character bound without truncation.
func (b *Account) RouteClientID(id string) (string, error) {
	return routeClientID(clientOrderIDPrefix, id)
}

func routeClientID(prefix, id string) (string, error) {
	raw, e := hex.DecodeString(id)
	if e != nil || len(raw) != 16 {
		return "", errors.New("invalid execution client ID")
	}
	return prefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func (b *Account) nativeClientID(o account.Order) (string, error) {
	if o.VenueClientID == "" {
		return o.ClientID, nil
	} // Historical untagged orders: query their original ID only.
	expected, e := b.RouteClientID(o.ClientID)
	if e != nil {
		return "", errors.New("persisted Broker route mismatch")
	}
	if expected != o.VenueClientID {
		return "", errors.New("persisted Broker route mismatch")
	}
	return expected, nil
}

func (b *Account) Venue() string { return "binance" }

// Testnet says whether the account is on Binance's test environment.
func (b *Account) Testnet() bool { return b.testnet }

// testnetBaseURL is Binance's test environment for USD-M futures. Its money is not real.
const testnetBaseURL = "https://demo-fapi.binance.com"

// NewExecutionTrader initializes Vergex without changing exchange account settings.
func NewExecutionTrader(key, secret string) *Account {
	c := futures.NewClient(key, secret)
	c.HTTPClient = &http.Client{Timeout: 30 * time.Second, Transport: service.VenueTransport}
	return &Account{client: c, cacheDuration: 15 * time.Second}
}

// onEnvironment points the account at the test environment when it is on one.
func (b *Account) onEnvironment(testnet bool) *Account {
	if testnet {
		b.client.BaseURL = testnetBaseURL
		b.testnet = true
	}
	return b
}

// NewReadOnlyAccount calibrates signed reads without changing position mode,
// leverage, margin mode, or orders. Callers replace long-lived cached clients.
func NewReadOnlyAccount(ctx context.Context, key, secret string, testnet bool) (*Account, error) {
	b := NewExecutionTrader(key, secret).onEnvironment(testnet)
	if err := b.calibrateClock(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Account) calibrateClock(ctx context.Context) error {
	start := time.Now().UnixMilli()
	serverTime, err := b.client.NewServerTimeService().Do(ctx)
	if err != nil {
		return binanceReadError("synchronize the Binance clock", err)
	}
	b.client.TimeOffset = (start+time.Now().UnixMilli())/2 - serverTime
	return nil
}

// Never expose signed request URLs or upstream credential-bearing errors.
func binanceReadError(action string, err error) error {
	var apiErr *common.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case -1021:
			return errors.New("Binance rejected the request timestamp. Refresh the connection to synchronize the server clock (code -1021).")
		case -2014, -2015:
			return fmt.Errorf("Binance rejected account access. Check the API key, server IP allowlist and Futures permissions (code %d).", apiErr.Code)
		case -1022:
			return errors.New("Binance rejected the API signature. Check the saved API secret (code -1022).")
		default:
			return fmt.Errorf("Could not %s (Binance code %d).", action, apiErr.Code)
		}
	}
	return fmt.Errorf("Could not %s. Check the Vergex server connection and try again.", action)
}

// orderStatus is Binance's order status in the terms a run and the ledger know. An order
// expired in matching, as self-trade prevention does, has ended like any other expiry.
// Any other status is an error rather than a guess, which callers treat as unconfirmed.
func orderStatus(status futures.OrderStatusType) (string, error) {
	switch status {
	case futures.OrderStatusTypeNew, futures.OrderStatusTypePartiallyFilled, futures.OrderStatusTypeFilled,
		futures.OrderStatusTypeCanceled, futures.OrderStatusTypeRejected, futures.OrderStatusTypeExpired:
		return string(status), nil
	case "EXPIRED_IN_MATCH":
		return string(futures.OrderStatusTypeExpired), nil
	}
	return "", fmt.Errorf("Binance reported an order status Vergex does not know: %s", status)
}

// binanceNoSuchOrder is how Binance says it has no such order.
const binanceNoSuchOrder = -2013

// binanceRefusal says whether an error is Binance clearly not taking an order, with its
// reason. Its own codes for a lost connection, a timeout and an overloaded server leave the
// order's fate open, and so does any failure to hear from it at all.
func binanceRefusal(err error) (string, bool) {
	var apiErr *common.APIError
	if !errors.As(err, &apiErr) {
		return "", false
	}
	switch apiErr.Code {
	case 0, -1000, -1001, -1006, -1007, -1008:
		return "", false
	}
	return fmt.Sprintf("%s (Binance code %d)", apiErr.Message, apiErr.Code), true
}

// CancelOpenOrder cancels an order known only by what the exchange lists. A conditional one
// lives among Binance's algo orders under its algo ID.
func (b *Account) CancelOpenOrder(ctx context.Context, symbol, orderID string, conditional bool) error {
	id, ok := new(big.Int).SetString(orderID, 10)
	if !ok || !id.IsInt64() {
		return errors.New("invalid exchange order ID")
	}
	var err error
	if conditional {
		_, err = b.client.NewCancelAlgoOrderService().AlgoID(id.Int64()).Do(ctx)
	} else {
		_, err = b.client.NewCancelOrderService().Symbol(symbol).OrderID(id.Int64()).Do(ctx)
	}
	if err != nil {
		return binanceReadError("cancel the order", err)
	}
	return nil
}

func ratString(s string) (*big.Rat, error) {
	n, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, errors.New("invalid exchange decimal")
	}
	return n, nil
}
func sameNativeDecimal(a, b string) bool {
	x, e := ratString(a)
	if e != nil {
		return false
	}
	y, e := ratString(b)
	return e == nil && x.Cmp(y) == 0
}
func (b *Account) AccountSnapshot(ctx context.Context, symbol string) (account.Snapshot, error) {
	var out account.Snapshot
	out.Position = "0"
	acct, e := b.client.NewGetAccountService().Do(ctx)
	if e != nil {
		return out, binanceReadError("read the Binance account", e)
	}
	if !acct.CanTrade || acct.MultiAssetsMargin {
		return out, errors.New("Account cannot trade or uses unsupported multi-asset margin")
	}
	out.Equity = account.Decimal(acct.TotalMarginBalance)
	out.Available = account.Decimal(acct.AvailableBalance)
	qty := new(big.Rat)
	for _, p := range acct.Positions {
		if p.Symbol == symbol {
			n, e := ratString(p.PositionAmt)
			if e != nil {
				return out, e
			}
			qty.Add(qty, n.Abs(n))
		}
	}
	out.Position = account.Decimal(qty.FloatString(12))
	orders, e := b.client.NewListOpenOrdersService().Symbol(symbol).Do(ctx)
	if e != nil {
		return out, binanceReadError("read Binance open orders", e)
	}
	algos, e := b.client.NewListOpenAlgoOrdersService().Symbol(symbol).Do(ctx)
	if e != nil {
		return out, binanceReadError("read Binance protective orders", e)
	}
	out.OpenOrders = len(orders) + len(algos)
	fee, e := b.client.NewCommissionRateService().Symbol(symbol).Do(ctx)
	if e != nil {
		return out, binanceReadError("read Binance actual fees", e)
	}
	maker, e := ratString(fee.MakerCommissionRate)
	if e != nil {
		return out, e
	}
	taker, e := ratString(fee.TakerCommissionRate)
	if e != nil {
		return out, e
	}
	if maker.Cmp(taker) > 0 {
		taker = maker
	}
	out.FeeBPS = account.Decimal(new(big.Rat).Mul(taker, big.NewRat(10000, 1)).FloatString(12))
	return out, nil
}
func (b *Account) PrepareExecution(ctx context.Context, symbol string) error {
	return b.PrepareExecutionLeverage(ctx, symbol, 1)
}
func (b *Account) PrepareExecutionLeverage(ctx context.Context, symbol string, leverage int) error {
	if leverage < 1 || leverage > 20 {
		return errors.New("unsupported planning leverage")
	}
	_, e := b.client.NewGetPositionModeService().Do(ctx)
	if e != nil {
		return errors.New("cannot verify Binance position mode")
	}
	result, e := b.client.NewChangeLeverageService().Symbol(symbol).Leverage(leverage).Do(ctx)
	if e != nil || result == nil || result.Leverage != leverage {
		return errors.New("cannot set the approved leverage; order blocked")
	}
	return nil
}
func executionPosition(side string, hedge bool) futures.PositionSideType {
	if !hedge {
		return futures.PositionSideTypeBoth
	}
	if side == "long" {
		return futures.PositionSideTypeLong
	}
	return futures.PositionSideTypeShort
}
func (b *Account) SubmitOrder(ctx context.Context, o account.Order) (account.Order, error) {
	if o.Type != "" && (o.Type != "market" || o.Role != "entry" || o.PostOnly) {
		return o, errors.New("unsupported execution order type")
	}
	nativeID, err := b.nativeClientID(o)
	if err != nil {
		return o, err
	}
	if o.VenueClientID == "" {
		return o, errors.New("Broker route must be persisted before submitting a new order")
	}

	if len(o.ClientID) != 32 || !account.ValidOrderPrice(o) || !account.Positive(o.Quantity) || (o.Side != "long" && o.Side != "short") {
		return o, errors.New("invalid locked order")
	}
	mode, e := b.client.NewGetPositionModeService().Do(ctx)
	if e != nil {
		return o, errors.New("position mode unavailable")
	}
	side := futures.SideTypeBuy
	if o.Side == "short" {
		side = futures.SideTypeSell
	}
	if o.Role != "entry" {
		if side == futures.SideTypeBuy {
			side = futures.SideTypeSell
		} else {
			side = futures.SideTypeBuy
		}
	}
	if o.Role == "entry" {
		tif := futures.TimeInForceTypeGTC
		if o.PostOnly {
			tif = futures.TimeInForceTypeGTX
		}
		positionSide := executionPosition(o.Side, mode.DualSidePosition)
		if o.ReduceOnly && mode.DualSidePosition {
			if o.Side == "long" {
				positionSide = futures.PositionSideTypeShort
			} else {
				positionSide = futures.PositionSideTypeLong
			}
		}
		orderType := futures.OrderTypeLimit
		service := b.client.NewCreateOrderService().Symbol(o.Symbol).Side(side).PositionSide(positionSide).Quantity(string(o.Quantity)).NewClientOrderID(nativeID)
		if o.Type == "market" {
			orderType = futures.OrderTypeMarket
			if o.ReduceOnly {
				service.NewOrderResponseType(futures.NewOrderRespTypeRESULT)
			}
		} else {
			service.TimeInForce(tif).Price(string(o.Price))
		}
		service.Type(orderType)
		if o.ReduceOnly && !mode.DualSidePosition {
			service.ReduceOnly(true)
		}
		r, e := service.Do(ctx)
		if e != nil {
			return o, errors.New("Binance entry submission is unconfirmed; reconcile by client ID")
		}
		limitMismatch := o.Type != "market" && (!sameNativeDecimal(r.Price, string(o.Price)) || r.TimeInForce != tif)
		if r.OrderID <= 0 || r.ClientOrderID != nativeID || r.Symbol != o.Symbol || !sameNativeDecimal(r.OrigQuantity, string(o.Quantity)) || limitMismatch || r.Side != side || r.Type != orderType || r.PositionSide != positionSide || (o.ReduceOnly && !mode.DualSidePosition && !r.ReduceOnly) {
			return o, errors.New("invalid Binance order receipt")
		}
		status, e := orderStatus(r.Status)
		if e != nil {
			return o, e
		}
		o.OrderID = fmt.Sprint(r.OrderID)
		o.Status = status
		o.Filled = account.Decimal(r.ExecutedQuantity)
		o.Average = account.Decimal(r.AvgPrice)
		return o, nil
	}
	typ := futures.AlgoOrderTypeStopMarket
	if o.Role == "tp" {
		typ = futures.AlgoOrderTypeTakeProfitMarket
	} else if o.Role != "sl" {
		return o, errors.New("unsupported protective order")
	}
	service := b.client.NewCreateAlgoOrderService().Symbol(o.Symbol).Side(side).PositionSide(executionPosition(o.Side, mode.DualSidePosition)).Type(typ).Quantity(string(o.Quantity)).TriggerPrice(string(o.Price)).WorkingType(futures.WorkingTypeContractPrice).ClientAlgoId(nativeID)
	if !mode.DualSidePosition {
		service.ReduceOnly(true)
	}
	r, e := service.Do(ctx)
	if e != nil {
		if reason, refused := binanceRefusal(e); refused {
			return o, account.RefusedOrder{Reason: reason}
		}
		return o, errors.New("Binance protective order submission is unconfirmed")
	}
	if r.AlgoId <= 0 || r.ClientAlgoId != nativeID || r.Symbol != o.Symbol || !sameNativeDecimal(r.Quantity, string(o.Quantity)) || !sameNativeDecimal(r.TriggerPrice, string(o.Price)) || r.Side != side || r.OrderType != typ || (!mode.DualSidePosition && !r.ReduceOnly) {
		return o, errors.New("invalid Binance protection receipt")
	}
	o.OrderID = fmt.Sprint(r.AlgoId)
	o.Status = "NEW"
	o.Filled = "0"
	return o, nil
}
func (b *Account) LookupOrder(ctx context.Context, o account.Order) (account.Order, error) {
	nativeID, err := b.nativeClientID(o)
	if err != nil {
		return o, err
	}

	if o.Role != "entry" {
		r, e := b.client.NewGetAlgoOrderService().ClientAlgoID(nativeID).Do(ctx)
		if e != nil {
			var apiErr *common.APIError
			if errors.As(e, &apiErr) && apiErr.Code == binanceNoSuchOrder {
				return o, account.ErrOrderNotFound
			}
			return o, errors.New("Binance protective order lookup is unconfirmed")
		}
		if r.AlgoId <= 0 || r.ClientAlgoId != nativeID || r.Symbol != o.Symbol || !sameNativeDecimal(r.Quantity, string(o.Quantity)) || !sameNativeDecimal(r.TriggerPrice, string(o.Price)) {
			return o, errors.New("protection identity mismatch")
		}
		o.OrderID = fmt.Sprint(r.AlgoId)
		if r.ActualOrderId != "" && r.ActualOrderId != "0" {
			id, ok := new(big.Int).SetString(r.ActualOrderId, 10)
			if !ok || !id.IsInt64() {
				return o, errors.New("invalid triggered order ID")
			}
			fill, e := b.client.NewGetOrderService().Symbol(o.Symbol).OrderID(id.Int64()).Do(ctx)
			if e != nil {
				return o, errors.New("triggered fill not yet confirmed")
			}
			status, e := orderStatus(fill.Status)
			if e != nil {
				return o, e
			}
			o.Status = status
			o.Filled = account.Decimal(fill.ExecutedQuantity)
			o.Average = account.Decimal(fill.AvgPrice)
			return o, nil
		}
		switch r.AlgoStatus {
		case "NEW":
			o.Status = "NEW"
		case "CANCELED":
			o.Status = "CANCELED"
		case "REJECTED":
			o.Status = "REJECTED"
		case "EXPIRED":
			o.Status = "EXPIRED"
		default:
			return o, errors.New("unknown Binance algo state")
		}
		return o, nil
	}
	r, e := b.client.NewGetOrderService().Symbol(o.Symbol).OrigClientOrderID(nativeID).Do(ctx)
	if e != nil {
		return o, errors.New("Binance order lookup is unconfirmed; missing does not mean filled")
	}
	priceMismatch := o.Type != "market" && !sameNativeDecimal(r.Price, string(o.Price))
	marketMismatch := o.Type == "market" && (r.Type != futures.OrderTypeMarket || (o.Side == "long" && r.Side != futures.SideTypeBuy) || (o.Side == "short" && r.Side != futures.SideTypeSell))
	if r.OrderID <= 0 || r.ClientOrderID != nativeID || r.Symbol != o.Symbol || !sameNativeDecimal(r.OrigQuantity, string(o.Quantity)) || priceMismatch || marketMismatch {
		return o, errors.New("order identity mismatch")
	}
	status, e := orderStatus(r.Status)
	if e != nil {
		return o, e
	}
	o.OrderID = fmt.Sprint(r.OrderID)
	o.Status = status
	o.Filled = account.Decimal(r.ExecutedQuantity)
	o.Average = account.Decimal(r.AvgPrice)
	return o, nil
}
func (b *Account) CancelExecutionOrder(ctx context.Context, o account.Order) error {
	nativeID, err := b.nativeClientID(o)
	if err != nil {
		return err
	}

	if o.Role == "entry" {
		r, e := b.client.NewCancelOrderService().Symbol(o.Symbol).OrigClientOrderID(nativeID).Do(ctx)
		if e != nil || r.ClientOrderID != nativeID {
			return errors.New("Binance entry cancellation is unconfirmed")
		}
		return nil
	}
	r, e := b.client.NewCancelAlgoOrderService().ClientAlgoID(nativeID).Do(ctx)
	if e != nil || r.ClientAlgoId != nativeID {
		return errors.New("Binance protective order cancellation is unconfirmed")
	}
	return nil
}

var (
	_ account.Executor       = (*Account)(nil)
	_ account.OrderCanceller = (*Account)(nil)
)
