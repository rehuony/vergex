# Models

Models are called directly with your own API key. Nothing goes through a gateway. Add them in **Settings → Agent models**.

## Providers

| Provider | Default base URL | Default model |
| :------- | :--------------- | :------------ |
| DeepSeek | `https://api.deepseek.com` | `deepseek-chat` |
| OpenAI | `https://api.openai.com/v1` | `gpt-5.4` |
| Claude | `https://api.anthropic.com/v1` | `claude-opus-4-6` |
| Qwen | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `qwen3-max` |
| Gemini | `https://generativelanguage.googleapis.com/v1beta/openai` | `gemini-3.1-pro` |
| Grok | `https://api.x.ai/v1` | `grok-3-latest` |
| Kimi | `https://api.moonshot.ai/v1` | `moonshot-v1-auto` |
| MiniMax | `https://api.minimax.io/v1` | `MiniMax-M2.7` |

The defaults live in `internal/agent/`. Leave the base URL and model name empty to use them.

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
