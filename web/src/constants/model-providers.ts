// Constants for AI model and provider configuration

export interface AIProviderConfig {
  defaultModel: string
  apiUrl: string
  apiName: string
}

// Get friendly AI model display name
export function getModelDisplayName(modelId: string): string {
  switch (modelId.toLowerCase()) {
    case 'deepseek':
      return 'DeepSeek'
    case 'qwen':
      return 'Qwen'
    case 'claude':
      return 'Claude'
    default:
      return modelId.toUpperCase()
  }
}

// Extract name part after underscore
export function getShortName(fullName: string): string {
  const parts = fullName.split('_')
  return parts.length > 1 ? parts[parts.length - 1] : fullName
}

// AI Provider configuration - default models and API links
export const AI_PROVIDER_CONFIG: Record<string, AIProviderConfig> = {
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
// else the provider's default model. Model IDs are "<user>_<provider>", so an
// unknown ID falls back to its provider.
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
  const model =
    models.find((m) => m.id === modelId) ??
    models.find((m) => m.provider === modelId)
  if (!model) return getModelDisplayName(getShortName(modelId))
  return (
    model.customModelName ||
    AI_PROVIDER_CONFIG[model.provider]?.defaultModel ||
    model.name
  )
}

// Helper function to get exchange display name from exchange ID (UUID)
export function getExchangeDisplayName(
  exchangeId: string | undefined,
  exchanges: {
    id: string
    exchange_type?: string
    name: string
    account_name?: string
  }[]
): string {
  if (!exchangeId) return 'Unknown'
  const exchange = exchanges.find((e) => e.id === exchangeId)
  if (!exchange) return exchangeId.substring(0, 8).toUpperCase() + '...' // Show truncated UUID if not found
  const typeName = exchange.exchange_type?.toUpperCase() || exchange.name
  return exchange.account_name
    ? `${typeName} - ${exchange.account_name}`
    : typeName
}
