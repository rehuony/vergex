import { Brain, Landmark, Receipt, Send, UserRound } from 'lucide-react'

// In the order the tabs are shown. Apart from the dialog, so what opens it does not
// load it.
export const SECTIONS = [
  { id: 'profile', label: 'profileTab', icon: UserRound },
  { id: 'accounts', label: 'accountsTab', icon: Landmark },
  { id: 'telegram', label: 'telegramTab', icon: Send },
  { id: 'models', label: 'modelsTab', icon: Brain },
  { id: 'costs', label: 'agentCostsTab', icon: Receipt },
] as const

export type SettingsSection = (typeof SECTIONS)[number]['id']

export function isSettingsSection(value: string): value is SettingsSection {
  return SECTIONS.some((section) => section.id === value)
}
