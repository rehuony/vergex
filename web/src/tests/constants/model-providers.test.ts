import { describe, expect, it } from 'vitest'
import {
  AI_PROVIDER_CONFIG,
  getTraderModelLabel,
} from '@/constants/model-providers'

describe('AI provider config', () => {
  it('offers every native provider with a default model and key link', () => {
    expect(Object.keys(AI_PROVIDER_CONFIG).sort()).toEqual(
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
    for (const cfg of Object.values(AI_PROVIDER_CONFIG)) {
      expect(cfg.defaultModel).toBeTruthy()
      expect(cfg.apiUrl).toMatch(/^https:\/\//)
    }
  })
})

describe('getTraderModelLabel', () => {
  const models = [
    {
      id: 'u1_deepseek',
      name: 'DeepSeek AI',
      provider: 'deepseek',
    },
    {
      id: 'u1_openai',
      name: 'OpenAI',
      provider: 'openai',
      customModelName: 'gpt-4o-mini',
    },
  ]

  it('names the model the trader runs on, not its ID', () => {
    expect(getTraderModelLabel('u1_deepseek', models)).toBe('deepseek-chat')
    expect(getTraderModelLabel('u1_openai', models)).toBe('gpt-4o-mini')
  })

  it('falls back to the provider when the model is not configured', () => {
    expect(getTraderModelLabel('u2_claude', models)).toBe('Claude')
    expect(getTraderModelLabel(undefined, models)).toBe('—')
  })
})
