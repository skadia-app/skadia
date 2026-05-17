import { useId, useState, useEffect } from 'react'
import Toggle from '@/components/Toggle'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchLLMConfig, saveLLMRole, fetchOllamaModels, testLLMConnection, runLLMEval, waiveLLMEval,
  type LLMSystemConfig, type LLMTestResult, type LLMAPIKeyState, type LLMSaveResult,
} from '@/api/llmSettings'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import type { EnrichmentSettings } from '@/api/types'

const ROLE_LABELS: Record<string, string> = {
  tool_agent: 'Tool-Agent',
  document_extract: 'Dokumente',
  markdown_extract: 'Markdown',
  vision: 'Vision',
}

const ROLE_DESCRIPTIONS: Record<string, string> = {
  tool_agent: 'Chat, Tool-Aufrufe, SQL-Readonly, Write-Preview, JSON-Argumente und passende Antwortsprache.',
  document_extract: 'OCR/PDF-Textextraktion, Gear-Dokumente und strukturierte Felder.',
  markdown_extract: 'Markdown-Konvertierung mit Tabellen-Erhalt.',
  vision: 'Bild-, PDF- und Screenshot-Verständnis.',
}

const ROLE_ORDER = ['tool_agent', 'document_extract', 'markdown_extract', 'vision']

const BACKEND_OPTIONS = [
  { value: 'disabled', label: 'Deaktiviert' },
  { value: 'ollama', label: 'Ollama (Container)' },
  { value: 'llama_server', label: 'llama-server / OpenAI-kompatibel' },
  { value: 'openai', label: 'OpenAI API' },
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

function validateContextWindowInput(value: string): string | null {
  const parsed = parseContextWindowInput(value)
  if (parsed === null) return null
  if (!Number.isInteger(parsed) || parsed < CONTEXT_WINDOW_MIN || parsed > CONTEXT_WINDOW_MAX) {
    return `Kontextfenster muss leer sein oder zwischen ${CONTEXT_WINDOW_MIN} und ${CONTEXT_WINDOW_MAX} liegen.`
  }
  return null
}

function evalStatusLabel(status: LLMSystemConfig['eval_status'] | string | undefined): string {
  switch (status) {
    case 'passed':
      return 'geprüft'
    case 'waived':
      return 'freigegeben'
    case 'failed':
      return 'Prüfung fehlgeschlagen'
    default:
      return 'nicht geprüft'
  }
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

function mutationErrorMessage(err: unknown): string {
  if (err && typeof err === 'object') {
    const json = (err as { json?: Record<string, unknown> }).json
    const jsonMessage = json?.message
    if (typeof jsonMessage === 'string' && jsonMessage.trim()) return jsonMessage
    const status = (err as { status?: number }).status
    const body = (err as { body?: string }).body
    if (status === 500 && typeof body === 'string' && body.trim() === '') {
      return 'Der Server oder Dev-Proxy hat die Verbindung ohne Fehlermeldung beendet.'
    }
  }
  if (err instanceof Error && err.message.trim()) return err.message
  return 'Unbekannter Fehler'
}

function OcrPromptsSection() {
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
    { key: 'ocr_invoice_prompt' as const, label: 'Rechnungen' },
    { key: 'ocr_gear_prompt' as const, label: 'Gear-Dokumente' },
  ]

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-4">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">OCR-Prompts</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Eigene Anweisungen für die Dokumentenerkennung. Leer lassen = Standard-Prompt.
        </p>
      </div>
      {fields.map(({ key, label }) => (
        <div key={key} className="space-y-2">
          <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{label}</h3>
          <textarea
            value={localPrompts[key] ?? settings[key] ?? ''}
            placeholder="Standard-Prompt verwenden (leer lassen)"
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
              Auf Standard zurücksetzen
            </button>
          )}
        </div>
      ))}
    </section>
  )
}

export default function LLMSettingsPage() {
  const qc = useQueryClient()
  const [separateRolesOverride, setSeparateRolesOverride] = useState<boolean | null>(null)
  const { data: configs = [], isLoading } = useQuery({
    queryKey: ['llm-config'],
    queryFn: fetchLLMConfig,
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade...</div>
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
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">KI & Modelle</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Direkte OpenAI-kompatible LLM-Backends konfigurieren. LiteLLM bleibt optional als internes Gateway-Artefakt.
        </p>
      </div>

      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5">
        <div className="flex items-start justify-between gap-4">
          <div>
            <h3 className="font-medium text-neutral-900 dark:text-neutral-100">Ein Backend für alle Rollen</h3>
            <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
              Standardmodus: Chat, Dokumente, Markdown und Vision verwenden dieselbe Verbindung. Separate Rollen nur aktivieren, wenn Modelle gezielt getrennt werden sollen.
            </p>
          </div>
          <Toggle
            checked={!separateRoles}
            onChange={checked => setSeparateRolesOverride(!checked)}
            aria-label="Ein Backend für alle Rollen verwenden"
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
    if (backend === 'disabled') return 'Aktive Rollen brauchen ein Backend.'
    if (needsURL && !apiBase.trim()) return 'Aktive Rollen brauchen eine API-Basis-URL.'
    if (!modelName.trim()) return 'Aktive Rollen brauchen ein Modell.'
    const contextWindowError = validateContextWindowInput(contextWindow)
    if (contextWindowError) return contextWindowError
    if (needsAPIKey && apiKeyState !== 'set') {
      const roleWithoutKey = activeRoleNames.find(roleName => !roleConfigsByName.get(roleName)?.has_api_key)
      if (roleWithoutKey) return 'Für den gemeinsamen OpenAI-Modus den API-Schlüssel neu eingeben, damit alle aktiven Rollen ihn erhalten.'
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
      setLocalSaveError(mutationErrorMessage(err))
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
    const contextWindowError = validateContextWindowInput(contextWindow)
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
      setTestResult({ success: false, message: mutationErrorMessage(err) })
    }
    setIsTesting(false)
  }

  const runEval = async () => {
    if (isDirty) {
      setEvalResult('Qualitätsprüfung erst nach dem Speichern starten')
      return
    }
    setIsEvaluating(true)
    setEvalResult(null)
    try {
      const results = await Promise.all(activeRoleNames.map(async roleName => ({ roleName, result: await runLLMEval(roleName) })))
      const failed = results.filter(item => item.result.status !== 'passed')
      setEvalResult(failed.length === 0
        ? 'Qualitätsprüfung: alle aktiven Rollen bestanden'
        : `Qualitätsprüfung: ${failed.map(item => `${ROLE_LABELS[item.roleName] ?? item.roleName} ${evalStatusLabel(item.result.status)}`).join(', ')}`)
      onSave()
    } catch (err) {
      setEvalResult(`Qualitätsprüfung fehlgeschlagen: ${mutationErrorMessage(err)}`)
    }
    setIsEvaluating(false)
  }

  const saveWaiver = async (roleName: string) => {
    if (isDirty) {
      setLocalSaveError('Erst speichern, dann Verbindung freigeben.')
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
      setLocalSaveError(mutationErrorMessage(err))
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

  const saveError = localSaveError ?? (saveMutation.isError ? mutationErrorMessage(saveMutation.error) : null)
  const evalDisabledReason = isDirty ? 'Erst speichern, dann Qualitätsprüfung starten.' : null
  const waiverDisabledReason = isDirty ? 'Erst speichern, dann Verbindung freigeben.' : null

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5 space-y-4">
      <div>
        <h3 className="font-medium text-neutral-900 dark:text-neutral-100">Gemeinsame Verbindung</h3>
        <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
          Diese Backend-Daten werden für alle unten aktivierten Rollen gespeichert.
        </p>
      </div>

      <div className="grid gap-3 md:grid-cols-2">
        {roles.map(role => (
          <div key={role.role} className="rounded-lg border border-neutral-100 dark:border-neutral-800 px-3 py-2 space-y-2">
            <div className="flex items-start justify-between gap-3">
              <div>
                <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100">{ROLE_LABELS[role.role] ?? role.role}</div>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">{ROLE_DESCRIPTIONS[role.role] ?? 'Benutzerdefinierte Rolle.'}</p>
                <span className={`mt-1 inline-block rounded px-2 py-0.5 text-xs ${
                  role.eval_status === 'passed'
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
                    : role.eval_status === 'waived'
                      ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
                      : role.eval_status === 'failed'
                        ? 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
                        : 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300'
                }`}>
                  Status: {evalStatusLabel(role.eval_status)}
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
                aria-label={`${ROLE_LABELS[role.role] ?? role.role} aktivieren`}
              />
            </div>
            {enabledByRole[role.role] && role.eval_status !== 'passed' && role.eval_status !== 'waived' && (
              <div className="flex gap-2">
                <input
                  type="text"
                  value={waiverReasons[role.role] ?? ''}
                  onChange={e => setWaiverReasons(current => ({ ...current, [role.role]: e.target.value }))}
                  placeholder="Begründung für Freigabe"
                  className="min-w-0 flex-1 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-2 py-1 text-xs text-neutral-900 dark:text-neutral-100 focus:outline-none"
                  aria-label={`${ROLE_LABELS[role.role] ?? role.role} Freigabe-Begründung`}
                />
                <button
                  type="button"
                  onClick={() => saveWaiver(role.role)}
                  disabled={!!waiverDisabledReason || waivingRole === role.role || !(waiverReasons[role.role] ?? '').trim()}
                  title={waiverDisabledReason ?? undefined}
                  className="cursor-pointer px-2 py-1 rounded-lg text-xs border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                >
                  {waivingRole === role.role ? 'Speichere...' : 'Freigeben'}
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
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">Backend</label>
            <select
              value={backend}
              onChange={e => handleBackendChange(e.target.value)}
              className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none"
            >
              {BACKEND_OPTIONS.filter(o => o.value !== 'disabled').map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {isOllama ? 'Ollama URL' : 'API-Basis-URL'}
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
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">Modell</label>
              {isOllama && ollamaModels.length > 0 ? (
                <select
                  value={modelName}
                  onChange={e => setModelName(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm text-neutral-900 dark:text-neutral-100 focus:outline-none"
                >
                  <option value="">-- Modell wählen --</option>
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
                      {isLoadingModels ? 'Lade...' : 'Modelle laden'}
                    </button>
                  )}
                </div>
              )}
            </div>
          )}

          {needsURL && (
            <div>
              <label htmlFor={contextWindowInputID} className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">Kontextfenster</label>
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
                placeholder="Standard aus Server-Umgebung"
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                Optional. Muss zum tatsächlich gestarteten LLM-Backend passen; leer verwendet den Server-Fallback.
              </p>
            </div>
          )}

          {canShowKeyControls && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                API-Schlüssel {roles.some(role => role.has_api_key) && apiKeyState === 'unchanged' && <span className="text-green-500">(gesetzt)</span>}
                {apiKeyState === 'clear' && <span className="text-amber-500">(wird gelöscht)</span>}
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
                  placeholder={roles.some(role => role.has_api_key) ? 'Neuen Schlüssel für aktive Rollen setzen' : 'sk-...'}
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
                    {apiKeyState === 'clear' ? 'Behalten' : 'Löschen'}
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
              {isTesting ? 'Teste...' : 'Verbindung testen'}
            </button>
            <button
              onClick={runEval}
              disabled={isEvaluating || !needsURL || !modelName || isDirty || activeRoleNames.length === 0}
              title={evalDisabledReason ?? undefined}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isEvaluating ? 'Prüfung läuft...' : 'Qualitätsprüfung starten'}
            </button>
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : 'Fehler'} {testResult.message}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
            {evalResult && (
              <span className={`text-xs ${evalResult.includes('bestanden') ? 'text-green-500' : 'text-rose-500'}`}>
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
          {saveMutation.isPending ? 'Speichere...' : 'Speichern'}
        </button>
      </div>
      {saveError && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          Speichern fehlgeschlagen: {saveError}
        </p>
      )}
      {saveWarning && (
        <p role="status" className="text-sm text-amber-600 dark:text-amber-400">
          Gespeichert, aber LiteLLM-Konfiguration konnte nicht aktualisiert werden: {saveWarning}
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
      setLocalSaveError(mutationErrorMessage(err))
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
    const contextWindowError = validateContextWindowInput(contextWindow)
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
      setTestResult({ success: false, message: mutationErrorMessage(err) })
    }
    setIsTesting(false)
  }

  const runEval = async () => {
    if (isDirty) {
      setEvalResult('Qualitätsprüfung erst nach dem Speichern starten')
      return
    }
    setIsEvaluating(true)
    setEvalResult(null)
    try {
      const results = await Promise.all(rolesToSave.map(async roleName => ({ roleName, result: await runLLMEval(roleName) })))
      const failed = results.filter(item => item.result.status !== 'passed')
      if (failed.length === 0) {
        setEvalResult(isSharedCard ? 'Qualitätsprüfung: alle bestanden' : 'Qualitätsprüfung bestanden')
      } else {
        setEvalResult(`Qualitätsprüfung: ${failed.map(item => `${ROLE_LABELS[item.roleName] ?? item.roleName} ${evalStatusLabel(item.result.status)}`).join(', ')}`)
      }
      onSave()
    } catch (err) {
      setEvalResult(`Qualitätsprüfung fehlgeschlagen: ${mutationErrorMessage(err)}`)
    }
    setIsEvaluating(false)
  }

  const saveWaiver = async () => {
    if (isDirty) {
      setLocalSaveError('Erst speichern, dann Verbindung freigeben.')
      return
    }
    const reason = waiverReason.trim()
    if (!reason) return
    setIsWaiving(true)
    setEvalResult(null)
    try {
      await waiveLLMEval(role, reason)
      setWaiverReason('')
      setEvalResult('Verbindung freigegeben')
      onSave()
    } catch (err) {
      setLocalSaveError(mutationErrorMessage(err))
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
  const saveError = localSaveError ?? (saveMutation.isError ? mutationErrorMessage(saveMutation.error) : null)
  const evalDisabledReason = isDirty ? 'Erst speichern, dann Qualitätsprüfung starten.' : null
  const waiverDisabledReason = isDirty ? 'Erst speichern, dann Verbindung freigeben.' : null

  const validateBeforeSave = (): string | null => {
    if (!enabled) return null
    if (backend === 'disabled') return 'Aktive Rollen brauchen ein Backend.'
    if (needsURL && !apiBase.trim()) return 'Aktive Rollen brauchen eine API-Basis-URL.'
    if (!modelName.trim()) return 'Aktive Rollen brauchen ein Modell.'
    const contextWindowError = validateContextWindowInput(contextWindow)
    if (contextWindowError) return contextWindowError
    if (needsAPIKey && !config?.has_api_key && apiKeyState !== 'set') return 'Aktive OpenAI-Rollen brauchen einen API-Schlüssel.'
    if (isSharedCard && needsAPIKey && apiKeyState !== 'set') {
      const roleWithoutKey = rolesToSave.find(roleName => !roleConfigsByName.get(roleName)?.has_api_key)
      if (roleWithoutKey) return 'Für den gemeinsamen OpenAI-Modus den API-Schlüssel neu eingeben, damit alle Rollen ihn erhalten.'
    }
    if (providerBaseChanged && apiKeyState === 'unchanged') return 'Backend oder URL wurde geändert. Schlüssel neu setzen oder löschen.'
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
            {roleLabel ?? ROLE_LABELS[role] ?? role}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {roleDescription ?? ROLE_DESCRIPTIONS[role] ?? 'Benutzerdefinierte Rolle.'}
          </p>
          {isSharedCard ? (
            <div className="flex flex-wrap items-center gap-2 mt-2 text-xs">
              {rolesToSave.map(roleName => {
                const roleConfig = roleConfigsByName.get(roleName)
                return (
                  <span key={roleName} className="rounded bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300 px-2 py-0.5">
                    {ROLE_LABELS[roleName] ?? roleName}: {evalStatusLabel(roleConfig?.eval_status)}
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
                Status: {evalStatusLabel(config.eval_status)}
              </span>
              {config.eval_status === 'waived' && (
                <span className="text-amber-600 dark:text-amber-400">Admin-Freigabe aktiv</span>
              )}
            </div>
          )}
        </div>
        <Toggle
          checked={enabled}
          onChange={handleEnabledChange}
          aria-label={`${roleLabel ?? ROLE_LABELS[role] ?? role} aktivieren`}
        />
      </div>

      {enabled && (
        <div className="space-y-3">
          {/* Backend-Auswahl */}
          <div>
            <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
              Backend
            </label>
            <select
              value={backend}
              onChange={e => handleBackendChange(e.target.value)}
              className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-3 py-2 text-sm
                         text-neutral-900 dark:text-neutral-100 focus:outline-none"
            >
              {BACKEND_OPTIONS.filter(o => o.value !== 'disabled').map(o => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
          </div>

          {/* API-Base URL */}
          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                {isOllama ? 'Ollama URL' : 'API-Basis-URL'}
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

          {/* Modell-Auswahl */}
          {needsURL && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                Modell
              </label>
              {isOllama && ollamaModels.length > 0 ? (
                <select
                  value={modelName}
                  onChange={e => setModelName(e.target.value)}
                  className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                             bg-white dark:bg-neutral-800 px-3 py-2 text-sm
                             text-neutral-900 dark:text-neutral-100 focus:outline-none"
                >
                  <option value="">-- Modell wählen --</option>
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
                      {isLoadingModels ? 'Lade...' : 'Modelle laden'}
                    </button>
                  )}
                </div>
              )}
            </div>
          )}

          {/* Kontextfenster */}
          {needsURL && (
            <div>
              <label htmlFor={contextWindowInputID} className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                Kontextfenster
              </label>
              <input
                id={contextWindowInputID}
                type="number"
                min={CONTEXT_WINDOW_MIN}
                max={CONTEXT_WINDOW_MAX}
                step={1024}
                value={contextWindow}
                onChange={e => handleContextWindowChange(e.target.value)}
                placeholder="Standard aus Server-Umgebung"
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                           bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                           text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
              <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
                Optional. Muss zum tatsächlich gestarteten LLM-Backend passen; leer verwendet den Server-Fallback.
              </p>
            </div>
          )}

          {/* API Key (nur Cloud) */}
          {canShowKeyControls && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                API-Schlüssel {config?.has_api_key && apiKeyState === 'unchanged' && <span className="text-green-500">(gesetzt)</span>}
                {apiKeyState === 'clear' && <span className="text-amber-500">(wird gelöscht)</span>}
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
                  placeholder={config?.has_api_key ? 'Neuen Schlüssel setzen' : 'sk-...'}
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
                    {apiKeyState === 'clear' ? 'Behalten' : 'Löschen'}
                  </button>
                )}
              </div>
              {providerBaseChanged && apiKeyState === 'unchanged' && (
                <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
                  Backend oder URL wurde geändert. Schlüssel neu setzen oder löschen.
                </p>
              )}
            </div>
          )}

          {/* Verbindung testen */}
          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={runTest}
              disabled={isTesting || !needsURL}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isTesting ? 'Teste...' : 'Verbindung testen'}
            </button>
            <button
              onClick={runEval}
              disabled={isEvaluating || !needsURL || !modelName || isDirty}
              title={evalDisabledReason ?? undefined}
              className="cursor-pointer px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                         disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isEvaluating ? 'Prüfung läuft...' : 'Qualitätsprüfung starten'}
            </button>
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : 'Fehler'} {testResult.message}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
            {testResult?.warning && (
              <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">⚠ {testResult.warning}</p>
            )}
            {evalResult && (
              <span className={`text-xs ${evalResult.includes('bestanden') ? 'text-green-500' : 'text-rose-500'}`}>
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
                Freigabe-Begründung
              </label>
              <div className="flex gap-2">
                <input
                  id={`waiver-${role}`}
                  type="text"
                  value={waiverReason}
                  onChange={e => setWaiverReason(e.target.value)}
                  placeholder="Warum diese Verbindung freigegeben wird"
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
                  {isWaiving ? 'Speichere...' : 'Freigeben'}
                </button>
              </div>
              {waiverDisabledReason && (
                <p className="text-xs text-amber-600 dark:text-amber-400">{waiverDisabledReason}</p>
              )}
            </div>
          )}
        </div>
      )}

      {/* Speichern – immer sichtbar, auch wenn deaktiviert */}
      <div className="flex justify-end pt-1">
        <button
          onClick={handleSave}
          disabled={saveMutation.isPending}
          className="cursor-pointer px-4 py-1.5 text-sm rounded-lg text-white bg-accent-DEFAULT disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {saveMutation.isPending ? 'Speichere...' : 'Speichern'}
        </button>
      </div>
      {saveError && (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          Speichern fehlgeschlagen: {saveError}
        </p>
      )}
      {saveWarning && (
        <p role="status" className="text-sm text-amber-600 dark:text-amber-400">
          Gespeichert, aber LiteLLM-Konfiguration konnte nicht aktualisiert werden: {saveWarning}
        </p>
      )}
    </div>
  )
}
