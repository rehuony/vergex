<p align="center">
  <img src="docs/assets/vergex-banner.svg" alt="vergex" width="100%"/>
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README.zh-CN.md">中文</a>
</p>

vergex 是一个自托管的 AI 交易系统，面向 **Binance** 与 **OKX** 的加密货币 USDT 永续合约。模型 API Key 和交易所 API Key 都由你自己提供；不经过任何付费网关，也不会把使用数据发送到你的机器之外。

- **Frontier** — 在实时图表上与 AI 一起推演交易，比较多种入场方案（市价、限价、阶梯、TWAP），每一笔订单都由你亲自批准。
- **Traders** — 持续运行策略：trader 读取行情、请模型给出决策，并在策略的风控限制内下单。
- **网格交易** — 带市场状态判断的网格策略。
- **Telegram** — 通过 Telegram 机器人管理 trader 和策略、查看持仓、排查问题、接收告警。
- **Forecast** — 基于本地行情数据的实验性币种信号，带可命名的策略和回测，每次判断都会事后核对。不用于交易，目前也没有显示出优势。

## 运行

需要：Docker（含 Compose）、`make`、`openssl`。

```bash
git clone <你的 vergex 仓库> vergex
cd vergex
make up
```

首次运行时，`make up` 会从 `.env.example` 生成 `.env`，补齐缺失的 `JWT_SECRET`、`DATA_ENCRYPTION_KEY` 和 `RSA_PRIVATE_KEY`，然后构建并启动容器。打开 **http://127.0.0.1:8000**。

| 命令 | 作用 |
| :--- | :--- |
| `make init` | 创建 `.env` 并补齐缺失的密钥（已有值不覆盖） |
| `make up` | 先执行 `init`，再构建并启动 |
| `make down` | 停止并移除容器（保留数据库卷和 `data/`） |
| `make restart` | 重启容器 |
| `make logs [SERVICE=vergex-backend]` | 跟随日志 |
| `make status` | 容器状态与健康检查 |
| `make regen-keys` | 重新生成全部密钥——已保存的交易所和模型密钥将无法解密 |

界面及其 `/api/` 代理监听在 `VERGEX_BIND_ADDRESS:VERGEX_FRONTEND_PORT`（默认 `127.0.0.1:8000`）。远程访问时保持该绑定，在前面加一层带额外认证的 TLS 反向代理，见 [docs/deployment.md](docs/deployment.md)。

更新时拉取你的改动后再次执行 `make up`。请保留 `.env`、`data/` 和 `vergex-postgres` 卷：它们保存密钥和数据库。停止服务不会平仓，重启后请到交易所核对。

### 不使用 Docker 开发

使用 [go.mod](go.mod) 中的 Go 版本、用于 PostgreSQL 的 Docker、Node.js 24 和 pnpm（执行一次 `corepack enable`，版本固定在 `web/package.json`）。

```bash
make init          # 只需一次，用于生成 .env 中的密钥
make run-backend   # 按 .env 启动 PostgreSQL，后端监听 :3000
make run-frontend  # 前端开发服务器 :8000，/api 代理到 :3000
make dev           # 同时启动前后端
```

`make test` 运行 Go 测试（需要 PostgreSQL，可用 `make run-postgresql` 启动）与前端测试；`make lint` 运行 golangci-lint 和 ESLint。

## 首次使用

1. **创建账号。** 第一个注册的用户就是本实例唯一的用户。
2. **在 Settings → AI models 中添加 AI 模型**：DeepSeek、OpenAI、Claude、Qwen、Gemini、Grok、Kimi 或 MiniMax，使用你自己的 API Key，可选自定义 base URL 和模型名。
3. **在 Settings → Exchanges 中添加交易所账户**：Binance 或 OKX USDT 永续（[API Key 指南](docs/exchanges.md)）。把 Key 绑定到该页面显示的服务器 IP。
4. **打开 Frontier** 规划交易，或在 Strategies 和 Traders 页面创建策略和 trader。

Frontier 的每一笔订单都需要批准。运行中的 trader 会自行下单；启动和停止是单独的操作。

## 模型、数据与成本

模型直接用你的 Key 调用。每次调用的 token 用量都保存在本地，并按公开的 [models.dev](https://models.dev) 价格表估算；Settings → AI costs 中按模型显示估算成本。没有公开价格的模型显示为未定价。

行情数据来自免费公开接口：交易所实时流提供 K 线和价格并存入本地历史，另有交易所盘口，以及基于 Binance 公开数据计算的选币排行（24h 成交额、涨幅榜、跌幅榜、持仓量变化）。OKX trader 的排行候选只保留 OKX 上可交易的币。

## 执行机制

- **可审阅的订单。** Frontier 在批准前展示账户、仓位、入场和出场细节。
- **按账户计算仓位。** Frontier 根据所选杠杆和可用保证金计算仓位并预留成本；价格精度、数量精度和交易所限制依然适用。
- **交易所侧止盈止损。** 观察到成交后挂出止损和止盈单。入场与保护不是原子操作。
- **对账。** 结果不确定的下单会到交易所核对，而不是盲目重发。
- **本地密钥。** 交易所和模型密钥用 `DATA_ENCRYPTION_KEY` 进行 AES 加密存储。请保护好 `.env`、数据库和备份。

## 架构

```text
Browser (React)                Telegram bot
  Dashboard · Forecast ·         skill-first agent
  Frontier · Traders ·                │
  Strategies · Settings               │
          │                           │
          └──────── Gin API (/api) ───┘
                        │
        ┌───────────────┼────────────────┐
   Frontier planning  Traders         Grid engine
   (planning/)        (trader/, kernel/)
        └───────────────┼────────────────┘
               Exchange adapters
               trader/binance · trader/okx
                        │
            Binance / OKX USDT perpetuals

Models: llm/ provider clients     Storage: PostgreSQL (GORM)
```

Go 包都在 `internal/` 下。代码结构见 [docs/architecture.md](docs/architecture.md)，开发约定见 [AGENTS.md](AGENTS.md)。

## 文档

技术文档只提供英文版。

| 指南 | 内容 |
| :--- | :--- |
| [入门](docs/getting-started.md) | 安装、首次使用、备份与账户恢复 |
| [AI 模型](docs/models.md) | Provider、自定义接口与成本 |
| [交易所 API Key](docs/exchanges.md) | Binance 与 OKX 配置 |
| [Frontier](docs/frontier.md) | 规划、实盘执行与恢复 |
| [Traders 和策略](docs/traders.md) | 策略、提示词、风控上限与网格 |
| [Telegram](docs/telegram.md) | 助手配置、skill、告警与诊断 |
| [Market data](docs/market-data.md) | 本地 K 线、限流与历史数据 |
| [Forecast](docs/forecast.md) | 实验性信号、策略与回测 |
| [远程访问](docs/deployment.md) | 带 TLS 和认证的反向代理 |
| [故障排查](docs/troubleshooting.md) | 常见问题与解决办法 |
| [架构](docs/architecture.md) | 代码结构与主要链路 |

## 许可证

[AGPL-3.0](LICENSE)。vergex 是 NOFX（github.com/NoFxAiOS/nofx）的个人 fork，独立维护。

<sub>自动交易存在重大风险。AI 驱动的策略仍属实验性质，可能亏损。请合理控制仓位，了解每个交易所的规则，切勿投入无法承受损失的资金。详见 [DISCLAIMER.md](DISCLAIMER.md)。</sub>
