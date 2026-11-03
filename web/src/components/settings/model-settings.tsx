import { useEffect, useId, useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import {
  Brain,
  Check,
  ExternalLink,
  Pencil,
  Plus,
  Trash2,
  Zap,
} from 'lucide-react'
import { toast } from 'sonner'
import type { AgentModel, ModelTest, TraderInfo } from '@/types'
import { t, type Language } from '@/i18n'
import { api } from '@/api'
import { cn } from '@/utils/cn'
import { confirmToast } from '@/components/common/notify'
import {
  configuredModels,
  MODEL_CATALOG_KEY,
  useModelConfigs,
  useSupportedModels,
  useTraders,
} from '@/hooks/use-connections'
import { dialogInputClass } from '@/components/common/dialog'
import { PanelLoading } from '@/components/common/panel'
import { getModelIcon } from '@/components/common/model-icons'
import { AGENT_PROVIDER_CONFIG } from '@/constants/model-providers'
import { Button } from '@/components/ui/button'
import { AppSelect } from '@/components/ui/select'
import { ModelField } from './model-field'
import {
  SettingBack,
  SettingBadge,
  SettingItem,
  SettingList,
  SettingRow,
  SettingState,
} from '@/components/common/setting-row'

const inputClass = cn(dialogInputClass, 'max-sm:h-10 max-sm:text-base')

const modelIcon = (model: AgentModel) =>
  getModelIcon(model.provider || model.id, { width: 20, height: 20 }) || (
    <Brain className="size-5" />
  )

const defaultModel = (model: AgentModel) =>
  model.default_model || model.provider

/** The models Futures, traders and Telegram call, with their keys. */
export function ModelSettings({ language }: { language: Language }) {
  // null shows the list, '' a new connection, an id that connection.
  const [editing, setEditing] = useState<string | null>(null)
  const { data: models, error, isLoading } = useModelConfigs()
  const { data: supported } = useSupportedModels()
  const saved = configuredModels(models)
  const unused = (supported ?? []).filter(
    (s) => !saved.some((m) => m.provider === s.provider)
  )

  if (editing !== null && models && supported)
    return (
      <ModelForm
        key={editing}
        existing={saved.find((m) => m.id === editing)}
        choices={unused}
        supported={supported}
        language={language}
        onDone={() => setEditing(null)}
      />
    )

  return (
    <SettingList
      actions={
        <Button
          type="button"
          size="sm"
          variant="primary"
          disabled={!models || !supported || unused.length === 0}
          onClick={() => setEditing('')}
        >
          <Plus className="size-4" />
          {t('connections.addModel', language)}
        </Button>
      }
    >
      {error ? (
        <SettingState role="alert">
          {t('connections.loadFailed', language)}
        </SettingState>
      ) : isLoading || !models ? (
        <PanelLoading className="flex-1" />
      ) : saved.length === 0 ? (
        <SettingState>
          <Brain className="size-6" aria-hidden="true" />
          {t('connections.noModels', language)}
        </SettingState>
      ) : (
        saved.map((model) => (
          <SettingItem
            key={model.id}
            icon={modelIcon(model)}
            title={model.name}
            badge={
              <SettingBadge tone={model.enabled ? 'success' : 'muted'}>
                {t(
                  model.enabled
                    ? 'connections.enabled'
                    : 'connections.disabled',
                  language
                )}
              </SettingBadge>
            }
            detail={model.model_name || defaultModel(model)}
          >
            <Button
              type="button"
              size="sm"
              variant="secondary"
              aria-label={`${t('connections.edit', language)} ${model.name}`}
              onClick={() => setEditing(model.id)}
            >
              <Pencil className="size-4" />
              {t('connections.edit', language)}
            </Button>
          </SettingItem>
        ))
      )}
    </SettingList>
  )
}

// Adds a provider, or edits one: a blank key keeps the saved key.
function ModelForm({
  existing,
  choices,
  supported,
  language,
  onDone,
}: {
  existing?: AgentModel
  choices: AgentModel[]
  supported: AgentModel[]
  language: Language
  onDone: () => void
}) {
  const id = useId()
  const client = useQueryClient()
  const { refetch } = useModelConfigs()
  const { data: traders } = useTraders()
  const [provider, setProvider] = useState(choices[0]?.id ?? '')
  const model = existing ?? choices.find((m) => m.id === provider)
  const [apiKey, setApiKey] = useState('')
  // null until edited: the field shows the provider's own endpoint.
  const [baseUrl, setBaseUrl] = useState<string | null>(
    existing?.base_url || null
  )
  // Like the endpoint: null until edited, and the field shows the provider's own.
  const [modelName, setModelName] = useState<string | null>(
    existing?.model_name || null
  )
  // What the provider offers with what is in the form, and whether that is still coming.
  const [catalog, setCatalog] = useState<string[]>([])
  const [listing, setListing] = useState(false)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  // The last test of the values in the form; editing a field clears it.
  const [test, setTest] = useState<ModelTest | 'running' | null>(null)
  const keySaved = Boolean(existing?.has_api_key)
  const ready = Boolean(model) && (apiKey.trim() !== '' || keySaved)
  const keyLink = model && AGENT_PROVIDER_CONFIG[model.provider]?.apiUrl
  const officialUrl =
    supported.find((row) => row.provider === model?.provider)
      ?.default_base_url ?? ''
  const shownUrl = baseUrl ?? officialUrl
  // The official endpoint is saved as none, so it stays whatever the provider's default is.
  const customUrl = shownUrl.trim() === officialUrl ? '' : shownUrl.trim()
  const key = apiKey.trim()
  const officialModel = model ? defaultModel(model) : ''
  const shownModel = modelName ?? officialModel
  // The provider's own default is saved as none, as the endpoint is.
  const customModel =
    shownModel.trim() === officialModel ? '' : shownModel.trim()
  const modelNames = [...new Set([officialModel, ...catalog])]
    .filter(Boolean)
    .sort()

  useEffect(() => {
    setCatalog([])
    if (!model || !ready) return
    let live = true
    setListing(true)
    const timer = setTimeout(() => {
      api
        .listAvailableModels({
          agent_model_id: model.id,
          api_key: key,
          base_url: customUrl,
        })
        .then((names) => live && setCatalog(names))
        // Only a convenience: a name can still be typed.
        .catch(() => {})
        .finally(() => live && setListing(false))
    }, 500)
    return () => {
      live = false
      clearTimeout(timer)
      setListing(false)
    }
  }, [model?.id, ready, key, customUrl])

  const write = async (
    fields: {
      enabled: boolean
      api_key: string
      base_url: string
      model_name: string
    },
    done: string,
    failed: string
  ) => {
    if (!model || busy) return
    setBusy(true)
    setMessage('')
    try {
      await api.updateModelConfigs({ models: { [model.id]: fields } })
      await refetch()
      void client.invalidateQueries({ queryKey: [MODEL_CATALOG_KEY] })
      toast.success(t(done, language))
      onDone()
    } catch {
      setMessage(t(failed, language))
    } finally {
      setBusy(false)
    }
  }

  const edit = (set: (value: string) => void) => (value: string) => {
    set(value)
    setTest(null)
  }

  const runTest = async () => {
    if (!model || !ready || test === 'running') return
    setMessage('')
    setTest('running')
    try {
      setTest(
        await api.testModelConfig({
          agent_model_id: model.id,
          api_key: key,
          base_url: customUrl,
          model_name: customModel,
        })
      )
    } catch (err) {
      setTest({
        ok: false,
        model: '',
        error:
          err instanceof Error && err.message
            ? err.message
            : t('connections.testFailed', language),
      })
    }
  }

  // A save or delete error wins over the test result.
  const status: { text: string; tone: 'danger' | 'success' | 'muted' } = message
    ? { text: message, tone: 'danger' }
    : test === 'running'
      ? { text: t('connections.testing', language), tone: 'muted' }
      : test?.ok
        ? {
            text: t('connections.testPassed', language, {
              model: test.model,
              seconds: ((test.latency_ms ?? 0) / 1000).toFixed(1),
            }),
            tone: 'success',
          }
        : test
          ? {
              text: test.error || t('connections.testFailed', language),
              tone: 'danger',
            }
          : { text: '', tone: 'danger' }

  const save = (e: FormEvent) => {
    e.preventDefault()
    if (!ready) return
    void write(
      {
        enabled: true,
        api_key: key,
        base_url: customUrl,
        model_name: customModel,
      },
      'agentTradersToast.modelConfigUpdated',
      'saveConfigFailed'
    )
  }

  // Deleting takes the key with it. A trader in use still needs its model: the server is
  // what refuses, and this says so sooner with the names it already has.
  const remove = async () => {
    if (!existing || busy) return
    const users = (traders ?? []).filter(
      (tr: TraderInfo) => tr.agent_model === existing.id
    )
    if (users.length > 0) {
      setMessage(
        t('connections.inUse', language, {
          names: users.map((tr) => tr.trader_name).join(', '),
        })
      )
      return
    }
    if (
      !(await confirmToast(
        t('confirmDeleteModel', language, { name: existing.name }),
        {
          title: t('deleteModelTitle', language),
          okText: t('delete', language),
          tone: 'danger',
        }
      ))
    )
      return
    setBusy(true)
    setMessage('')
    try {
      await api.deleteModel(existing.id)
      await refetch()
      void client.invalidateQueries({ queryKey: [MODEL_CATALOG_KEY] })
      toast.success(t('agentTradersToast.configUpdated', language))
      onDone()
    } catch (err) {
      setMessage(
        err instanceof Error && err.message
          ? err.message
          : t('deleteConfigFailed', language)
      )
    } finally {
      setBusy(false)
    }
  }

  return (
    <form
      onSubmit={save}
      aria-label={t('modelsTab', language)}
      className="h-full"
    >
      <SettingList
        head={
          <SettingBack
            onBack={onDone}
            backLabel={t('back', language)}
            icon={model && modelIcon(model)}
            title={
              existing ? existing.name : t('connections.addModel', language)
            }
          >
            {keyLink && (
              <Button asChild variant="link" size="sm" className="text-[13px]">
                <a href={keyLink} target="_blank" rel="noopener noreferrer">
                  {t('modelConfig.getApiKey', language)}
                  <ExternalLink className="size-3.5" />
                </a>
              </Button>
            )}
          </SettingBack>
        }
        message={status.text}
        messageTone={status.tone}
        actions={
          <>
            {existing && (
              <Button
                type="button"
                size="sm"
                variant="danger"
                disabled={busy}
                onClick={() => void remove()}
              >
                <Trash2 className="size-4" />
                {t('delete', language)}
              </Button>
            )}
            <Button
              type="button"
              size="sm"
              variant="secondary"
              disabled={!ready || busy || test === 'running'}
              onClick={() => void runTest()}
            >
              <Zap className="size-4" />
              {test === 'running'
                ? t('connections.testing', language)
                : t('connections.test', language)}
            </Button>
            <Button
              type="submit"
              size="sm"
              variant="primary"
              disabled={!ready || busy}
            >
              <Check className="size-4" />
              {busy ? t('saving', language) : t('saveConfig', language)}
            </Button>
          </>
        }
      >
        {!existing && (
          <SettingRow
            label={t('connections.provider', language)}
            htmlFor={`${id}-provider`}
          >
            <AppSelect
              id={`${id}-provider`}
              value={provider}
              onChange={(next) => {
                edit(setProvider)(next)
                // What was filled in was for the provider before.
                setBaseUrl(null)
                setModelName(null)
              }}
              options={choices.map((m) => ({ value: m.id, label: m.name }))}
              className="h-9"
            />
          </SettingRow>
        )}
        <SettingRow label="API Key" htmlFor={`${id}-key`}>
          <input
            id={`${id}-key`}
            type="password"
            autoComplete="off"
            required={!keySaved}
            value={apiKey}
            onChange={(e) => edit(setApiKey)(e.target.value)}
            placeholder={t(
              keySaved ? 'connections.keySaved' : 'enterAPIKey',
              language
            )}
            className={inputClass}
          />
        </SettingRow>
        <SettingRow
          label={t('connections.baseUrl', language)}
          htmlFor={`${id}-url`}
        >
          <input
            id={`${id}-url`}
            type="url"
            value={shownUrl}
            onChange={(e) => edit(setBaseUrl)(e.target.value)}
            placeholder={officialUrl}
            className={inputClass}
          />
        </SettingRow>
        <SettingRow
          label={t('connections.modelName', language)}
          htmlFor={`${id}-model`}
        >
          <ModelField
            id={`${id}-model`}
            value={shownModel}
            placeholder={officialModel}
            names={modelNames}
            listing={listing}
            className={inputClass}
            language={language}
            onChange={edit(setModelName)}
          />
        </SettingRow>
      </SettingList>
    </form>
  )
}
