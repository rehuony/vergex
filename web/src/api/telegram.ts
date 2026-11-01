import type { TelegramConfig } from '@/types'
import { API_BASE, httpClient } from './helpers'

export const telegramApi = {
  async getTelegramConfig(): Promise<TelegramConfig> {
    const result = await httpClient.get<TelegramConfig>(`${API_BASE}/telegram`)
    if (!result.success) throw new Error('Failed to fetch Telegram config')
    return result.data!
  },

  async updateTelegramConfig(token: string, modelId?: string): Promise<void> {
    const result = await httpClient.post(`${API_BASE}/telegram`, {
      bot_token: token,
      agent_model_id: modelId ?? '',
    })
    if (!result.success) throw new Error('Failed to save Telegram config')
  },

  async unbindTelegram(): Promise<void> {
    const result = await httpClient.delete(`${API_BASE}/telegram/binding`)
    if (!result.success) throw new Error('Failed to unbind Telegram')
  },

  async updateTelegramModel(modelId: string): Promise<void> {
    const result = await httpClient.post(`${API_BASE}/telegram/model`, {
      agent_model_id: modelId,
    })
    if (!result.success) throw new Error('Failed to update Telegram model')
  },

  async updateTelegramAlerts(
    alerts: Record<string, boolean>
  ): Promise<Record<string, boolean>> {
    const result = await httpClient.put<{ alerts: Record<string, boolean> }>(
      `${API_BASE}/telegram/alerts`,
      { alerts }
    )
    if (!result.success) throw new Error('Failed to save Telegram alerts')
    return result.data!.alerts
  },

  async testTelegramAlert(): Promise<void> {
    const result = await httpClient.post(`${API_BASE}/telegram/alerts/test`)
    if (!result.success)
      throw new Error(result.message || 'Failed to send a test alert')
  },
}
