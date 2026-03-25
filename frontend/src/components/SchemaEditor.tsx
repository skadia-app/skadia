import { useRef, useMemo, useState } from 'react'

// Wichtigste Variablen — immer sichtbar
const VARIABLES_PRIMARY = [
  { key: '{year}', label: 'Jahr' },
  { key: '{month}', label: 'Monat' },
  { key: '{day}', label: 'Tag' },
  { key: '{sport}', label: 'Sportart' },
  { key: '{subCategory}', label: 'Unterkategorie' },
  { key: '{city}', label: 'Stadt' },
  { key: '{country}', label: 'Land' },
  { key: '{title}', label: 'Titel' },
] as const

// Erweiterte Variablen — hinter "Alle anzeigen"
const VARIABLES_EXTENDED = [
  { key: '{time}', label: 'Uhrzeit' },
  { key: '{state}', label: 'Bundesland/Region' },
  { key: '{distance_km}', label: 'Distanz (km)' },
  { key: '{duration_h}', label: 'Dauer (h)' },
  { key: '{duration_min}', label: 'Dauer (min)' },
  { key: '{elevation_m}', label: 'Höhenmeter' },
  { key: '{avg_speed_kmh}', label: 'Ø Geschwindigkeit (km/h)' },
  { key: '{avg_hr}', label: 'Ø Herzfrequenz' },
  { key: '{avg_power}', label: 'Ø Leistung (W)' },
  { key: '{gear}', label: 'Ausrüstung' },
  { key: '{source}', label: 'Quelle (z.B. Garmin)' },
  { key: '{id}', label: 'Aktivitäts-ID' },
] as const

const ALL_VARIABLES = [...VARIABLES_PRIMARY, ...VARIABLES_EXTENDED]

const EXAMPLE_VALUES: Record<string, string> = {
  '{year}': '2024',
  '{month}': '03',
  '{day}': '15',
  '{time}': '14-30',
  '{sport}': 'cycling',
  '{subCategory}': 'mountain',
  '{city}': 'Berlin',
  '{country}': 'Deutschland',
  '{state}': 'Brandenburg',
  '{title}': 'Feierabendrunde',
  '{distance_km}': '42',
  '{duration_h}': '2',
  '{duration_min}': '135',
  '{elevation_m}': '850',
  '{avg_speed_kmh}': '22',
  '{avg_hr}': '145',
  '{avg_power}': '210',
  '{gear}': 'Canyon-Spectral',
  '{source}': 'garmin',
  '{id}': 'abc123',
}

interface SchemaEditorProps {
  value: string
  onChange: (value: string) => void
  onSave: () => void
  isSaving: boolean
  isDirty: boolean
}

export default function SchemaEditor({ value, onChange, onSave, isSaving, isDirty }: SchemaEditorProps) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [showAll, setShowAll] = useState(false)

  const preview = useMemo(() => {
    let result = value
    for (const [key, val] of Object.entries(EXAMPLE_VALUES)) {
      result = result.replaceAll(key, val)
    }
    return result || '(leer)'
  }, [value])

  function insertVariable(variable: string) {
    const input = inputRef.current
    if (!input) return

    const start = input.selectionStart ?? value.length
    const end = input.selectionEnd ?? value.length
    const newValue = value.slice(0, start) + variable + value.slice(end)
    onChange(newValue)

    // Cursor nach der eingefügten Variable positionieren
    requestAnimationFrame(() => {
      const pos = start + variable.length
      input.setSelectionRange(pos, pos)
      input.focus()
    })
  }

  const visibleVars = showAll ? ALL_VARIABLES : VARIABLES_PRIMARY

  return (
    <div className="space-y-3">
      <div>
        <label className="block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1">
          Schema-Template
        </label>
        <div className="flex gap-2">
          <input
            ref={inputRef}
            type="text"
            value={value}
            onChange={e => onChange(e.target.value)}
            placeholder="{year}/{month}/{day}_{sport}_{city}"
            className="flex-1 border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2
                       bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100
                       placeholder:text-neutral-400 dark:placeholder:text-neutral-600
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT text-sm font-mono"
          />
          <button
            onClick={onSave}
            disabled={!isDirty || isSaving}
            className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium text-white
                       bg-accent-DEFAULT hover:bg-accent-600
                       disabled:opacity-40 disabled:cursor-not-allowed
                       transition-colors"
          >
            {isSaving ? 'Speichert...' : 'Speichern'}
          </button>
        </div>
      </div>

      {/* Variablen-Chips */}
      <div className="space-y-2">
        <div className="flex flex-wrap gap-1.5">
          {visibleVars.map(v => (
            <button
              key={v.key}
              onClick={() => insertVariable(v.key)}
              className="inline-flex items-center gap-1 px-2 py-1 text-xs rounded-md
                         border border-neutral-200 dark:border-neutral-700
                         bg-neutral-50 dark:bg-neutral-800
                         text-neutral-600 dark:text-neutral-400
                         hover:bg-orange-50 hover:border-orange-300 hover:text-accent-600
                         dark:hover:bg-orange-900/20 dark:hover:border-orange-700 dark:hover:text-orange-400
                         cursor-pointer transition-colors"
            >
              <span className="font-mono">{v.key}</span>
              <span className="text-neutral-400 dark:text-neutral-600">{v.label}</span>
            </button>
          ))}
        </div>

        <button
          onClick={() => setShowAll(prev => !prev)}
          className="cursor-pointer text-xs text-neutral-400 hover:text-accent-DEFAULT dark:hover:text-orange-400 transition-colors"
        >
          {showAll
            ? '↑ Weniger anzeigen'
            : `↓ Alle anzeigen (${VARIABLES_EXTENDED.length} weitere Variablen)`}
        </button>
      </div>

      {/* Live-Vorschau */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 p-3">
        <span className="text-xs text-neutral-500 dark:text-neutral-500 block mb-1">Vorschau</span>
        <span className="text-sm font-mono text-neutral-800 dark:text-neutral-200">{preview}</span>
      </div>
    </div>
  )
}
