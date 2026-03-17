import { useState } from 'react'
import {
  PROFILES,
  FIELD_METADATA,
  SECTION_LABELS,
  getProfileDefaults,
  type ProfileConfig,
  type FieldSection,
} from '@/lib/routingProfiles'

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
}

function SliderField({ label, value, min, max, step, unit, hint, onChange }: SliderFieldProps) {
  const display = step < 0.1 ? value.toFixed(2) : step < 1 ? value.toFixed(2) : String(Math.round(value))

  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-xs text-neutral-700 dark:text-neutral-300">{label}</span>
        <span className="text-xs font-mono text-neutral-500 dark:text-neutral-400 tabular-nums">
          {display}{unit ? ` ${unit}` : ''}
        </span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={e => onChange(parseFloat(e.target.value))}
        className="w-full h-1.5 rounded-full appearance-none cursor-pointer accent-[var(--color-accent-DEFAULT,#3b82f6)] bg-neutral-200 dark:bg-neutral-700"
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
          value ? 'bg-[var(--color-accent-DEFAULT,#3b82f6)]' : 'bg-neutral-300 dark:bg-neutral-600'
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
                ? 'bg-[var(--color-accent-DEFAULT,#3b82f6)] text-white font-medium'
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
  const defaults = getProfileDefaults(currentProfile)

  function updateField<K extends keyof ProfileConfig>(key: K, value: ProfileConfig[K]) {
    onChange({ ...config, [key]: value })
  }

  function resetSection(section: FieldSection) {
    const sectionFields = FIELD_METADATA.filter(f => f.section === section)
    const patch: Partial<ProfileConfig> = {}
    for (const f of sectionFields) {
      const k = f.key as keyof ProfileConfig
      const defaultVal = defaults[k]
      if (defaultVal !== undefined) {
        // Type-safe assignment
        ;(patch as Record<string, unknown>)[k] = defaultVal
      }
    }
    onChange({ ...config, ...patch })
  }

  function handleProfileSwitch(profileName: string) {
    onChange(getProfileDefaults(profileName))
  }

  const sections = ['elevation', 'difficulty', 'environment', 'paths', 'behavior'] as FieldSection[]

  return (
    <div className="flex flex-col">
      {/* Profile tabs */}
      <div className="px-3 pt-2.5 pb-2 border-b border-neutral-100 dark:border-neutral-800">
        <div className="text-[10px] font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-1.5">
          Profil
        </div>
        <div className="flex gap-1 flex-wrap">
          {PROFILES.map(p => (
            <button
              key={p.name}
              onClick={() => handleProfileSwitch(p.name)}
              title={p.description}
              className={`px-2 py-1 rounded text-xs font-medium transition-colors cursor-pointer ${
                currentProfile === p.name
                  ? 'bg-[var(--color-accent-DEFAULT,#3b82f6)] text-white'
                  : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
              }`}
            >
              {p.label}
            </button>
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
                const boolVal = rawValue !== undefined ? Boolean(rawValue) : (defaults[field.key] as boolean | undefined) ?? false
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
                  : Number(defaults[field.key] ?? field.min ?? 0)

              if (field.type === 'int') {
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
                />
              )
            })}
          </Section>
        )
      })}
    </div>
  )
}
