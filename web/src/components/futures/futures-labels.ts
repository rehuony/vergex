import { t, type Language } from '@/i18n'

// Display helpers shared by the Futures page and its dialogs.
export const format = (n: string | number) =>
  Number(n).toLocaleString('en-US', { maximumFractionDigits: 8 })
// A known state's label; a state the app does not know shows as it is.
const stateLabel = (group: string, state: string, language: Language) => {
  const key = `futures.${group}.${state}`
  const text = t(key, language)
  return text === key ? state : text
}
export const planStateLabel = (state: string, language: Language) =>
  stateLabel('planState', state, language)
export const orderStateLabel = (state: string, language: Language) =>
  stateLabel('orderStatus', state, language)
