import type { FAQContent } from '@/i18n/types'

export const faq: FAQContent = [
  {
    id: 'getting-started',
    title: '开始使用',
    items: [
      {
        id: 'what-is-vergex',
        question: 'Vergex 是什么？',
        blocks: [
          {
            type: 'p',
            text: 'Vergex 是一个开源、自托管的 Agent 交易终端。交易员按计划运行一个 Agent：它读取策略给出的候选币种、行情数据和你的账户状态，然后在你连接的交易所上决策并执行。这一切都运行在你自己的机器上，密钥不会离开你的服务器。',
          },
          {
            type: 'p',
            text: '你可以在“策略”页面编写自定义策略，并同时运行多个交易员。',
          },
        ],
      },
      {
        id: 'what-do-i-need',
        question: '启动交易员之前需要准备什么？',
        blocks: [
          {
            type: 'p',
            text: '需要两个连接，都在“设置”中配置：',
          },
          {
            type: 'list',
            items: [
              '在“设置 → 模型”中添加模型服务商的 API Key（DeepSeek、OpenAI、Claude 等）。',
              '在“设置 → 交易所”中添加具备交易权限（禁用提现）的 Binance 或 OKX 合约 API Key，并确保至少有 `12 USDT` 可用保证金。',
            ],
          },
          {
            type: 'p',
            text: '启动交易员时，服务器会运行启动检查，核对每一项前提条件，并指出具体缺少哪一步，所以不会启动一个配置不完整的交易员。',
          },
        ],
      },
      {
        id: 'which-markets',
        question: '可以交易哪些市场？',
        blocks: [
          {
            type: 'p',
            text: 'Vergex 交易 Binance 和 OKX 上以 USDT 保证金结算的加密货币永续合约。候选币种来自你的策略：固定列表，或 Binance 免费榜单（持仓量、成交额、涨幅榜和跌幅榜）。',
          },
        ],
      },
      {
        id: 'models',
        question: '它使用哪些模型？需要 API Key 吗？',
        blocks: [
          {
            type: 'p',
            text: 'Vergex 使用你自己的 API Key 直接连接你选择的模型服务商：DeepSeek、OpenAI、Claude、Gemini、Qwen、Grok、Kimi 或 MiniMax。通过自定义 Base URL 和模型名称，可以使用任何兼容 OpenAI 的接口。',
          },
          {
            type: 'p',
            text: '每次模型调用的 token 用量都记录在本地，预估费用使用 models.dev 的公开价格。没有公开价格的模型只统计 token。详见“设置 → Agent 费用”。',
          },
        ],
      },
      {
        id: 'is-it-profitable',
        question: '它能赚钱吗？',
        blocks: [
          {
            type: 'p',
            text: '没有人能保证这一点，也不要相信这样承诺的人。Agent 按系统化的流程交易，但市场是对抗性的，过去的表现不能保证未来的结果。',
          },
          {
            type: 'p',
            text: '每个交易员的页面都如实展示表现：区分已实现和未实现盈亏，并按你真实的初始余额显示手续费、盈利因子和最大回撤。请关注这些数字，从小资金开始，只用你能承受损失的钱交易。',
          },
          {
            type: 'note',
            text: '交易存在重大亏损风险。Vergex 是软件，不构成投资建议。',
          },
        ],
      },
    ],
  },
  {
    id: 'launch-accounts',
    title: '启动和账户',
    items: [
      {
        id: 'exchange-api-keys',
        question: '应该如何创建交易所 API Key？安全吗？',
        blocks: [
          {
            type: 'steps',
            items: [
              '在 Binance 或 OKX 上为 Vergex 创建专用的 API Key。',
              '只开启合约交易权限，绝不要开启提现。',
              '把这个 Key 限制到你的服务器 IP（IP 白名单）。',
              'OKX 还需要创建 Key 时设置的 Passphrase。',
            ],
          },
          {
            type: 'p',
            text: 'Key 加密保存在你自己的数据库中，只有你的服务器会用它交易。',
          },
        ],
      },
      {
        id: 'launch-preflight',
        question: '启动检查会检查什么？',
        blocks: [
          {
            type: 'p',
            text: '在创建或更改任何内容之前，服务器会用实时数据核对完整的链路：',
          },
          {
            type: 'list',
            items: [
              '模型已启用并保存了凭证。',
              '交易所 API 凭证完整，账户可以访问。',
              '交易资金：计入持仓权益后至少有 `12 USDT`。',
            ],
          },
          {
            type: 'p',
            text: '每一项失败的检查都会写明具体的修复方法。每次启动时服务器都会执行同样的检查，因此界面不会被意外绕过。',
          },
        ],
      },
    ],
  },
  {
    id: 'trading',
    title: '交易和执行',
    items: [
      {
        id: 'trader-pipeline',
        question: '交易员是如何一步步工作的？',
        blocks: [
          {
            type: 'p',
            text: '每一轮都走同样的筛选流程，每个环节都可以淘汰候选，只有通过所有环节的机会才会被交易：',
          },
          {
            type: 'steps',
            items: [
              '确定范围：从策略的币种来源（固定列表或榜单）收集候选币种，并去掉排除的币种。',
              '读取行情：多周期 OHLCV K 线，以及策略启用的指标（EMA、MACD、RSI、ATR、布林带、成交量、持仓量、资金费率）。',
              '加入账户信息：权益、可用保证金、当前持仓和交易员自己的近期表现。',
              '决策和计算仓位：只有达到策略置信度阈值和最低盈亏比的机会才会开仓；平仓总是先于开仓执行。',
            ],
          },
          {
            type: 'p',
            text: '最后一层完全独立于 Agent：硬性风控（持仓数上限、杠杆上限、保证金上限、交易节流）会拒绝任何违反规则的决策，无论模型多么自信。',
          },
        ],
      },
      {
        id: 'data-sources',
        question: '它使用哪些数据？',
        blocks: [
          {
            type: 'list',
            items: [
              '交易所行情：来自免费公开接口的 OHLCV K 线、持仓量和资金费率。',
              '通过交易所 API 读取你的账户：权益、可用保证金、带盈亏的当前持仓。',
              '它自己的历史：已平仓交易的胜率、盈利因子和回撤会写入下一轮提示词，让 Agent 了解自己的近期状态。',
            ],
          },
        ],
      },
      {
        id: 'decision-cycle',
        question: 'Agent 多久决策一次？',
        blocks: [
          {
            type: 'p',
            text: '交易员每 5–15 分钟运行一轮扫描，取决于你的启动方式（每个交易员可单独配置，最短 3 分钟）。第一轮在启动后立即开始；单轮通常需要 30–60 秒，因为 Agent 会先读取完整的行情再决策。',
          },
        ],
      },
      {
        id: 'what-the-agent-sees',
        question: 'Agent 每一轮能看到哪些信息？',
        blocks: [
          {
            type: 'list',
            items: [
              '你的账户：权益、可用保证金、带盈亏的当前持仓。',
              '策略币种来源给出的候选币种。',
              '每个候选币种的多周期 K 线和已启用的指标。',
              '它自己的战绩：胜率、盈利因子、回撤和近期交易。',
            ],
          },
          {
            type: 'p',
            text: '每一轮都会保存为一条决策记录。交易员页面的“决策”标签会显示推理过程、操作和被拦截的订单。',
          },
        ],
      },
      {
        id: 'leverage-and-risk',
        question: '它使用什么杠杆和风控？',
        blocks: [
          {
            type: 'p',
            text: '杠杆来自策略的风险设置（默认 5 倍）。硬性风控独立于 Agent 运行，Agent 无法绕过：',
          },
          {
            type: 'list',
            items: [
              '策略配置的持仓数上限：达到上限后拒绝新开仓。',
              '按资产类别设置的杠杆上限（BTC/ETH 和山寨币）。',
              '交易节流阻止频繁操作，例如开仓几分钟、价格几乎没动就平仓。',
              '安全模式（见下文）在模型本身出问题时保护账户。',
            ],
          },
        ],
      },
      {
        id: 'safe-mode',
        question: '什么是安全模式？',
        blocks: [
          {
            type: 'p',
            text: '如果模型连续 3 轮失败（服务商故障、API 额度用尽、响应异常），交易员会进入安全模式：不再开新仓，现有持仓保留保护单，循环继续重试。下一次模型调用成功后会自动退出安全模式。',
          },
          {
            type: 'p',
            text: '安全模式会以横幅显示在交易员页面，并出现在总览的“需要处理”中，不会悄悄发生。',
          },
        ],
      },
      {
        id: 'trading-fees',
        question: '我要支付哪些费用？',
        blocks: [
          {
            type: 'list',
            items: [
              '每笔订单的交易所手续费（按 Binance 或 OKX 的费率）。',
              '模型服务商的费用，直接计入你自己的服务商账户（token 用量和预估费用显示在“设置 → Agent 费用”）。',
            ],
          },
          {
            type: 'p',
            text: '手续费是高频策略的隐形杀手。交易员页面的“表现”标签会把手续费和已实现盈亏放在一起，并按持仓时长和方向拆分，让你看清它们是否吃掉了收益。',
          },
        ],
      },
      {
        id: 'stop-and-manual',
        question: '如何停止机器人或手动平仓？',
        blocks: [
          {
            type: 'list',
            items: [
              '停止：在交易员页面点“停止”，或在“交易员”页面点“全部停止”。停止只会中断决策循环，持仓不会被平掉，由你自行管理。',
              '手动平仓：在交易员页面的“持仓”标签平仓，或在 Frontier 的“持仓和订单”中平仓。手动平仓会同步到持仓历史。',
              '紧急情况：你随时可以直接在交易所管理持仓，Vergex 不会把你锁在自己的账户之外。',
            ],
          },
        ],
      },
    ],
  },
  {
    id: 'strategies',
    title: '策略和网格',
    items: [
      {
        id: 'strategy-editor',
        question: '策略里可以修改什么？',
        blocks: [
          {
            type: 'p',
            text: '所有内容都在“策略”页面编辑。Agent 策略有四个分区：',
          },
          {
            type: 'list',
            items: [
              '币种：固定列表、榜单（持仓量、成交额、涨幅榜、跌幅榜）或它们的组合，以及始终排除的币种。',
              '风险：预设档位或单独的限制，例如持仓数、杠杆、仓位上限、保证金使用率和最低置信度。',
              '行情数据：模型读取的周期和 K 线数量，以及据此计算的指标。',
              '提示词：系统提示词的各个部分，以及附加在后面的自定义说明。',
            ],
          },
          {
            type: 'p',
            text: '策略类型（Agent 或网格）在创建时确定，之后不能更改。底部的栏会估算每轮使用的 token 数量。',
          },
        ],
      },
      {
        id: 'strategy-test',
        question: '“测试”会做什么，要花钱吗？',
        blocks: [
          {
            type: 'p',
            text: '测试会用表单中当前的策略（无论是否保存），在实时公开行情和一个并不存在的 1000 USDT 账户上试运行。',
          },
          {
            type: 'list',
            items: [
              '“预览提示词”只生成模型会收到的提示词，不调用模型，也不花钱。',
              '“运行测试”还会用你自己的 API Key 把它们发送给你选择的模型，这次请求由服务商向你计费。对话框会先显示预估的 token 数量。',
            ],
          },
          {
            type: 'note',
            text: '两者都不会下单。',
          },
        ],
      },
      {
        id: 'strategy-running',
        question: '保存的修改会影响正在运行的交易员吗？',
        blocks: [
          {
            type: 'p',
            text: '会。运行中的交易员在每一轮开始时重新加载策略，所以保存的修改从下一轮开始生效，包括币种和风险限制。',
          },
        ],
      },
      {
        id: 'grid-how',
        question: '网格策略是如何工作的？',
        blocks: [
          {
            type: 'p',
            text: '它在一个币种的价格区间内分层挂单，不超过你设置的投入和杠杆。区间可以是你指定的上下限，也可以根据 ATR 计算。分布决定仓位如何分配到各个价位。',
          },
          {
            type: 'p',
            text: '网格在震荡中赚钱，在趋势中吃亏，所以交易员会用 1 小时 K 线检查突破。确认突破短期箱体时把仓位减到 50%，突破中期箱体时暂停网格并撤单，突破长期箱体时还会平掉所有持仓。开启方向调整后，突破也可以让网格转为偏多或偏空。',
          },
          {
            type: 'note',
            text: '最大回撤、每日亏损上限和止损都是硬性止损。',
          },
        ],
      },
    ],
  },
  {
    id: 'frontier',
    title: 'Frontier',
    items: [
      {
        id: 'frontier-what',
        question: 'Frontier 是什么，和交易员有什么区别？',
        blocks: [
          {
            type: 'p',
            text: 'Frontier 和你一起一次规划一笔交易。你在实时图表上和 Agent 讨论行情，对比入场计划，然后批准其中一个。它不是持续运行的策略（那是交易员），也不是回测工具。',
          },
        ],
      },
      {
        id: 'frontier-workflow',
        question: '如何规划并下单？',
        blocks: [
          {
            type: 'steps',
            items: [
              '在工具栏选择行情、交易所、账户和杠杆。账户在“设置”中添加。',
              '在图表旁选择模型并描述交易。它可以读取行情、在图表上绘制和起草计划，但不能批准或下单。',
              '在图表上对比最多四份草稿。',
              '复核价格、数量、计划、成本和账户，然后点击“确认并实盘下单”。只有这次点击才会锁定并启动计划。',
            ],
          },
        ],
      },
      {
        id: 'frontier-entries',
        question: '计划可以使用哪些入场方式？',
        blocks: [
          {
            type: 'list',
            items: [
              '市价：批准后立即发送，且不超出批准的价格范围。',
              '限价：一笔限价入场。',
              '阶梯：2–8 个加权限价，一起释放。',
              'TWAP：2–24 份固定价格的分批订单，间隔 5–300 秒，总时长不超过一小时。迟到的分批会过期，不会补单。',
            ],
          },
          {
            type: 'p',
            text: '草稿在创建五分钟后过期，未成交的限价入场在 15 分钟后过期。',
          },
        ],
      },
      {
        id: 'frontier-protection',
        question: '计划的止损有保证吗？',
        blocks: [
          {
            type: 'p',
            text: '没有。止损和止盈单在看到入场成交后才下单，大约每两秒检查一次，所以成交和保护之间存在空档，下单也可能失败。很小的部分成交可能需要手动处理。止损不能保证最大亏损。',
          },
          {
            type: 'note',
            text: '停止计划会取消未发送和挂单中的入场，但不会平掉已成交的仓位，也不会移除它们的保护单。',
          },
        ],
      },
      {
        id: 'frontier-restart',
        question: '服务器重启时，运行中的计划会怎样？',
        blocks: [
          {
            type: 'p',
            text: '它会变成“需要对账”，不会恢复入场。使用“对账交易所状态”恢复对账和保护监控。已经在交易所的止损和目标单在服务器停机期间仍然有效。',
          },
        ],
      },
    ],
  },
  {
    id: 'telegram',
    title: 'Telegram 助手',
    items: [
      {
        id: 'telegram-setup',
        question: '如何设置 Telegram 助手？',
        blocks: [
          {
            type: 'steps',
            items: [
              '通过 @BotFather 创建机器人，把 Token 粘贴到“设置 → Telegram”。',
              '向你的机器人发送 /start，它会绑定到你的账户。',
              '选择模型，或保持自动选择。',
            ],
          },
        ],
      },
      {
        id: 'telegram-can-do',
        question: '它能做什么？',
        blocks: [
          {
            type: 'list',
            items: [
              '创建、更新、启动、停止和删除交易员和策略。',
              '回答余额、持仓、订单、决策、统计和 Agent 费用方面的问题。',
              '平仓。',
              '诊断交易员无法启动、模型或账户故障、订单未成功的原因。',
            ],
          },
          {
            type: 'p',
            text: '它和网页使用同样的服务器检查和风险限制。模型和交易所的密钥和接口地址只能在网页中修改。',
          },
        ],
      },
      {
        id: 'telegram-confirm',
        question: '它会不经询问就动用真实资金吗？',
        blocks: [
          {
            type: 'p',
            text: '高风险操作会等待你确认：启动、停止或删除交易员，更新运行中的交易员，平仓，删除策略，以及修改策略的风险或网格设置。你的下一条消息必须是明确的确认，例如“确认”或“yes”；其他任何内容都会取消，且 5 分钟后过期。',
          },
          {
            type: 'p',
            text: '这项检查由代码完成，不由模型决定。',
          },
        ],
      },
    ],
  },
  {
    id: 'forecast',
    title: '预测',
    items: [
      {
        id: 'forecast-what',
        question: '预测是什么，可以用它交易吗？',
        blocks: [
          {
            type: 'p',
            text: '预测根据本地保存的 K 线和衍生品数据，为币种接下来的走势打分，记录每次判断并在之后核对。这是一项实验，用来验证这类信号是否有价值。Vergex 中没有任何功能依据它交易。',
          },
          {
            type: 'note',
            text: '到目前为止，它没有显示出优势。在 BTC、ETH 和 SOL 两年的数据上，默认策略的正确率是 49.9%，而价格本身上涨的比例是 50.5%，扣除手续费后每个信号都在亏钱。',
          },
        ],
      },
      {
        id: 'forecast-score',
        question: '为什么显示评分而不是概率？',
        blocks: [
          {
            type: 'p',
            text: '原始评分不是概率，所以显示为 `评分 68`。当一个策略和周期积累了 200 条不重叠且已验证的预测后，评分会与实际结果校准，并显示为 `68%`。',
          },
        ],
      },
      {
        id: 'forecast-checked',
        question: '预测是如何核对的？',
        blocks: [
          {
            type: 'p',
            text: '每根收盘的 K 线都有一条预测。当目标 K 线收盘时（默认 4 根之后）进行判定：方向正确算命中，错误算未命中，中性或价格没有变化则不判定。每个币种的页面都会显示它的记录。',
          },
        ],
      },
      {
        id: 'forecast-strategies',
        question: '如何修改预测的生成方式？',
        blocks: [
          {
            type: 'steps',
            items: [
              '打开“预测 → 策略”，修改因子、权重、阈值或预测跨度。',
              '另存为新策略。已保存的策略不会改变，记录也始终属于生成它们的策略。',
              '在“预测 → 回测”中对它运行回测。',
              '如果结果可靠，在“预测 → 策略”中启用它。',
            ],
          },
          {
            type: 'note',
            text: '为了在历史数据上好看而反复调整的策略，通常在新数据上会失效。请用未参与调整的数据，并与基准上涨率对比。',
          },
        ],
      },
      {
        id: 'forecast-data',
        question: '覆盖哪些币种，会占用我的 API 额度吗？',
        blocks: [
          {
            type: 'p',
            text: '“预测 → 设置”可以设置关注列表（保留 15 分钟、1 小时、4 小时和 1 天周期），以及按成交额保留多少个热门币种（保留 1 小时、4 小时和 1 天周期）。打开其他币种时会按需获取。K 线保存在本地，只获取新数据，只占用 Binance 请求额度的一小部分，并且在 IP 繁忙时自动让路。',
          },
        ],
      },
      {
        id: 'forecast-alerts',
        question: '它能给我发告警吗？',
        blocks: [
          {
            type: 'p',
            text: '可以，通过 Telegram。关注列表中的币种在评分越过“预测 → 设置”中的阈值时告警一次，回到阈值内之后才会再次告警。可以在“设置 → Telegram”中关闭“预测信号”。',
          },
        ],
      },
    ],
  },
  {
    id: 'dashboard',
    title: '总览和指标',
    items: [
      {
        id: 'metrics-meaning',
        question: '总览显示什么？',
        blocks: [
          {
            type: 'p',
            text: '这是一个只读的概览。操作都在各自所属的页面上，这里的行会链接过去。',
          },
          {
            type: 'list',
            items: [
              '需要处理：安全模式、账户无法读取、Frontier 计划需要对账等问题。',
              '场景：系统的每个部分，以及每个真实事件发生时在它们之间的流动。',
              '事件：最近 24 小时发生的事，可以筛选并打开。',
              '所有交易员和 Frontier 计划的持仓。',
            ],
          },
          {
            type: 'note',
            text: '如果账户无法读取，它的数字显示为“—”而不是 0，总览也会提示部分账户无法读取。',
          },
        ],
      },
      {
        id: 'trader-metrics',
        question: '交易员页面上的数字是什么意思？',
        blocks: [
          {
            type: 'list',
            items: [
              '权益：包含未实现盈亏的实时账户价值。',
              '已实现盈亏：只计算已平仓交易的净结果；胜率、盈利因子和回撤都由它计算。',
              '盈利因子：已平仓交易的总盈利 ÷ 总亏损；大于 1.0 表示已平仓交易整体盈利。',
              '最大回撤：已实现权益曲线从高点到低点的最大跌幅，按你真实的初始余额计算。',
            ],
          },
        ],
      },
      {
        id: 'pl-contradiction',
        question: '为什么总盈亏是正的，已实现盈亏却是负的？',
        blocks: [
          {
            type: 'p',
            text: '它们衡量的内容不同。已实现盈亏只计算已平仓交易；总盈亏还包括未平仓持仓的未实现收益。交易员可能在已平仓交易上亏损，而持仓的未实现盈利足以让总盈亏转正，反之亦然。',
          },
        ],
      },
      {
        id: 'execution-log',
        question: '在哪里能看到 Agent 为什么这么做（或拒绝这么做）？',
        blocks: [
          {
            type: 'p',
            text: '交易员页面的“决策”标签列出每一轮的操作、模型调用耗时，以及被拦截的订单和触发的规则（节流、持仓上限、风控）。打开一轮可以查看它的提示词、推理过程和模型的原始回复。',
          },
        ],
      },
    ],
  },
  {
    id: 'security',
    title: '安全',
    items: [
      {
        id: 'key-storage',
        question: '我的密钥是如何保存的？',
        blocks: [
          {
            type: 'list',
            items: [
              '所有密钥（模型服务商和交易所的 API Key）都用 AES-256 加密保存在你自己的数据库中。',
              '可选的 RSA 传输加密保护浏览器和服务器之间传输的密钥。',
              'Vergex 是自托管的：不会向任何第三方服务器发送数据。代码开源，可以审计。',
            ],
          },
        ],
      },
      {
        id: 'can-vergex-steal-funds',
        question: 'Vergex 能提走或盗取我的资金吗？',
        blocks: [
          {
            type: 'p',
            text: '只要 API Key 没有提现权限就不能。创建交易所 API Key 时只开启交易权限，禁用提现并设置 IP 白名单。你的资金始终在你自己的交易所账户中。',
          },
        ],
      },
      {
        id: 'registration-model',
        question: '为什么别人不能在我的实例上注册？',
        blocks: [
          {
            type: 'p',
            text: '这是有意的设计：一个实例只有一个操作者。第一个注册的账户成为操作者，之后注册就会关闭（“System already initialized”）。这样可以防止陌生人在暴露的部署上创建账户。每个操作者运行一个实例。',
          },
        ],
      },
    ],
  },
  {
    id: 'self-hosting',
    title: '自托管和故障排查',
    items: [
      {
        id: 'how-to-install',
        question: '如何安装 Vergex？',
        blocks: [
          {
            type: 'p',
            text: '在你克隆的仓库中用 Docker 本地构建并运行：',
          },
          {
            type: 'list',
            items: [
              'Docker：运行 `make up`（首次运行会生成带新密钥的 .env，然后构建并启动容器）。',
              'Windows：安装 Docker Desktop，并在 WSL 中运行同样的命令。',
              '从源码运行：安装 Go 和 Node.js，然后运行 `make run-backend` 和 `make run-frontend`。',
            ],
          },
          {
            type: 'p',
            text: '然后打开 `http://127.0.0.1:8000`。网页界面通过 /api/ 代理接口。如需远程访问，请在前面加一层 TLS 反向代理（见 docs/deployment.md）。',
          },
        ],
      },
      {
        id: 'how-to-update',
        question: '如何更新？',
        blocks: [
          {
            type: 'p',
            text: '拉取仓库的更新并运行 `make up` 重新构建。数据库保存在 `vergex-postgres` 卷中，密钥保存在 .env 中，更新不会影响它们。后端恢复后，运行中的交易员会自动重启。',
          },
        ],
      },
      {
        id: 'launch-blocked',
        question: '启动被检查项拦住了，怎么办？',
        blocks: [
          {
            type: 'p',
            text: '阅读提示信息：每一项检查失败都会写明修复方法，并带你到相应的设置步骤，例如修复模型凭证、补全交易所 API Key 或充值 USDT。余额会实时重新检查，修复后即可启动。',
          },
        ],
      },
      {
        id: 'exchange-unreachable',
        question: '交易所账户显示“凭证无效”或“不可用”。',
        blocks: [
          {
            type: 'list',
            items: [
              '凭证无效：保存的 API Key 已被撤销、过期或输入错误，请创建新 Key 并更新交易所账户。',
              '不可用：交易所接口没有响应；账户状态会缓存 30 秒，请稍等后刷新。',
              '交易所 Key：请确认交易权限、IP 白名单，以及已开通合约交易。',
            ],
          },
        ],
      },
      {
        id: 'where-are-logs',
        question: '日志在哪里？',
        blocks: [
          {
            type: 'list',
            items: [
              '后端：`docker logs vergex-backend`（或运行 `make run-backend` 的终端）。',
              '每一轮的 Agent 推理和错误：交易员页面的“决策”标签。',
              '前端构建或运行问题：浏览器开发者工具的控制台。',
            ],
          },
        ],
      },
      {
        id: 'port-conflicts',
        question: '端口 8000 或 3000 已被占用。',
        blocks: [
          {
            type: 'p',
            text: '停止冲突的服务，或在 .env 中选择其他端口（`VERGEX_FRONTEND_PORT`，如果公开了接口还有 `VERGEX_BACKEND_PORT`），然后重新运行 `make up`。',
          },
        ],
      },
    ],
  },
]
