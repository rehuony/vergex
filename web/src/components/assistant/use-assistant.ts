import { useEffect, useMemo, useRef, useState } from 'react'
import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '@/api'
import {
  streamAssistant,
  type AssistantConfirmation,
  type AssistantActionResult,
  type AssistantEvent,
  type AssistantLink,
  type AssistantProposal,
  type AssistantThread,
} from '@/api/assistant'
import { withUsage, type ActivityItem } from '@/components/common/turn-steps'
import { notify } from '@/components/common/notify'
import { useLanguage } from '@/contexts/language-context'
import {
  openAssistantForm,
  readAssistantPage,
} from '@/hooks/use-assistant-page'
import { useModelConfigs } from '@/hooks/use-connections'
import { t } from '@/i18n'
import type { AgentModel } from '@/types'
import { formatRequestError } from '@/utils/request-errors'
import { useLocation } from 'react-router-dom'

export const conversationsQuery = queryOptions({
  queryKey: ['assistant-conversations'],
  queryFn: () => api.getAssistantConversations(),
  refetchOnWindowFocus: false,
})

// What was said is added to here as it is said, so nothing reads it over again.
const threadQuery = (id: string) =>
  queryOptions({
    queryKey: ['assistant-conversation', id],
    queryFn: () => api.getAssistantConversation(id),
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  })

const EMPTY_THREAD: AssistantThread = { messages: [], confirm: null }
const NO_MODELS: AgentModel[] = []
const MODEL_KEY = 'vergex-assistant-model'
// What a page says about itself. Past this it says only where it is.
const MAX_PAGE_CHARS = 8000

function pageNow(route: string) {
  const details = readAssistantPage()
  const whole = { route, ...details }
  return JSON.stringify(whole).length <= MAX_PAGE_CHARS
    ? whole
    : { route, page: details?.page }
}

function savedModel(): { id: string; name: string } {
  try {
    const saved = JSON.parse(localStorage.getItem(MODEL_KEY) ?? '{}')
    return { id: String(saved.id ?? ''), name: String(saved.name ?? '') }
  } catch {
    return { id: '', name: '' }
  }
}

function saveModel(id: string, name: string) {
  try {
    localStorage.setItem(MODEL_KEY, JSON.stringify({ id, name }))
  } catch {
    /* A browser that keeps nothing goes back to the first model. */
  }
}

// useAssistant holds the assistant's conversation for as long as someone is signed in,
// whatever page they are on and whether or not its window is open. Nothing is read until
// the window has been opened once.
export function useAssistant(open: boolean) {
  const { language } = useLanguage()
  const client = useQueryClient()
  const { pathname, search } = useLocation()
  const label = (key: string) => t(`assistant.${key}`, language)
  const [started, setStarted] = useState(open)
  useEffect(() => {
    if (open) setStarted(true)
  }, [open])

  const { data: configs } = useModelConfigs()
  const models = useMemo(
    () => (configs ?? NO_MODELS).filter((m) => m.enabled && m.has_api_key),
    [configs]
  )
  const [picked, setPicked] = useState(savedModel)
  const known = models.some((model) => model.id === picked.id)
  const modelID = known ? picked.id : (models[0]?.id ?? '')
  const modelName = known ? picked.name : ''
  const chooseModel = (id: string, name = '') => {
    setPicked({ id, name })
    saveModel(id, name)
  }

  const list = useQuery({ ...conversationsQuery, enabled: started })
  const [conversationId, setConversationId] = useState<string | null>(null)
  const thread = useQuery({
    ...threadQuery(conversationId ?? ''),
    enabled: Boolean(conversationId),
  })
  const messages = thread.data?.messages ?? EMPTY_THREAD.messages
  const confirm = thread.data?.confirm ?? null
  const links = thread.data?.links ?? []
  const proposals = thread.data?.proposals ?? []

  const [instruction, setInstruction] = useState('')
  const [trace, setTrace] = useState<ActivityItem[]>([])
  // The reply as the model writes it, until it is finished.
  const [streaming, setStreaming] = useState('')
  const [busy, setBusy] = useState(false)
  const [answering, setAnswering] = useState(false)
  const [error, setError] = useState('')
  const [unread, setUnread] = useState(false)
  const abort = useRef<AbortController | null>(null)
  const follow = useRef(true)
  const steps = useRef(0)
  const openNow = useRef(open)
  useEffect(() => {
    openNow.current = open
    if (open) setUnread(false)
  }, [open])
  useEffect(() => () => abort.current?.abort(), [])

  const patchThread = (
    id: string,
    patch: (thread: AssistantThread) => AssistantThread
  ) =>
    client.setQueryData<AssistantThread>(threadQuery(id).queryKey, (current) =>
      patch(current ?? EMPTY_THREAD)
    )

  // Whatever a page shows may be out of date once a skill has changed something. What this
  // conversation holds is not, and reading it again would drop what only a reply says.
  const refreshPages = () =>
    void client.invalidateQueries({
      predicate: ({ queryKey }) => queryKey[0] !== threadQuery('').queryKey[0],
    })

  const addReply = (
    id: string,
    text: string,
    waiting: AssistantConfirmation | null,
    links?: AssistantLink[] | null,
    proposals?: AssistantProposal[] | null,
    results?: AssistantActionResult[] | null
  ) =>
    patchThread(id, (current) => ({
      confirm: waiting,
      links,
      proposals,
      messages: [
        ...current.messages,
        // How confirmed actions went reads in parts, before whatever else was said.
        ...(results?.length
          ? [{ role: 'assistant' as const, content: '', results }]
          : []),
        ...(text ? [{ role: 'assistant' as const, content: text }] : []),
      ],
    }))

  // A new conversation needs nothing read: there is nothing yet to read.
  const startConversation = () => {
    const id = crypto.randomUUID()
    client.setQueryData(threadQuery(id).queryKey, EMPTY_THREAD)
    setConversationId(id)
    return id
  }

  const resetTurn = () => {
    setTrace([])
    setStreaming('')
    setError('')
    follow.current = true
  }

  // Opening picks up the latest conversation, or starts one when none can be read.
  const latest = list.data?.[0]?.id
  const listSettled = list.isSuccess || list.isError
  useEffect(() => {
    if (!started || conversationId || !listSettled) return
    if (latest) setConversationId(latest)
    else startConversation()
  }, [started, conversationId, listSettled, latest])

  const finishSteps = (state: ActivityItem['state']) => {
    const now = Date.now()
    setTrace((items) =>
      items.map((item) =>
        item.state === 'running' ? { ...item, state, ended: now } : item
      )
    )
  }

  async function send(text = instruction) {
    const question = text.trim()
    if (!question || !conversationId) return
    if (busy) {
      setError(label('busy'))
      return
    }
    if (!modelID) return
    const id = conversationId
    const route = pathname + search
    setBusy(true)
    resetTurn()
    setInstruction('')
    patchThread(id, (current) => {
      const last = current.messages[current.messages.length - 1]
      return {
        confirm: null,
        // Asking again after a failure says nothing new.
        messages:
          last?.role === 'user' && last.content === question
            ? current.messages
            : [...current.messages, { role: 'user', content: question }],
      }
    })
    const controller = new AbortController()
    abort.current = controller
    let partial = ''
    const setPartial = (written: string) => {
      partial = written
      setStreaming(written)
    }
    const hear = (event: AssistantEvent) => {
      switch (event.type) {
        case 'stage': {
          // What a round wrote before moving on to a skill is not its answer.
          setPartial('')
          const now = Date.now()
          steps.current += 1
          const step: ActivityItem = {
            id: `stage-${steps.current}`,
            label: event.data.text,
            detail: '',
            state: 'running',
            started: now,
          }
          setTrace((items) => [
            ...items.map((item) =>
              item.state === 'running'
                ? { ...item, state: 'done' as const, ended: now }
                : item
            ),
            step,
          ])
          break
        }
        case 'delta':
          setPartial(partial + event.data.text)
          break
        case 'usage':
          setTrace((items) =>
            withUsage(items, event.data, Date.now(), language)
          )
          break
        case 'reply':
          setPartial('')
          addReply(
            id,
            event.data.text,
            event.data.confirm,
            event.data.links,
            event.data.proposals?.map((proposal) => ({ ...proposal, route })),
            event.data.results
          )
          if (event.data.changed) refreshPages()
          if (!openNow.current) setUnread(true)
          break
        case 'error':
          setPartial('')
          setError(event.data.message)
          finishSteps('failed')
          addReply(id, '', event.data.confirm)
          if (event.data.changed) refreshPages()
          break
        case 'done':
          finishSteps('done')
          break
      }
    }
    try {
      await streamAssistant(
        {
          conversation_id: id,
          message: question,
          agent_model_id: modelID,
          agent_model_name: modelName || undefined,
          language,
          page: pageNow(route),
        },
        controller.signal,
        hear
      )
    } catch (failure) {
      if (!controller.signal.aborted) {
        setError(formatRequestError(failure, language))
        finishSteps('failed')
      }
    } finally {
      // Stopping keeps what was written so far.
      if (partial) addReply(id, partial, null)
      setPartial('')
      if (controller.signal.aborted) {
        const now = Date.now()
        setTrace((items) => [
          ...items.map((item) =>
            item.state === 'running'
              ? { ...item, state: 'done' as const, ended: now }
              : item
          ),
          {
            id: 'stopped',
            label: label('stopped'),
            detail: label('stoppedDetail'),
            state: 'done',
            started: now,
            ended: now,
          },
        ])
      }
      setBusy(false)
      void client.invalidateQueries({ queryKey: conversationsQuery.queryKey })
    }
  }

  const stop = () => abort.current?.abort()

  // Asks again what went unanswered.
  const retry = () => {
    const last = messages[messages.length - 1]
    if (last?.role === 'user') void send(last.content)
  }

  async function answer(confirmed: boolean) {
    if (!conversationId || !confirm || busy) return
    const id = conversationId
    setBusy(true)
    setAnswering(true)
    setError('')
    follow.current = true
    try {
      const reply = await api.answerAssistant(
        id,
        confirm.id,
        confirmed,
        language
      )
      addReply(
        id,
        reply.live ? reply.text : label('stale'),
        reply.confirm,
        reply.links,
        undefined,
        reply.results
      )
      if (reply.changed) refreshPages()
    } catch (failure) {
      setError(formatRequestError(failure, language))
    } finally {
      setBusy(false)
      setAnswering(false)
    }
  }

  // Writes a suggestion into its form, if that is still on screen. It reports whether it did.
  const fillProposal = (proposal: AssistantProposal) => {
    const form = openAssistantForm(proposal.form)
    if (!form || !conversationId) return false
    form.apply(proposal.values)
    patchThread(conversationId, (current) => ({
      ...current,
      proposals: current.proposals?.map((item) =>
        item === proposal ? { ...item, filled: true } : item
      ),
    }))
    return true
  }

  const dismissProposal = (proposal: AssistantProposal) => {
    if (!conversationId) return
    patchThread(conversationId, (current) => ({
      ...current,
      proposals: current.proposals?.filter((item) => item !== proposal),
    }))
  }

  // Moving to another conversation waits for the answer under way.
  const whenFree = (move: () => void) => {
    if (busy) {
      setError(label('busy'))
      return false
    }
    resetTurn()
    setInstruction('')
    move()
    return true
  }

  const newConversation = () => whenFree(startConversation)
  const openConversation = (id: string) => whenFree(() => setConversationId(id))

  async function deleteConversation(id: string) {
    if (busy) {
      setError(label('busy'))
      return
    }
    try {
      await api.deleteAssistantConversation(id)
      // Move off it first, so nothing goes back to read what is gone.
      if (id === conversationId) newConversation()
      client.removeQueries({ queryKey: threadQuery(id).queryKey })
      await client.invalidateQueries({ queryKey: conversationsQuery.queryKey })
    } catch (failure) {
      notify.error(
        formatRequestError(failure, language) || label('deleteFailed')
      )
    }
  }

  async function clearConversations() {
    if (busy) {
      setError(label('busy'))
      return
    }
    try {
      await api.clearAssistantConversations()
      resetTurn()
      setInstruction('')
      const kept = startConversation()
      client.removeQueries({
        predicate: ({ queryKey }) =>
          queryKey[0] === threadQuery(kept).queryKey[0] && queryKey[1] !== kept,
      })
      await client.invalidateQueries({ queryKey: conversationsQuery.queryKey })
    } catch (failure) {
      notify.error(
        formatRequestError(failure, language) || label('clearFailed')
      )
    }
  }

  const title =
    list.data?.find((row) => row.id === conversationId)?.title ||
    messages.find((message) => message.role === 'user')?.content ||
    label('title')

  return {
    conversations: list,
    conversationId,
    thread,
    messages,
    confirm,
    links,
    proposals,
    fillProposal,
    dismissProposal,
    title,
    models,
    modelID,
    modelName,
    chooseModel,
    instruction,
    setInstruction,
    trace,
    streaming,
    busy,
    answering,
    error,
    dismissError: () => setError(''),
    unread,
    follow,
    send,
    stop,
    retry,
    answer,
    newConversation,
    openConversation,
    deleteConversation,
    clearConversations,
  }
}

export type Assistant = ReturnType<typeof useAssistant>
