import { useState, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchTags } from '@/api/tags'
import { generateGearSportOptions } from '@/lib/sportOptions'
import type { Tag } from '@/api/tags'

// ─── BRouter profiles ─────────────────────────────────────────────────────────

export const BROUTER_PROFILES: { value: string; label: string }[] = [
  { value: 'hiking-sac2', label: 'Wandern (SAC)' },
  { value: 'trekking', label: 'Trekking' },
  { value: 'fastbike', label: 'Rennrad' },
  { value: 'mtb', label: 'Mountainbike' },
  { value: 'gravel', label: 'Gravel' },
  { value: 'safety', label: 'Stadtrad (sicher)' },
  { value: 'shortest', label: 'Kürzeste Route' },
]

// ─── Types ────────────────────────────────────────────────────────────────────

export interface RouteMetadataValues {
  name: string
  description: string
  sport_type: string
  difficulty: string
  tag_ids: string[]
  auto_tag: boolean
  profile_override: string
}

interface Props {
  initialValues?: Partial<RouteMetadataValues>
  currentProfile: string
  onChange: (values: RouteMetadataValues) => void
  /** If true, show a "Als neue Route speichern" hint beneath the name field */
  isNew?: boolean
}

// ─── TagMultiSelect ───────────────────────────────────────────────────────────

function TagMultiSelect({
  selected,
  onChange,
}: {
  selected: string[]
  onChange: (ids: string[]) => void
}) {
  const [search, setSearch] = useState('')
  const [open, setOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: allTags = [] } = useQuery<Tag[]>({
    queryKey: ['tags'],
    queryFn: fetchTags,
  })

  const selectedSet = new Set(selected)
  const filtered = allTags.filter((t) =>
    !t.is_system && t.name.toLowerCase().includes(search.toLowerCase().trim()),
  )

  function toggle(id: string) {
    if (selectedSet.has(id)) {
      onChange(selected.filter((s) => s !== id))
    } else {
      onChange([...selected, id])
    }
  }

  const selectedTags = allTags.filter((t) => selectedSet.has(t.id))

  return (
    <div>
      {/* Selected chips */}
      {selectedTags.length > 0 && (
        <div className="flex flex-wrap gap-1.5 mb-2">
          {selectedTags.map((t) => (
            <span
              key={t.id}
              className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium"
              style={{ backgroundColor: t.color + '22', color: t.color, border: `1px solid ${t.color}55` }}
            >
              {t.name}
              <button
                type="button"
                onClick={() => toggle(t.id)}
                className="ml-0.5 opacity-60 hover:opacity-100 cursor-pointer"
                aria-label={`${t.name} entfernen`}
              >
                &#10005;
              </button>
            </span>
          ))}
        </div>
      )}

      {/* Search input */}
      <div className="relative">
        <input
          ref={inputRef}
          type="text"
          value={search}
          onChange={(e) => { setSearch(e.target.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          onBlur={() => setTimeout(() => setOpen(false), 150)}
          placeholder="Tag suchen und hinzufügen…"
          className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />
        {search && (
          <button
            type="button"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setSearch('')}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
          >
            &#10005;
          </button>
        )}
      </div>

      {/* Dropdown */}
      {open && filtered.length > 0 && (
        <div className="mt-1 max-h-36 overflow-y-auto space-y-0.5">
          {filtered.map((tag) => (
            <button
              key={tag.id}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => { toggle(tag.id); setSearch('') }}
              className={`w-full flex items-center gap-2 px-2.5 py-1.5 text-sm rounded-md transition-colors text-left cursor-pointer ${
                selectedSet.has(tag.id)
                  ? 'bg-accent-50 dark:bg-accent-900/20 text-accent-700 dark:text-accent-300'
                  : 'hover:bg-neutral-50 dark:hover:bg-neutral-800 text-neutral-800 dark:text-neutral-200'
              }`}
            >
              <span
                className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                style={{ backgroundColor: tag.color }}
              />
              <span>{tag.name}</span>
              {selectedSet.has(tag.id) && (
                <span className="ml-auto text-accent-DEFAULT text-xs">&#10003;</span>
              )}
            </button>
          ))}
        </div>
      )}

      {open && search.trim() !== '' && filtered.length === 0 && (
        <div className="mt-1 px-3 py-2 text-sm text-neutral-400 dark:text-neutral-500">
          Keine Tags gefunden
        </div>
      )}
    </div>
  )
}

// ─── RouteMetadataForm ────────────────────────────────────────────────────────

export default function RouteMetadataForm({ initialValues, currentProfile, onChange, isNew }: Props) {
  const sportOptions = generateGearSportOptions()

  const [name, setName] = useState(initialValues?.name ?? '')
  const [description, setDescription] = useState(initialValues?.description ?? '')
  const [sportType, setSportType] = useState(initialValues?.sport_type ?? 'hiking')
  const [difficulty, setDifficulty] = useState(initialValues?.difficulty ?? '')
  const [tagIds, setTagIds] = useState<string[]>(initialValues?.tag_ids ?? [])
  const [autoTag, setAutoTag] = useState(initialValues?.auto_tag ?? true)
  const [profileOverride, setProfileOverride] = useState(
    initialValues?.profile_override ?? currentProfile,
  )

  function notify(patch: Partial<RouteMetadataValues>) {
    onChange({
      name,
      description,
      sport_type: sportType,
      difficulty,
      tag_ids: tagIds,
      auto_tag: autoTag,
      profile_override: profileOverride,
      ...patch,
    })
  }

  const inputClass =
    'w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-2 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent'

  const labelClass = 'block text-sm font-medium text-neutral-700 dark:text-neutral-300 mb-1'

  return (
    <div className="space-y-4">
      {/* Name (required) */}
      <div>
        <label className={labelClass}>
          Name <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => { setName(e.target.value); notify({ name: e.target.value }) }}
          required
          autoFocus
          placeholder="z.B. Alpenüberquerung Etappe 1"
          className={inputClass}
        />
        {isNew && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
            Neue Route — wird separat gespeichert
          </p>
        )}
      </div>

      {/* Description */}
      <div>
        <label className={labelClass}>Beschreibung</label>
        <textarea
          value={description}
          onChange={(e) => { setDescription(e.target.value); notify({ description: e.target.value }) }}
          rows={3}
          placeholder="Markdown wird unterstützt…"
          className={`${inputClass} resize-none`}
        />
      </div>

      {/* Sport type + Difficulty side by side */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className={labelClass}>Sportart</label>
          <select
            value={sportType}
            onChange={(e) => { setSportType(e.target.value); notify({ sport_type: e.target.value }) }}
            className={inputClass}
          >
            {sportOptions.map(({ value, label }) => (
              <option key={value} value={value}>{label}</option>
            ))}
          </select>
        </div>

        <div>
          <label className={labelClass}>Schwierigkeit</label>
          <input
            type="text"
            value={difficulty}
            onChange={(e) => { setDifficulty(e.target.value); notify({ difficulty: e.target.value }) }}
            placeholder="z.B. T3, schwer"
            className={inputClass}
          />
        </div>
      </div>

      {/* Tags multi-select */}
      <div>
        <label className={labelClass}>Tags</label>
        <TagMultiSelect
          selected={tagIds}
          onChange={(ids) => { setTagIds(ids); notify({ tag_ids: ids }) }}
        />
      </div>

      {/* Auto-Tag toggle */}
      <label className="flex items-center gap-3 cursor-pointer select-none">
        <div className="relative flex-shrink-0">
          <input
            type="checkbox"
            checked={autoTag}
            onChange={(e) => { setAutoTag(e.target.checked); notify({ auto_tag: e.target.checked }) }}
            className="sr-only peer"
          />
          <div className="w-9 h-5 rounded-full bg-neutral-300 dark:bg-neutral-700 peer-checked:bg-accent-DEFAULT transition-colors" />
          <div className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform peer-checked:translate-x-4" />
        </div>
        <div>
          <span className="text-sm font-medium text-neutral-800 dark:text-neutral-200">
            Auto-Tag
          </span>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            Tags automatisch anhand der Route vergeben
          </p>
        </div>
      </label>

      {/* Profile override */}
      <div>
        <label className={labelClass}>Routing-Profil (Override)</label>
        <select
          value={profileOverride}
          onChange={(e) => { setProfileOverride(e.target.value); notify({ profile_override: e.target.value }) }}
          className={inputClass}
        >
          {BROUTER_PROFILES.map(({ value, label }) => (
            <option key={value} value={value}>{label}</option>
          ))}
        </select>
        {profileOverride !== currentProfile && (
          <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
            Abweichend vom aktuellen Planer-Profil ({currentProfile})
          </p>
        )}
      </div>
    </div>
  )
}
