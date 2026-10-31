import {
  BookOpen,
  Crosshair,
  Layers,
  LineChart,
  Monitor,
  Send,
  Shield,
  TrendingUp,
  Wrench,
  Zap,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { Language } from './translations'

/**
 * FAQ content model. Answers are composed from typed blocks so the renderer
 * stays generic — no per-question JSX special cases. Inline `code` spans are
 * written with backticks and parsed by the renderer.
 */
export type FAQBlock =
  | { type: 'p'; text: string }
  | { type: 'list'; items: string[] }
  | { type: 'steps'; items: string[] }
  | { type: 'note'; text: string }
  | { type: 'links'; links: { label: string; href: string }[] }

export interface FAQItem {
  id: string
  question: string
  blocks: FAQBlock[]
}

export interface FAQCategory {
  id: string
  title: string
  icon: LucideIcon
  items: FAQItem[]
}

const english: FAQCategory[] = [
  // ───────────────────────── Getting started ─────────────────────────
  {
    id: 'getting-started',
    title: 'Getting started',
    icon: BookOpen,
    items: [
      {
        id: 'what-is-vergex',
        question: 'What is Vergex?',
        blocks: [
          {
            type: 'p',
            text: 'Vergex is an open-source, self-hosted AI trading terminal. Its Autopilot mode runs an AI agent on a schedule: it reads the candidate coins from your strategy, market data and your account state, then decides and executes on your connected exchange — all on your own machine, with your keys never leaving your server.',
          },
          {
            type: 'p',
            text: 'Beyond the Autopilot you can build custom strategies under Autopilot → Strategies and run multiple AI traders side by side.',
          },
        ],
      },
      {
        id: 'what-do-i-need',
        question: 'What do I need before launching the Autopilot?',
        blocks: [
          {
            type: 'p',
            text: 'Two connections, both set up in Settings:',
          },
          {
            type: 'list',
            items: [
              'An AI provider API key (DeepSeek, OpenAI, Claude, …) under Settings → AI models.',
              'A Binance or OKX futures API key with trading permission (withdrawals disabled) under Settings → Exchanges, and at least `12 USDT` available as margin.',
            ],
          },
          {
            type: 'p',
            text: 'Starting a trader runs a server-side preflight that checks every prerequisite and points you at the exact step that is missing, so you cannot start a half-configured bot.',
          },
        ],
      },
      {
        id: 'which-markets',
        question: 'Which markets can it trade?',
        blocks: [
          {
            type: 'p',
            text: 'Vergex trades USDT-margined crypto perpetuals on Binance and OKX. Candidate coins come from your strategy: a static list, or free Binance rankings (open interest, volume, gainers and losers).',
          },
        ],
      },
      {
        id: 'ai-models',
        question: 'Which AI models does it use? Do I need API keys?',
        blocks: [
          {
            type: 'p',
            text: 'Vergex connects directly to the AI provider you choose, with your own API key: DeepSeek, OpenAI, Claude, Gemini, Qwen, Grok, Kimi or MiniMax. A custom base URL and model name let you use any OpenAI-compatible endpoint.',
          },
          {
            type: 'p',
            text: 'Token usage of every AI call is recorded locally; estimated costs use public prices from models.dev. Models without a published price are tracked by tokens only. See Settings → AI costs.',
          },
        ],
      },
      {
        id: 'is-it-profitable',
        question: 'Will it make money?',
        blocks: [
          {
            type: 'p',
            text: 'No one can promise that, and you should distrust anyone who does. The AI trades a systematic process, but markets are adversarial and past performance never guarantees future results.',
          },
          {
            type: 'p',
            text: 'Each trader’s page is deliberately honest about performance: it separates realized from unrealized P/L and shows fees, profit factor and max drawdown from your real starting balance. Watch those numbers, start small, and only trade money you can afford to lose.',
          },
          {
            type: 'note',
            text: 'Trading involves substantial risk of loss. Vergex is software, not investment advice.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Launch & accounts ─────────────────────────
  {
    id: 'launch-accounts',
    title: 'Launch & accounts',
    icon: Zap,
    items: [
      {
        id: 'exchange-api-keys',
        question: 'How should I create the exchange API key? Is it safe?',
        blocks: [
          {
            type: 'steps',
            items: [
              'Create a dedicated API key on Binance or OKX for Vergex.',
              'Enable futures trading only. Never enable withdrawals.',
              'Restrict the key to your server IP (IP whitelist).',
              'OKX also requires the passphrase you set when creating the key.',
            ],
          },
          {
            type: 'p',
            text: 'Keys are stored encrypted in your own database and only used by your server to trade.',
          },
        ],
      },
      {
        id: 'launch-preflight',
        question: 'What does the launch preflight check?',
        blocks: [
          {
            type: 'p',
            text: 'Before anything is created or changed, the server verifies the full chain with live data:',
          },
          {
            type: 'list',
            items: [
              'AI model is enabled and has a credential.',
              'Exchange API credentials are complete and the account is reachable.',
              'Trading funds: at least `12 USDT` counting equity in open positions.',
            ],
          },
          {
            type: 'p',
            text: 'Each failing check names the exact fix. The same checks are enforced server-side on every start, so the UI cannot be bypassed accidentally.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Trading & execution ─────────────────────────
  {
    id: 'trading',
    title: 'Trading & execution',
    icon: TrendingUp,
    items: [
      {
        id: 'autopilot-pipeline',
        question: 'How does the Autopilot strategy work, step by step?',
        blocks: [
          {
            type: 'p',
            text: 'Every cycle runs the same funnel — each stage can reject a candidate, and only setups that survive all of them get traded:',
          },
          {
            type: 'steps',
            items: [
              'Build the universe — collect candidate coins from the strategy coin source (a static list or a ranking-based source), minus excluded coins.',
              'Read market context — multi-timeframe OHLCV candles plus the indicators enabled in the strategy (EMA, MACD, RSI, ATR, Bollinger, volume, open interest, funding rate).',
              'Add account context — equity, available margin, open positions and the trader’s own recent performance.',
              'Decide and size — only setups clearing the strategy’s confidence threshold and minimum risk/reward get positions; closes always execute before opens.',
            ],
          },
          {
            type: 'p',
            text: 'A final layer sits outside the AI entirely: hard risk controls (position cap, leverage caps, margin ceiling, trade throttle) reject any decision that violates them, no matter how confident the model is.',
          },
        ],
      },
      {
        id: 'data-sources',
        question: 'What data does it use?',
        blocks: [
          {
            type: 'list',
            items: [
              'Exchange market data — OHLCV candles, open interest and funding rate from free public APIs.',
              'Your account via the exchange API — equity, available margin, open positions with PnL.',
              'Its own history — closed trades feed win rate, profit factor and drawdown back into the next prompt, so the AI knows its recent form.',
            ],
          },
        ],
      },
      {
        id: 'decision-cycle',
        question: 'How often does the AI make decisions?',
        blocks: [
          {
            type: 'p',
            text: 'The Autopilot runs a scan cycle every 5–15 minutes depending on how you launched it (configurable per trader, minimum 3 minutes). The first cycle starts right after launch; a single cycle usually takes 30–60 seconds because the AI reads the full market context before deciding.',
          },
        ],
      },
      {
        id: 'what-ai-sees',
        question: 'What information does the AI see each cycle?',
        blocks: [
          {
            type: 'list',
            items: [
              'Your account: equity, available margin, open positions with PnL.',
              'Candidate coins from the strategy coin source.',
              'Multi-timeframe candles and the enabled indicators per candidate.',
              'Its own track record: win rate, profit factor, drawdown, recent trades.',
            ],
          },
          {
            type: 'p',
            text: 'Every cycle is stored as a decision record. The Decisions tab on the trader’s page shows the reasoning, actions and any blocked orders.',
          },
        ],
      },
      {
        id: 'leverage-and-risk',
        question: 'What leverage and risk controls does it use?',
        blocks: [
          {
            type: 'p',
            text: 'Leverage comes from the strategy risk settings (default 5x). Hard risk controls run outside the AI and cannot be overridden by it:',
          },
          {
            type: 'list',
            items: [
              'Position-count cap from the strategy config — new opens are rejected at the cap.',
              'Leverage limits per asset class (BTC/ETH vs altcoins).',
              'A trade throttle blocks churn, e.g. closing a barely-moved position minutes after opening it.',
              'Safe mode (below) protects the book when the AI itself is failing.',
            ],
          },
        ],
      },
      {
        id: 'safe-mode',
        question: 'What is safe mode?',
        blocks: [
          {
            type: 'p',
            text: 'If the AI fails 3 cycles in a row (provider outage, exhausted API quota, bad responses), the trader enters safe mode: no new positions are opened, existing positions keep their protection, and the loop keeps retrying. It exits safe mode automatically on the next successful AI call.',
          },
          {
            type: 'p',
            text: 'Safe mode shows as a banner on the trader’s page and under Needs attention on the Dashboard, so it never happens silently.',
          },
        ],
      },
      {
        id: 'trading-fees',
        question: 'What fees am I paying?',
        blocks: [
          {
            type: 'list',
            items: [
              'Exchange trading fees on every order (Binance or OKX fee schedule).',
              'AI provider costs, billed to your own provider account (token usage and estimated cost are shown under Settings → AI costs).',
            ],
          },
          {
            type: 'p',
            text: 'Fees are the silent killer of high-frequency strategies. The Performance tab on the trader’s page shows fees beside realized P/L, split by hold time and side, so you can see whether they are eating the edge.',
          },
        ],
      },
      {
        id: 'stop-and-manual',
        question: 'How do I stop the bot or close positions manually?',
        blocks: [
          {
            type: 'list',
            items: [
              'Stop: use Stop on the trader’s page, or Stop all on the Autopilot page. Stopping halts the decision loop; open positions remain open and are yours to manage.',
              'Manual close: close a position from the Positions tab on its trader’s page, or from Positions and orders on Frontier. Manual closes sync back into the position history.',
              'Emergencies: you can always manage positions directly on the exchange; Vergex never locks you out of your own account.',
            ],
          },
        ],
      },
    ],
  },

  // ───────────────────────── Strategies & grid ─────────────────────────
  {
    id: 'strategies',
    title: 'Strategies & grid',
    icon: Layers,
    items: [
      {
        id: 'strategy-editor',
        question: 'What can I change in a strategy?',
        blocks: [
          {
            type: 'p',
            text: 'Autopilot → Strategies edits everything. An AI strategy has four sections:',
          },
          {
            type: 'list',
            items: [
              'Coins: a static list, a ranking (open interest, volume, gainers, losers) or a mix of them, plus coins to always exclude.',
              'Risk: a preset profile or the individual limits, such as positions, leverage, position caps, margin usage and minimum confidence.',
              'Market data: the timeframes and bars the model reads, and which indicators are worked out from them.',
              'Prompt: the sections of the system prompt, and your own instructions added after them.',
            ],
          },
          {
            type: 'p',
            text: 'A strategy’s type, AI or grid, is set when you create it and cannot change. The bar at the bottom estimates how many tokens each cycle will use.',
          },
        ],
      },
      {
        id: 'strategy-test',
        question: 'What does Test do, and does it cost money?',
        blocks: [
          {
            type: 'p',
            text: 'Test tries the strategy as it stands in the form, saved or not, on live public market data and a 1000 USDT account that does not exist.',
          },
          {
            type: 'list',
            items: [
              'Preview prompts builds the prompts the model would receive. It calls no model and costs nothing.',
              'Run test also sends them to the model you choose, on your own API key, and your provider bills that request. The dialog states the estimated tokens first.',
            ],
          },
          {
            type: 'note',
            text: 'Neither places orders.',
          },
        ],
      },
      {
        id: 'strategy-running',
        question: 'Do saved changes affect traders already running?',
        blocks: [
          {
            type: 'p',
            text: 'Yes. A running trader reloads its strategy at the start of each cycle, so saved changes apply from its next cycle, including to coins and risk limits.',
          },
        ],
      },
      {
        id: 'grid-how',
        question: 'How does a grid strategy work?',
        blocks: [
          {
            type: 'p',
            text: 'It places layered orders across a price range on one symbol, within the investment and leverage you set. The range is either your own upper and lower price or worked out from ATR. The distribution sets how the size spreads across levels.',
          },
          {
            type: 'p',
            text: 'Grids earn in ranges and suffer in trends, so the trader checks for breakouts on 1h candles. A confirmed breakout of the short box reduces the position to 50%, the mid box pauses the grid and cancels its orders, and the long box also closes all positions. With direction adjustment on, breakouts can instead shift the grid to a long or short bias.',
          },
          {
            type: 'note',
            text: 'Max drawdown, daily loss limit and stop loss act as hard stops.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Frontier ─────────────────────────
  {
    id: 'frontier',
    title: 'Frontier',
    icon: Crosshair,
    items: [
      {
        id: 'frontier-what',
        question: 'What is Frontier, and how does it differ from Autopilot?',
        blocks: [
          {
            type: 'p',
            text: 'Frontier plans one trade at a time with you. You discuss a market with the AI on a live chart, compare entry plans and approve one. It is not a running strategy, which is what Autopilot is, and it is not a backtester.',
          },
        ],
      },
      {
        id: 'frontier-workflow',
        question: 'How do I plan and place a trade?',
        blocks: [
          {
            type: 'steps',
            items: [
              'Pick the market, exchange, account and leverage in the toolbar. Accounts are added in Settings.',
              'Choose an AI model beside the chart and describe the trade. It can read market data, draw on the chart and draft plans, but it cannot approve or place orders.',
              'Compare up to four drafts on the chart.',
              'Review prices, quantities, schedule, costs and account, then click Confirm and place live order. Only that click locks and starts the plan.',
            ],
          },
        ],
      },
      {
        id: 'frontier-entries',
        question: 'What kinds of entry can a plan use?',
        blocks: [
          {
            type: 'list',
            items: [
              'Market: sent right after approval within an approved price bound.',
              'Limit: one limit entry.',
              'Ladder: 2–8 weighted limit prices released together.',
              'TWAP: 2–24 fixed-price slices, 5–300 seconds apart, at most one hour in total. Late slices expire instead of catching up.',
            ],
          },
          {
            type: 'p',
            text: 'A draft expires five minutes after it is created, and unfilled limit entries expire after 15 minutes.',
          },
        ],
      },
      {
        id: 'frontier-protection',
        question: 'Is a plan’s stop-loss guaranteed?',
        blocks: [
          {
            type: 'p',
            text: 'No. Stop-loss and take-profit orders are placed after entry fills are seen, about every two seconds, so there is a gap between fill and protection, and placing them can fail. Very small partial fills may need handling by hand. Stops do not guarantee a maximum loss.',
          },
          {
            type: 'note',
            text: 'Stopping a plan cancels unsent and pending entries. It does not close filled positions or remove their protection.',
          },
        ],
      },
      {
        id: 'frontier-restart',
        question: 'What happens to a running plan if the server restarts?',
        blocks: [
          {
            type: 'p',
            text: 'It becomes “needs attention” and does not resume entries. Use Reconcile exchange state to resume reconciliation and protection monitoring. Stop and target orders already on the exchange keep working while the server is down.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Telegram ─────────────────────────
  {
    id: 'telegram',
    title: 'Telegram assistant',
    icon: Send,
    items: [
      {
        id: 'telegram-setup',
        question: 'How do I set up the Telegram assistant?',
        blocks: [
          {
            type: 'steps',
            items: [
              'Create a bot with @BotFather and paste its token into Settings → Telegram.',
              'Send /start to your bot. It binds to your account.',
              'Choose the AI model, or leave it on auto-select.',
            ],
          },
        ],
      },
      {
        id: 'telegram-can-do',
        question: 'What can it do?',
        blocks: [
          {
            type: 'list',
            items: [
              'Create, update, start, stop and delete traders and strategies.',
              'Answer questions about balances, positions, orders, decisions, statistics and AI costs.',
              'Close a position.',
              'Diagnose why a trader will not start, a model or account fails, or an order did not go through.',
            ],
          },
          {
            type: 'p',
            text: 'It uses the same server checks and risk limits as the web pages. Keys and endpoints for models and exchanges can only be changed in the web UI.',
          },
        ],
      },
      {
        id: 'telegram-confirm',
        question: 'Can it act on real money without asking?',
        blocks: [
          {
            type: 'p',
            text: 'Risky actions wait for you: starting, stopping or deleting a trader, updating a running trader, closing a position, deleting a strategy, and changing a strategy’s risk or grid settings. Your next message must be an explicit confirmation such as “confirm” or “yes”; anything else cancels it, and it expires after 5 minutes.',
          },
          {
            type: 'p',
            text: 'Code makes that check, not the model.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Forecast ─────────────────────────
  {
    id: 'forecast',
    title: 'Forecast',
    icon: LineChart,
    items: [
      {
        id: 'forecast-what',
        question: 'What is Forecast, and can I trade on it?',
        blocks: [
          {
            type: 'p',
            text: 'Forecast scores where a coin may go next from locally stored candles and derivatives data, records each call and checks it later. It is an experiment to find out whether such signals have value. Nothing in Vergex trades on it.',
          },
          {
            type: 'note',
            text: 'So far it has shown no edge. On two years of BTC, ETH and SOL, the default strategy was right 49.9% of the time while price rose 50.5% of the time anyway, and lost money per signal after fees.',
          },
        ],
      },
      {
        id: 'forecast-score',
        question: 'Why does it show a score instead of a probability?',
        blocks: [
          {
            type: 'p',
            text: 'A raw score is not a probability, so it shows as `Score 68`. Once a strategy version and interval have 200 non-overlapping verified forecasts, the scores are matched to what actually happened and shown as `68%`.',
          },
        ],
      },
      {
        id: 'forecast-checked',
        question: 'How are forecasts checked?',
        blocks: [
          {
            type: 'p',
            text: 'Every closed candle gets one forecast. When its target candle closes, 4 bars later by default, the call is judged: Hit if the direction was right, Miss if not, and Not judged if it was Neutral or price did not move. Each coin’s page shows its record.',
          },
        ],
      },
      {
        id: 'forecast-strategies',
        question: 'How do I change how forecasts are made?',
        blocks: [
          {
            type: 'steps',
            items: [
              'Open Forecast → Strategies and change the factors, weights, thresholds or horizons.',
              'Save as a new version. Saved versions never change, and records stay with the version that made them.',
              'Run a backtest on it from Forecast → Backtests.',
              'If the result holds up, click Use this version.',
            ],
          },
          {
            type: 'note',
            text: 'A strategy tuned until it looks good on past data usually fails on new data. Compare it against the base rate, on data you did not tune it on.',
          },
        ],
      },
      {
        id: 'forecast-data',
        question: 'Which coins are covered, and does it use up my API limits?',
        blocks: [
          {
            type: 'p',
            text: 'Settings → Forecast sets a watchlist, kept at 15m, 1h, 4h and 1d, and how many top coins by volume to keep at 1h, 4h and 1d. Opening any other coin fetches it on demand. Candles are stored locally and only new ones are fetched, within a small share of the Binance request allowance, and collection steps aside when the IP is busy.',
          },
        ],
      },
      {
        id: 'forecast-alerts',
        question: 'Can it alert me?',
        blocks: [
          {
            type: 'p',
            text: 'Yes, over Telegram. Watchlist coins alert when a score crosses the threshold in Settings → Forecast, once each time, and again only after coming back inside. Turn it off with Forecast signals in Settings → Telegram.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Dashboard & metrics ─────────────────────────
  {
    id: 'dashboard',
    title: 'Dashboard & metrics',
    icon: Monitor,
    items: [
      {
        id: 'metrics-meaning',
        question: 'What does the Dashboard show?',
        blocks: [
          {
            type: 'p',
            text: 'A read-only overview. Actions live on the page each thing belongs to, and the rows here link there.',
          },
          {
            type: 'list',
            items: [
              'Needs attention: problems such as safe mode, a rejected order or a Frontier plan that needs reconciling.',
              'Headline numbers: total equity, traders running, services healthy, positions, open orders and Autopilot P/L today.',
              'Orchestration: which AI models, traders, Frontier plans, accounts and background services are connected, and their state.',
              'Positions and recent orders across every trader and Frontier plan.',
            ],
          },
          {
            type: 'note',
            text: 'If an account cannot be read, its numbers show as “—” rather than 0, and the overview says some accounts could not be read.',
          },
        ],
      },
      {
        id: 'trader-metrics',
        question: 'What do the numbers on a trader’s page mean?',
        blocks: [
          {
            type: 'list',
            items: [
              'Equity: live account value including unrealized P/L.',
              'Realized P/L: net result of closed trades only; win rate, profit factor and drawdown are computed from these.',
              'Profit factor: gross wins ÷ gross losses on closed trades; above 1.0 means the closed trades are net positive.',
              'Max drawdown: the worst peak-to-trough dip of the realized equity curve, measured against your real starting balance.',
            ],
          },
        ],
      },
      {
        id: 'pl-contradiction',
        question:
          'Why can total P/L be positive while realized P/L is negative?',
        blocks: [
          {
            type: 'p',
            text: 'They measure different things. Realized P/L only counts closed trades; total P/L also includes the unrealized gains of positions still open. A trader can be down on its closed trades while its open positions carry enough unrealized profit to put the total in the green, and vice versa.',
          },
        ],
      },
      {
        id: 'execution-log',
        question: 'Where can I see why the AI did (or refused) something?',
        blocks: [
          {
            type: 'p',
            text: 'The Decisions tab on the trader’s page lists every cycle with the actions taken, the AI call duration and any blocked orders with the guard that fired (throttle, position cap, risk control). Open a cycle to read its prompts, reasoning and the model’s raw response.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Security ─────────────────────────
  {
    id: 'security',
    title: 'Security',
    icon: Shield,
    items: [
      {
        id: 'key-storage',
        question: 'How are my keys stored?',
        blocks: [
          {
            type: 'list',
            items: [
              'All secrets (AI provider keys and exchange API keys) are AES-256 encrypted at rest in your own database.',
              'Optional RSA transport encryption protects secrets in flight between browser and server.',
              'Vergex is self-hosted: nothing is sent to any third-party server. The code is open source and auditable.',
            ],
          },
        ],
      },
      {
        id: 'can-vergex-steal-funds',
        question: 'Can Vergex withdraw or steal my funds?',
        blocks: [
          {
            type: 'p',
            text: 'Not if the API key cannot withdraw. Create the exchange API key with trading permission only — disable withdrawals and set an IP whitelist. Your funds stay in your own exchange account.',
          },
        ],
      },
      {
        id: 'registration-model',
        question: 'Why can’t anyone else register on my instance?',
        blocks: [
          {
            type: 'p',
            text: 'By design, an instance is single-operator: the first account registered becomes the operator and registration closes ("System already initialized"). This prevents strangers from creating accounts on an exposed deployment. Run one instance per operator.',
          },
        ],
      },
    ],
  },

  // ───────────────────────── Self-hosting ─────────────────────────
  {
    id: 'self-hosting',
    title: 'Self-hosting & troubleshooting',
    icon: Wrench,
    items: [
      {
        id: 'how-to-install',
        question: 'How do I install Vergex?',
        blocks: [
          {
            type: 'p',
            text: 'Build and run locally with Docker from your clone of the repository:',
          },
          {
            type: 'list',
            items: [
              'Docker: run `make up` (creates .env with fresh keys on first run, then builds and starts the containers).',
              'Windows: install Docker Desktop and run the same commands from WSL.',
              'From source: Go and Node.js, then `make run-backend` and `make run-frontend`.',
            ],
          },
          {
            type: 'p',
            text: 'Then open `http://127.0.0.1:8000`. The web UI proxies the API under /api/. For remote access, put a TLS reverse proxy in front (see docs/deployment.md).',
          },
        ],
      },
      {
        id: 'how-to-update',
        question: 'How do I update?',
        blocks: [
          {
            type: 'p',
            text: 'Pull your repository changes and run `make up` to rebuild. Your database lives in the mounted `data/` directory and your keys in .env, so both survive updates. Running traders are restarted automatically after the backend comes back.',
          },
        ],
      },
      {
        id: 'launch-blocked',
        question: 'Launch is blocked by a failing check — now what?',
        blocks: [
          {
            type: 'p',
            text: 'Read the message: every preflight failure names its fix and routes you to the right setup step — fix the AI model credential, complete the exchange API key, or deposit trading USDT. Balances are re-checked live, so once you fix the item the launch goes through.',
          },
        ],
      },
      {
        id: 'exchange-unreachable',
        question:
          'The exchange account shows "invalid credentials" or "unavailable".',
        blocks: [
          {
            type: 'list',
            items: [
              'Invalid credentials: the saved API key was revoked, expired or mistyped — create a new key and update the exchange account.',
              'Unavailable: the exchange API did not respond; the account state is cached for 30 seconds, so wait and refresh.',
              'CEX keys: verify trading permission, IP whitelist, and that futures/perp access is enabled.',
            ],
          },
        ],
      },
      {
        id: 'where-are-logs',
        question: 'Where are the logs?',
        blocks: [
          {
            type: 'list',
            items: [
              'Backend: `docker logs vergex-backend` (or the terminal running `make run-backend`).',
              'Per-cycle AI reasoning and errors: the Decisions tab on the trader’s page.',
              'Frontend build/runtime issues: browser devtools console.',
            ],
          },
        ],
      },
      {
        id: 'port-conflicts',
        question: 'Port 8000 or 3000 is already in use.',
        blocks: [
          {
            type: 'p',
            text: 'Stop the conflicting service or choose other host ports in .env (`VERGEX_FRONTEND_PORT`, and `VERGEX_BACKEND_PORT` if you published the API), then run `make up` again.',
          },
        ],
      },
    ],
  },
]

// Help is written in English for now; other languages fall back to it.
export function faqCategories(_language: Language): FAQCategory[] {
  return english
}
