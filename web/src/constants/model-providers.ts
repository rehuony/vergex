// Constants for model and provider configuration

export interface AgentProviderConfig {
  defaultModel: string
  apiUrl: string
  apiName: string
}

// Model provider configuration - default models and API links
export const AGENT_PROVIDER_CONFIG: Record<string, AgentProviderConfig> = {
  deepseek: {
    defaultModel: 'deepseek-chat',
    apiUrl: 'https://platform.deepseek.com/api_keys',
    apiName: 'DeepSeek',
  },
  qwen: {
    defaultModel: 'qwen3-max',
    apiUrl: 'https://dashscope.console.aliyun.com/apiKey',
    apiName: 'Alibaba Cloud',
  },
  openai: {
    defaultModel: 'gpt-5.4',
    apiUrl: 'https://platform.openai.com/api-keys',
    apiName: 'OpenAI',
  },
  claude: {
    defaultModel: 'claude-opus-4-6',
    apiUrl: 'https://console.anthropic.com/settings/keys',
    apiName: 'Anthropic',
  },
  gemini: {
    defaultModel: 'gemini-3.1-pro',
    apiUrl: 'https://aistudio.google.com/app/apikey',
    apiName: 'Google AI Studio',
  },
  grok: {
    defaultModel: 'grok-3-latest',
    apiUrl: 'https://console.x.ai/',
    apiName: 'xAI',
  },
  kimi: {
    defaultModel: 'moonshot-v1-auto',
    apiUrl: 'https://platform.moonshot.ai/console/api-keys',
    apiName: 'Moonshot',
  },
  minimax: {
    defaultModel: 'MiniMax-M2.7',
    apiUrl: 'https://platform.minimax.io',
    apiName: 'MiniMax',
  },
}

// Model a trader runs on, as the Connections page names it: the custom model name,
// else the provider's default model.
export function getTraderModelLabel(
  modelId: string | undefined,
  models: {
    id: string
    name: string
    provider: string
    customModelName?: string
  }[]
): string {
  if (!modelId) return '—'
  const model = models.find((m) => m.id === modelId)
  if (!model) return '—'
  return (
    model.customModelName ||
    AGENT_PROVIDER_CONFIG[model.provider]?.defaultModel ||
    model.name
  )
}

// How the interface names an account from its ID (UUID)
export function getAccountDisplayName(
  accountId: string | undefined,
  accounts: {
    id: string
    venue?: string
    name: string
    venue_name?: string
  }[]
): string {
  if (!accountId) return 'Unknown'
  const account = accounts.find((e) => e.id === accountId)
  if (!account) return accountId.substring(0, 8).toUpperCase() + '...' // Show truncated UUID if not found
  const venueName = account.venue?.toUpperCase() || account.venue_name || ''
  return account.name ? `${venueName} - ${account.name}` : venueName
}
