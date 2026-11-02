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

The list above the Futures assistant's input holds each connected provider's models, its set model first. A choice there applies to that page's next turns, sent as `agent_model_name`. It changes nothing in Settings, and goes back to the set model on reloading the page. Everything else, such as traders and Telegram, uses the set model.

Each call may write up to `VERGEX_AGENT_MAX_TOKENS` tokens, 8000 unless `.env` says otherwise. A model that thinks by default counts its thinking within that.

## Custom endpoints

Any OpenAI-compatible service works: fill in the base URL and model name on a provider that uses the OpenAI format.

- **Base URL.** Give the base, such as `https://openrouter.ai/api/v1`; `/chat/completions` is appended.
- **Full URL.** If the endpoint has a different path, end the URL with `#` to use it as written: `https://api.example.com/v2/ai/chat/completions#`.
- **Reachability.** The URL must be publicly reachable. Localhost and private network addresses are rejected to prevent SSRF, so a model on the same machine (Ollama, LM Studio) needs a public endpoint.
- **API format.** The service must accept `Authorization: Bearer <key>` and return the standard Chat Completions response.

Use the test button in the model form to check a configuration before saving it.

## Costs

Every call is stored locally in `agent.calls`, failed attempts included, with its tokens, time and any error (never the API key). Tokens are priced from the public [models.dev](https://models.dev) list, cached in the database and refreshed daily. **Settings → Agent costs** shows the estimate per model, for all traders or one. Models without a published price, such as custom model names, are shown as unpriced.

## Which models see what

- **Traders** send market data and your account state to the model bound to the trader on every cycle.
- **Futures** uses the model you select on the page. Keys never enter prompts or browser responses.
- **Telegram** uses the model chosen in Settings → Telegram, or picks an enabled one automatically.
