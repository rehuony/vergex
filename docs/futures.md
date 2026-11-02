# Futures

Futures is a user-initiated planning workspace: you discuss a trade with the agent on a live chart, compare entry plans, and approve one plan for live execution. It is not a continuously running strategy (that is a trader) and not a backtester.

## Workflow

1. Open **Futures**.
2. Pick the venue, account and leverage beside the tabs, and the market in the chart's head. Public markets can be browsed without an account; trading needs an enabled Binance or OKX account, added in Settings. A venue with an account always has one selected, and "No account" shows only for a venue that has none. Market discovery lists crypto USDT perpetuals only.
3. Choose the model (any model in Settings → Agent models) in the assistant's input beside the chart and describe the trade. The agent can read market data, draw support/resistance and trend lines, and draft plans. It cannot approve or place orders.
4. Look over the drafts under **Plans**, beside the chart, which the assistant goes to when a turn leaves some. Selecting a plan opens it and fits the chart to its entry, stop and target. Pressing it again, or the × beside its name, closes it and takes it off the chart, so pressing it twice fits it again. With two or more, **Compare plans** lays them out side by side and fits them all.
5. **Review order** under the selected plan. Check prices, quantities, schedule, cost assumptions and account, then click **Confirm and place live order**. Only that click locks and starts the plan.

Model calls use the user's own provider key and may incur provider charges; token usage is recorded locally. Keys never enter prompts or browser responses. Sessions and discussion preferences are saved per user and cannot lower the hard risk limits.

## Page

| Tab | Shows |
| :-- | :---- |
| Trade | The chart, beside the assistant's conversation and the plans it drafted |
| Positions and orders | The selected account's equity, positions, open orders and plans at work, closing a position, and manual entry. It keeps itself current: the exchange is read every 5 seconds while it is on screen, its time says when that last worked, and P/L follows the live mark price every second |
| Chat history | The selected account's past conversations, finished plans with what each realized and expired drafts, as a directory of folders beside the one opened. A conversation can be continued, renamed or deleted, a draft deleted, and any plan put back on the chart. A plan that ran can't be deleted, as it is the record of its orders, and no plan can be renamed, as its name is part of what its approval signs |
| Settings | The risk limits, kept on this browser as they change, the discussion preferences, saved on leaving the field, and the entry calculators the current conversation has built |

From `lg` up, Trade fills the first screen and the page does not scroll. Positions and orders holds at least the first screen, and grows with what it lists while the page scrolls. When there is something to say, one line floats above the assistant's input: what went wrong, an order being validated, or a trade that reserves the account, with what can be done about it opening from there. It takes no room, so nothing moves when it comes or goes. In Positions and orders the same goes for the panel's title row: what needs attention sits there, with all of it opening in a popover.

Before a conversation starts, a few suggestions sit above the input. Picking one fills the input for you to edit or send, and they go once a message is sent. While the model works, one line says it is thinking with its latest steps under it. When it is done they fold into one line that opens again, and only a step that failed stays out.

Plans lists every plan of the latest generation, one row each and in the order of the table below, with its side, entry, stop, target and net reward to risk. Asking to change the selected plan replaces only its row, as does refreshing one for its review, and the others stay. A way with no plan says on one line that it is waiting or not feasible, and opens to why, which a reload keeps. One the model never proposed is not listed. Below the rows is what the open plan rests on: its reasoning, planned loss and target return, margin and position value, and its child orders. Closing a plan only changes what is shown, and does nothing to any orders it has placed.

The answer appears as the model writes it. Stopping keeps what it had written so far on the page. A turn has 5 minutes in all, and a round 3. When a turn ends, its steps fold into one line that also says what it came to: the tokens in and out, and the estimated cost as in [Models](models.md#costs).

The assistant opens on its conversation, after a reload too, whatever plans were left. The address says what else is open, as `/futures?venue=&symbol=&account=&plan=&tab=`, so a view can be linked to and survives a reload. `plan` opens that plan's own market and account and fits it on the chart. What the address cannot use is ignored.

A plan's realized P/L comes from the trading ledger: the positions its run opened, once they closed. Reading it asks no exchange and has no time limit. A plan shows 0 until the account's sync has booked its fills, and a position it opened that something else closed still counts for the plan.

## Entry structures and limits

| Kind | Behaviour |
| :--- | :-------- |
| Market | Sent immediately after approval within an approved price bound; the shown entry is an estimate |
| Limit | One GTC limit entry |
| Ladder | 2–8 weighted limit prices released together |
| TWAP | 2–24 fixed-price slices, 5–300 s apart, at most one hour in total; late slices expire instead of catching up |

- Leverage 1–20x (default 5x), subject to venue limits.
- Every child and the aggregate must pass exact-decimal fee/slippage, net reward/risk, tick/step, minimum quantity, minimum notional, budget and book-liquidity checks.
- With an account connected, sizing is automatic. The position is as large as free margin allows at the leverage, after reserving estimated costs, as long as its stop stays within **Maximum loss per trade** under the Settings tab: 1% of equity by default, and 0 lifts the limit. So a closer stop allows a larger position. Venue minimums, liquidity and rounding can reduce size. A stop that would consume 80% of initial margin is rejected.
- An open position a plan holds can have its stop and target moved, or its stop taken to breakeven, from **Stop and target** on its row. Only a position with nothing more to come can be changed. A new price has to be on the market's tick and on the side of a fresh book that does not fire at once. Each order is replaced with the new one placed first, so the position is never without a stop, and the new one is recorded before it is sent. If the exchange refuses it the old one stays and you are told why. If the answer is lost, the plan needs attention and the new order is never sent again: reconciling finds out what happened, and whichever order is left standing protects the position. The approved plan itself does not change. `POST /api/futures/amend-exit` does this.
- An order on the account that no unfinished plan holds, such as one placed by hand or left behind, can be cancelled from its row, unless a trader is running on the account. `POST /api/futures/cancel-order` reads the open orders first and goes by those.
- Equity, available balance, fees, a fresh order book and account identity are rechecked at execution. Existing positions or pending orders on the symbol block execution. An account leased by an unresolved plan cannot start a trader.

## Protection and expiry

- Entry fills are observed about every two seconds; reduce-only stop-loss and take-profit orders are placed for newly filled quantity. This is **not an atomic bracket**: there is a fill-to-protection interval and protection can fail. Inspect the exchange immediately when a run shows **Needs attention**.
- Partial fills can be too small for an exchange protection order and then need manual handling. Stops do not guarantee a maximum realized loss.
- A draft expires five minutes after creation. Unfilled limit entries expire after 15 minutes; a TWAP window ends two minutes after its last release.
- Stopping cancels unsent and pending entries. **It does not close filled positions or remove their protection.**

## Approval, durability and recovery

- The approval digest binds the full plan, the owner, the account and the execution policy. Only one unresolved approved plan can lease an account.
- Each child has a deterministic client order ID, persisted **before** submission. Binance and OKX IDs use the `vergex` prefix. Missing or timed-out acknowledgements become `UNKNOWN`, are queried by client ID and are never blindly resubmitted. Receipts must match the locked intent.
- After a restart, active runs become `needs_attention` and do not resume entries. **Check trade status** reads what is on the exchange, and a position it finds fully protected goes back to being watched, so its stop or target firing is noticed and the other one cancelled. Moving a stop or target does the same. Exchange-side stop and target orders keep working while the process is down, but protection of new fills can be delayed: check positions and orders at the exchange.
- What a run intends and what the exchange told it are in `futures.run_orders`, written only by the run's controller, and only trusted after they match the approved plan. A row is saved before its order is sent, and if it cannot be saved the order is not sent.
- An order that reaches the exchange is also copied to the ledger (`account.orders`), which adds up its fills and keeps its position. The copy happens in the background and the controller never reads it, so a failure there is only logged and cannot affect execution: the fills still arrive, and find the run through its position or its lease on the account. The two can disagree for a moment, because the run asks the exchange and the ledger reads trades.
- A manual close from the account panel is saved in `futures.account_closes` before it is sent, so a retry can never close twice, and is copied to the ledger the same way.
- An account can be deleted while nothing has happened on it, which takes its unused approvals, drafts and conversations along. A run still holding its lease, or any order in the ledger, keeps the account.

## Agent

- A planning turn has a 180-second limit and each model round 75 seconds; progress and a Stop action stay visible. Streams always end with a terminal event; an unexpected disconnect is shown as an error, not retried.
- A reply has room for 16384 tokens, since a thinking model's reasoning counts against it, or 8192 for a model that refuses that many. A round that runs out of length or time before any answer is tried once more, asking the model to reason less, and after that the turn closes with what it has.
- Tool reads are bounded and repeated identical reads are cached. The agent keeps at most five unique drafts per turn.
- Models without native function calling use a text-tool protocol: tool schemas are given as text, the model returns a bounded JSON envelope, and Go validates the tool name and arguments before dispatching. Approval and execution are never tools.
- Data plugins: public funding rate and open interest of the selected market (`get_data_plugin`).
- Entry calculators run in a bounded Starlark runtime with tests and immutable source hashes; they have no network, shell, keys or order access, and their output still goes through the plan compiler and human approval.

## Conversations

Each turn is stored in `agent.messages` as it happens: your words, the workspace and market data sent with them (role `context`), the model's thinking where it returns one, its tool calls and each result. The fixed system prompt and the history the browser resends are not stored again. A conversation belongs to the account it started on, so switching account starts another. `GET /api/futures/conversations?account_id=` lists an account's conversations without their messages, and `GET /api/futures/conversations/:id` opens one with what you and the model said. `PUT` on the same path renames it, without moving it in the list, and `DELETE` removes it with its messages; the plans and usage it led to stay and only lose their link to it. `DELETE /api/futures/plans/:id` removes a plan with no run, and answers 409 for one that ran. The browser still sends its recent history with each turn, and the interface's language, in which the model writes its replies, plan names, rationales and drawing labels unless you ask for another. Stored messages are never cleaned up automatically: [Data and backups](getting-started.md#data-and-backups) gives the expected growth.

## Chart

The chart (`web/src/components/futures/futures-chart/`, lightweight-charts) loads history from `/api/futures/candles`, streams updates from `/api/futures/feed` (SSE: live candles from the market stream, or polling every 3 seconds while no stream is up), reads older candles a screenful before scrolling reaches them, back to the market's first candle, and reads what is missing instead of drawing across a gap. If the history can't be read, it says so once and keeps trying, after 3 seconds at first and at most every 30, for as long as that market is shown. It draws plan levels and risk/reward zones, comparison lanes, up to five, each headed by its way of entering with its prices in the colours of the plan's row, and agent drawings (`horizontal_line`, `trend_line`, `rectangle`). What the account holds on this market is on it too, read from the same account watch as the positions tab: where each position was entered and where the exchange's stop and take-profit orders sit, dotted so they read apart from a plan. It only reads, and comparing plans hides them.

The **Forecast** switch on the chart overlays this coin's [forecast](forecast.md) under the strategy in use, off until asked for since this chart is for live trades, and remembered on this browser. It draws the same simulated candles as the Forecast page, with one line above them that links to it: the score, and the hit rate with how many calls it is of and what the same calls would hit by chance. Under 30 calls it gives only their number, since a percentage of a handful reads as far more than it is. An OKX USDT swap shows the Binance coin's forecast and says so, since Forecast reads only Binance. Below 15m, or on any other market, it says why there is nothing to show. A coin that is not tracked starts fetching when the switch is on. The forecast is only asked for once the market's real candles are on the chart, so simulated candles never show alone. Comparing plans hides the overlay, and it never moves plan levels or zones, nor does it reach the assistant.

Futures reads one read-only endpoint, `/api/forecast/overlay/:symbol`, and choosing a strategy stays on the Forecast page.

## Verification boundary

Automated tests use local HTTP exchange fixtures and temporary databases, never funded accounts or real orders. Passing tests do not prove that an account has trading permission or that a live order succeeds. Validate the flow on a [testnet account](accounts.md#testnet), then with a small account before real money.

A plan on a testnet account reads its book on the test environment and is bound to it by the approval digest, so it cannot run on a real account, or a real plan on a testnet one. Its chart still shows the real market, and the review dialog says the order is a testnet one.
