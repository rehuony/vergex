export interface TelegramConfig {
  token_masked: string // Masked token like "123456:ABC***XYZ"
  is_bound: boolean // Whether a chat has been bound with a binding link
  username?: string // The bound user's @username, or their name without one
  bound_chat_id?: number // The bound user's id, which is also their chat's
  bound_at?: string | null // When they bound it
  agent_model_id?: string // model selected for Telegram replies
  alerts?: Record<string, boolean> // Each alert kind and whether it is on
  bot_username?: string // The bot that is running, if one is
  bot_state?: string // How the bot is doing: running, degraded, failed or off
  bot_error?: string // Why it is not running
  alerts_error?: string // Why alerts could not go out last time
}

// Someone who tried to use the bot and was refused.
export interface TelegramVisitor {
  chat_id: number
  sender_id: number // 0 when Telegram names no sender, as in a channel
  chat_type: string // private, group, supergroup or channel
  chat_title?: string
  username?: string
  name?: string
  attempts: number
  last_text?: string
  first_at: number
  last_at: number
}

// A one-time way to bind a chat: the link, or the code to send by hand.
export interface TelegramBindLink {
  code: string
  link: string
  expires_at: string
}
