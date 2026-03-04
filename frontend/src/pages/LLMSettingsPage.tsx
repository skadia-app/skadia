import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchLLMConfig, saveLLMRole, fetchOllamaModels, testLLMConnection,
  type LLMSystemConfig, type LLMTestResult,
} from '@/api/llmSettings'

const ROLE_LABELS: Record<string, string> = {
  default: 'Standard-LLM',
  vlm: 'VLM (Bild-Analyse)',
  vlm_fallback: 'VLM-Fallback',
  analysis: 'Erweitertes Analyse-Modell',
}

const ROLE_DESCRIPTIONS: Record<string, string> = {
  default: 'Fur Text-Analyse, NL-Abfragen und automatische Aktivitats-Benennung (skadia-default)',
  vlm: 'Fur Rechnungs-OCR und Foto-Analyse (skadia-vlm)',
  vlm_fallback: 'Fallback bei VRAM-Knappheit des primaren VLM (skadia-vlm-fallback)',
  analysis: 'Optionales grosses Modell fur tiefe Analyse (>=24 GB VRAM empfohlen)',
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
  llama_server: 'http://host.containers.internal:8080/v1',
  openai: 'https://api.openai.com/v1',
  anthropic: 'https://api.anthropic.com',
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
          LLM-Backends konfigurieren. Aenderungen werden nach dem naechsten Container-Neustart von LiteLLM uebernommen.
        </p>
      </div>

      <div className="space-y-4">
        {['default', 'vlm', 'vlm_fallback', 'analysis'].map(role => {
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
    setBackend(newBackend)
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
        <button
          onClick={() => setEnabled(!enabled)}
          className="relative inline-flex h-5 w-9 items-center rounded-full transition-colors flex-shrink-0"
          style={{ backgroundColor: enabled ? 'var(--color-accent-DEFAULT)' : '#d4d4d4' }}
          role="switch"
          aria-checked={enabled}
        >
          <span className={`inline-block h-3 w-3 transform rounded-full bg-white transition-transform ${enabled ? 'translate-x-5' : 'translate-x-1'}`} />
        </button>
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
                      className="px-3 py-2 rounded-lg text-sm border border-neutral-200 dark:border-neutral-700
                                 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                                 disabled:opacity-50 transition-colors whitespace-nowrap"
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

          {/* Test + Speichern */}
          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={runTest}
              disabled={isTesting || !needsURL}
              className="px-3 py-1.5 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700
                         text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800
                         disabled:opacity-50 transition-colors"
            >
              {isTesting ? 'Teste...' : 'Verbindung testen'}
            </button>
            <button
              onClick={() => saveMutation.mutate()}
              disabled={saveMutation.isPending}
              className="px-4 py-1.5 text-sm rounded-lg text-white disabled:opacity-50 transition-colors"
              style={{ backgroundColor: 'var(--color-accent-DEFAULT)' }}
            >
              {saveMutation.isPending ? 'Speichere...' : 'Speichern'}
            </button>
            {testResult && (
              <span className={`text-xs ${testResult.success ? 'text-green-500' : 'text-rose-500'}`}>
                {testResult.success ? 'OK' : 'Fehler'} {testResult.message}
                {testResult.latency_ms ? ` (${testResult.latency_ms}ms)` : ''}
              </span>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
