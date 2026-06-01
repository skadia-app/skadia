import { useId, useState, useEffect } from 'react'
import Toggle from '@/components/Toggle'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  fetchLLMConfig, saveLLMRole, fetchOllamaModels, testLLMConnection, runLLMEval, waiveLLMEval,
  type LLMSystemConfig, type LLMTestResult, type LLMAPIKeyState, type LLMSaveResult,
} from '@/api/llmSettings'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import type { EnrichmentSettings } from '@/api/types'

const ROLE_ORDER = ['tool_agent', 'document_extract', 'markdown_extract', 'vision']
const MANAGED_ROLE_KEYS = new Set(ROLE_ORDER)
const EVAL_STATUS_KEYS = new Set(['passed', 'waived', 'failed', 'untested', 'unknown'])

const BACKEND_OPTIONS = [
  { value: 'disabled', labelKey: 'common.disabled' },
  { value: 'ollama', labelKey: 'llmSettings.backends.ollama' },
  { value: 'llama_server', labelKey: 'llmSettings.backends.llamaServer' },
  { value: 'openai', labelKey: 'llmSettings.backends.openai' },
]

const DEFAULT_URLS: Record<string, string> = {
  ollama: 'http://ollama:11434/v1',
  llama_server: 'http://host.containers.internal:8000/v1',
  openai: 'https://api.openai.com/v1',
}

const CONTEXT_WINDOW_MIN = 1024
const CONTEXT_WINDOW_MAX = 2000000

function contextWindowToInput(value?: number | null): string {
  return typeof value === 'number' && Number.isFinite(value) ? String(value) : ''
}

function parseContextWindowInput(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  return Number(trimmed)
}

function getRoleLabel(role: string, t: TFunction): string {
  if (MANAGED_ROLE_KEYS.has(role)) return t(`llmSettings.roles.${role}.label`)
  return t('llmSettings.roles.custom.label', { role })
}

function getRoleDescription(role: string, t: TFunction): string {
  if (MANAGED_ROLE_KEYS.has(role)) return t(`llmSettings.roles.${role}.description`)
  return t('llmSettings.roles.custom.description')
}

function validateContextWindowInput(value: string, t: TFunction): string | null {
  const parsed = parseContextWindowInput(value)
  if (parsed === null) return null
  if (!Number.isInteger(parsed) || parsed < CONTEXT_WINDOW_MIN || parsed > CONTEXT_WINDOW_MAX) {
    return t('llmSettings.validation.contextWindow', { min: CONTEXT_WINDOW_MIN, max: CONTEXT_WINDOW_MAX })
  }
  return null
}

function evalStatusLabel(status: LLMSystemConfig['eval_status'] | string | undefined, t: TFunction): string {
  const key = status && EVAL_STATUS_KEYS.has(status) ? status : 'unknown'
  return t(`llmSettings.evalStatus.${key}`)
}

function isSuccessfulEvalMessage(message: string, t: TFunction): boolean {
  return message === t('llmSettings.eval.allActivePassed') ||
    message === t('llmSettings.eval.allPassed') ||
    message === t('llmSettings.eval.passed')
}

function isManagedRole(config: LLMSystemConfig): boolean {
  return ROLE_ORDER.includes(config.role)
}

function sameConnection(a?: LLMSystemConfig, b?: LLMSystemConfig): boolean {
  if (!a || !b) return false
  return a.backend_type === b.backend_type &&
    (a.api_base || '') === (b.api_base || '') &&
    (a.model_name || '') === (b.model_name || '') &&
    (a.context_window ?? null) === (b.context_window ?? null) &&
    a.has_api_key === b.has_api_key
}

function mutationErrorMessage(err: unknown, t: TFunction): string {
  if (err && typeof err === 'object') {
    const json = (err as { json?: Record<string, unknown> }).json
    const jsonMessage = json?.message
    if (typeof jsonMessage === 'string' && jsonMessage.trim()) return jsonMessage
    const jsonDetail = json?.detail
    if (typeof jsonDetail === 'string' && jsonDetail.trim()) return jsonDetail
    const status = (err as { status?: number }).status
    const body = (err as { body?: string }).body
    if (status === 500 && typeof body === 'string' && body.trim() === '') {
      return t('llmSettings.errors.emptyProxyResponse')
    }
  }
  if (err instanceof Error && err.message.trim()) return err.message
  return t('common.unknownError')
}

function llmTestMessage(result: LLMTestResult, t: TFunction): string {
  const detail = result.message.trim()
  switch (result.message_code) {
    case 'api_reachable':
      return t('llmSettings.connectionTest.messages.apiReachable', { detail })
    case 'api_responded_with_status':
      return t('llmSettings.connectionTest.messages.apiRespondedWithStatus', { detail })
    case 'connection_failed':
      return detail
        ? t('llmSettings.connectionTest.messages.connectionFailedWithDetail', { detail })
        : t('llmSettings.connectionTest.messages.connectionFailed')
    case 'invalid_url':
      return detail
        ? t('llmSettings.connectionTest.messages.invalidUrlWithDetail', { detail })
        : t('llmSettings.connectionTest.messages.invalidUrl')
    case 'unknown_backend_type':
      return t('llmSettings.connectionTest.messages.unknownBackendType')
    default:
      return detail
  }
}

function llmTestWarning(result: LLMTestResult, t: TFunction): string | null {
  switch (result.warning_code) {
    case 'localhost_rewritten_for_container':
      return t('llmSettings.connectionTest.warnings.localhostRewrittenForContainer')
    default:
      return result.warning?.trim() || null
  }
}

function OcrPromptsSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [localPrompts, setLocalPrompts] = useState<Record<string, string>>({})

  const { data: settings } = useQuery({
    queryKey: ['enrichment-settings'],
    queryFn: fetchEnrichmentSettings,
  })

  const mut = useMutation({
    mutationFn: (patch: Partial<EnrichmentSettings>) => updateEnrichmentSettings(patch),
    onSuccess: (updated) => queryClient.setQueryData(['enrichment-settings'], updated),
  })

  if (!settings) return null

  const fields = [
    { key: 'ocr_invoice_prompt' as const, label: t('llmSettings.ocr.invoice') },
    { key: 'ocr_gear_prompt' as const, label: t('llmSettings.ocr.gearDocuments') },
  ]

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('llmSettings.ocr.title')}</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          {t('llmSettings.ocr.description')}
        </p>
      </div>
      {fields.map(({ key, label }) => (
        <div key={key} className="space-y-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{label}</h3>
          <textarea
            value={localPrompts[key] ?? settings[key] ?? ''}
            placeholder={t('llmSettings.ocr.placeholder')}
            onChange={e => setLocalPrompts(prev => ({ ...prev, [key]: e.target.value }))}
            onBlur={e => {
              const val = e.target.value || null
              if (val !== (settings[key] ?? null)) {
                mut.mutate({ [key]: val })
              }
              setLocalPrompts(prev => { const next = { ...prev }; delete next[key]; return next })
            }}
            rows={6}
            className="w-full text-sm font-mono bg-neutral-50 dark:bg-neutral-900 border border-neutral-300 dark:border-neutral-700 rounded-lg p-3 resize-y text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
          />
          {settings[key] && (
            <button
              onClick={() => mut.mutate({ [key]: null })}
              className="cursor-pointer text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors"
            >
              {t('settings.adminSystem.actions.reset')}
            </button>
          )}
        </div>
      ))}
    </section>
  )
}

export default function LLMSettingsPage() {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const [separateRolesOverride, setSeparateRolesOverride] = useState<boolean | null>(null)
  const { data: configs = [], isLoading } = useQuery({
    queryKey: ['llm-config'],
    queryFn: fetchLLMConfig,
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">{t('common.loading')}</div>
  }

  const roles = configs.filter(isManagedRole).sort((a, b) => {
    const ai = ROLE_ORDER.indexOf(a.role)
    const bi = ROLE_ORDER.indexOf(b.role)
    if (ai !== -1 || bi !== -1) {
      return (ai === -1 ? Number.MAX_SAFE_INTEGER : ai) - (bi === -1 ? Number.MAX_SAFE_INTEGER : bi)
    }
    return a.role.localeCompare(b.role)
  })
  const primaryRole = roles.find(config => config.role === 'tool_agent') ?? roles[0]
  const hasActiveSeparateConfig = roles.some(config =>
    config.role !== primaryRole?.role && config.enabled && !sameConnection(config, primaryRole),
  )
  const separateRoles = separateRolesOverride ?? hasActiveSeparateConfig
  const sharedRoleNames = roles.map(config => config.role)

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('llmSettings.title')}</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          {t('llmSettings.description')}
        </p>
      </div>

      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="font-medium text-neutral-900 dark:text-neutral-100">{t('llmSettings.sharedToggle.title')}</h3>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              {t('llmSettings.sharedToggle.description')}
            </p>
          </div>
          <Toggle
            checked={!separateRoles}
            onChange={checked => setSeparateRolesOverride(!checked)}
            aria-label={t('llmSettings.sharedToggle.ariaLabel')}
          />
        </div>
      </section>

      <div className="space-y-4">
        {!separateRoles && primaryRole ? (
          <LLMSharedConfigCard
            roles={roles}
            primaryConfig={primaryRole}
            roleNames={sharedRoleNames}
            onSave={() => qc.invalidateQueries({ queryKey: ['llm-config'] })}
          />
        ) : (
          roles.map(config => {
            const role = config.role
            return (
            <LLMRoleCard
              key={role}
              role={role}
              config={config}
              onSave={() => qc.invalidateQueries({ queryKey: ['llm-config'] })}
            />
            )
          })
        )}
      </div>
      <OcrPromptsSection />
    </div>
  )
}

function LLMSharedConfigCard({ roles, primaryConfig, roleNames, onSave }: {
  roles: LLMSystemConfig[]
  primaryConfig: LLMSystemConfig
  roleNames: string[]
  onSave: () => void
}) {
  const { t } = useTranslation()
  const [backend, setBackend] = useState(primaryConfig.backend_type || 'disabled')
  const [apiBase, setApiBase] = useState(primaryConfig.api_base || '')
  const [modelName, setModelName] = useState(primaryConfig.model_name || '')
  const [contextWindow, setContextWindow] = useState(contextWindowToInput(primaryConfig.context_window))
  const [apiKey, setApiKey] = useState('')
  const [apiKeyState, setApiKeyState] = useState<LLMAPIKeyState>('unchanged')
  const [enabledByRole, setEnabledByRole] = useState<Record<string, boolean>>({})
  const [testResult, setTestResult] = useState<LLMTestResult | null>(null)
  const [evalResult, setEvalResult] = useState<string | null>(null)
  const [localSaveError, setLocalSaveError] = useState<string | null>(null)
  const [saveWarning, setSaveWarning] = useState<string | null>(null)
  const [waiverReasons, setWaiverReasons] = useState<Record<string, string>>({})
  const [waivingRole, setWaivingRole] = useState<string | null>(null)
  const [ollamaModels, setOllamaModels] = useState<string[]>([])
  const [isLoadingModels, setIsLoadingModels] = useState(false)
  const [isTesting, setIsTesting] = useState(false)
  const [isEvaluating, setIsEvaluating] = useState(false)
  const contextWindowInputID = useId()

  useEffect(() => {
    setBackend(primaryConfig.backend_type || 'disabled')
    setApiBase(primaryConfig.api_base || '')
    setModelName(primaryConfig.model_name || '')
    setContextWindow(contextWindowToInput(primaryConfig.context_window))
    setApiKey('')
    setApiKeyState('unchanged')
    setEnabledByRole(Object.fromEntries(roles.map(role => [role.role, role.enabled])))
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
    setWaiverReasons({})
    setWaivingRole(null)
  }, [primaryConfig, roles])

  const roleConfigsByName = new Map(roles.map(role => [role.role, role]))
  const needsURL = backend !== 'disabled'
  const needsAPIKey = backend === 'openai'
  const isOllama = backend === 'ollama'
  const canShowKeyControls = needsAPIKey || roles.some(role => role.has_api_key)
  const activeRoleNames = roleNames.filter(roleName => enabledByRole[roleName])
  const isDirty = backend !== (primaryConfig.backend_type || 'disabled') ||
    (apiBase || '') !== (primaryConfig.api_base || '') ||
    (modelName || '') !== (primaryConfig.model_name || '') ||
    contextWindow !== contextWindowToInput(primaryConfig.context_window) ||
    apiKeyState !== 'unchanged' ||
    roles.some(role => enabledByRole[role.role] !== role.enabled)

  const validateBeforeSave = (): string | null => {
    if (activeRoleNames.length === 0) return null
    if (backend === 'disabled') return t('llmSettings.validation.backendRequired')
    if (needsURL && !apiBase.trim()) return t('llmSettings.validation.apiBaseRequired')
    if (!modelName.trim()) return t('llmSettings.validation.modelRequired')
    const contextWindowError = validateContextWindowInput(contextWindow, t)
    if (contextWindowError) return contextWindowError
    if (needsAPIKey && apiKeyState !== 'set') {
      const roleWithoutKey = activeRoleNames.find(roleName => !roleConfigsByName.get(roleName)?.has_api_key)
      if (roleWithoutKey) return t('llmSettings.validation.sharedOpenAiKeyRequired')
    }
    return null
  }

  const saveMutation = useMutation({
    mutationFn: async () => {
      const results = await Promise.all(roleNames.map(roleName => saveLLMRole(roleName, {
        backend_type: backend,
        api_base: apiBase || undefined,
        model_name: modelName || undefined,
        context_window: parseContextWindowInput(contextWindow),
        api_key: apiKeyState === 'set' ? apiKey : undefined,
        api_key_state: apiKeyState,
        enabled: !!enabledByRole[roleName],
      })))
      return results.find(result => result?.warning) ?? results[0]
    },
    onMutate: () => {
      setLocalSaveError(null)
      setSaveWarning(null)
    },
    onSuccess: result => {
      setSaveWarning(result?.warning ?? null)
      onSave()
    },
    onError: err => {
      setLocalSaveError(mutationErrorMessage(err, t))
    },
  })

  const handleBackendChange = (newBackend: string) => {
    setBackend(newBackend as Parameters<typeof setBackend>[0])
    if (DEFAULT_URLS[newBackend] && !apiBase) setApiBase(DEFAULT_URLS[newBackend])
    setOllamaModels([])
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
  }

  const loadOllamaModels = async () => {
    setIsLoadingModels(true)
    try {
      const models = await fetchOllamaModels(apiBase || DEFAULT_URLS.ollama)
      setOllamaModels(models.map(m => m.name))
    } finally {
      setIsLoadingModels(false)
    }
  }

  const runTest = async () => {
    const contextWindowError = validateContextWindowInput(contextWindow, t)
    if (contextWindowError) {
      setTestResult({ success: false, message: contextWindowError })
      return
    }
    setIsTesting(true)
    setTestResult(null)
    try {
      const result = await testLLMConnection({
        backend_type: backend,
        api_base: apiBase || DEFAULT_URLS[backend] || '',
        model_name: modelName,
        api_key: apiKey || undefined,
      })
      setTestResult(result)
    } catch (err) {
      setTestResult({ success: false, message: mutationErrorMessage(err, t) })
    }
    setIsTesting(false)
  }

  const runEval = async () => {
    if (isDirty) {
      setEvalResult(t('llmSettings.eval.saveBeforeEval'))
      return
    }
    setIsEvaluating(true)
    setEvalResult(null)
    try {
      const results = await Promise.all(activeRoleNames.map(async roleName => ({ roleName, result: await runLLMEval(roleName) })))
      const failed = results.filter(item => item.result.status !== 'passed')
      setEvalResult(failed.length === 0
        ? t('llmSettings.eval.allActivePassed')
        : t('llmSettings.eval.failedSummary', { summary: failed.map(item => `${getRoleLabel(item.roleName, t)} ${evalStatusLabel(item.result.status, t)}`).join(', ') }))
      onSave()
    } catch (err) {
      setEvalResult(t('llmSettings.eval.failed', { error: mutationErrorMessage(err, t) }))
    }
    setIsEvaluating(false)
  }

  const saveWaiver = async (roleName: string) => {
    if (isDirty) {
      setLocalSaveError(t('llmSettings.eval.saveBeforeWaiver'))
      return
    }
    const reason = (waiverReasons[roleName] ?? '').trim()
    if (!reason) return
    setWaivingRole(roleName)
    setEvalResult(null)
    setLocalSaveError(null)
    try {
      await waiveLLMEval(roleName, reason)
      setWaiverReasons(current => {
        const next = { ...current }
        delete next[roleName]
        return next
      })
      onSave()
    } catch (err) {
      setLocalSaveError(mutationErrorMessage(err, t))
    } finally {
      setWaivingRole(null)
    }
  }

  const handleSave = () => {
    const validationError = validateBeforeSave()
    if (validationError) {
      setLocalSaveError(validationError)
      return
    }
    saveMutation.mutate()
  }

  const saveError = localSaveError ?? (saveMutation.isError ? mutationErrorMessage(saveMutation.error, t) : null)
  const evalDisabledReason = isDirty ? t('llmSettings.eval.saveBeforeEvalSentence') : null
  const waiverDisabledReason = isDirty ? t('llmSettings.eval.saveBeforeWaiver') : null

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5 space-y-4">
      <div>
        <h3 className="font-medium text-neutral-900 dark:text-neutral-100">{t('llmSettings.shared.title')}</h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          {t('llmSettings.shared.description')}
        </p>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        {roles.map(role => (
          <div key={role.role} className="rounded-lg border border-neutral-100 dark:border-neutral-800 px-3 py-2 space-y-2">
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{getRoleLabel(role.role, t)}</div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">{getRoleDescription(role.role, t)}</p>
                <span className={`mt-1 inline-block rounded px-2 py-0.5 text-xs ${
                  role.eval_status === 'passed'
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                    : role.eval_status === 'waived'
                      ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                      : role.eval_status === 'failed'
                        ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
                        : 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300'
                }`}>
                  {t('llmSettings.statusWithValue', { status: evalStatusLabel(role.eval_status, t) })}
                </span>
              </div>
              <Toggle
                checked={!!enabledByRole[role.role]}
                onChange={checked => {
                  setEnabledByRole(current => ({ ...current, [role.role]: checked }))
                  setEvalResult(null)
                  setLocalSaveError(null)
                  setSaveWarning(null)
                  if (checked && backend === 'disabled') {
                    setBackend('llama_server')
                    if (!apiBase) setApiBase(DEFAULT_URLS.llama_server)
                  }
                }}
                aria-label={t('llmSettings.enableRole', { role: getRoleLabel(role.role, t) })}
              />
            </div>
            {enabledByRole[role.role] && role.eval_status !== 'passed' && role.eval_status !== 'waived' && (
              <div className="flex gap-2">
                <input
                  type="text"
                  value={waiverReasons[role.role] ?? ''}
                  onChange={e => setWaiverReasons(current => ({ ...current, [role.role]: e.target.value }))}
                  placeholder={t('llmSettings.waiver.reasonPlaceholder')}
                  className="min-w-0 flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2 py-1 text-xs text-neutral-900 dark:text-neutral-100 focus:outline-none"
                  aria-label={t('llmSettings.waiver.reasonAriaLabel', { role: getRoleLabel(role.role, t) })}
                />
                <button
                  type="button"
                  onClick={() => saveWaiver(role.role)}
                  disabled={!!waiverDisabledReason || waivingRole === role.role || !(waiverReasons[role.role] ?? '').trim()}
                  title={waiverDisabledReason ?? undefined}
                  className="cursor-pointer px-2 py-1 rounded-lg text-xs border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                >
                  {waivingRole === role.role ? t('common.saving') : t('llmSettings.waiver.submit')}
                </button>
              </div>
            )}
            {enabledByRole[role.role] && role.eval_status !== 'passed' && role.eval_status !== 'waived' && waiverDisabledReason && (
              <p className="text-xs text-amber-600 dark:text-amber-400">{waiverDisabledReason}</p>
            )}
          </div>
        ))}
      </div>

      {activeRoleNames.length > 0 && (
        <div className="space-y-3">
          <div>
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">{t('llmSettings.fields.backend')}</label>
            <select
              value={backend}
              onChange={e => handleBackendChange(e.target.value)}
              className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none"
            >
              {BACKEND_OPTIONS.filter(o => o.value !== 'disabled').map(o => (
                <option key={o.value} value={o.value}>{t(o.labelKey)}</option>
              ))}
            </select>
          </div>

          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {isOllama ? 'Ollama URL' : t('llmSettings.fields.apiBaseUrl')}
              </label>
              <input
                type="text"
                value={apiBase}
                onChange={e => {
                  setApiBase(e.target.value)
                  setTestResult(null)
                  setEvalResult(null)
                  setLocalSaveError(null)
                  setSaveWarning(null)
                }}
                placeholder={DEFAULT_URLS[backend] || 'https://...'}
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
            </div>
          )}

          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">{t('llmSettings.fields.model')}</label>
              {isOllama && ollamaModels.length > 0 ? (
                <select
                  value={modelName}
                  onChange={e => setModelName(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none"
                >
                  <option value="">{t('llmSettings.fields.selectModel')}</option>
                  {ollamaModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              ) : (
                <div className="flex gap-2 mt-1">
                  <input
                    type="text"
                    value={modelName}
                    onChange={e => {
                      setModelName(e.target.value)
                      setTestResult(null)
                      setEvalResult(null)
                      setLocalSaveError(null)
                      setSaveWarning(null)
                    }}
                    placeholder={isOllama ? 'qwen3:8b' : 'gpt-4o'}
                    className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono text-neutral-900 dark:text-neutral-100 focus:outline-none"
                  />
                  {isOllama && (
                    <button
                      onClick={loadOllamaModels}
                      disabled={isLoadingModels}
                      className="cursor-pointer px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                    >
                      {isLoadingModels ? t('common.loading') : t('llmSettings.actions.loadModels')}
                    </button>
                  )}
                </div>
              )}
            </div>
          )}

          {needsURL && (
            <div>
              <label htmlFor={contextWindowInputID} className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">{t('llmSettings.fields.contextWindow')}</label>
              <input
                id={contextWindowInputID}
                type="number"
                min={CONTEXT_WINDOW_MIN}
                max={CONTEXT_WINDOW_MAX}
                step={1024}
                value={contextWindow}
                onChange={e => {
                  setContextWindow(e.target.value)
                  setEvalResult(null)
                  setLocalSaveError(null)
                  setSaveWarning(null)
                }}
                placeholder={t('llmSettings.fields.contextWindowPlaceholder')}
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                {t('llmSettings.fields.contextWindowHint')}
              </p>
            </div>
          )}

          {canShowKeyControls && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {t('llmSettings.fields.apiKey')} {roles.some(role => role.has_api_key) && apiKeyState === 'unchanged' && <span className="text-green-500">{t('llmSettings.apiKey.set')}</span>}
                {apiKeyState === 'clear' && <span className="text-amber-500">{t('llmSettings.apiKey.willDelete')}</span>}
              </label>
              <div className="mt-1 flex gap-2">
                <input
                  type="password"
                  value={apiKey}
                  onChange={e => {
                    setApiKey(e.target.value)
                    setApiKeyState(e.target.value ? 'set' : 'unchanged')
                    setTestResult(null)
                    setEvalResult(null)
                    setLocalSaveError(null)
                    setSaveWarning(null)
                  }}
                  placeholder={roles.some(role => role.has_api_key) ? t('llmSettings.apiKey.sharedPlaceholder') : 'sk-...'}
                  className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono text-neutral-900 dark:text-neutral-100 focus:outline-none"
                />
                {roles.some(role => role.has_api_key) && (
                  <button
                    type="button"
                    onClick={() => {
                      setApiKey('')
                      setApiKeyState(apiKeyState === 'clear' ? 'unchanged' : 'clear')
                    }}
                    className="cursor-pointer px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors whitespace-nowrap"
                  >
                    {apiKeyState === 'clear' ? t('llmSettings.apiKey.keep') : t('common.delete')}
                  </button>
                )}
              </div>
            </div>
          )}

          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={runTest}
              disabled={isTesting || !needsURL}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isTesting ? t('llmSettings.actions.testing') : t('llmSettings.actions.testConnection')}
            </button>
            <button
              onClick={runEval}
              disabled={isEvaluating || !needsURL || !modelName || isDirty || activeRoleNames.length === 0}
              title={evalDisabledReason ?? undefined}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isEvaluating ? t('llmSettings.actions.evaluating') : t('llmSettings.actions.startEval')}
            </button>
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : t('common.error')} {llmTestMessage(testResult, t)}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
            {testResult && llmTestWarning(testResult, t) && (
              <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">! {llmTestWarning(testResult, t)}</p>
            )}
            {evalResult && (
              <span className={`text-xs ${isSuccessfulEvalMessage(evalResult, t) ? 'text-green-500' : 'text-rose-500'}`}>
                {evalResult}
              </span>
            )}
          </div>
          {evalDisabledReason && <p className="text-xs text-amber-600 dark:text-amber-400">{evalDisabledReason}</p>}
        </div>
      )}

      <div className="flex justify-end pt-1">
        <button
          onClick={handleSave}
          disabled={saveMutation.isPending}
          className="cursor-pointer px-4 py-1.5 text-sm rounded-lg text-white bg-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {saveMutation.isPending ? t('common.saving') : t('common.save')}
        </button>
      </div>
      {saveError && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {t('llmSettings.saveFailed', { error: saveError })}
        </p>
      )}
      {saveWarning && (
        <p role="status" className="text-sm text-amber-600 dark:text-amber-400">
          {t('llmSettings.saveWarning', { warning: saveWarning })}
        </p>
      )}
    </div>
  )
}

function LLMRoleCard({
  role,
  roleLabel,
  roleDescription,
  config,
  allRoleConfigs,
  saveRoleNames,
  onSave,
}: {
  role: string
  roleLabel?: string
  roleDescription?: string
  config?: LLMSystemConfig
  allRoleConfigs?: LLMSystemConfig[]
  saveRoleNames?: string[]
  onSave: () => void
}) {
  const { t } = useTranslation()
  const [backend, setBackend] = useState(config?.backend_type || 'disabled')
  const [apiBase, setApiBase] = useState(config?.api_base || '')
  const [modelName, setModelName] = useState(config?.model_name || '')
  const [contextWindow, setContextWindow] = useState(contextWindowToInput(config?.context_window))
  const [apiKey, setApiKey] = useState('')
  const [apiKeyState, setApiKeyState] = useState<LLMAPIKeyState>('unchanged')
  const [enabled, setEnabled] = useState(config?.enabled || false)
  const [testResult, setTestResult] = useState<LLMTestResult | null>(null)
  const [evalResult, setEvalResult] = useState<string | null>(null)
  const [waiverReason, setWaiverReason] = useState('')
  const [isWaiving, setIsWaiving] = useState(false)
  const [localSaveError, setLocalSaveError] = useState<string | null>(null)
  const [saveWarning, setSaveWarning] = useState<string | null>(null)
  const rolesToSave = saveRoleNames?.length ? saveRoleNames : [role]
  const isSharedCard = rolesToSave.length > 1
  const roleConfigsByName = new Map((allRoleConfigs ?? (config ? [config] : [])).map(item => [item.role, item]))

  useEffect(() => {
    setBackend(config?.backend_type || 'disabled')
    setApiBase(config?.api_base || '')
    setModelName(config?.model_name || '')
    setContextWindow(contextWindowToInput(config?.context_window))
    setApiKey('')
    setApiKeyState('unchanged')
    setEnabled(config?.enabled || false)
    setLocalSaveError(null)
    setSaveWarning(null)
  }, [config])
  const [ollamaModels, setOllamaModels] = useState<string[]>([])
  const [isLoadingModels, setIsLoadingModels] = useState(false)
  const [isTesting, setIsTesting] = useState(false)
  const [isEvaluating, setIsEvaluating] = useState(false)
  const contextWindowInputID = useId()

  const saveMutation = useMutation({
    mutationFn: async () => {
      const update = { backend_type: backend, api_base: apiBase || undefined,
        model_name: modelName || undefined, context_window: parseContextWindowInput(contextWindow),
        api_key: apiKeyState === 'set' ? apiKey : undefined,
        api_key_state: apiKeyState, enabled }
      const results = await Promise.all(rolesToSave.map(roleName => saveLLMRole(roleName, update)))
      return results.find(result => result?.warning) ?? results[0]
    },
    onMutate: () => {
      setLocalSaveError(null)
      setSaveWarning(null)
    },
    onSuccess: (result?: LLMSaveResult) => {
      setLocalSaveError(null)
      setSaveWarning(result?.warning ?? null)
      onSave()
    },
    onError: err => {
      setLocalSaveError(mutationErrorMessage(err, t))
    },
  })

  const handleBackendChange = (newBackend: string) => {
    setBackend(newBackend as Parameters<typeof setBackend>[0])
    if (DEFAULT_URLS[newBackend] && !apiBase) {
      setApiBase(DEFAULT_URLS[newBackend])
    }
    setOllamaModels([])
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
  }

  const loadOllamaModels = async () => {
    setIsLoadingModels(true)
    try {
      const models = await fetchOllamaModels(apiBase || DEFAULT_URLS.ollama)
      setOllamaModels(models.map(m => m.name))
    } finally {
      setIsLoadingModels(false)
    }
  }

  const runTest = async () => {
    const contextWindowError = validateContextWindowInput(contextWindow, t)
    if (contextWindowError) {
      setTestResult({ success: false, message: contextWindowError })
      return
    }
    setIsTesting(true)
    setTestResult(null)
    try {
      const result = await testLLMConnection({
        backend_type: backend,
        api_base: apiBase || DEFAULT_URLS[backend] || '',
        model_name: modelName,
        api_key: apiKey || undefined,
      })
      setTestResult(result)
    } catch (err) {
      setTestResult({ success: false, message: mutationErrorMessage(err, t) })
    }
    setIsTesting(false)
  }

  const runEval = async () => {
    if (isDirty) {
      setEvalResult(t('llmSettings.eval.saveBeforeEval'))
      return
    }
    setIsEvaluating(true)
    setEvalResult(null)
    try {
      const results = await Promise.all(rolesToSave.map(async roleName => ({ roleName, result: await runLLMEval(roleName) })))
      const failed = results.filter(item => item.result.status !== 'passed')
      if (failed.length === 0) {
        setEvalResult(isSharedCard ? t('llmSettings.eval.allPassed') : t('llmSettings.eval.passed'))
      } else {
        setEvalResult(t('llmSettings.eval.failedSummary', { summary: failed.map(item => `${getRoleLabel(item.roleName, t)} ${evalStatusLabel(item.result.status, t)}`).join(', ') }))
      }
      onSave()
    } catch (err) {
      setEvalResult(t('llmSettings.eval.failed', { error: mutationErrorMessage(err, t) }))
    }
    setIsEvaluating(false)
  }

  const saveWaiver = async () => {
    if (isDirty) {
      setLocalSaveError(t('llmSettings.eval.saveBeforeWaiver'))
      return
    }
    const reason = waiverReason.trim()
    if (!reason) return
    setIsWaiving(true)
    setEvalResult(null)
    try {
      await waiveLLMEval(role, reason)
      setWaiverReason('')
      setEvalResult(t('llmSettings.waiver.saved'))
      onSave()
    } catch (err) {
      setLocalSaveError(mutationErrorMessage(err, t))
    }
    setIsWaiving(false)
  }

  const needsURL = backend !== 'disabled'
  const needsAPIKey = backend === 'openai'
  const canShowKeyControls = needsAPIKey || !!config?.has_api_key
  const isOllama = backend === 'ollama'
  const providerBaseChanged = !!config?.has_api_key &&
    (backend !== config.backend_type || (apiBase || '') !== (config.api_base || ''))
  const isDirty = backend !== (config?.backend_type || 'disabled') ||
    (apiBase || '') !== (config?.api_base || '') ||
    (modelName || '') !== (config?.model_name || '') ||
    contextWindow !== contextWindowToInput(config?.context_window) ||
    enabled !== (config?.enabled || false) ||
    apiKeyState !== 'unchanged'
  const saveError = localSaveError ?? (saveMutation.isError ? mutationErrorMessage(saveMutation.error, t) : null)
  const evalDisabledReason = isDirty ? t('llmSettings.eval.saveBeforeEvalSentence') : null
  const waiverDisabledReason = isDirty ? t('llmSettings.eval.saveBeforeWaiver') : null

  const validateBeforeSave = (): string | null => {
    if (!enabled) return null
    if (backend === 'disabled') return t('llmSettings.validation.backendRequired')
    if (needsURL && !apiBase.trim()) return t('llmSettings.validation.apiBaseRequired')
    if (!modelName.trim()) return t('llmSettings.validation.modelRequired')
    const contextWindowError = validateContextWindowInput(contextWindow, t)
    if (contextWindowError) return contextWindowError
    if (needsAPIKey && !config?.has_api_key && apiKeyState !== 'set') return t('llmSettings.validation.openAiKeyRequired')
    if (isSharedCard && needsAPIKey && apiKeyState !== 'set') {
      const roleWithoutKey = rolesToSave.find(roleName => !roleConfigsByName.get(roleName)?.has_api_key)
      if (roleWithoutKey) return t('llmSettings.validation.sharedOpenAiKeyRequired')
    }
    if (providerBaseChanged && apiKeyState === 'unchanged') return t('llmSettings.validation.keyResetRequired')
    return null
  }

  const handleEnabledChange = (nextEnabled: boolean) => {
    setEnabled(nextEnabled)
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
    if (nextEnabled && backend === 'disabled') {
      setBackend('llama_server')
      if (!apiBase) setApiBase(DEFAULT_URLS.llama_server)
    }
  }

  const handleApiBaseChange = (nextApiBase: string) => {
    setApiBase(nextApiBase)
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
  }

  const handleModelNameChange = (nextModelName: string) => {
    setModelName(nextModelName)
    setTestResult(null)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
  }

  const handleContextWindowChange = (nextContextWindow: string) => {
    setContextWindow(nextContextWindow)
    setEvalResult(null)
    setLocalSaveError(null)
    setSaveWarning(null)
  }

  const handleSave = () => {
    const validationError = validateBeforeSave()
    if (validationError) {
      setLocalSaveError(validationError)
      return
    }
    saveMutation.mutate()
  }

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5 space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="font-medium text-neutral-900 dark:text-neutral-100">
            {roleLabel ?? getRoleLabel(role, t)}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {roleDescription ?? getRoleDescription(role, t)}
          </p>
          {isSharedCard ? (
            <div className="flex flex-wrap items-center gap-2 mt-2 text-xs">
              {rolesToSave.map(roleName => {
                const roleConfig = roleConfigsByName.get(roleName)
                return (
                  <span key={roleName} className="rounded bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300 px-2 py-0.5">
                    {getRoleLabel(roleName, t)}: {evalStatusLabel(roleConfig?.eval_status, t)}
                  </span>
                )
              })}
            </div>
          ) : config && (
            <div className="flex flex-wrap items-center gap-2 mt-2 text-xs">
              <span className={`rounded px-2 py-0.5 ${
                config.eval_status === 'passed'
                  ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                  : config.eval_status === 'waived'
                    ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                    : config.eval_status === 'failed'
                      ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
                      : 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300'
              }`}>
                {t('llmSettings.statusWithValue', { status: evalStatusLabel(config.eval_status, t) })}
              </span>
              {config.eval_status === 'waived' && (
                <span className="text-amber-600 dark:text-amber-400">{t('llmSettings.waiver.active')}</span>
              )}
            </div>
          )}
        </div>
        <Toggle
          checked={enabled}
          onChange={handleEnabledChange}
          aria-label={t('llmSettings.enableRole', { role: roleLabel ?? getRoleLabel(role, t) })}
        />
      </div>

      {enabled && (
        <div className="space-y-3">
          {/* Backend selector */}
          <div>
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
              {t('llmSettings.fields.backend')}
            </label>
            <select
              value={backend}
              onChange={e => handleBackendChange(e.target.value)}
              className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-3 py-2 text-sm
                         text-neutral-900 dark:text-neutral-100 focus:outline-none"
            >
              {BACKEND_OPTIONS.filter(o => o.value !== 'disabled').map(o => (
                <option key={o.value} value={o.value}>{t(o.labelKey)}</option>
              ))}
            </select>
          </div>

          {/* API base URL */}
          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {isOllama ? 'Ollama URL' : t('llmSettings.fields.apiBaseUrl')}
              </label>
              <input
                type="text"
                value={apiBase}
                onChange={e => handleApiBaseChange(e.target.value)}
                placeholder={DEFAULT_URLS[backend] || 'https://...'}
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                           bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                           text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
            </div>
          )}

          {/* Model selector */}
          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {t('llmSettings.fields.model')}
              </label>
              {isOllama && ollamaModels.length > 0 ? (
                <select
                  value={modelName}
                  onChange={e => setModelName(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                             bg-white dark:bg-neutral-800 px-3 py-2 text-sm
                             text-neutral-900 dark:text-neutral-100 focus:outline-none"
                >
                  <option value="">{t('llmSettings.fields.selectModel')}</option>
                  {ollamaModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              ) : (
                <div className="flex gap-2 mt-1">
                  <input
                    type="text"
                    value={modelName}
                    onChange={e => handleModelNameChange(e.target.value)}
                    placeholder={isOllama ? 'qwen3:8b' : 'gpt-4o'}
                    className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700
                               bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                               text-neutral-900 dark:text-neutral-100 focus:outline-none"
                  />
                  {isOllama && (
                    <button
                      onClick={loadOllamaModels}
                      disabled={isLoadingModels}
                      className="cursor-pointer px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700
                                 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                                 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                    >
                      {isLoadingModels ? t('common.loading') : t('llmSettings.actions.loadModels')}
                    </button>
                  )}
                </div>
              )}
            </div>
          )}

          {/* Context window */}
          {needsURL && (
            <div>
              <label htmlFor={contextWindowInputID} className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {t('llmSettings.fields.contextWindow')}
              </label>
              <input
                id={contextWindowInputID}
                type="number"
                min={CONTEXT_WINDOW_MIN}
                max={CONTEXT_WINDOW_MAX}
                step={1024}
                value={contextWindow}
                onChange={e => handleContextWindowChange(e.target.value)}
                placeholder={t('llmSettings.fields.contextWindowPlaceholder')}
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                           bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                           text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                {t('llmSettings.fields.contextWindowHint')}
              </p>
            </div>
          )}

          {/* API key */}
          {canShowKeyControls && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {t('llmSettings.fields.apiKey')} {config?.has_api_key && apiKeyState === 'unchanged' && <span className="text-green-500">{t('llmSettings.apiKey.set')}</span>}
                {apiKeyState === 'clear' && <span className="text-amber-500">{t('llmSettings.apiKey.willDelete')}</span>}
              </label>
              <div className="mt-1 flex gap-2">
                <input
                  type="password"
                  value={apiKey}
                  onChange={e => {
                    setApiKey(e.target.value)
                    setApiKeyState(e.target.value ? 'set' : 'unchanged')
                    setTestResult(null)
                    setEvalResult(null)
                    setLocalSaveError(null)
                    setSaveWarning(null)
                  }}
                  placeholder={config?.has_api_key ? t('llmSettings.apiKey.placeholder') : 'sk-...'}
                  className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700
                             bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                             text-neutral-900 dark:text-neutral-100 focus:outline-none"
                />
                {config?.has_api_key && (
                  <button
                    type="button"
                    onClick={() => {
                      setApiKey('')
                      setApiKeyState(apiKeyState === 'clear' ? 'unchanged' : 'clear')
                      setTestResult(null)
                      setEvalResult(null)
                      setLocalSaveError(null)
                      setSaveWarning(null)
                    }}
                    className="cursor-pointer px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700
                               text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                               transition-colors whitespace-nowrap"
                  >
                    {apiKeyState === 'clear' ? t('llmSettings.apiKey.keep') : t('common.delete')}
                  </button>
                )}
              </div>
              {providerBaseChanged && apiKeyState === 'unchanged' && (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  {t('llmSettings.validation.keyResetRequired')}
                </p>
              )}
            </div>
          )}

          {/* Connection checks */}
          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={runTest}
              disabled={isTesting || !needsURL}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isTesting ? t('llmSettings.actions.testing') : t('llmSettings.actions.testConnection')}
            </button>
            <button
              onClick={runEval}
              disabled={isEvaluating || !needsURL || !modelName || isDirty}
              title={evalDisabledReason ?? undefined}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isEvaluating ? t('llmSettings.actions.evaluating') : t('llmSettings.actions.startEval')}
            </button>
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : t('common.error')} {llmTestMessage(testResult, t)}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
            {testResult && llmTestWarning(testResult, t) && (
              <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">! {llmTestWarning(testResult, t)}</p>
            )}
            {evalResult && (
              <span className={`text-xs ${isSuccessfulEvalMessage(evalResult, t) ? 'text-green-500' : 'text-rose-500'}`}>
                {evalResult}
              </span>
            )}
          </div>
          {evalDisabledReason && (
            <p className="text-xs text-amber-600 dark:text-amber-400">{evalDisabledReason}</p>
          )}

          {!isSharedCard && config?.eval_status !== 'passed' && config?.eval_status !== 'waived' && (
            <div className="pt-2 space-y-2">
              <label htmlFor={`waiver-${role}`} className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {t('llmSettings.waiver.reasonLabel')}
              </label>
              <div className="flex gap-2">
                <input
                  id={`waiver-${role}`}
                  type="text"
                  value={waiverReason}
                  onChange={e => setWaiverReason(e.target.value)}
                  placeholder={t('llmSettings.waiver.reasonPlaceholderLong')}
                  className="flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700
                             bg-white dark:bg-neutral-800 px-3 py-2 text-sm
                             text-neutral-900 dark:text-neutral-100 focus:outline-none"
                />
                <button
                  type="button"
                  onClick={saveWaiver}
                  disabled={!!waiverDisabledReason || isWaiving || !waiverReason.trim()}
                  title={waiverDisabledReason ?? undefined}
                  className="cursor-pointer px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700
                             text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                             disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                >
                  {isWaiving ? t('common.saving') : t('llmSettings.waiver.submit')}
                </button>
              </div>
              {waiverDisabledReason && (
                <p className="text-xs text-amber-600 dark:text-amber-400">{waiverDisabledReason}</p>
              )}
            </div>
          )}
        </div>
      )}

      {/* Save button stays visible even when disabled */}
      <div className="flex justify-end pt-1">
        <button
          onClick={handleSave}
          disabled={saveMutation.isPending}
          className="cursor-pointer px-4 py-1.5 text-sm rounded-lg text-white bg-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {saveMutation.isPending ? t('common.saving') : t('common.save')}
        </button>
      </div>
      {saveError && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {t('llmSettings.saveFailed', { error: saveError })}
        </p>
      )}
      {saveWarning && (
        <p role="status" className="text-sm text-amber-600 dark:text-amber-400">
          {t('llmSettings.saveWarning', { warning: saveWarning })}
        </p>
      )}
    </div>
  )
}
