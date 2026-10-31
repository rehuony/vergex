# Frontier

Frontier is a user-initiated planning workspace: you discuss a trade with the AI on a live chart, compare entry plans, and approve one plan for live execution. It is not a continuously running strategy (that is Autopilot) and not a backtester.

## Workflow

1. Open **Frontier**.
2. Pick the market, exchange, account and leverage in the toolbar. Public markets can be browsed without an account; trading needs an enabled Binance or OKX account, added in Settings. Market discovery lists crypto USDT perpetuals only.
3. Choose the AI model (any model in Settings → AI models) beside the chart and describe the trade. The AI can read market data, draw support/resistance and trend lines, and draft plans. It cannot approve or place orders.
4. Compare up to four drafts on the chart. Selecting a plan focuses the chart on its entry, stop and target; **Compare plans** lays them out side by side.
5. Review prices, quantities, schedule, cost assumptions and account, then click **Confirm and place live order**. Only that click locks and starts the plan.

Model calls use the user's own provider key and may incur provider charges; token usage is recorded locally. Keys never enter prompts or browser responses. Sessions and discussion preferences are saved per user and cannot lower the hard risk limits.

## Page

The chart and the assistant fill the first screen. Below them:

| Tab | Shows |
| :-- | :---- |
| Plans | The latest entry plans, the selected plan's details and the advanced tools |
| Positions and orders | The selected account's positions and open orders, closing a position, and manual entry |
| History | Finished plans with what each realized, earlier drafts and past conversations |

A plan's realized P/L comes from the exchange's closed P/L for the account, read only while History is open. A close counts for a plan when it is on the same symbol and within the plan's lifetime, so a manual close between plans counts for none. It looks back 30 days and at the exchange's own record limit (500 on Binance, 100 on OKX); a plan outside that shows a dash.

## Entry structures and limits

| Kind | Behaviour |
| :--- | :-------- |
| Market | Sent immediately after approval within an approved price bound; the shown entry is an estimate |
| Limit | One GTC limit entry |
| Ladder | 2–8 weighted limit prices released together |
| TWAP | 2–24 fixed-price slices, 5–300 s apart, at most one hour in total; late slices expire instead of catching up |

- Leverage 1–20x (default 5x), subject to venue limits.
- Every child and the aggregate must pass exact-decimal fee/slippage, net reward/risk, tick/step, minimum quantity, minimum notional, budget and book-liquidity checks.
- With an account connected, sizing is automatic: 1% of equity is the planned stop-risk ceiling and 95% of free margin, after reserving estimated costs, sets the leveraged notional ceiling. Venue minimums, liquidity and rounding can reduce size. A stop that would consume 80% of initial margin is rejected.
- Equity, available balance, fees, a fresh order book and account identity are rechecked at execution. Existing positions or pending orders on the symbol block execution. An account leased by an unresolved plan cannot start an Autopilot trader.

## Protection and expiry

- Entry fills are observed about every two seconds; reduce-only stop-loss and take-profit orders are placed for newly filled quantity. This is **not an atomic bracket**: there is a fill-to-protection interval and protection can fail. Inspect the exchange immediately when a run shows **Needs attention**.
- Partial fills can be too small for an exchange protection order and then need manual handling. Stops do not guarantee a maximum realized loss.
- A draft expires five minutes after creation. Unfilled limit entries expire after 15 minutes; a TWAP window ends two minutes after its last release.
- Stopping cancels unsent and pending entries. **It does not close filled positions or remove their protection.**

## Approval, durability and recovery

- The approval digest binds the full plan, the owner, the account and the execution policy. Only one unresolved approved plan can lease an account.
- Each child has a deterministic client order ID, persisted **before** submission. Binance and OKX IDs use the `vergex` prefix. Missing or timed-out acknowledgements become `UNKNOWN`, are queried by client ID and are never blindly resubmitted. Receipts must match the locked intent.
- After a restart, active runs become `needs_attention` and do not resume entries. Use **Reconcile exchange state** to resume reconciliation and protection monitoring. Exchange-side stop and target orders keep working while the process is down, but protection of new fills can be delayed: check positions and orders at the exchange.

## AI harness

- A planning turn has a 180-second limit and each model round 75 seconds; progress and a Stop action stay visible. Streams always end with a terminal event; an unexpected disconnect is shown as an error, not retried.
- Tool reads are bounded and repeated identical reads are cached. The harness keeps at most five unique drafts per turn.
- Models without native function calling use a text-tool protocol: tool schemas are given as text, the model returns a bounded JSON envelope, and Go validates the tool name and arguments before dispatching. Approval and execution are never tools.
- Data plugins: public funding rate and open interest of the selected market (`get_data_plugin`).
- Entry calculators run in a bounded Starlark runtime with tests and immutable source hashes; they have no network, shell, keys or order access, and their output still goes through the plan compiler and human approval.

## Chart

The chart (`web/src/components/planning/planning-chart/`, lightweight-charts) loads history from `/api/planning/candles`, streams updates from `/api/planning/feed` (SSE, every ~3 s), loads older candles when scrolling back and refetches history instead of drawing across gaps. It draws plan levels and risk/reward zones, comparison lanes, and AI drawings (`horizontal_line`, `trend_line`, `rectangle`).

## Verification boundary

Automated tests use local HTTP exchange fixtures and temporary databases, never funded accounts or real orders. Passing tests do not prove that an account has trading permission or that a live order succeeds; validate with a small account (exchange testnets are not supported).
