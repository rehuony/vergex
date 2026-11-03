import type { ChatMessage } from '@/components/common/chat-thread'
import { readEventStream } from '@/utils/event-stream'
import {
  API_BASE,
  getAuthHeaders,
  handleJSONResponse,
  httpClient,
} from './helpers'

// One action waiting for an answer, in parts, as the server puts it.
export interface AssistantConfirmItem {
  action?: string
  target: string
  account?: string
  money?: 'real' | 'testnet'
  details?: string[]
  destructive?: boolean
}

export interface AssistantConfirmation {
  id: string
  lines: string[]
  // One of them cannot be taken back.
  destructive?: boolean
  items?: AssistantConfirmItem[]
}

// How one confirmed action went.
export interface AssistantActionResult {
  ok: boolean
  text: string
  note?: string
}

// What was said, with how confirmed actions went where a message is that.
export interface AssistantChatMessage extends ChatMessage {
  results?: AssistantActionResult[] | null
}

// Where a page could go next, from the server rather than the model.
export interface AssistantLink {
  to: string
  label: string
}

// What the assistant suggests for a form on the page it was asked from.
export interface AssistantProposal {
  form: string
  title: string
  values: Record<string, unknown>
  reason: string
  // The page it was asked from, to go back to if the form has gone. Filled in here.
  route?: string
  filled?: boolean
}

export interface AssistantConversation {
  id: string
  title: string
  updated_at: string
}

// A conversation as it opens: what was said lately, and what waits for an answer.
export interface AssistantThread {
  messages: AssistantChatMessage[]
  confirm: AssistantConfirmation | null
  // Where the latest reply points and what it suggests, kept until the next message.
  links?: AssistantLink[] | null
  proposals?: AssistantProposal[] | null
}

export interface AssistantReply {
  text: string
  results?: AssistantActionResult[] | null
  confirm: AssistantConfirmation | null
  changed: boolean
  links?: AssistantLink[] | null
  proposals?: AssistantProposal[] | null
}

export type AssistantEvent =
  | { type: 'stage'; data: { text: string } }
  | { type: 'delta'; data: { text: string } }
  | { type: 'usage'; data: Record<string, unknown> }
  | { type: 'reply'; data: AssistantReply }
  | {
      type: 'error'
      data: {
        message: string
        confirm: AssistantConfirmation | null
        changed: boolean
      }
    }
  | { type: 'done'; data: { ok: boolean } }

export interface AssistantRequest {
  conversation_id: string
  message: string
  agent_model_id: string
  agent_model_name?: string
  language: string
  page?: unknown
}

// streamAssistant runs one turn. Ending it with the signal stops the turn.
export async function streamAssistant(
  body: AssistantRequest,
  signal: AbortSignal,
  onEvent: (event: AssistantEvent) => void
): Promise<void> {
  const response = await fetch(`${API_BASE}/assistant/chat`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(body),
    signal,
  })
  if (!response.ok) {
    await handleJSONResponse(response)
    return
  }
  if (!response.body) throw new Error('The server returned no stream')
  let finished = false
  await readEventStream<AssistantEvent>(response.body, (event) => {
    if (event.type === 'done') finished = true
    onEvent(event)
  })
  if (!finished) throw new Error('The connection ended before the reply did')
}

export const assistantApi = {
  async getAssistantConversations(): Promise<AssistantConversation[]> {
    const result = await httpClient.get<AssistantConversation[]>(
      `${API_BASE}/assistant/conversations`
    )
    if (!result.success) throw new Error(result.message)
    return result.data ?? []
  },

  async getAssistantConversation(id: string): Promise<AssistantThread> {
    const result = await httpClient.get<AssistantThread>(
      `${API_BASE}/assistant/conversations/${id}`
    )
    if (!result.success) throw new Error(result.message)
    return result.data!
  },

  // live is false when what was answered no longer waits. What was confirmed may take a
  // while, one action after another, so this waits for as long as it takes.
  async answerAssistant(
    id: string,
    confirmationId: string,
    confirmed: boolean,
    language: string
  ): Promise<AssistantReply & { live: boolean }> {
    const response = await fetch(
      `${API_BASE}/assistant/conversations/${id}/answer`,
      {
        method: 'POST',
        headers: getAuthHeaders(),
        body: JSON.stringify({
          confirmation_id: confirmationId,
          confirmed,
          language,
        }),
      }
    )
    return handleJSONResponse(response)
  },

  async clearAssistantConversations(): Promise<void> {
    const result = await httpClient.delete(
      `${API_BASE}/assistant/conversations`
    )
    if (!result.success) throw new Error(result.message)
  },

  async deleteAssistantConversation(id: string): Promise<void> {
    const result = await httpClient.delete(
      `${API_BASE}/assistant/conversations/${id}`
    )
    if (!result.success) throw new Error(result.message)
  },
}
