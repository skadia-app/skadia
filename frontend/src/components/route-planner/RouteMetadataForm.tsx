import { useState, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { fetchTags } from '@/api/tags'
import { routeSportOptions } from '@/lib/sportTaxonomy'
import { supportedLanguageFrom } from '@/i18n/localizedString'
import type { Tag } from '@/api/tags'

// ─── BRouter profiles ─────────────────────────────────────────────────────────

export const BROUTER_PROFILES: { value: string; labelKey: string }[] = [
  { value: 'hiking-mountain', labelKey: 'routePlanner.metadata.brouterProfiles.hikingMountain' },
  { value: 'trekking', labelKey: 'routePlanner.metadata.brouterProfiles.trekking' },
  { value: 'fastbike', labelKey: 'routePlanner.metadata.brouterProfiles.fastbike' },
  { value: 'mtb', labelKey: 'routePlanner.metadata.brouterProfiles.mtb' },
  { value: 'gravel', labelKey: 'routePlanner.metadata.brouterProfiles.gravel' },
  { value: 'shortest', labelKey: 'routePlanner.metadata.brouterProfiles.shortest' },
]

// ─── Types ────────────────────────────────────────────────────────────────────

export interface RouteMetadataValues {
  name: string
  description: string
  sport_type: string
  sub_type: string
  is_electric: boolean
  is_indoor: boolean
  is_virtual: boolean
  is_commute: boolean
  difficulty: string
  tag_ids: string[]
  auto_tag: boolean
  profile_override: string
}

interface Props {
  initialValues?: Partial<RouteMetadataValues>
  currentProfile: string
  onChange: (values: RouteMetadataValues) => void
  /** If true, show a save-as-new hint beneath the name field */
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
  const { t } = useTranslation()
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
          {selectedTags.map((tag) => (
            <span
              key={tag.id}
              className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium"
              style={{ backgroundColor: tag.color + '22', color: tag.color, border: `1px solid ${tag.color}55` }}
            >
              {tag.name}
              <button
                type="button"
                onClick={() => toggle(tag.id)}
                className="ml-0.5 opacity-60 hover:opacity-100 cursor-pointer"
                aria-label={t('routePlanner.metadata.removeTag', { name: tag.name })}
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
          placeholder={t('routePlanner.metadata.searchTagPlaceholder')}
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
                  ? 'bg-accent-100 dark:bg-accent-DEFAULT/15 text-accent-700 dark:text-accent-300'
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
          {t('routePlanner.metadata.noTagsFound')}
        </div>
      )}
    </div>
  )
}

// ─── RouteMetadataForm ────────────────────────────────────────────────────────

export default function RouteMetadataForm({ initialValues, currentProfile, onChange, isNew }: Props) {
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(i18n.language)
  const sportOptions = routeSportOptions(language)

  const [name, setName] = useState(initialValues?.name ?? '')
  const [description, setDescription] = useState(initialValues?.description ?? '')
  const [sportType, setSportType] = useState(initialValues?.sport_type ?? 'hiking')
  const [subType, setSubType] = useState(initialValues?.sub_type ?? '')
  const [isElectric, setIsElectric] = useState(initialValues?.is_electric ?? false)
  const isIndoor = initialValues?.is_indoor ?? false
  const isVirtual = initialValues?.is_virtual ?? false
  const isCommute = initialValues?.is_commute ?? false
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
      sub_type: subType,
      is_electric: isElectric,
      is_indoor: isIndoor,
      is_virtual: isVirtual,
      is_commute: isCommute,
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
          {t('routePlanner.metadata.name')} <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => { setName(e.target.value); notify({ name: e.target.value }) }}
          required
          autoFocus
          placeholder={t('routePlanner.metadata.namePlaceholder')}
          className={inputClass}
        />
        {isNew && (
          <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-1">
            {t('routePlanner.metadata.newRouteHint')}
          </p>
        )}
      </div>

      {/* Description */}
      <div>
        <label className={labelClass}>{t('routePlanner.metadata.description')}</label>
        <textarea
          value={description}
          onChange={(e) => { setDescription(e.target.value); notify({ description: e.target.value }) }}
          rows={3}
          placeholder={t('routePlanner.metadata.descriptionPlaceholder')}
          className={`${inputClass} resize-none`}
        />
      </div>

      {/* Sport type + Difficulty side by side */}
      <div className="grid grid-cols-2 gap-4">
        <div>
          <label className={labelClass}>{t('routePlanner.metadata.sport')}</label>
          <select
            value={`${sportType}:${subType}`}
            onChange={(e) => {
              const [nextSportType, nextSubType = ''] = e.target.value.split(':')
              setSportType(nextSportType)
              setSubType(nextSubType)
              notify({ sport_type: nextSportType, sub_type: nextSubType })
            }}
            className={inputClass}
          >
            {sportOptions.map(({ sportType, subType, label }) => (
              <option key={`${sportType}:${subType ?? ''}`} value={`${sportType}:${subType ?? ''}`}>{label}</option>
            ))}
          </select>
        </div>

        <div>
          <label className={labelClass}>{t('routePlanner.metadata.difficulty')}</label>
          <input
            type="text"
            value={difficulty}
            onChange={(e) => { setDifficulty(e.target.value); notify({ difficulty: e.target.value }) }}
            placeholder={t('routePlanner.metadata.difficultyPlaceholder')}
            className={inputClass}
          />
        </div>
      </div>

      <label className="flex items-center justify-between gap-3 cursor-pointer select-none rounded-md border border-neutral-200 dark:border-neutral-800 px-3 py-2">
        <span className="text-sm font-medium text-neutral-700 dark:text-neutral-300">{t('routePlanner.metadata.eDrive')}</span>
        <div className="relative flex-shrink-0">
          <input
            type="checkbox"
            checked={isElectric}
            onChange={(e) => {
              setIsElectric(e.target.checked)
              notify({ is_electric: e.target.checked })
            }}
            className="sr-only peer"
          />
          <div className="w-9 h-5 rounded-full bg-neutral-300 dark:bg-neutral-700 peer-checked:bg-accent-DEFAULT transition-colors" />
          <div className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow transition-transform peer-checked:translate-x-4" />
        </div>
      </label>

      {/* Tags multi-select */}
      <div>
        <label className={labelClass}>{t('routePlanner.metadata.tags')}</label>
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
            {t('routePlanner.metadata.autoTag')}
          </span>
          <p className="text-xs text-neutral-500 dark:text-neutral-400">
            {t('routePlanner.metadata.autoTagDescription')}
          </p>
        </div>
      </label>

      {/* Profile override */}
      <div>
        <label className={labelClass}>{t('routePlanner.metadata.routingProfile')}</label>
        <select
          value={profileOverride}
          onChange={(e) => { setProfileOverride(e.target.value); notify({ profile_override: e.target.value }) }}
          className={inputClass}
        >
          {BROUTER_PROFILES.map(({ value, labelKey }) => (
            <option key={value} value={value}>{t(labelKey)}</option>
          ))}
        </select>
        {profileOverride !== currentProfile && (
          <p className="text-xs text-amber-600 dark:text-amber-400 mt-1">
            {t('routePlanner.metadata.profileOverrideHint', { profile: currentProfile })}
          </p>
        )}
      </div>
    </div>
  )
}
