# Models

Models are called directly with your own API key. Nothing goes through a gateway. Add them in **Settings → Agent models**.

## Providers

| Provider | Default base URL | Default model |
| :------- | :--------------- | :------------ |
| DeepSeek | `https://api.deepseek.com` | `deepseek-v4-pro` |
| OpenAI | `https://api.openai.com/v1` | `gpt-6-astra` |
| Claude | `https://api.anthropic.com/v1` | `claude-opus-5-5` |
| Qwen | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen3.8-max` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-4-argon` |
| Grok | `https://api.x.ai/v1` | `grok-4.7` |
| Kimi | `https://api.moonshot.ai/v1` | `kimi-k3.1-pro` |
| MiniMax | `https://api.minimax.io/v1` | `MiniMax-M3` |

The defaults live in `internal/agent/`. Choosing a provider fills in its official base URL, and leaving that as it is saves no endpoint of its own, so it follows the default. Leaving the model on the provider's default saves no name of its own either.

## Choosing a model

The model name is a field: choosing a provider fills in its default, and any name can be typed over it. Its arrow opens what the provider offers, read from the `/models` endpoint beside its chat endpoint once the form has a key: `GET /api/agent/models/:id/available` with a saved key, `POST /api/agent/models/available` with what is in the form. The list leaves out what cannot chat, such as embedding and speech models, and costs no tokens. Picking from it replaces what is in the field, so the field always holds the one name that will be used, and a provider that gives no list only leaves it to be typed.

The list above the Futures assistant's input holds each connected provider's models, its set model first. A choice there applies to that page's next turns, sent as `agent_model_name`. It changes nothing in Settings, and goes back to the set model on reloading the page. The [assistant](assistant.md) has the same list, and remembers its choice on this browser. Everything else, such as traders and Telegram, uses the set model.

Each call may write up to `VERGEX_AGENT_MAX_TOKENS` tokens, 8000 unless `.env` says otherwise. A model that thinks by default counts its thinking within that. The Futures assistant asks for 16384 per round on every provider, and 8192 from a model that refuses that much.

## Streaming

The Futures assistant, the assistant and Telegram show a reply as it is written, with thinking left out. Traders do not: nobody is waiting, and a decision is only read whole. A stream that says nothing for 90 seconds counts as timed out, and how long one may run in all is up to what started it. A provider that will not stream, as custom endpoints may not, is asked again the ordinary way, and shows its reply when it is done.

## When a call fails

A call is tried up to three times if the failure may pass: a dropped connection, or a provider answering 429, 500, 502, 503, 504, 520, 522, 524 or 529. It waits as long as the provider's `Retry-After` asks, up to 30 seconds. Anything else, such as a bad key or an unknown model, fails at once, as does a call that was stopped or ran out of time.

What you are told says why in terms of what to do about it: the key, the balance, rate limits, an unknown model, too much context, a provider in trouble, a timeout or the network, with the provider's own line after it. The Futures assistant, the assistant, Telegram, trader decisions and the trader dry run all say it the same way, and the full error stays in `agent.calls`. The API key never appears in any of these.

## Custom endpoints

Any OpenAI-compatible service works: fill in the base URL and model name on a provider that uses the OpenAI format.

- **Base URL.** Give the base, such as `https://openrouter.ai/api/v1`; `/chat/completions` is appended.
- **Full URL.** If the endpoint has a different path, end the URL with `#` to use it as written: `https://api.example.com/v2/ai/chat/completions#`.
- **Reachability.** The URL must be publicly reachable. Localhost and private network addresses are rejected to prevent SSRF, so a model on the same machine (Ollama, LM Studio) needs a public endpoint.
- **API format.** The service must accept `Authorization: Bearer <key>` and return the standard Chat Completions response.

Use the test button in the model form to check a configuration before saving it.

Deleting a model removes it and its key. While a trader in use runs on it, the delete is refused and names them; an archived trader simply gives it up.

## Costs

Every call is stored locally in `agent.calls`, failed attempts included, with its tokens, time and any error (never the API key). Tokens are priced from the public [models.dev](https://models.dev) list, cached in the database and refreshed daily. **Settings → Agent costs** shows the estimate by what made the calls, traders, Futures, Telegram, the assistant or a test, and per model, for everything or one trader, with how many failed and the last few errors. Days are UTC days, as in the Telegram daily report. Models without a published price, such as custom model names, are shown as unpriced. A call that failed or was stopped may have no tokens to count, though a provider may still have charged for it, so the estimate is a lower bound. Claude reports its tokens as it goes, so a stopped call counts what it had; the OpenAI format only gives them at the end.

## Which models see what

- **Traders** send market data and your account state to the model bound to the trader on every cycle.
- **Futures** uses the model you select on the page. Keys never enter prompts or browser responses.
- **Telegram** uses the model chosen in Settings → Telegram, or picks an enabled one automatically.
- **The assistant** uses the model picked in its window. Like Telegram it sends the account state and what its skills read.
