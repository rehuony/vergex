export interface TelegramConfig {
  token_masked: string // Masked token like "123456:ABC***XYZ"
  is_bound: boolean // Whether a user has sent /start
  bound_chat_id?: number // The bound chat ID (if any)
  agent_model_id?: string // model selected for Telegram replies
  alerts?: Record<string, boolean> // Each alert kind and whether it is on
}
