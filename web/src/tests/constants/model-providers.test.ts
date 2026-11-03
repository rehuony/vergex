import { describe, expect, it } from 'vitest'
import {
  AGENT_PROVIDER_CONFIG,
  getTraderModelLabel,
} from '@/constants/model-providers'

describe('model provider config', () => {
  it('offers every native provider with a key link', () => {
    expect(Object.keys(AGENT_PROVIDER_CONFIG).sort()).toEqual(
      [
        'claude',
        'deepseek',
        'gemini',
        'grok',
        'kimi',
        'minimax',
        'openai',
        'qwen',
      ].sort()
    )
    for (const cfg of Object.values(AGENT_PROVIDER_CONFIG)) {
      expect(cfg.apiUrl).toMatch(/^https:\/\//)
    }
  })
})

describe('getTraderModelLabel', () => {
  const models = [
    {
      id: 'model-deepseek',
      name: 'DeepSeek AI',
      provider: 'deepseek',
      default_model: 'deepseek-v4-pro',
    },
    {
      id: 'model-openai',
      name: 'OpenAI',
      provider: 'openai',
      model_name: 'gpt-4o-mini',
    },
  ]

  it('names the model the trader runs on, not its ID', () => {
    expect(getTraderModelLabel('model-deepseek', models)).toBe(
      'deepseek-v4-pro'
    )
    expect(getTraderModelLabel('model-openai', models)).toBe('gpt-4o-mini')
  })

  it('shows nothing when the model is not configured', () => {
    expect(getTraderModelLabel('model-2', models)).toBe('—')
    expect(getTraderModelLabel(undefined, models)).toBe('—')
  })
})
