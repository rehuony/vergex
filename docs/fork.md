# Fork record

vergex is a personal fork of NOFX (github.com/NoFxAiOS/nofx), maintained independently and no longer synced with it. The baseline is upstream commit `4a5e9ac2` (2026-10-21). Anything removed below can be recovered from git history at that commit.

## Decisions (2026-10-28)

| Topic | Decision |
| :---- | :------- |
| AI models | Direct calls to each provider with your own key: DeepSeek, OpenAI, Claude, Qwen, Gemini, Grok, Kimi, MiniMax |
| Paid gateway | Claw402 removed: x402 payments, Permit2, USDC wallet, onboarding wallet flow, per-call settlement |
| Paid data | NofxOS and upstream's paid signal sources removed; rankings are computed from free Binance and OKX public data |
| Fees and referrals | All removed: builder fee, broker IDs and tags, referral links |
| Exchanges | Binance and OKX USDT perpetuals only; the other eight adapters were deleted |
| Features | Traders, Frontier, grid trading and the Telegram bot stay |
| Community | Landing page, competition, leaderboard, strategy market and public endpoints removed |
| Instruments | Crypto only; HIP-3 stocks and commodities, Alpaca and TwelveData removed |
| Telemetry | Removed entirely |
| Deployment | Local builds only; no image registry. One setup for local and remote, with a reverse proxy in front for remote |
| Entry point | The Makefile; no separate `start.sh` |
| Name | `vergex` everywhere; upstream is credited once in the README's License section, as AGPL requires |
| Database | PostgreSQL only |
| AI costs | Estimated locally from token usage and the models.dev price list |

## Later changes

- **Compatibility.** There are no old deployments to support, so upstream compatibility paths and data migrations were removed. See the compatibility section of [AGENTS.md](../AGENTS.md).
- **Layout.** The Go code moved under `cmd/vergex` and `internal/`, and the `mcp` package became `llm`. The frontend moved to responsibility directories with kebab-case paths, and its tests to `web/src/tests/`.
- **Frontend tooling.** npm was replaced by pnpm, and git hooks were removed in favour of CI.
- **Documentation language.** The original plan kept multi-language docs. Technical docs are now English only and the README is English and Chinese; other languages can return when they can be maintained.
