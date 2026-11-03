// Constants for model and provider configuration

export interface AgentProviderConfig {
  apiUrl: string
  apiName: string
}

// Where each provider's API keys are made. Default models come from the server.
export const AGENT_PROVIDER_CONFIG: Record<string, AgentProviderConfig> = {
  deepseek: {
    apiUrl: 'https://platform.deepseek.com/api_keys',
    apiName: 'DeepSeek',
  },
  qwen: {
    apiUrl: 'https://dashscope.console.aliyun.com/apiKey',
    apiName: 'Alibaba Cloud',
  },
  openai: {
    apiUrl: 'https://platform.openai.com/api-keys',
    apiName: 'OpenAI',
  },
  claude: {
    apiUrl: 'https://console.anthropic.com/settings/keys',
    apiName: 'Anthropic',
  },
  gemini: {
    apiUrl: 'https://aistudio.google.com/app/apikey',
    apiName: 'Google AI Studio',
  },
  grok: {
    apiUrl: 'https://console.x.ai/',
    apiName: 'xAI',
  },
  kimi: {
    apiUrl: 'https://platform.moonshot.ai/console/api-keys',
    apiName: 'Moonshot',
  },
  minimax: {
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
    model_name?: string
    default_model?: string
  }[]
): string {
  if (!modelId) return '—'
  const model = models.find((m) => m.id === modelId)
  if (!model) return '—'
  return model.model_name || model.default_model || model.name
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
