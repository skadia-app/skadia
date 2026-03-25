import { useState } from 'react'
import Toggle from '@/components/Toggle'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchLLMConfig, saveLLMRole, fetchOllamaModels, testLLMConnection,
  type LLMSystemConfig, type LLMTestResult,
} from '@/api/llmSettings'
import { fetchEnrichmentSettings, updateEnrichmentSettings } from '@/api/enrichment'
import type { EnrichmentSettings } from '@/api/types'

const ROLE_LABELS: Record<string, string> = {
  default: 'Standard-LLM',
}

const ROLE_DESCRIPTIONS: Record<string, string> = {
  default: 'Fur Text-Analyse, NL-Abfragen, Foto-Analyse und automatische Aktivitats-Benennung (skadia-default).',
}

const BACKEND_OPTIONS = [
  { value: 'disabled', label: 'Deaktiviert' },
  { value: 'ollama', label: 'Ollama (Container)' },
  { value: 'llama_server', label: 'llama-server / OpenAI-kompatibel' },
  { value: 'openai', label: 'OpenAI API' },
  { value: 'anthropic', label: 'Anthropic API' },
]

const DEFAULT_URLS: Record<string, string> = {
  ollama: 'http://ollama:11434',
  llama_server: 'http://host.containers.internal:8181/v1',
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com',
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
  const { data: configs = [], isLoading } = useQuery({
    queryKey: ['llm-config'],
    queryFn: fetchLLMConfig,
  })

  if (isLoading) {
    return <div className="flex items-center justify-center h-64 text-neutral-400">Lade...</div>
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">KI & Modelle</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          LLM-Backends konfigurieren. Änderungen werden nach dem nächsten Container-Neustart von LiteLLM übernommen.
        </p>
      </div>

      <div className="space-y-4">
        {['default'].map(role => {
          const config = configs.find(c => c.role === role)
          return (
            <LLMRoleCard
              key={role}
              role={role}
              config={config}
              onSave={() => qc.invalidateQueries({ queryKey: ['llm-config'] })}
            />
          )
        })}
      </div>
      <OcrPromptsSection />
    </div>
  )
}

function LLMRoleCard({ role, config, onSave }: {
  role: string
  config?: LLMSystemConfig
  onSave: () => void
}) {
  const [backend, setBackend] = useState(config?.backend_type || 'disabled')
  const [apiBase, setApiBase] = useState(config?.api_base || '')
  const [modelName, setModelName] = useState(config?.model_name || '')
  const [apiKey, setApiKey] = useState('')
  const [enabled, setEnabled] = useState(config?.enabled || false)
  const [testResult, setTestResult] = useState<LLMTestResult | null>(null)
  const [ollamaModels, setOllamaModels] = useState<string[]>([])
  const [isLoadingModels, setIsLoadingModels] = useState(false)
  const [isTesting, setIsTesting] = useState(false)

  const saveMutation = useMutation({
    mutationFn: () => saveLLMRole(role, { backend_type: backend, api_base: apiBase || undefined,
      model_name: modelName || undefined, api_key: apiKey || undefined, enabled }),
    onSuccess: onSave,
  })

  const handleBackendChange = (newBackend: string) => {
    setBackend(newBackend as Parameters<typeof setBackend>[0])
    if (DEFAULT_URLS[newBackend] && !apiBase) {
      setApiBase(DEFAULT_URLS[newBackend])
    }
    setOllamaModels([])
    setTestResult(null)
  }

  const loadOllamaModels = async () => {
    setIsLoadingModels(true)
    const models = await fetchOllamaModels(apiBase || DEFAULT_URLS.ollama)
    setOllamaModels(models.map(m => m.name))
    setIsLoadingModels(false)
  }

  const runTest = async () => {
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
    } catch {
      setTestResult({ success: false, message: 'Verbindungsfehler' })
    }
    setIsTesting(false)
  }

  const needsURL = backend !== 'disabled'
  const needsAPIKey = backend === 'openai' || backend === 'anthropic'
  const isOllama = backend === 'ollama'

  return (
    <div className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-5 space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <h3 className="font-medium text-neutral-900 dark:text-neutral-100">
            {ROLE_LABELS[role]}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {ROLE_DESCRIPTIONS[role]}
          </p>
        </div>
        <Toggle
          checked={enabled}
          onChange={() => setEnabled(!enabled)}
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
                onChange={e => setApiBase(e.target.value)}
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
                  <option value="">-- Modell waehlen --</option>
                  {ollamaModels.map(m => <option key={m} value={m}>{m}</option>)}
                </select>
              ) : (
                <div className="flex gap-2 mt-1">
                  <input
                    type="text"
                    value={modelName}
                    onChange={e => setModelName(e.target.value)}
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

          {/* API Key (nur Cloud) */}
          {needsAPIKey && (
            <div>
              <label className="text-xs font-medium text-neutral-600 dark:text-neutral-400 uppercase tracking-wide">
                API-Schluessel {config?.has_api_key && <span className="text-green-500">(gesetzt)</span>}
              </label>
              <input
                type="password"
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                placeholder={config?.has_api_key ? '************' : 'sk-...'}
                className="mt-1 w-full rounded-lg border border-neutral-200 dark:border-neutral-700
                           bg-white dark:bg-neutral-800 px-3 py-2 text-sm font-mono
                           text-neutral-900 dark:text-neutral-100 focus:outline-none"
              />
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
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : 'Fehler'} {testResult.message}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
            {testResult?.warning && (
              <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">⚠ {testResult.warning}</p>
            )}
          </div>
        </div>
      )}

      {/* Speichern – immer sichtbar, auch wenn deaktiviert */}
      <div className="flex justify-end pt-1">
        <button
          onClick={() => saveMutation.mutate()}
          disabled={saveMutation.isPending}
          className="cursor-pointer px-4 py-1.5 text-sm rounded-lg text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
        >
          {saveMutation.isPending ? 'Speichere...' : 'Speichern'}
        </button>
      </div>
    </div>
  )
}
