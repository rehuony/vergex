# Assistant

The assistant in the web app. It is the same assistant as in [Telegram](telegram.md), built around fixed skills: Go code runs the known tasks, and the model only understands the request, picks a skill, fills in missing details and writes the reply. It places no orders, and credentials never reach it.

## The window

The round icon in the corner of the screen is on every page once you are signed in, and opens the window beside the page, which goes on working under it. On a phone the window fills the screen below the top bar. Moving between pages keeps the conversation, and so does closing the window. A dialog covers both until it closes.

The icon spins while the assistant answers. A dot on it says something is waiting: a warning colour for what needs your confirmation, ink for a reply that came while the window was closed.

It answers in the language you write in. What the server writes itself, such as what you confirm, is in Chinese if you write in Chinese or the interface is, and otherwise in English.

Type and press Enter to send, or Shift+Enter for a new line. The reply appears as the model writes it, and how it got there sits above the reply, folded into one line with its tokens and estimated cost once it is done. **Stop** keeps what was written so far.

The list above the input holds your models, and the choice is remembered on this browser. It changes nothing in Settings. With no model set up, the window says so and links to where one is added.

The clock in the head lists your conversations, and the pencil starts a new one. Opening the window picks up the latest. A conversation in the list opens when pressed, and is deleted from its `…` menu. The bin beside the pencil, in the list, clears them all after asking, along with anything waiting for your confirmation.

When something needs your confirmation, it waits just above the input, where it stays however long the reply above it is. Each action is a row in the server's own words, never the model's: what it does and to what, a mark for real money or testnet, and its account and settings under it, a config key by key. The button says what it does, such as **Delete trader**, or **Run all** for several, and is red for what cannot be taken back. Writing another message instead cancels it, as does leaving it for 5 minutes. Once answered, how each action went is a row in the conversation, with a check or a warning and why it failed. Whatever page you are on reads again once something has changed.

Asking for a trader goes the same way: the assistant asks for what is missing, then shows what it will create, on which account and with which settings, for you to confirm. Saving goes through the same checks as the Create trader form, and a button then leads to the new trader. It starts stopped, and starting it is a confirmation of its own.

If the model cannot answer, one line above the input says why, opens in full when pressed, and offers to ask again.

## What it sees

Each message says what you have open, so "this trader" or "these plans" mean what is on your screen. A page declares that itself: ids, names and what only the browser knows. It holds no figures, and no screenshot or text from the page goes with it: the assistant reads what it needs through its skills, so what it reads is current. It is sent as data the model is told not to take instructions from, and is stored with the conversation under the `context` role.

| Page | What it says |
| :--- | :----------- |
| Dashboard | The events filter, and the gate or route selected |
| Forecast, Signals | The coin, the interval and the chart's options |
| Forecast, Strategies | The strategy with whether it is in use or retired, the tab, whether a new one is being drafted |
| Forecast, Backtests | The run with its strategy and status, whether a new one is being set up |
| Futures | The tab, venue, market, account and whether it is testnet, leverage, interval and visible range, this generation's plans, the plan open and those compared |
| Auto trading, Traders | The trader with its kind and whether it runs, the tab, whether its settings have unsaved changes |
| Forecast settings, Auto trading overview, Help | Only which page it is |

Before a conversation starts, the window offers a few questions for the page you are on. Picking one fills the input for you to edit or send.

## Filling in forms

Ask for values, or for a form to be filled in, on a page that has one. The window shows what would change beside what the form holds now, with why. **Fill in** writes them into the form, as typing them would, and closes the window so the form is in view. Saving is still yours, with the page's own button, its own checks and the server's. If the form has gone by then, the card says so and leads back to the page.

| Form | Where | What can be filled in |
| :--- | :---- | :--------------------- |
| Trader settings | A trader's Settings tab | Model, account, interval and margin mode while it is stopped, and any of its config |
| New backtest | Forecast, Backtests, while setting one up | Strategy, name, coins, intervals and period |
| New strategy | Forecast, Strategies | Name and rules, starting a draft from the strategy in view if none is open |
| Request | Futures | What goes in the Futures assistant's input, for you to send |

Nothing that holds a key, a password or an order is on this list. When the next step is elsewhere, such as keys in Settings, the assistant shows a button to that page, which goes nowhere until pressed. `open_page` can only lead to the pages in its table, and neither of these skills is offered in Telegram.

## What it can read beyond Telegram

These read-only additions reach Telegram too:

| Skill | Scope | Reads |
| :---- | :---- | :---- |
| `account_query` | `system` | The whole system at once: accounts, what they hold, what runs and what needs attention |
| `forecast_query` | `strategies` | The saved strategies, and which is in use |
| | `performance` | One strategy's rules and live record, beside its latest backtest. With none named, the one in use |
| | `backtests`, `backtest` | The latest runs, or one run's report |
| `futures_query` | `compare` | Plans side by side, as the plan compiler sized them: entry, stop, target, quantity, risk, reward, net reward to risk, and whether they ran out. With none named, the latest drafts still waiting |

Whatever Forecast says comes with how to read it: strength is no probability, a hit rate means nothing until it beats what chance would hit, and figures from fewer than 30 separate moments say little. `signals` lists the strongest calls first and at most 40 coins, and `detail` leaves out the candles.

## Deleting conversations

`conversation_management` lists and deletes your conversations with a model wherever you held them: the Futures assistant's, this window's and Telegram's, and those of any feature added later that you chat in. Name them, or ask for all of them, on one account or in one feature. Deleting waits for your confirmation, in Telegram too: a row for each conversation named, or one for all of them with how many in each feature. The conversation you are asking in is never among them, so delete it from the list. A trader's conversations are the record of its cycles, not chats, and are never listed.

Wherever a conversation is deleted from, it takes all it held: what was said, its model calls, the entry calculators and drafts written in it, and what it was waiting for. A Futures plan that ran stays, without the link, and can be deleted on its own. [database.md](database.md#deleting-a-conversation) lists what goes and how.

## Deleting plans

`plan_management` deletes Futures plans that are done with: drafts that never ran, and finished plans whose runs have all settled. Name them, or ask for every finished plan, every draft or both, on one account or symbol. Deleting waits for your confirmation, in Telegram too, as a row for each plan named or one for all of them with how many of each kind. A plan still at work on the exchange is never among them, and what a finished plan sent stays in the account's orders, fills and positions: see [futures.md](futures.md).

## What is the same as Telegram

The skills, how a target is found and how an action waits for your confirmation are one piece of code, `internal/assistant`. Skills call the local REST API with fixed paths and a token signed for each call, so server-side validation and risk limits always apply. See [Skills](telegram.md#skills), [Confirmation](telegram.md#confirmation) and [Finding the target](telegram.md#finding-the-target).

What differs:

| | Telegram | Web app |
| :-- | :------- | :------ |
| Closing a position | `close_position`, after confirming | Not on offer: a position's row has its button |
| `fill_form`, `open_page` | Not on offer: they need a page | Suggest values for a form on the page, and show a button to another page |
| Model | The one chosen in Settings → Telegram | The one picked in the window |
| Conversations | One per chat until `/start` | As many as you start |

## Conversations

Conversations are kept in `agent.conversations` and `agent.messages`, with `feature` set to `assistant`: what you and the assistant said, the account state it was given, its thinking where the model returns one, and each skill call with its result. Nothing cleans them up automatically (see [Data and backups](getting-started.md#data-and-backups)). What is waiting for your confirmation stays in memory: it runs out after 5 minutes, and a restart drops it.

Every model call is in `agent.calls` under `assistant`, and counted in **Settings → Agent costs** and the daily report until its conversation is deleted.

## API

All of it needs the signed-in user's token.

| Endpoint | Does |
| :------- | :--- |
| `POST /api/assistant/chat` | Runs one turn, as server-sent events. Takes `conversation_id`, a UUID the browser chooses, `message`, `agent_model_id`, an optional `agent_model_name`, `language` and `page`, what the page says about itself, of which 16000 characters are used |
| `GET /api/assistant/conversations` | The latest 100, newest first, without their messages |
| `GET /api/assistant/conversations/:id` | The last 40 things said, and what is waiting for an answer |
| `POST /api/assistant/conversations/:id/answer` | Answers what is waiting, with `confirmation_id`, `confirmed` and `language`. `live` is false when that no longer waits |
| `DELETE /api/assistant/conversations/:id` | Removes it with all it held, and what it was waiting for |
| `DELETE /api/assistant/conversations` | Does that for all of them |
| `GET /api/conversations` | Every conversation in a feature people chat in, newest first, without their messages. `feature` and `account_id` narrow it, and `limit` takes up to 200 |
| `DELETE /api/conversations` | Deletes those of `ids`, up to 500, that are your conversations, in any of those features, and says how many went |

A turn's events are `stage`, what is under way in Go's own words; `delta`, the reply as it is written; `usage`, what the turn's model calls came to; `reply`, with `text`, `results` for how confirmed actions went, `confirm` with its `items` in parts, `changed`, `links`, places in the app to go next, named by the server, and `proposals`, what it suggests for forms on the page; `error`, when the model could not answer; and `done`, always last. `changed` says something was written, so what a page shows may be out of date. What a skill wrote before moving on to the next is not the answer: a `stage` means the text so far goes.

One turn runs at a time. Another turn, an answer, or deleting or clearing from the window meanwhile gets 409; `DELETE /api/conversations` does not wait, as a confirmed deletion runs within a turn. Ending the request stops the turn: what was already done stays done, and what it was holding goes with it. A turn has 3 minutes at most, and 10 rounds of model calls.
