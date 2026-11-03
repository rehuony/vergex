import { useEffect, useSyncExternalStore } from 'react'

// What a page tells the assistant about itself: ids, names and what only the browser knows,
// such as which plans are side by side. It holds no figures, which the assistant reads for
// itself, so it is never out of date and never large.
export type AssistantPageDetails = Record<string, unknown>

interface Declared {
  name: string
  describe: () => AssistantPageDetails
}

let declared: Declared | null = null
const listeners = new Set<() => void>()

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

// useAssistantPage declares the page on screen under a name, with a function asked for its
// details when a message is sent, so it always says how things stand by then.
export function useAssistantPage(
  name: string,
  describe: () => AssistantPageDetails = () => ({})
) {
  useEffect(() => {
    const mine: Declared = { name, describe }
    declared = mine
    listeners.forEach((listener) => listener())
    return () => {
      if (declared !== mine) return
      declared = null
      listeners.forEach((listener) => listener())
    }
  }, [name])
  // A page's next render may know more, and that replaces nothing anyone is listening for.
  useEffect(() => {
    if (declared?.name === name) declared.describe = describe
  })
}

// The name of the page on screen, for what depends on it.
export const useAssistantPageName = () =>
  useSyncExternalStore(subscribe, () => declared?.name ?? '')

export interface AssistantFormField {
  name: string
  label: string
  type: 'text' | 'number' | 'choice' | 'list' | 'object'
  options?: { value: string; label: string }[]
  min?: number
  max?: number
  value: unknown
}

// A form the assistant can suggest values for. apply only does what typing them would: it
// changes what the form holds, and saving stays with the page's own button and checks.
export interface AssistantForm {
  id: string
  title: string
  fields: AssistantFormField[]
  apply: (values: Record<string, unknown>) => void
}

const forms = new Map<string, AssistantForm>()

// useAssistantForm offers a form for as long as it is on screen. Nothing that holds a key,
// a password or an order to send belongs here.
export function useAssistantForm(form: AssistantForm | null) {
  const id = form?.id
  useEffect(() => {
    if (form) forms.set(form.id, form)
  })
  useEffect(() => {
    if (!id) return
    listeners.forEach((listener) => listener())
    return () => {
      forms.delete(id)
      listeners.forEach((listener) => listener())
    }
  }, [id])
}

// Whether a form is on screen now, for what depends on it.
export const useAssistantFormOpen = (id: string) =>
  useSyncExternalStore(subscribe, () => forms.has(id))

export const openAssistantForm = (id: string) => forms.get(id)

// A value as a form's values merge it: objects key by key, anything else whole.
export function mergeFormValue(current: unknown, next: unknown): unknown {
  if (!isPlainObject(current) || !isPlainObject(next)) return next
  const merged: Record<string, unknown> = { ...current }
  for (const [key, value] of Object.entries(next))
    merged[key] = mergeFormValue(current[key], value)
  return merged
}

const isPlainObject = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

// What a long text in a form weighs in what is sent, such as a custom prompt. The rest of
// it stays out, and the form keeps all of it.
const LONG_TEXT = 500
const withoutLongTexts = (_key: string, value: unknown) =>
  typeof value === 'string' && value.length > LONG_TEXT
    ? `${value.slice(0, LONG_TEXT)}…`
    : value

// What is said to the server about the page on screen.
export function readAssistantPage(): AssistantPageDetails | null {
  if (!declared) return null
  const open = [...forms.values()].map(({ id, title, fields }) => ({
    id,
    title,
    fields,
  }))
  return {
    page: declared.name,
    ...declared.describe(),
    ...(open.length > 0 && {
      forms: JSON.parse(JSON.stringify(open, withoutLongTexts)),
    }),
  }
}
