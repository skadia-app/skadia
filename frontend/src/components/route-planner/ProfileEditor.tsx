import { useState, useEffect, useCallback, useRef } from 'react'
import {
  FIELD_METADATA,
  SECTION_LABELS,
  getProfileDefaults,
  cacheServerDefaults,
  type ProfileConfig,
  type FieldSection,
} from '@/lib/routingProfiles'
import { PROFILE_ORDER, PROFILE_LABELS } from '@/lib/profileLabels'
import {
  getProfileDefaults as fetchProfileDefaults,
  type ProfileDefaultsResponse,
} from '@/api/routes'
import { routeSportOptions } from '@/lib/sportTaxonomy'
import type { ModifierFilters, SportPredicate } from '@/lib/sportPredicates'

// ─── Highway cost slider definitions ──────────────────────────────────────

type HighwayCostKey = keyof Pick<
  ProfileConfig,
  | 'path_cost'
  | 'track_cost'
  | 'track_grade1_cost'
  | 'track_grade2_cost'
  | 'track_grade3_cost'
  | 'track_grade4_cost'
  | 'track_grade5_cost'
  | 'footway_cost'
  | 'cycleway_cost'
  | 'bridleway_cost'
  | 'pedestrian_cost'
  | 'residential_cost'
  | 'service_cost'
  | 'unclassified_cost'
  | 'tertiary_cost'
  | 'secondary_cost'
  | 'primary_cost'
>

interface HighwayCostDef {
  key: HighwayCostKey
  label: string
  group: 'trails' | 'roads'
  hint?: string
}

const HIGHWAY_COST_DEFS: HighwayCostDef[] = [
  // Trails group
  { key: 'path_cost', label: 'Path', group: 'trails' },
  { key: 'track_cost', label: 'Track (Basis)', group: 'trails' },
  { key: 'track_grade1_cost', label: 'Track Grade 1', group: 'trails' },
  { key: 'track_grade2_cost', label: 'Track Grade 2', group: 'trails' },
  { key: 'track_grade3_cost', label: 'Track Grade 3', group: 'trails' },
  { key: 'track_grade4_cost', label: 'Track Grade 4', group: 'trails' },
  { key: 'track_grade5_cost', label: 'Track Grade 5', group: 'trails' },
  { key: 'footway_cost', label: 'Footway', group: 'trails' },
  { key: 'bridleway_cost', label: 'Bridleway', group: 'trails' },
  { key: 'pedestrian_cost', label: 'Pedestrian', group: 'trails' },
  // Roads group
  { key: 'cycleway_cost', label: 'Cycleway', group: 'roads' },
  { key: 'residential_cost', label: 'Residential', group: 'roads' },
  { key: 'service_cost', label: 'Service', group: 'roads' },
  { key: 'unclassified_cost', label: 'Unclassified', group: 'roads' },
  { key: 'tertiary_cost', label: 'Tertiary', group: 'roads' },
  { key: 'secondary_cost', label: 'Secondary', group: 'roads' },
  { key: 'primary_cost', label: 'Primary', group: 'roads' },
]

const HIGHWAY_GROUP_LABELS: Record<string, string> = {
  trails: 'Trails',
  roads: 'Strassen',
}

const ROUTE_SPORT_OPTIONS = routeSportOptions()
const MODIFIER_FILTER_OPTIONS: { key: keyof ModifierFilters; label: string }[] = [
  { key: 'is_electric', label: 'E-Bike' },
  { key: 'is_indoor', label: 'Indoor' },
  { key: 'is_virtual', label: 'Virtuell' },
  { key: 'is_commute', label: 'Pendeln' },
]

// ─── Sub-components ────────────────────────────────────────────────────────

interface SliderFieldProps {
  label: string
  value: number
  min: number
  max: number
  step: number
  unit?: string
  hint?: string
  onChange: (v: number) => void
  fieldKey: string
  editingField: string | null
  editValue: string
  onStartEdit: (key: string, currentValue: number) => void
  onEditChange: (v: string) => void
  onCommitEdit: (key: string, min: number) => void
  onCancelEdit: () => void
}

function SliderField({ label, value, min, max, step, unit, hint, onChange, fieldKey, editingField, editValue, onStartEdit, onEditChange, onCommitEdit, onCancelEdit }: SliderFieldProps) {
  const display = step < 0.1 ? value.toFixed(2) : step < 1 ? value.toFixed(2) : String(Math.round(value))
  const isEditing = editingField === fieldKey
  const isOverMax = value > max

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-xs text-neutral-700 dark:text-neutral-300">{label}</span>
        {isEditing ? (
          <input
            type="number"
            step="any"
            autoFocus
            value={editValue}
            onChange={e => onEditChange(e.target.value)}
            onFocus={e => e.target.select()}
            onBlur={() => onCommitEdit(fieldKey, min)}
            onKeyDown={e => {
              if (e.key === 'Enter') onCommitEdit(fieldKey, min)
              else if (e.key === 'Escape') onCancelEdit()
            }}
            className="w-16 text-xs font-mono tabular-nums text-right bg-neutral-100 dark:bg-neutral-800 border border-neutral-300 dark:border-neutral-600 rounded px-1 py-0 focus:outline-none focus:border-[var(--color-accent-DEFAULT,#F97316)]"
          />
        ) : (
          <button
            onClick={() => onStartEdit(fieldKey, value)}
            title="Klicken zum Bearbeiten"
            className={`text-xs font-mono tabular-nums cursor-pointer hover:opacity-70 transition-opacity ${
              isOverMax
                ? 'text-orange-500'
                : 'text-neutral-500 dark:text-neutral-400'
            }`}
          >
            {display}{unit ? ` ${unit}` : ''}
          </button>
        )}
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={Math.min(value, max)}
        onChange={e => onChange(parseFloat(e.target.value))}
        className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--color-accent-DEFAULT,#F97316)] bg-neutral-200 dark:bg-neutral-700"
      />
      {hint && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">{hint}</p>
      )}
    </div>
  )
}

interface SwitchFieldProps {
  label: string
  value: boolean
  hint?: string
  onChange: (v: boolean) => void
}

function SwitchField({ label, value, hint, onChange }: SwitchFieldProps) {
  return (
    <div className="flex items-start justify-between gap-2">
      <div className="flex-1 min-w-0">
        <span className="text-xs text-neutral-700 dark:text-neutral-300">{label}</span>
        {hint && (
          <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">{hint}</p>
        )}
      </div>
      <button
        role="switch"
        aria-checked={value}
        onClick={() => onChange(!value)}
        className={`relative inline-flex h-4 w-8 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 focus:outline-none mt-0.5 ${
          value ? 'bg-[var(--color-accent-DEFAULT,#F97316)]' : 'bg-neutral-300 dark:bg-neutral-600'
        }`}
      >
        <span
          className={`pointer-events-none inline-block h-3 w-3 transform rounded-full bg-white shadow ring-0 transition duration-200 ${
            value ? 'translate-x-4' : 'translate-x-0'
          }`}
        />
      </button>
    </div>
  )
}

interface IntSegmentedProps {
  label: string
  value: number
  min: number
  max: number
  hint?: string
  onChange: (v: number) => void
}

function IntSegmented({ label, value, min, max, hint, onChange }: IntSegmentedProps) {
  const options = Array.from({ length: max - min + 1 }, (_, i) => min + i)

  return (
    <div className="space-y-1">
      <span className="text-xs text-neutral-700 dark:text-neutral-300">{label}</span>
      <div className="flex gap-0.5">
        {options.map(opt => (
          <button
            key={opt}
            onClick={() => onChange(opt)}
            className={`flex-1 text-xs py-0.5 rounded transition-colors cursor-pointer ${
              value === opt
                ? 'bg-[var(--color-accent-DEFAULT,#F97316)] text-white font-medium'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {opt}
          </button>
        ))}
      </div>
      {hint && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">{hint}</p>
      )}
    </div>
  )
}

// ─── Highway Cost Slider ──────────────────────────────────────────────────

interface HighwayCostSliderProps {
  label: string
  value: number
  defaultValue: number | null
  hint?: string
  isModified: boolean
  onChange: (v: number) => void
  onReset: () => void
  fieldKey: string
  editingField: string | null
  editValue: string
  onStartEdit: (key: string, currentValue: number) => void
  onEditChange: (v: string) => void
  onCommitEdit: (key: string, min: number) => void
  onCancelEdit: () => void
}

function HighwayCostSlider({
  label,
  value,
  defaultValue,
  hint,
  isModified,
  onChange,
  onReset,
  fieldKey,
  editingField,
  editValue,
  onStartEdit,
  onEditChange,
  onCommitEdit,
  onCancelEdit,
}: HighwayCostSliderProps) {
  const blocked = defaultValue === null
  const displayVal = value.toFixed(1)
  const defaultDisplay = defaultValue !== null ? defaultValue.toFixed(1) : 'gesperrt'
  const isEditing = editingField === fieldKey
  const hwMin = 0.5
  const hwMax = 10
  const isOverMax = value > hwMax

  return (
    <div className={`space-y-1 ${blocked ? 'opacity-40' : ''}`}>
      <div className="flex items-center justify-between gap-1">
        <span className="text-xs text-neutral-700 dark:text-neutral-300 flex-1 min-w-0 truncate">
          {label}
        </span>
        <div className="flex items-center gap-1.5 flex-shrink-0">
          {isModified && !blocked && (
            <button
              onClick={onReset}
              className="text-[9px] text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer transition-colors"
              title={`Auf Standard zuruecksetzen (${defaultDisplay})`}
            >
              <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.433a.75.75 0 000-1.5H4.28a.75.75 0 00-.75.75v3.955a.75.75 0 001.5 0v-2.146l.312.31a7 7 0 0011.712-3.138.75.75 0 00-1.449-.386zm-10.624-2.85a5.5 5.5 0 019.201-2.466l.312.311h-2.433a.75.75 0 000 1.5h3.952a.75.75 0 00.75-.75V3.214a.75.75 0 00-1.5 0v2.146l-.312-.31A7 7 0 003.244 8.188a.75.75 0 001.449.386z" clipRule="evenodd" />
              </svg>
            </button>
          )}
          {blocked ? (
            <span className="text-xs font-mono text-neutral-500 dark:text-neutral-400 tabular-nums w-8 text-right">
              --
            </span>
          ) : isEditing ? (
            <input
              type="number"
              step="any"
              autoFocus
              value={editValue}
              onChange={e => onEditChange(e.target.value)}
              onFocus={e => e.target.select()}
              onBlur={() => onCommitEdit(fieldKey, hwMin)}
              onKeyDown={e => {
                if (e.key === 'Enter') onCommitEdit(fieldKey, hwMin)
                else if (e.key === 'Escape') onCancelEdit()
              }}
              className="w-14 text-xs font-mono tabular-nums text-right bg-neutral-100 dark:bg-neutral-800 border border-neutral-300 dark:border-neutral-600 rounded px-1 py-0 focus:outline-none focus:border-[var(--color-accent-DEFAULT,#F97316)]"
            />
          ) : (
            <button
              onClick={() => onStartEdit(fieldKey, value)}
              title="Klicken zum Bearbeiten"
              className={`text-xs font-mono tabular-nums w-8 text-right cursor-pointer hover:opacity-70 transition-opacity ${
                isOverMax
                  ? 'text-orange-500'
                  : 'text-neutral-500 dark:text-neutral-400'
              }`}
            >
              {displayVal}
            </button>
          )}
        </div>
      </div>
      <input
        type="range"
        min={hwMin}
        max={hwMax}
        step={0.5}
        value={blocked ? hwMin : Math.min(value, hwMax)}
        disabled={blocked}
        onChange={e => onChange(parseFloat(e.target.value))}
        title={`Standard: ${defaultDisplay}`}
        className={`w-full h-1.5 rounded-full appearance-none ${
          blocked
            ? 'cursor-not-allowed bg-neutral-200 dark:bg-neutral-700'
            : 'cursor-pointer accent-[var(--color-accent-DEFAULT,#F97316)] bg-neutral-200 dark:bg-neutral-700'
        }`}
      />
      {hint && (
        <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">{hint}</p>
      )}
    </div>
  )
}

// ─── Collapsible Section ───────────────────────────────────────────────────

interface SectionProps {
  title: string
  children: React.ReactNode
  onReset: () => void
  defaultOpen?: boolean
}

function Section({ title, children, onReset, defaultOpen = false }: SectionProps) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="border-b border-neutral-100 dark:border-neutral-800 last:border-0">
      <div className="flex items-center justify-between px-3 py-2">
        <button
          onClick={() => setOpen(o => !o)}
          className="flex items-center gap-1.5 flex-1 text-left cursor-pointer"
        >
          <svg
            className={`w-3 h-3 text-neutral-400 transition-transform flex-shrink-0 ${open ? 'rotate-90' : ''}`}
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path fillRule="evenodd" d="M7.21 14.77a.75.75 0 01.02-1.06L11.168 10 7.23 6.29a.75.75 0 111.04-1.08l4.5 4.25a.75.75 0 010 1.08l-4.5 4.25a.75.75 0 01-1.06-.02z" clipRule="evenodd" />
          </svg>
          <span className="text-[10px] font-semibold uppercase tracking-wide text-neutral-500 dark:text-neutral-400">
            {title}
          </span>
        </button>
        <button
          onClick={onReset}
          className="text-[10px] text-neutral-400 dark:text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-200 cursor-pointer transition-colors"
          title="Sektion zurücksetzen"
        >
          Zurücksetzen
        </button>
      </div>
      {open && (
        <div className="px-3 pb-3 space-y-3">
          {children}
        </div>
      )}
    </div>
  )
}

// ─── ProfileEditor ─────────────────────────────────────────────────────────

export interface ProfileEditorProps {
  config: ProfileConfig
  onChange: (config: ProfileConfig) => void
}

export default function ProfileEditor({ config, onChange }: ProfileEditorProps) {
  const currentProfile = config.profile ?? 'hiking'
  // getProfileDefaults returns null when server cache is not yet populated
  const defaults = getProfileDefaults(currentProfile)

  // Inline editing state
  const [editingField, setEditingField] = useState<string | null>(null)
  const [editValue, setEditValue] = useState('')

  function handleStartEdit(key: string, currentValue: number) {
    setEditingField(key)
    setEditValue(String(currentValue))
  }

  function handleCommitEdit(key: string, min: number) {
    const parsed = parseFloat(editValue)
    if (!isNaN(parsed)) {
      onChange({ ...config, [key]: Math.max(min, parsed) })
    }
    setEditingField(null)
  }

  function handleCancelEdit() {
    setEditingField(null)
  }

  // Server-side defaults (includes highway costs with null for blocked types)
  const [serverDefaults, setServerDefaults] = useState<ProfileDefaultsResponse | null>(null)

  // Track which profile the user most recently switched to (for stale-fetch guard)
  const switchTargetRef = useRef<string>(currentProfile)

  const loadServerDefaults = useCallback((profileName: string) => {
    fetchProfileDefaults(profileName)
      .then(resp => {
        // Cache in the global store so getProfileDefaults() returns server values
        cacheServerDefaults(profileName, resp)
        setServerDefaults(resp)
      })
      .catch(() => {
        // Silently fall back — sliders will show without server defaults
        setServerDefaults(null)
      })
  }, [])

  useEffect(() => {
    loadServerDefaults(currentProfile)
  }, [currentProfile, loadServerDefaults])

  function updateField<K extends keyof ProfileConfig>(key: K, value: ProfileConfig[K]) {
    onChange({ ...config, [key]: value })
  }

  function resetSection(section: FieldSection) {
    if (!defaults) return
    const sectionFields = FIELD_METADATA.filter(f => f.section === section)
    const patch: Partial<ProfileConfig> = {}
    for (const f of sectionFields) {
      const k = f.key as keyof ProfileConfig
      const defaultVal = defaults[k]
      if (defaultVal !== undefined) {
        ;(patch as Record<string, unknown>)[k] = defaultVal
      }
    }
    onChange({ ...config, ...patch })
  }

  function resetHighwayCosts() {
    // Remove all highway cost overrides (revert to server defaults)
    const patch = { ...config }
    for (const def of HIGHWAY_COST_DEFS) {
      delete patch[def.key]
    }
    onChange(patch)
  }

  function handleProfileSwitch(profileName: string) {
    // Mark this as the most recent switch target (stale-fetch guard)
    switchTargetRef.current = profileName

    // Immediate: apply cached server defaults if available
    const cached = getProfileDefaults(profileName)
    if (cached) onChange(cached)

    // Async: fetch authoritative server defaults and update if they differ
    fetchProfileDefaults(profileName)
      .then(resp => {
        cacheServerDefaults(profileName, resp)
        setServerDefaults(resp)

        // Only apply if user hasn't switched to a different profile in the meantime
        if (switchTargetRef.current === profileName) {
          const fresh = getProfileDefaults(profileName)
          if (fresh) onChange(fresh)
        }
      })
      .catch(() => {
        // Server unreachable — keep whatever was applied above
        setServerDefaults(null)
      })
  }

  /** Get the server default for a highway cost key. Returns null for blocked types. */
  function getServerDefault(key: HighwayCostKey): number | null {
    if (!serverDefaults) return null
    return serverDefaults[key]
  }

  /** Build a tooltip hint, adding "nur befestigte Tracks" for road profile track slider */
  function getHighwayHint(def: HighwayCostDef): string | undefined {
    if (def.key === 'track_cost' && currentProfile === 'road') {
      return 'nur befestigte Tracks'
    }
    return def.hint
  }

  const sections = ['elevation', 'difficulty', 'environment', 'paths', 'behavior'] as FieldSection[]
  const firstPredicate = config.predicates?.[0]
  const selectedRouteSport = firstPredicate
    ? `${firstPredicate.sport}/${firstPredicate.sub_type ?? ''}`
    : ''

  function updatePrimaryPredicate(value: string) {
    if (value === '') {
      onChange({ ...config, predicates: undefined })
      return
    }
    const [sportType, subType = ''] = value.split('/')
    const predicate: SportPredicate = { sport: sportType }
    if (subType !== '') {
      predicate.sub_type = subType
    }
    onChange({
      ...config,
      predicates: [predicate],
      sport_types: undefined,
    })
  }

  function updateModifierFilter(key: keyof ModifierFilters, checked: boolean) {
    const next = { ...(config.modifier_filters ?? {}) }
    if (checked) {
      next[key] = true
    } else {
      delete next[key]
    }
    onChange({ ...config, modifier_filters: next })
  }

  return (
    <div className="flex flex-col">
      {/* Profile tabs */}
      <div className="px-3 pt-2.5 pb-2 border-b border-neutral-100 dark:border-neutral-800">
        <div className="text-[10px] font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-1.5">
          Profil
        </div>
        <div className="flex gap-1 flex-wrap">
          {PROFILE_ORDER.map(name => {
            const { label, description } = PROFILE_LABELS[name]
            return (
              <button
                key={name}
                onClick={() => handleProfileSwitch(name)}
                title={description}
                className={`px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer ${
                  currentProfile === name
                    ? 'bg-[var(--color-accent-DEFAULT,#F97316)] text-white'
                    : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
                }`}
              >
                {label}
              </button>
            )
          })}
        </div>
      </div>

      <div className="px-3 py-2.5 border-b border-neutral-100 dark:border-neutral-800 space-y-2">
        <div className="text-[10px] font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Sport-Zuordnung
        </div>
        <select
          value={selectedRouteSport}
          onChange={e => updatePrimaryPredicate(e.target.value)}
          className="w-full text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-800 dark:text-neutral-200 px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
          aria-label="Sport-Zuordnung"
        >
          <option value="">Nicht festgelegt</option>
          {ROUTE_SPORT_OPTIONS.map(option => (
            <option key={`${option.sportType}/${option.subType ?? ''}`} value={`${option.sportType}/${option.subType ?? ''}`}>
              {option.label}
            </option>
          ))}
        </select>
        <div className="grid grid-cols-2 gap-1.5">
          {MODIFIER_FILTER_OPTIONS.map(option => (
            <label key={option.key} className="flex items-center gap-1.5 text-xs text-neutral-700 dark:text-neutral-300">
              <input
                type="checkbox"
                checked={Boolean(config.modifier_filters?.[option.key])}
                onChange={e => updateModifierFilter(option.key, e.target.checked)}
                className="h-3.5 w-3.5 flex-shrink-0 accent-[var(--color-accent-DEFAULT,#F97316)] cursor-pointer"
              />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
      </div>

      {/* Collapsible sections */}
      {sections.map((section, idx) => {
        const fields = FIELD_METADATA.filter(f => f.section === section)
        if (fields.length === 0) return null

        return (
          <Section
            key={section}
            title={SECTION_LABELS[section]}
            onReset={() => resetSection(section)}
            defaultOpen={idx === 0}
          >
            {fields.map(field => {
              const rawValue = config[field.key]

              if (field.type === 'boolean') {
                const boolVal = rawValue !== undefined ? Boolean(rawValue) : (defaults?.[field.key] as boolean | undefined) ?? false
                return (
                  <SwitchField
                    key={field.key}
                    label={field.label}
                    value={boolVal}
                    hint={field.hint}
                    onChange={v => updateField(field.key, v as ProfileConfig[typeof field.key])}
                  />
                )
              }

              const numVal =
                rawValue !== undefined
                  ? Number(rawValue)
                  : Number(defaults?.[field.key] ?? field.min ?? 0)

              if (field.type === 'int') {
                if (field.key === 'max_via_ferrata') {
                  const vfVal = Math.round(numVal)
                  return (
                    <div key={field.key} className="space-y-1">
                      <IntSegmented
                        label={field.label}
                        value={vfVal}
                        min={field.min ?? 0}
                        max={field.max ?? 6}
                        hint={field.hint}
                        onChange={v => updateField(field.key, v as ProfileConfig[typeof field.key])}
                      />
                      {vfVal > 0 && (
                        <>
                          <p className="text-[10px] text-neutral-400 dark:text-neutral-500 leading-snug">
                            Nur bestätigt gescalede Klettersteige (K1–K{vfVal}) werden erlaubt.
                            Für Klettersteige ohne OSM-Schwierigkeitsangabe siehe Opt-In unten.
                          </p>
                          <label className="flex items-start gap-2 py-1 cursor-pointer">
                            <input
                              type="checkbox"
                              checked={config.allow_unscaled_via_ferrata ?? false}
                              onChange={e => updateField('allow_unscaled_via_ferrata', e.target.checked)}
                              className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 accent-[var(--color-accent-DEFAULT,#F97316)] cursor-pointer"
                            />
                            <div>
                              <span className="text-xs text-neutral-700 dark:text-neutral-300">
                                Unscaled Klettersteige erlauben
                              </span>
                              <p className="text-[10px] text-red-600 dark:text-red-400 leading-snug">
                                Achtung: Aktiviert Routing über Klettersteig-Ways ohne bestätigte
                                Schwierigkeitsangabe in OSM. Tatsächliche Schwierigkeit kann K1 bis K6
                                sein. Nur mit Ortskenntnis aktivieren.
                              </p>
                            </div>
                          </label>
                        </>
                      )}
                    </div>
                  )
                }

                return (
                  <IntSegmented
                    key={field.key}
                    label={field.label}
                    value={Math.round(numVal)}
                    min={field.min ?? 0}
                    max={field.max ?? 10}
                    hint={field.hint}
                    onChange={v => updateField(field.key, v as ProfileConfig[typeof field.key])}
                  />
                )
              }

              // type === 'number'
              return (
                <SliderField
                  key={field.key}
                  label={field.label}
                  value={numVal}
                  min={field.min ?? 0}
                  max={field.max ?? 1}
                  step={field.step ?? 0.1}
                  unit={field.unit}
                  hint={field.hint}
                  onChange={v => updateField(field.key, v as ProfileConfig[typeof field.key])}
                  fieldKey={field.key}
                  editingField={editingField}
                  editValue={editValue}
                  onStartEdit={handleStartEdit}
                  onEditChange={setEditValue}
                  onCommitEdit={handleCommitEdit}
                  onCancelEdit={handleCancelEdit}
                />
              )
            })}
          </Section>
        )
      })}

      {/* Highway-Kosten section */}
      {serverDefaults && (
        <Section
          title="Highway-Kosten"
          onReset={resetHighwayCosts}
          defaultOpen={false}
        >
          {(['trails', 'roads'] as const).map(group => {
            const groupDefs = HIGHWAY_COST_DEFS.filter(d => d.group === group)
            return (
              <div key={group} className="space-y-2">
                <div className="text-[10px] font-medium text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">
                  {HIGHWAY_GROUP_LABELS[group]}
                </div>
                {groupDefs.map(def => {
                  const serverDefault = getServerDefault(def.key)
                  const blocked = serverDefault === null
                  const currentValue = config[def.key]
                  const displayValue = currentValue ?? (blocked ? 1 : serverDefault ?? 1)
                  const isModified = currentValue !== undefined && !blocked

                  return (
                    <HighwayCostSlider
                      key={def.key}
                      label={def.label}
                      value={displayValue}
                      defaultValue={serverDefault}
                      hint={getHighwayHint(def)}
                      isModified={isModified}
                      onChange={v => {
                        // Only set if different from server default
                        if (serverDefault !== null && v === serverDefault) {
                          // Remove the override (back to default)
                          const patch = { ...config }
                          delete patch[def.key]
                          onChange(patch)
                        } else {
                          updateField(def.key, v)
                        }
                      }}
                      onReset={() => {
                        const patch = { ...config }
                        delete patch[def.key]
                        onChange(patch)
                      }}
                      fieldKey={def.key}
                      editingField={editingField}
                      editValue={editValue}
                      onStartEdit={handleStartEdit}
                      onEditChange={setEditValue}
                      onCommitEdit={handleCommitEdit}
                      onCancelEdit={handleCancelEdit}
                    />
                  )
                })}
              </div>
            )
          })}
        </Section>
      )}
    </div>
  )
}
