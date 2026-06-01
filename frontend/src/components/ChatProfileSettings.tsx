import { useId, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
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

function validateChatProfileSetting(setting: ChatUserSetting, t: (key: string, options?: Record<string, unknown>) => string): string | null {
  const value = setting.overrides.max_context_tokens
  if (value === undefined) return null
  if (!Number.isInteger(value) || value < CONTEXT_WINDOW_MIN || value > CONTEXT_WINDOW_MAX) {
    return t('components.chatProfile.validation.contextTokens', { min: CONTEXT_WINDOW_MIN, max: CONTEXT_WINDOW_MAX })
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
  const { t } = useTranslation()
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
      addToast({ type: 'success', title: t('components.chatProfile.toasts.saved') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('components.chatProfile.toasts.saveFailed'), duration: 0 })
    },
  })

  const isDirty = local !== null

  const handleSave = () => {
    const validationError = validateChatProfileSetting(setting, t)
    if (validationError) {
      addToast({ type: 'error', title: validationError, duration: 0 })
      return
    }
    saveMutation.mutate(setting)
  }

  if (defaultsLoading || settingsLoading) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('components.chatProfile.title')}</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">{t('components.chatProfile.loading')}</p>
      </section>
    )
  }

  if (settingsError) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('components.chatProfile.title')}</h2>
        <p className="text-sm text-red-600 dark:text-red-400 mt-1">{t('components.chatProfile.loadError')}</p>
      </section>
    )
  }

  if (defaultsError || !profileDefaults || !profile) {
    return (
      <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6">
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('components.chatProfile.title')}</h2>
        <p className="text-sm text-red-600 dark:text-red-400 mt-1">{t('components.chatProfile.defaultsError')}</p>
      </section>
    )
  }

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-5">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">{t('components.chatProfile.title')}</h2>
          <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
            {t('components.chatProfile.description')}
          </p>
        </div>
        <div className="flex items-center gap-3 flex-shrink-0">
          {isDirty && (
            <button
              type="button"
              onClick={() => setLocal(null)}
              className="text-sm text-neutral-500 dark:text-neutral-400 hover:text-neutral-700 dark:hover:text-neutral-200 underline cursor-pointer transition-colors"
            >
              {t('common.cancel')}
            </button>
          )}
          <button
            type="button"
              onClick={handleSave}
            disabled={!isDirty || saveMutation.isPending}
            className="px-4 py-1.5 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {saveMutation.isPending ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>

      {/* Grundeinstellungen */}
      <div className="space-y-3">
        <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          {t('components.chatProfile.sections.basic')}
        </p>

        <ToggleRow
          label={t('components.chatProfile.enabled.label')}
          description={t('components.chatProfile.enabled.description')}
          value={setting.enabled}
          onChange={v => patchSetting({ enabled: v })}
          disabled={false}
        />

        <div className="flex items-center justify-between gap-4">
          <label className="text-sm text-neutral-700 dark:text-neutral-300 flex-1">
            {t('components.chatProfile.profile.label')}
          </label>
          <div className="flex items-center gap-2">
            <select
              value={setting.base_profile}
              onChange={e => handlePresetChange(e.target.value as ProfilePreset)}
              className="text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1"
            >
              <option value="tiny">{t('components.chatProfile.profile.options.tiny')}</option>
              <option value="small">{t('components.chatProfile.profile.options.small')}</option>
              <option value="medium">{t('components.chatProfile.profile.options.medium')}</option>
              <option value="large">{t('components.chatProfile.profile.options.large')}</option>
              <option value="custom">{t('components.chatProfile.profile.options.custom')}</option>
            </select>
            {!isCustom && setting.base_profile in profileDefaults.presets && (
              <button
                type="button"
                onClick={handleResetToPreset}
                className="cursor-pointer text-xs text-neutral-400 dark:text-neutral-500 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
                title={t('components.chatProfile.profile.resetTitle')}
              >
                ↺
              </button>
            )}
          </div>
        </div>

        {!isCustom && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 italic">
            {t('components.chatProfile.profile.hint')}
          </p>
        )}
      </div>

      {/* Tools & Routing */}
      <CollapsibleSection title={t('components.chatProfile.sections.tools')}>
        <ToggleRow
          label={t('components.chatProfile.semanticRouting.label')}
          description={t('components.chatProfile.semanticRouting.description')}
          value={profile.use_router}
          onChange={v => patchProfile({ use_router: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label={t('components.chatProfile.maxToolsPerTurn')}
          value={profile.max_tools_per_turn}
          min={1}
          max={5}
          onChange={v => patchProfile({ max_tools_per_turn: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label={t('components.chatProfile.toolResultLimit')}
          value={profile.tool_result_limit}
          min={3}
          max={50}
          onChange={v => patchProfile({ tool_result_limit: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      {/* Schema & SQL */}
      <CollapsibleSection title={t('components.chatProfile.sections.schema')}>
        <ToggleRow
          label={t('components.chatProfile.sqlAgent.label')}
          description={t('components.chatProfile.sqlAgent.description')}
          value={profile.enable_sql_agent}
          onChange={v => patchProfile({ enable_sql_agent: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      {/* Prompt & Kontext */}
      <CollapsibleSection title={t('components.chatProfile.sections.prompt')}>
        <SliderRow
          label={t('components.chatProfile.fewShotCount')}
          value={profile.few_shot_count}
          min={0}
          max={5}
          onChange={v => patchProfile({ few_shot_count: v })}
          disabled={!isCustom}
        />
        <SelectRow
          label={t('components.chatProfile.systemPromptMode.label')}
          value={profile.system_prompt_mode}
          options={[
            { value: 'compact', label: t('components.chatProfile.systemPromptMode.options.compact') },
            { value: 'standard', label: t('components.chatProfile.systemPromptMode.options.standard') },
            { value: 'rich', label: t('components.chatProfile.systemPromptMode.options.rich') },
          ]}
          onChange={v => patchProfile({ system_prompt_mode: v })}
          disabled={!isCustom}
        />
        <NumberRow
	          label={t('components.chatProfile.maxContextTokens')}
	          value={profile.max_context_tokens}
	          min={CONTEXT_WINDOW_MIN}
	          max={CONTEXT_WINDOW_MAX}
          step={1024}
          onChange={v => patchProfile({ max_context_tokens: v })}
          disabled={!isCustom}
        />
        <SliderRow
          label={t('components.chatProfile.historyMessages')}
          value={profile.history_messages}
          min={5}
          max={30}
          onChange={v => patchProfile({ history_messages: v })}
          disabled={!isCustom}
        />
      </CollapsibleSection>

      <CollapsibleSection title={t('components.chatProfile.sections.response')}>
        <SliderRow
          label={t('components.chatProfile.maxResponseTokens')}
          value={profile.max_response_tokens}
          min={500}
          max={16000}
          step={500}
          onChange={v => patchProfile({ max_response_tokens: v })}
          disabled={!isCustom}
        />
        <p className="text-xs text-neutral-500 dark:text-neutral-400">
          {t('components.chatProfile.responseHint')}
        </p>
      </CollapsibleSection>

      {isCustom && (
        <p className="text-xs text-neutral-400 dark:text-neutral-500 pt-1 border-t border-neutral-100 dark:border-neutral-800/60">
          {t('components.chatProfile.customHint')}
        </p>
      )}
    </section>
  )
}
