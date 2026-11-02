package run

import (
	"errors"
	"math/big"
	"vergex/internal/account"
	"vergex/internal/futures/plan"
)

// DefaultMaxRiskPct is the share of equity a plan's stop may lose when nothing says
// otherwise.
const DefaultMaxRiskPct = "1"

// AccountLimits derives exact budgets from a fresh Vergex account snapshot.
// Use free margin at the selected leverage, reserving estimated execution costs.
// Existing exposure is not a reason to reject a draft. The controller checks
// positions and working orders again before approving or submitting an entry.
func AccountLimits(a Snapshot, c plan.Constraints) (plan.Constraints, error) {
	if !plan.Positive(a.Equity) || !plan.Positive(a.Available) {
		return c, errors.New("No available margin for a new order. You can still generate plans; review an order again when trading funds are available.")
	}
	if c.Leverage < 1 || c.Leverage > 20 {
		return c, errors.New("select leverage between 1x and 20x")
	}
	equity, _ := decimal(a.Equity)
	available, _ := decimal(a.Available)
	if available.Cmp(equity) > 0 {
		available = new(big.Rat).Set(equity)
	}
	c.Equity = a.Equity
	c.PlanningOnly = false
	c.AvailableMargin = plan.Decimal(available.FloatString(12))
	c.SizingPolicy = plan.SizingAccountMargin
	// Auto sizing allocates margin up to what the stop may lose: a share of equity, or all
	// of it when that share is 0. The compiler still validates stop distance, reward/risk
	// and market depth.
	if c.MaxRiskPct == "" {
		c.MaxRiskPct = DefaultMaxRiskPct
	}
	pct, err := decimal(c.MaxRiskPct)
	if err != nil {
		return c, errors.New("invalid maximum loss per trade")
	}
	c.MaxRisk = a.Equity
	if pct.Sign() > 0 {
		c.MaxRisk = plan.Decimal(new(big.Rat).Quo(new(big.Rat).Mul(equity, pct), big.NewRat(100, 1)).FloatString(12))
	}
	if !plan.Positive(a.FeeBPS) && a.FeeBPS != "0" {
		return c, errors.New("account fee estimate unavailable")
	}
	if cmp(a.FeeBPS, c.FeeBPS) > 0 {
		c.FeeBPS = a.FeeBPS
	}
	if !plan.Positive(c.FeeBPS) || !plan.Positive(c.SlippageBPS) {
		return c, errors.New("positive fee and slippage estimates are required")
	}
	fee, _ := decimal(c.FeeBPS)
	slip, _ := decimal(c.SlippageBPS)
	perUnit := new(big.Rat).Add(big.NewRat(1, int64(c.Leverage)), new(big.Rat).Quo(new(big.Rat).Add(fee, slip), big.NewRat(5000, 1)))
	c.MaxNotional = plan.Decimal(new(big.Rat).Quo(available, perUnit).FloatString(12))
	return c, c.Validate()
}

// DraftAccountLimits permits market analysis when existing exposure uses all
// free margin. Equity is a reference budget, never spendable collateral. The
// preview flag and actual available margin are persisted in the plan hash.
func DraftAccountLimits(a Snapshot, c plan.Constraints) (plan.Constraints, error) {
	available, err := account.Number(string(a.Available))
	if err != nil || !plan.Positive(a.Equity) {
		return c, errors.New("Could not verify account equity and available margin for planning. Refresh the account or explore without an account.")
	}
	if available.Sign() > 0 {
		return AccountLimits(a, c)
	}
	reference := a
	reference.Available = a.Equity
	limits, err := AccountLimits(reference, c)
	if err != nil {
		return c, err
	}
	limits.PlanningOnly = true
	limits.AvailableMargin = a.Available
	return limits, limits.Validate()
}

const EquityPreviewReviewMessage = "This is an equity-based preview. Refresh and review it with available margin before approval."

// Check actual locked orders, not unused planning budget or historical equity.
func CheckAccountCapacity(p plan.Plan, a Snapshot) error {
	if p.Constraints.PlanningOnly {
		return errors.New(EquityPreviewReviewMessage)
	}
	if !plan.Positive(a.Equity) || !plan.Positive(a.Available) {
		return errors.New("live account has no available margin")
	}
	total := new(big.Rat)
	specs := p.Orders
	if len(specs) == 0 {
		specs = []plan.OrderSpec{{Price: p.Idea.Entry, Quantity: p.Quantity}}
	}
	for _, o := range specs {
		q, e := decimal(o.Quantity)
		if e != nil {
			return e
		}
		price, e := decimal(o.Price)
		if e != nil {
			return e
		}
		total.Add(total, new(big.Rat).Mul(q, price))
	}
	margin := new(big.Rat).Quo(total, big.NewRat(int64(p.Constraints.EffectiveLeverage()), 1))
	fee, _ := decimal(p.Constraints.FeeBPS)
	slip, _ := decimal(p.Constraints.SlippageBPS)
	costs := new(big.Rat).Mul(total, new(big.Rat).Quo(new(big.Rat).Add(fee, slip), big.NewRat(5000, 1)))
	available, _ := decimal(a.Available)
	if available.Cmp(new(big.Rat).Add(margin, costs)) < 0 || cmp(a.Equity, p.Risk) < 0 {
		return errors.New("available margin no longer covers the locked orders and cost reserve; regenerate with the current account balance")
	}
	return nil
}
