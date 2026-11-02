package okx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"vergex/internal/account"
)

var _ account.Executor = (*Account)(nil)

func (t *Account) Venue() string                           { return "okx" }
func (t *Account) RouteClientID(id string) (string, error) { return account.CanonicalClientID(id) }
func (t *Account) readExecution(ctx context.Context, path string, out any) error {
	b, e := t.doRequestContext(ctx, "GET", path, nil)
	if e != nil {
		return e
	}
	if string(b) == "null" {
		return errors.New("missing OKX data")
	}
	return json.Unmarshal(b, out)
}

type exactInstrument struct{ InstId, InstFamily, GroupId, InstType, State, CtType, SettleCcy, CtValCcy, CtVal, CtMult, LotSz, MinSz, TickSz, MaxMktSz, MaxLmtSz string }

func (t *Account) executionInstrument(ctx context.Context, symbol string) (exactInstrument, error) {
	var rows []exactInstrument
	e := t.readExecution(ctx, okxInstrumentsPath+"?instType=SWAP&instId="+url.QueryEscape(symbol), &rows)
	if e != nil {
		return exactInstrument{}, e
	}
	if len(rows) != 1 {
		return exactInstrument{}, errors.New("OKX instrument not found")
	}
	r := rows[0]
	if r.InstId != symbol || !strings.HasSuffix(symbol, "-USDT-SWAP") || r.InstType != "SWAP" || r.State != "live" || r.CtType != "linear" || r.SettleCcy != "USDT" || r.CtValCcy != strings.TrimSuffix(symbol, "-USDT-SWAP") || (r.CtMult != "" && !account.Equal(r.CtMult, "1")) || !account.Positive(account.Decimal(r.CtVal)) {
		return r, errors.New("Futures trading requires active OKX USDT linear perpetuals")
	}
	return r, nil
}
func (t *Account) executionMode(ctx context.Context) (string, error) {
	var rows []struct{ PosMode, AcctLv string }
	if e := t.readExecution(ctx, okxAccountConfigPath, &rows); e != nil {
		return "", e
	}
	if len(rows) != 1 || (rows[0].PosMode != "net_mode" && rows[0].PosMode != "long_short_mode") {
		return "", errors.New("unknown OKX position mode")
	}
	if rows[0].AcctLv != "2" && rows[0].AcctLv != "3" {
		return "", errors.New("Futures trading supports OKX futures and multi-currency margin accounts; portfolio margin is not supported")
	}
	return rows[0].PosMode, nil
}

type executionPosition struct{ InstId, Pos, PosSide, MgnMode, AvgPx, MarkPx, Upl, Lever, LiqPx string }

func (t *Account) executionPositions(ctx context.Context, symbol string) ([]executionPosition, error) {
	var rows []executionPosition
	e := t.readExecution(ctx, okxPositionPath+"?instType=SWAP&instId="+url.QueryEscape(symbol), &rows)
	return rows, e
}

type executionReceipt struct {
	OrdId, ClOrdId, AlgoId, AlgoClOrdId, InstId, Side, PosSide, OrdType, Px, Sz, AccFillSz, AvgPx, State, SlTriggerPx, TpTriggerPx, TriggerPx, SlTriggerPxType, TpTriggerPxType, SlOrdPx, TpOrdPx string
	OrdIdList                                                                                                                                                                                     []string
	ReduceOnly                                                                                                                                                                                    json.RawMessage
}

func (t *Account) pendingExecution(ctx context.Context, symbol string) ([]executionReceipt, error) {
	var result []executionReceipt
	if e := t.readExecution(ctx, okxPendingOrdersPath+"?instType=SWAP&instId="+url.QueryEscape(symbol), &result); e != nil {
		return nil, e
	}
	for _, kind := range []string{"conditional", "oco", "trigger", "move_order_stop"} {
		var rows []executionReceipt
		if e := t.readExecution(ctx, okxAlgoPendingPath+"?instType=SWAP&instId="+url.QueryEscape(symbol)+"&ordType="+kind, &rows); e != nil {
			return nil, e
		}
		result = append(result, rows...)
	}
	return result, nil
}
func (t *Account) AccountSnapshot(ctx context.Context, symbol string) (account.Snapshot, error) {
	var out account.Snapshot
	inst, e := t.executionInstrument(ctx, symbol)
	if e != nil {
		return out, e
	}
	if _, e = t.executionMode(ctx); e != nil {
		return out, e
	}
	var balances []struct {
		Details []struct{ Ccy, Eq, AvailEq, AvailBal string }
	}
	if e = t.readExecution(ctx, okxAccountPath+"?ccy=USDT", &balances); e != nil {
		return out, e
	}
	found := false
	for _, b := range balances {
		for _, d := range b.Details {
			if d.Ccy == "USDT" {
				out.Equity = account.Decimal(d.Eq)
				available := d.AvailEq
				if available == "" {
					available = d.AvailBal
				}
				out.Available = account.Decimal(available)
				found = true
			}
		}
	}
	if !found {
		return out, errors.New("OKX USDT trading balance unavailable")
	}
	if out.Equity, e = account.BalanceDecimal(string(out.Equity)); e != nil {
		return out, fmt.Errorf("invalid OKX account equity: %w", e)
	}
	if out.Available, e = account.BalanceDecimal(string(out.Available)); e != nil {
		return out, fmt.Errorf("invalid OKX available margin: %w", e)
	}
	var fees []struct {
		Taker, TakerU string
		FeeGroup      []struct{ GroupId, Taker string }
	}
	family := inst.InstFamily
	if family == "" {
		family = strings.TrimSuffix(symbol, "-SWAP")
	}
	if e = t.readExecution(ctx, "/api/v5/account/trade-fee?instType=SWAP&instFamily="+url.QueryEscape(family), &fees); e != nil {
		return out, e
	}
	if len(fees) != 1 {
		return out, errors.New("OKX fee rate unavailable")
	}
	fee := fees[0].TakerU
	if fee == "" {
		fee = fees[0].Taker
	}
	if len(fees[0].FeeGroup) > 0 {
		fee = ""
		for _, group := range fees[0].FeeGroup {
			if group.GroupId == inst.GroupId || (inst.GroupId == "" && len(fees[0].FeeGroup) == 1) {
				fee = group.Taker
				break
			}
		}
		if fee == "" {
			return out, errors.New("OKX instrument fee group unavailable")
		}
	}
	f, e := account.Number(fee)
	if e != nil {
		return out, e
	}
	f.Abs(f)
	out.FeeBPS, e = account.ExactDecimal(f.Mul(f, big.NewRat(10000, 1)))
	if e != nil {
		return out, e
	}
	rows, e := t.executionPositions(ctx, symbol)
	if e != nil {
		return out, e
	}
	gross := new(big.Rat)
	for _, p := range rows {
		if p.InstId != symbol {
			continue
		}
		n, e := account.Number(p.Pos)
		if e != nil {
			return out, e
		}
		gross.Add(gross, n.Abs(n))
	}
	contracts, e := account.ExactDecimal(gross)
	if e != nil {
		return out, e
	}
	out.Position, e = account.Multiply(string(contracts), inst.CtVal)
	if e != nil {
		return out, e
	}
	orders, e := t.pendingExecution(ctx, symbol)
	if e != nil {
		return out, e
	}
	out.OpenOrders = len(orders)
	return out, nil
}
func (t *Account) PrepareExecution(ctx context.Context, symbol string) error {
	return t.PrepareExecutionLeverage(ctx, symbol, 1)
}
func (t *Account) PrepareExecutionLeverage(ctx context.Context, symbol string, leverage int) error {
	if leverage < 1 || leverage > 125 {
		return errors.New("invalid OKX leverage")
	}
	a, e := t.AccountSnapshot(ctx, symbol)
	if e != nil {
		return e
	}
	if !account.Equal(string(a.Position), "0") || a.OpenOrders != 0 {
		return errors.New("OKX market already has a position or pending orders")
	}
	// Cross leverage is configured per instrument, in both position modes.
	body := map[string]string{"instId": symbol, "lever": strconv.Itoa(leverage), "mgnMode": "cross"}
	raw, e := t.doRequestContext(ctx, "POST", okxLeveragePath, body)
	if e != nil {
		return e
	}
	var rows []struct{ Lever string }
	if json.Unmarshal(raw, &rows) != nil || len(rows) == 0 {
		return errors.New("OKX leverage confirmation missing")
	}
	for _, r := range rows {
		if !account.Equal(r.Lever, strconv.Itoa(leverage)) {
			return errors.New("OKX did not confirm approved leverage")
		}
	}
	return nil
}
func okxSide(o account.Order) (side, held string) {
	side = "buy"
	if o.Side == "short" {
		side = "sell"
	}
	held = o.Side
	if o.Role != "entry" {
		if side == "buy" {
			side = "sell"
		} else {
			side = "buy"
		}
	}
	if o.ReduceOnly {
		held = "short"
		if o.Side == "short" {
			held = "long"
		}
	}
	return
}
func (t *Account) SubmitOrder(ctx context.Context, o account.Order) (account.Order, error) {
	if e := account.ValidateIntent(o); e != nil {
		return o, e
	}
	inst, e := t.executionInstrument(ctx, o.Symbol)
	if e != nil {
		return o, e
	}
	sz, e := account.Divide(string(o.Quantity), inst.CtVal)
	if e != nil {
		return o, e
	}
	if !account.Aligned(string(sz), inst.LotSz) {
		return o, errors.New("quantity is not an exact OKX contract increment")
	}
	n, _ := account.Number(string(sz))
	min, e := account.Number(inst.MinSz)
	if e != nil || n.Cmp(min) < 0 {
		return o, errors.New("quantity below OKX contract minimum")
	}
	max := inst.MaxLmtSz
	if o.Type == "market" || o.Role != "entry" {
		max = inst.MaxMktSz
	}
	if max != "" {
		m, e := account.Number(max)
		if e != nil || n.Cmp(m) > 0 {
			return o, errors.New("quantity exceeds OKX order limit")
		}
	}
	if (o.Type != "market" || o.Role != "entry") && !account.Aligned(string(o.Price), inst.TickSz) {
		return o, errors.New("price is not an exact OKX tick")
	}
	mode, e := t.executionMode(ctx)
	if e != nil {
		return o, e
	}
	side, held := okxSide(o)
	margin := "cross"
	if o.ReduceOnly || o.Role != "entry" {
		positions, e := t.executionPositions(ctx, o.Symbol)
		if e != nil {
			return o, e
		}
		available := new(big.Rat)
		found := false
		for _, p := range positions {
			if p.InstId != o.Symbol {
				continue
			}
			v, e := account.Number(p.Pos)
			if e != nil {
				return o, e
			}
			s := p.PosSide
			if s == "net" {
				s = "long"
				if v.Sign() < 0 {
					s = "short"
				}
			}
			if s != held || v.Sign() == 0 {
				continue
			}
			if p.MgnMode != "cross" && p.MgnMode != "isolated" {
				return o, errors.New("unknown OKX margin mode")
			}
			if found && margin != p.MgnMode {
				return o, errors.New("multiple OKX margin modes require exchange review")
			}
			margin = p.MgnMode
			found = true
			available.Add(available, v.Abs(v))
		}
		if !found || n.Cmp(available) > 0 {
			return o, errors.New("OKX position changed; refresh before closing or protecting")
		}
	}
	body := map[string]any{"instId": o.Symbol, "tdMode": margin, "side": side, "sz": string(sz)}
	if mode == "long_short_mode" {
		body["posSide"] = held
	} else {
		body["posSide"] = "net"
		if o.ReduceOnly || o.Role != "entry" {
			body["reduceOnly"] = true
		}
	}
	path := okxOrderPath
	if o.Role == "entry" {
		body["clOrdId"] = o.ClientID
		body["ordType"] = "limit"
		body["px"] = string(o.Price)
		if o.Type == "market" {
			body["ordType"] = "market"
			delete(body, "px")
		} else if o.PostOnly {
			body["ordType"] = "post_only"
		}
	} else {
		path = okxAlgoOrderPath
		body["algoClOrdId"] = o.ClientID
		body["ordType"] = "conditional"
		prefix := o.Role
		body[prefix+"TriggerPx"] = string(o.Price)
		body[prefix+"OrdPx"] = "-1"
		body[prefix+"TriggerPxType"] = "mark"
	}
	data, e := t.doRequestContext(ctx, "POST", path, body)
	if e != nil {
		return o, e
	}
	var rows []struct{ OrdId, AlgoId, ClOrdId, AlgoClOrdId, SCode, SMsg string }
	if json.Unmarshal(data, &rows) != nil || len(rows) != 1 {
		return o, errors.New("unconfirmed OKX order response")
	}
	r := rows[0]
	if r.SCode != "0" {
		return o, fmt.Errorf("OKX order rejected (%s): %s", r.SCode, r.SMsg)
	}
	id, cid := r.OrdId, r.ClOrdId
	if o.Role != "entry" {
		id, cid = r.AlgoId, r.AlgoClOrdId
	}
	if id == "" || (cid != "" && cid != o.ClientID) {
		return o, errors.New("OKX order identity not confirmed")
	}
	o.OrderID = id
	o.Status = "NEW"
	o.Filled = "0"
	return o, nil
}
func (t *Account) LookupOrder(ctx context.Context, o account.Order) (account.Order, error) {
	previousFilled := o.Filled
	if e := account.ValidateIntent(o); e != nil {
		return o, e
	}
	var rows []executionReceipt
	path := okxOrderPath + "?instId=" + url.QueryEscape(o.Symbol) + "&clOrdId=" + url.QueryEscape(o.ClientID)
	if o.Role != "entry" {
		path = okxAlgoOrderPath + "?algoClOrdId=" + url.QueryEscape(o.ClientID)
	}
	if e := t.readExecution(ctx, path, &rows); e != nil {
		return o, e
	}
	if len(rows) != 1 {
		return o, errors.New("OKX order not found; submission remains unconfirmed")
	}
	r := rows[0]
	id, cid := r.OrdId, r.ClOrdId
	if o.Role != "entry" {
		id, cid = r.AlgoId, r.AlgoClOrdId
	}
	if id == "" || cid != o.ClientID || r.InstId != o.Symbol || (o.OrderID != "" && id != o.OrderID) {
		return o, errors.New("OKX receipt identity mismatch")
	}
	inst, e := t.executionInstrument(ctx, o.Symbol)
	if e != nil {
		return o, e
	}
	quantity, e := account.Multiply(r.Sz, inst.CtVal)
	if e != nil || !account.Equal(string(quantity), string(o.Quantity)) {
		return o, errors.New("OKX receipt quantity mismatch")
	}
	side, held := okxSide(o)
	if r.Side != side || (r.PosSide != "net" && r.PosSide != held) {
		return o, errors.New("OKX receipt direction mismatch")
	}
	o.OrderID = id
	o.Filled = "0"
	if o.Role == "entry" {
		expected := "limit"
		if o.Type == "market" {
			expected = "market"
		} else if o.PostOnly {
			expected = "post_only"
		}
		if r.OrdType != expected || expected != "market" && !account.Equal(r.Px, string(o.Price)) {
			return o, errors.New("OKX receipt order terms mismatch")
		}
		o.Status = map[string]string{"live": "NEW", "partially_filled": "PARTIALLY_FILLED", "filled": "FILLED", "canceled": "CANCELED", "mmp_canceled": "CANCELED"}[r.State]
		o.Filled, e = account.Multiply(r.AccFillSz, inst.CtVal)
		if e != nil {
			return o, e
		}
		if account.Positive(o.Filled) {
			if !account.Positive(account.Decimal(r.AvgPx)) {
				return o, errors.New("OKX fill price missing")
			}
			o.Average = account.Decimal(r.AvgPx)
		}
	} else {
		price, trigger, executionPrice := r.SlTriggerPx, r.SlTriggerPxType, r.SlOrdPx
		if o.Role == "tp" {
			price, trigger, executionPrice = r.TpTriggerPx, r.TpTriggerPxType, r.TpOrdPx
		}
		if r.OrdType != "conditional" || !account.Equal(price, string(o.Price)) || trigger != "mark" || executionPrice != "-1" {
			return o, errors.New("OKX protection terms mismatch")
		}
		o.Status = map[string]string{"live": "NEW", "effective": "TRIGGERED", "order_failed": "REJECTED", "canceled": "CANCELED", "pause": "UNKNOWN"}[r.State]
	}
	if (o.ReduceOnly || o.Role != "entry") && r.PosSide == "net" && string(r.ReduceOnly) != "true" && string(r.ReduceOnly) != "\"true\"" {
		return o, errors.New("OKX close or protection was not reduce-only")
	}
	if o.Status == "" {
		return o, errors.New("unrecognized OKX order state")
	}
	if o.Role != "entry" && o.Status == "TRIGGERED" {
		return t.lookupTriggeredProtection(ctx, o, r, inst, previousFilled)
	}
	return o, nil
}

// Triggering an algo creates regular orders. Only their verified fills prove
// an exit; the algo's effective state alone does not confirm a closed position.
func (t *Account) lookupTriggeredProtection(ctx context.Context, o account.Order, parent executionReceipt, inst exactInstrument, previousFilled account.Decimal) (account.Order, error) {
	ids := parent.OrdIdList
	if len(ids) == 0 && parent.OrdId != "" {
		ids = []string{parent.OrdId}
	}
	if len(ids) == 0 {
		return o, nil // The exchange may publish linked order IDs later.
	}
	if len(ids) > 32 {
		return o, errors.New("OKX protection has too many linked orders to verify")
	}
	seen := map[string]bool{}
	filled, total, cost := new(big.Rat), new(big.Rat), new(big.Rat)
	terminal := true
	side, held := okxSide(o)
	for _, id := range ids {
		if id == "" || seen[id] {
			return o, errors.New("invalid OKX protection order linkage")
		}
		seen[id] = true
		var rows []executionReceipt
		if err := t.readExecution(ctx, okxOrderPath+"?instId="+url.QueryEscape(o.Symbol)+"&ordId="+url.QueryEscape(id), &rows); err != nil {
			return o, err
		}
		if len(rows) != 1 {
			return o, errors.New("OKX triggered exit order not yet confirmed")
		}
		r := rows[0]
		if r.OrdId != id || r.InstId != o.Symbol || r.Side != side || (r.PosSide != "net" && r.PosSide != held) || r.OrdType != "market" {
			return o, errors.New("OKX triggered exit identity or direction mismatch")
		}
		if r.PosSide == "net" && string(r.ReduceOnly) != "true" && string(r.ReduceOnly) != "\"true\"" {
			return o, errors.New("OKX triggered exit was not reduce-only")
		}
		quantity, err := account.Multiply(r.Sz, inst.CtVal)
		if err != nil || !account.Positive(quantity) {
			return o, errors.New("invalid OKX triggered exit quantity")
		}
		q, _ := account.Number(string(quantity))
		f, err := account.Multiply(r.AccFillSz, inst.CtVal)
		if err != nil {
			return o, errors.New("invalid OKX triggered exit fill")
		}
		fn, _ := account.Number(string(f))
		if fn.Sign() < 0 || fn.Cmp(q) > 0 {
			return o, errors.New("invalid OKX triggered exit fill")
		}
		switch r.State {
		case "filled":
			if fn.Cmp(q) != 0 {
				return o, errors.New("incomplete OKX triggered exit fill")
			}
		case "live":
			if fn.Sign() != 0 {
				return o, errors.New("unexpected OKX live exit fill")
			}
			terminal = false
		case "partially_filled":
			if fn.Sign() <= 0 || fn.Cmp(q) >= 0 {
				return o, errors.New("invalid OKX partial exit fill")
			}
			terminal = false
		case "canceled", "mmp_canceled":
		default:
			return o, errors.New("unknown OKX triggered exit state")
		}
		if fn.Sign() > 0 {
			price, err := account.Number(r.AvgPx)
			if err != nil || price.Sign() <= 0 {
				return o, errors.New("OKX triggered exit fill price missing")
			}
			cost.Add(cost, new(big.Rat).Mul(fn, price))
		}
		filled.Add(filled, fn)
		total.Add(total, q)
	}
	approved, _ := account.Number(string(o.Quantity))
	if previousFilled == "" {
		previousFilled = "0"
	}
	previous, err := account.Number(string(previousFilled))
	if err != nil || total.Cmp(approved) > 0 || filled.Cmp(approved) > 0 || filled.Cmp(previous) < 0 {
		return o, errors.New("OKX triggered exit quantity mismatch")
	}
	o.Filled, err = account.ExactDecimal(filled)
	if err != nil {
		return o, err
	}
	if filled.Sign() > 0 {
		o.Average = account.Decimal(new(big.Rat).Quo(cost, filled).FloatString(12))
	}
	if filled.Cmp(approved) == 0 {
		o.Status = "FILLED"
	} else if terminal && total.Cmp(approved) == 0 {
		o.Status = "CANCELED"
	}
	return o, nil
}
func (t *Account) CancelExecutionOrder(ctx context.Context, o account.Order) error {
	if e := account.ValidateIntent(o); e != nil {
		return e
	}
	if o.OrderID == "" {
		return errors.New("reconcile OKX order before cancellation")
	}
	path := okxCancelOrderPath
	var body any = map[string]string{"instId": o.Symbol, "ordId": o.OrderID, "clOrdId": o.ClientID}
	if o.Role != "entry" {
		path = okxCancelAlgoPath
		body = []map[string]string{{"instId": o.Symbol, "algoId": o.OrderID}}
	}
	raw, e := t.doRequestContext(ctx, "POST", path, body)
	if e != nil {
		return e
	}
	var rows []struct{ SCode, OrdId, AlgoId string }
	if json.Unmarshal(raw, &rows) != nil || len(rows) != 1 || rows[0].SCode != "0" || (rows[0].OrdId != o.OrderID && rows[0].AlgoId != o.OrderID) {
		return errors.New("OKX cancellation unconfirmed; reconcile order")
	}
	return nil
}

func (t *Account) futuresPositions() ([]map[string]interface{}, error) {
	ctx := context.Background()
	rows, e := t.executionPositions(ctx, "")
	if e != nil {
		return nil, e
	}
	out := []map[string]interface{}{}
	for _, p := range rows {
		v, e := account.Number(p.Pos)
		if e != nil {
			return nil, e
		}
		if v.Sign() == 0 {
			continue
		}
		// Futures only executes these markets; other collateral must never be reinterpreted as USDT.
		if !strings.HasSuffix(p.InstId, "-USDT-SWAP") {
			continue
		}
		inst, e := t.executionInstrument(ctx, p.InstId)
		if e != nil {
			return nil, e
		}
		side := p.PosSide
		if side == "net" {
			side = "long"
			if v.Sign() < 0 {
				side = "short"
			}
		}
		if side != "long" && side != "short" {
			return nil, errors.New("unknown OKX position direction")
		}
		contracts, e := account.ExactDecimal(v.Abs(v))
		if e != nil {
			return nil, e
		}
		qty, e := account.Multiply(string(contracts), inst.CtVal)
		if e != nil {
			return nil, e
		}
		row := map[string]interface{}{"symbol": p.InstId, "side": side, "mgnMode": p.MgnMode}
		for key, value := range map[string]string{"positionAmt": string(qty), "entryPrice": p.AvgPx, "markPrice": p.MarkPx, "unRealizedProfit": p.Upl, "leverage": p.Lever, "liquidationPrice": p.LiqPx} {
			if value == "" && (key == "liquidationPrice" || key == "leverage") {
				value = "0"
			}
			n, e := strconv.ParseFloat(value, 64)
			if e != nil {
				return nil, errors.New("invalid OKX position value")
			}
			row[key] = n
		}
		out = append(out, row)
	}
	return out, nil
}
func (t *Account) futuresOrders(symbol string) ([]account.OpenOrder, error) {
	ctx := context.Background()
	symbol = t.convertSymbol(symbol)
	inst, e := t.executionInstrument(ctx, symbol)
	if e != nil {
		return nil, e
	}
	rows, e := t.pendingExecution(ctx, symbol)
	if e != nil {
		return nil, e
	}
	out := []account.OpenOrder{}
	for _, r := range rows {
		if r.InstId != symbol {
			return nil, errors.New("OKX returned a different instrument")
		}
		size, e := account.Multiply(r.Sz, inst.CtVal)
		if e != nil {
			return nil, e
		}
		qty, _ := strconv.ParseFloat(string(size), 64)
		price, _ := strconv.ParseFloat(r.Px, 64)
		id := r.OrdId
		kind := strings.ToUpper(r.OrdType)
		trigger := r.TriggerPx
		if r.AlgoId != "" {
			id = r.AlgoId
			if r.SlTriggerPx != "" {
				kind = "STOP_MARKET"
				trigger = r.SlTriggerPx
			}
			if r.TpTriggerPx != "" {
				kind = "TAKE_PROFIT_MARKET"
				trigger = r.TpTriggerPx
			}
		}
		stop, _ := strconv.ParseFloat(trigger, 64)
		out = append(out, account.OpenOrder{OrderID: id, Symbol: symbol, Side: strings.ToUpper(r.Side), PositionSide: strings.ToUpper(r.PosSide), Type: kind, Price: price, StopPrice: stop, Quantity: qty, Status: "NEW"})
	}
	return out, nil
}

func (t *Account) futuresBalance() (map[string]interface{}, error) {
	var rows []struct {
		Details []struct{ Ccy, Eq, AvailEq, AvailBal, Upl string }
	}
	if e := t.readExecution(context.Background(), okxAccountPath+"?ccy=USDT", &rows); e != nil {
		return nil, e
	}
	for _, r := range rows {
		for _, d := range r.Details {
			if d.Ccy != "USDT" {
				continue
			}
			available := d.AvailEq
			if available == "" {
				available = d.AvailBal
			}
			out := map[string]interface{}{}
			for k, v := range map[string]string{"total_equity": d.Eq, "availableBalance": available, "totalUnrealizedProfit": d.Upl} {
				if k == "totalUnrealizedProfit" && v == "" {
					v = "0"
				}
				n, e := strconv.ParseFloat(v, 64)
				if e != nil {
					return nil, errors.New("OKX USDT balance unavailable")
				}
				out[k] = n
			}
			out["totalWalletBalance"] = out["total_equity"].(float64) - out["totalUnrealizedProfit"].(float64)
			return out, nil
		}
	}
	return nil, errors.New("OKX USDT trading balance unavailable")
}
