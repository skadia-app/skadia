import { useId, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/auth'
import { useToast } from '@/lib/toast'
import Toggle from '@/components/Toggle'

// -----------------------------------------------------------------------
// Types
// -----------------------------------------------------------------------

interface ChatProfile {
  // Tools & Routing
  max_tools: number
  use_router: boolean
  max_tools_per_turn: number
  tool_result_limit: number

  // Schema & SQL
  schema_mode: 'filtered' | 'full'
  enable_sql_agent: boolean
  sql_complexity: 'simple' | 'moderate' | 'advanced'
  max_sql_retries: number

  // Prompt & Context
  few_shot_count: number
  system_prompt_mode: 'compact' | 'standard' | 'rich'
  max_context_tokens: number
  history_messages: number

  temperature: number
  top_k: number
  presence_penalty: number
  max_response_tokens: number
}

type ProfilePreset = 'tiny' | 'small' | 'medium' | 'large' | 'custom'

type BackendProfilePreset = Exclude<ProfilePreset, 'custom'>

interface ChatUserSetting {
  enabled: boolean
  model: string
  base_profile: ProfilePreset
  overrides: Partial<ChatProfile>
}

interface ChatProfileDefaults {
  default_profile: BackendProfilePreset
  presets: Record<BackendProfilePreset, ChatProfile>
}

const DEFAULT_SETTING: ChatUserSetting = {
  enabled: true,
  model: '',
  base_profile: 'small',
  overrides: {},
}

const CONTEXT_WINDOW_MIN = 1024
const CONTEXT_WINDOW_MAX = 2000000

function resolveProfile(setting: ChatUserSetting, presets: ChatProfileDefaults['presets']): ChatProfile {
  const base = setting.base_profile === 'custom'
    ? presets.small
    : presets[setting.base_profile]
  return { ...base, ...setting.overrides }
}

function validateChatProfileSetting(setting: ChatUserSetting): string | null {
  const value = setting.overrides.max_context_tokens
  if (value === undefined) return null
  if (!Number.isInteger(value) || value < CONTEXT_WINDOW_MIN || value > CONTEXT_WINDOW_MAX) {
    return `Max. Kontext-Tokens muss zwischen ${CONTEXT_WINDOW_MIN} und ${CONTEXT_WINDOW_MAX} liegen.`
  }
  return null
}

async function fetchJSON<T>(path: string): Promise<T> {
  const res = await apiFetch(path)
  if (!res.ok) {
    throw new Error(`request failed with status ${res.status}`)
  }
  return res.json() as Promise<T>
}

async function saveChatProfile(value: ChatUserSetting): Promise<void> {
  const res = await apiFetch('/api/v1/settings/ml/user/chat_profile', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(value),
  })
  if (!res.ok) {
    throw new Error(`request failed with status ${res.status}`)
  }
}

// -----------------------------------------------------------------------
// Sub-components
// -----------------------------------------------------------------------

function SliderRow({ label, value, min, max, step = 1, format, onChange, disabled }: {
  label: string
  value: number
  min: number
  max: number
  step?: number
  format?: (v: number) => string
  onChange: (v: number) => void
  disabled: boolean
}) {
  const display = format ? format(value) : String(value)
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <label className="text-sm text-neutral-700 dark:text-neutral-300">{label}</label>
        <span className="text-sm font-mono text-neutral-500 dark:text-neutral-400 w-12 text-right">{display}</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        onChange={e => onChange(Number(e.target.value))}
        className="w-full h-1.5 rounded-full appearance-none cursor-pointer disabled:cursor-not-allowed disabled:opacity-40 [--track-color:#d4d4d4] dark:[--track-color:#525252]"
        style={{
          accentColor: 'var(--color-accent-DEFAULT, #f97316)',
          background: disabled
            ? 'var(--track-color)'
            : `linear-gradient(to right, var(--color-accent-DEFAULT, #f97316) 0%, var(--color-accent-DEFAULT, #f97316) ${((value - min) / (max - min)) * 100}%, var(--track-color) ${((value - min) / (max - min)) * 100}%, var(--track-color) 100%)`,
        }}
      />
    </div>
  )
}

function SelectRow<T extends string>({ label, value, options, onChange, disabled }: {
  label: string
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
  disabled: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <label className="text-sm text-neutral-700 dark:text-neutral-300 flex-1">{label}</label>
      <select
        value={value}
        disabled={disabled}
        onChange={e => onChange(e.target.value as T)}
        className="text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1 disabled:opacity-40 disabled:cursor-not-allowed"
      >
        {options.map(o => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </div>
  )
}

function NumberRow({ label, value, min, max, step = 1, onChange, disabled }: {
  label: string
  value: number
  min: number
  max: number
  step?: number
  onChange: (v: number) => void
  disabled: boolean
}) {
  const id = useId()
  return (
    <div className="flex items-center justify-between gap-4">
      <label htmlFor={id} className="text-sm text-neutral-700 dark:text-neutral-300 flex-1">{label}</label>
      <input
        id={id}
        type="number"
        min={min}
        max={max}
        step={step}
        value={value}
        disabled={disabled}
        onChange={e => {
          const next = Number(e.target.value)
          if (Number.isFinite(next)) onChange(next)
        }}
        className="w-36 text-sm font-mono rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1 disabled:opacity-40 disabled:cursor-not-allowed"
      />
    </div>
  )
}

function ToggleRow({ label, description, value, onChange, disabled }: {
  label: string
  description?: string
  value: boolean
  onChange: (v: boolean) => void
  disabled: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-0.5">
      <div className="min-w-0 flex-1">
        <span className="text-sm text-neutral-700 dark:text-neutral-300">{label}</span>
        {description && (
          <span className="text-xs text-neutral-500 dark:text-neutral-400 ml-2">— {description}</span>
        )}
      </div>
      <Toggle checked={value} onChange={onChange} disabled={disabled} aria-label={label} />
    </div>
  )
}

interface CollapsibleSectionProps {
  title: string
  children: React.ReactNode
  defaultOpen?: boolean
}

function CollapsibleSection({ title, children, defaultOpen = false }: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <div className="border border-neutral-100 dark:border-neutral-800/60 rounded-lg overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(v => !v)}
        className="w-full flex items-center justify-between px-4 py-3 text-sm font-medium text-neutral-800 dark:text-neutral-200 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 cursor-pointer transition-colors"
      >
        <span>{title}</span>
        <ChevronDown className={`h-4 w-4 text-neutral-400 transition-transform duration-200 ${open ? 'rotate-180' : ''}`} />
      </button>
      {open && (
        <div className="px-4 pb-4 pt-2 space-y-3 border-t border-neutral-100 dark:border-neutral-800/60">
          {children}
        </div>
      )}
    </div>
  )
}

// -----------------------------------------------------------------------
// Main component
// -----------------------------------------------------------------------

export default function ChatProfileSettings() {
  const qc = useQueryClient()
  const { addToast } = useToast()

  const { data: allUserSettings, isLoading: settingsLoading, isError: settingsError } = useQuery({
    queryKey: ['ml-user-settings', 'chat-profile'],
    queryFn: () => fetchJSON<Record<string, unknown>>('/api/v1/settings/ml/user'),
  })

  const { data: profileDefaults, isLoading: defaultsLoading, isError: defaultsError } = useQuery({
    queryKey: ['ml-user-settings', 'chat-profile-defaults'],
    queryFn: () => fetchJSON<ChatProfileDefaults>('/api/v1/settings/ml/chat-profile-defaults'),
  })

  // Extract chat_profile from the flat user settings map
  const fallbackSetting: ChatUserSetting = {
    ...DEFAULT_SETTING,
    base_profile: profileDefaults?.default_profile ?? DEFAULT_SETTING.base_profile,
  }
  const savedSetting: ChatUserSetting = (allUserSettings?.['chat_profile'] as ChatUserSetting) ?? fallbackSetting

  const [local, setLocal] = useState<ChatUserSetting | null>(null)
  const setting = local ?? savedSetting

  const isCustom = setting.base_profile === 'custom'
  const profile = profileDefaults ? resolveProfile(setting, profileDefaults.presets) : null

  function patchSetting(patch: Partial<ChatUserSetting>) {
    setLocal(prev => ({ ...(prev ?? savedSetting), ...patch }))
  }

  function patchProfile(patch: Partial<ChatProfile>) {
    setLocal(prev => {
      const current = prev ?? savedSetting
      return {
        ...current,
        overrides: { ...current.overrides, ...patch },
      }
    })
  }

  function handlePresetChange(preset: ProfilePreset) {
    if (!profile) return
    if (preset === 'custom') {
      // Switch to custom — keep current values as overrides
      patchSetting({ base_profile: 'custom', overrides: profile })
    } else {
      // Switch to preset — clear overrides
      patchSetting({ base_profile: preset, overrides: {} })
    }
  }

  function handleResetToPreset() {
    if (setting.base_profile !== 'custom') {
      patchSetting({ overrides: {} })
    }
  }

  const saveMutation = useMutation({
    mutationFn: saveChatProfile,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ml-user-settings'] })
      setLocal(null)
      addToast({ type: 'success', title: 'AI-Agent-Einstellungen gespeichert' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Speichern', duration: 0 })
    },
  })

  const isDirty = local !== null

  const handleSave = () => {
    const validationError = validateChatProfileSetting(setting)
    if (validationError) {
      addToast({ type: 'error', title: validationError, duration: 0 })
      return
    }
    saveMutation.mutate(setting)
  }

  if (defaultsLoading || settingsLoading) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">AI-Agent</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">Agent-Einstellungen werden geladen…</p>
      </section>
    )
  }

  if (settingsError) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">AI-Agent</h2>
        <p className="text-sm text-red-600 dark:text-red-400 mt-1">Agent-Einstellungen konnten nicht geladen werden.</p>
      </section>
    )
  }

  if (defaultsError || !profileDefaults || !profile) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">AI-Agent</h2>
        <p className="text-sm text-red-600 dark:text-red-400 mt-1">Profil-Defaults konnten nicht geladen werden.</p>
      </section>
    )
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">AI-Agent</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            Verhalten, Tool-Nutzung und Antwortumfang konfigurieren.
          </p>
        </div>
        <div className="flex items-center gap-3 flex-shrink-0">
          {isDirty && (
            <button
              type="button"
              onClick={() => setLocal(null)}
              className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline cursor-pointer transition-colors"
            >
              Abbrechen
            </button>
          )}
          <button
            type="button"
              onClick={handleSave}
            disabled={!isDirty || saveMutation.isPending}
            className="px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {saveMutation.isPending ? 'Speichern…' : 'Speichern'}
          </button>
        </div>
      </div>

      {/* Grundeinstellungen */}
      <div className="space-y-3">
        <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Grundeinstellungen
        </p>

        <ToggleRow
          label="Aktiviert"
          description="Neue Chat-Antworten erlauben"
          value={setting.enabled}
          onChange={v => patchSetting({ enabled: v })}
          disabled={false}
        />

        <div className="flex items-center justify-between gap-4">
          <label className="text-sm text-neutral-700 dark:text-neutral-300 flex-1">
            Agent-Profil
          </label>
          <div className="flex items-center gap-2">
            <select
              value={setting.base_profile}
              onChange={e => handlePresetChange(e.target.value as ProfilePreset)}
              className="text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1"
            >
              <option value="tiny">Kompakt</option>
              <option value="small">Standard</option>
              <option value="medium">Robust</option>
              <option value="large">Maximal / API</option>
              <option value="custom">Benutzerdefiniert</option>
            </select>
            {!isCustom && setting.base_profile in profileDefaults.presets && (
              <button
                type="button"
                onClick={handleResetToPreset}
                className="cursor-pointer text-xs text-neutral-400 dark:text-neutral-500 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
                title="Auf Preset-Standardwerte zurücksetzen"
              >
                ↺
              </button>
            )}
          </div>
        </div>

        {!isCustom && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">
            Presets steuern Agent-Verhalten, nicht die Modellqualität. Für individuelle Anpassung auf „Benutzerdefiniert" wechseln.
          </p>
        )}
      </div>

      {/* Tools & Routing */}
      <CollapsibleSection title="Tools & Routing">
        <ToggleRow
          label="Semantic Routing"
          description="Intent-basierte Tool-Auswahl"
          value={profile.use_router}
          onChange={v => patchProfile({ use_router: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label="Tool-Aufrufe pro Antwort"
          value={profile.max_tools_per_turn}
          min={1}
          max={5}
          onChange={v => patchProfile({ max_tools_per_turn: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label="Tool-Ergebnis-Limit"
          value={profile.tool_result_limit}
          min={3}
          max={50}
          onChange={v => patchProfile({ tool_result_limit: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      {/* Schema & SQL */}
      <CollapsibleSection title="Schema & SQL">
        <ToggleRow
          label="SQL-Agent"
          description="Direkte DB-Abfragen erlauben"
          value={profile.enable_sql_agent}
          onChange={v => patchProfile({ enable_sql_agent: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      {/* Prompt & Kontext */}
      <CollapsibleSection title="Prompt & Kontext">
        <SliderRow
          label="Few-Shot-Beispiele"
          value={profile.few_shot_count}
          min={0}
          max={5}
          onChange={v => patchProfile({ few_shot_count: v })}
          disabled={!isCustom}
        />
        <SelectRow
          label="System-Prompt-Modus"
          value={profile.system_prompt_mode}
          options={[
            { value: 'compact', label: 'Kompakt (wenige Token)' },
            { value: 'standard', label: 'Standard' },
            { value: 'rich', label: 'Ausführlich (mehr Kontext)' },
          ]}
          onChange={v => patchProfile({ system_prompt_mode: v })}
          disabled={!isCustom}
        />
        <NumberRow
	          label="Max. Kontext-Tokens"
	          value={profile.max_context_tokens}
	          min={CONTEXT_WINDOW_MIN}
	          max={CONTEXT_WINDOW_MAX}
          step={1024}
          onChange={v => patchProfile({ max_context_tokens: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label="Chat-History (Nachrichten)"
          value={profile.history_messages}
          min={5}
          max={30}
          onChange={v => patchProfile({ history_messages: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      <CollapsibleSection title="Antwortumfang">
        <SliderRow
          label="Max. Antwort-Tokens"
          value={profile.max_response_tokens}
          min={500}
          max={16000}
          step={500}
          onChange={v => patchProfile({ max_response_tokens: v })}
          disabled={!isCustom}
        />
        <p className="text-xs text-neutral-500 dark:text-neutral-400">
          Höher wählen, wenn der Agent längere Regeln, strukturierte Vorschläge oder ausführliche Tool-Zusammenfassungen erzeugen soll.
        </p>
      </CollapsibleSection>

      {isCustom && (
        <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1 border-t border-neutral-100 dark:border-neutral-800/60">
          Alle Parameter frei konfigurierbar. Änderungen gelten für neue Chat-Sitzungen.
        </p>
      )}
    </section>
  )
}
