import { useState, useMemo } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { useAnalyticsFilters, getPresets, savePreset, deletePreset } from '@/lib/analyticsFilters'
import type { AnalyticsFilterState, FilterPreset } from '@/lib/analyticsFilters'
import { generateActivityFilterOptions, getOptionKey, filterToExistingSports } from '@/lib/sportOptions'
import type { SportOption } from '@/lib/sportOptions'
import { fetchSportTypeStats } from '@/api/stats'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import TagFilterInput from '@/components/TagFilterInput'
import type { TagFilter } from '@/components/TagFilterInput'
import GearFilterInput from '@/components/GearFilterInput'
import type { GearFilter } from '@/components/GearFilterInput'
import { toLocalISO } from '@/lib/dateUtils'

const tabs = [
  { to: '/analytics/training', label: 'Training' },
  { to: '/analytics/health', label: 'Gesundheit' },
  { to: '/analytics/stats', label: 'Statistiken' },
  { to: '/analytics/insights', label: 'Insights' },
]

// Quick-select date presets
const DATE_PRESETS: { label: string; days: number | null }[] = [
  { label: '30 Tage', days: 30 },
  { label: '90 Tage', days: 90 },
  { label: '6 Monate', days: 180 },
  { label: '1 Jahr', days: 365 },
  { label: 'Alles', days: null },
]

function toISODate(d: Date): string {
  return toLocalISO(d)
}

function activeFilterCount(filters: AnalyticsFilterState): number {
  let count = 0
  if (filters.from) count++
  if (filters.to) count++
  if (filters.sportTypes.length > 0) count++
  if (filters.gearId) count++
  if (filters.isCommute) count++
  if (filters.isRace) count++
  if (filters.distMin || filters.distMax) count++
  if (filters.elevMin || filters.elevMax) count++
  if (filters.hrMin || filters.hrMax) count++
  if (filters.powerMin || filters.powerMax) count++
  if (filters.paceMin || filters.paceMax) count++
  if (filters.countries.length > 0) count++
  if (filters.seasons.length > 0) count++
  if (filters.weekdays.length > 0) count++
  if (filters.tags.length > 0) count++
  if (filters.includeTags.length > 0) count++
  if (filters.excludeTags.length > 0) count++
  if (filters.includeGear.length > 0) count++
  if (filters.excludeGear.length > 0) count++
  return count
}

// ── Sport Type Selector (checkbox tree — matches ActivityFilters layout) ─────

const ANALYTICS_SPORT_OPTIONS = generateActivityFilterOptions()

interface SportTypeSelectorProps {
  selected: string[]
  onChange: (types: string[]) => void
}

function SportTypeSelector({ selected, onChange }: SportTypeSelectorProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [sportSearch, setSportSearch] = useState('')
  const { hideUnused } = useGlobalSportSettings()
  const { data: statsData } = useQuery({
    queryKey: ['sport-type-stats'],
    queryFn: fetchSportTypeStats,
    enabled: hideUnused,
  })
  const existingSportTypes = useMemo<Set<string>>(() => {
    if (!statsData) return new Set()
    return new Set(statsData.map((s) => s.sport_type))
  }, [statsData])
  const visibleOptions = useMemo(() => {
    if (hideUnused && existingSportTypes.size > 0) {
      return filterToExistingSports(ANALYTICS_SPORT_OPTIONS, existingSportTypes)
    }
    return ANALYTICS_SPORT_OPTIONS
  }, [hideUnused, existingSportTypes])

  // Analytics uses plain sport type values (not composite sport:subType keys)
  // because the backend filters on a.sport_type directly.
  const selectedSet = useMemo(() => new Set(selected), [selected])

  function toggleSport(sportType: string) {
    if (selectedSet.has(sportType)) {
      onChange(selected.filter(s => s !== sportType))
    } else {
      onChange([...selected, sportType])
    }
  }

  function toggleParent(parent: SportOption) {
    if (!parent.children) return
    const childValues = parent.children.map(c => c.value)
    const unique = [...new Set(childValues)]
    const allSelected = unique.every(v => selectedSet.has(v))
    if (allSelected) {
      onChange(selected.filter(s => !unique.includes(s)))
    } else {
      const toAdd = unique.filter(v => !selectedSet.has(v))
      onChange([...selected, ...toAdd])
    }
  }

  function isParentChecked(parent: SportOption): boolean {
    if (!parent.children) return false
    const childValues = [...new Set(parent.children.map(c => c.value))]
    return childValues.every(v => selectedSet.has(v))
  }

  function isParentIndeterminate(parent: SportOption): boolean {
    if (!parent.children) return false
    const childValues = [...new Set(parent.children.map(c => c.value))]
    const count = childValues.filter(v => selectedSet.has(v)).length
    return count > 0 && count < childValues.length
  }

  function toggleGroup(groupKey: string) {
    setExpandedGroups(prev => {
      const next = new Set(prev)
      if (next.has(groupKey)) next.delete(groupKey)
      else next.add(groupKey)
      return next
    })
  }

  const sportSearchLower = sportSearch.toLowerCase().trim()
  function matchesSportSearch(opt: SportOption): boolean {
    if (!sportSearchLower) return true
    if (opt.label.toLowerCase().includes(sportSearchLower)) return true
    if (opt.children) return opt.children.some(c => c.label.toLowerCase().includes(sportSearchLower))
    return false
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportarten</span>
        <button
          onClick={() => {
            if (selected.length > 0) onChange([])
            else {
              const all: string[] = []
              for (const opt of visibleOptions) {
                if (opt.children) {
                  for (const c of opt.children) {
                    if (!all.includes(c.value)) all.push(c.value)
                  }
                } else {
                  if (!all.includes(opt.value)) all.push(opt.value)
                }
              }
              onChange(all)
            }
          }}
          className="text-xs text-[var(--color-accent-DEFAULT)] hover:text-[var(--color-accent-600)] cursor-pointer"
        >
          {selected.length > 0 ? 'Keine' : 'Alle'}
        </button>
      </div>
      <input
        type="text"
        value={sportSearch}
        onChange={e => setSportSearch(e.target.value)}
        placeholder="Sportart suchen..."
        className="w-full mb-2 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5
                   text-neutral-800 dark:text-neutral-200 placeholder:text-neutral-400 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
      />
      <div className="space-y-1.5 max-h-48 overflow-y-auto">
        {visibleOptions.filter(matchesSportSearch).map(opt => (
          <div key={opt.groupKey ?? opt.value}>
            {opt.children && opt.groupKey ? (
              <div>
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={isParentChecked(opt)}
                    ref={el => { if (el) el.indeterminate = isParentIndeterminate(opt) }}
                    onChange={() => toggleParent(opt)}
                    className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-[var(--color-accent-DEFAULT)] focus:ring-[var(--color-accent-DEFAULT)] focus:ring-1"
                  />
                  <button
                    onClick={() => toggleGroup(opt.groupKey!)}
                    className="flex items-center gap-1 text-sm text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100 cursor-pointer"
                  >
                    <svg
                      className={`w-3 h-3 text-neutral-400 transition-transform ${expandedGroups.has(opt.groupKey!) ? '' : '-rotate-90'}`}
                      viewBox="0 0 20 20" fill="currentColor"
                    >
                      <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                    </svg>
                    {opt.label}
                  </button>
                </div>
                {(expandedGroups.has(opt.groupKey!) || sportSearchLower) && (
                  <div className="ml-6 mt-1 space-y-1">
                    {opt.children
                      .filter(c => !sportSearchLower || c.label.toLowerCase().includes(sportSearchLower))
                      .map(child => (
                        <label key={getOptionKey(child)} className="flex items-center gap-2 cursor-pointer group">
                          <input
                            type="checkbox"
                            checked={selectedSet.has(child.value)}
                            onChange={() => toggleSport(child.value)}
                            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-[var(--color-accent-DEFAULT)] focus:ring-[var(--color-accent-DEFAULT)] focus:ring-1"
                          />
                          <span className="text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                            {child.label}
                          </span>
                        </label>
                      ))}
                  </div>
                )}
              </div>
            ) : (
              <label className="flex items-center gap-2 cursor-pointer group">
                <input
                  type="checkbox"
                  checked={selectedSet.has(opt.value)}
                  onChange={() => toggleSport(opt.value)}
                  className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-[var(--color-accent-DEFAULT)] focus:ring-[var(--color-accent-DEFAULT)] focus:ring-1"
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                  {opt.label}
                </span>
              </label>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

// ── FilterPanel ───────────────────────────────────────────────────────────────

interface FilterPanelProps {
  filters: AnalyticsFilterState
  onFilterChange: (updates: Partial<AnalyticsFilterState>) => void
  onReset: () => void
  onClose: () => void
}

const SEASON_OPTIONS = [
  { value: 'fruehling', label: 'Frühling' },
  { value: 'sommer', label: 'Sommer' },
  { value: 'herbst', label: 'Herbst' },
  { value: 'winter', label: 'Winter' },
]

const WEEKDAY_OPTIONS = [
  { value: 1, label: 'Mo' },
  { value: 2, label: 'Di' },
  { value: 3, label: 'Mi' },
  { value: 4, label: 'Do' },
  { value: 5, label: 'Fr' },
  { value: 6, label: 'Sa' },
  { value: 0, label: 'So' },
]

function FilterPanel({ filters, onFilterChange, onReset, onClose }: FilterPanelProps) {
  const [presets, setPresets] = useState<FilterPreset[]>(getPresets)
  const [presetName, setPresetName] = useState('')
  const [showSavePreset, setShowSavePreset] = useState(false)

  // Local state for tri-state tag/gear filters (name+color are resolved by the child components)
  const [tagFilters, setTagFilters] = useState<TagFilter[]>(() => [
    ...filters.includeTags.map(id => ({ id, name: id, color: '#9ca3af', state: 'include' as const })),
    ...filters.excludeTags.map(id => ({ id, name: id, color: '#9ca3af', state: 'exclude' as const })),
  ])
  const [gearFilters, setGearFilters] = useState<GearFilter[]>(() => [
    ...filters.includeGear.map(id => ({ id, name: id, type: '', state: 'include' as const })),
    ...filters.excludeGear.map(id => ({ id, name: id, type: '', state: 'exclude' as const })),
  ])

  function handleTagFiltersChange(newFilters: TagFilter[]) {
    setTagFilters(newFilters)
    onFilterChange({
      includeTags: newFilters.filter(f => f.state === 'include').map(f => f.id),
      excludeTags: newFilters.filter(f => f.state === 'exclude').map(f => f.id),
    })
  }

  function handleGearFiltersChange(newFilters: GearFilter[]) {
    setGearFilters(newFilters)
    onFilterChange({
      includeGear: newFilters.filter(f => f.state === 'include').map(f => f.id),
      excludeGear: newFilters.filter(f => f.state === 'exclude').map(f => f.id),
    })
  }

  const handleSavePreset = () => {
    if (!presetName.trim()) return
    savePreset({ name: presetName.trim(), filters })
    setPresets(getPresets())
    setPresetName('')
    setShowSavePreset(false)
  }

  const handleDeletePreset = (name: string) => {
    deletePreset(name)
    setPresets(getPresets())
  }

  const applyDatePreset = (days: number | null) => {
    if (days === null) {
      onFilterChange({ from: '', to: '' })
    } else {
      const to = new Date()
      const from = new Date()
      from.setDate(from.getDate() - days)
      onFilterChange({ from: toISODate(from), to: toISODate(to) })
    }
  }

  const toggleSeason = (season: string) => {
    const current = filters.seasons
    if (current.includes(season)) {
      onFilterChange({ seasons: current.filter(s => s !== season) })
    } else {
      onFilterChange({ seasons: [...current, season] })
    }
  }

  const toggleWeekday = (day: number) => {
    const current = filters.weekdays
    if (current.includes(day)) {
      onFilterChange({ weekdays: current.filter(d => d !== day) })
    } else {
      onFilterChange({ weekdays: [...current, day] })
    }
  }

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 shadow-lg p-4 space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">Filter</h3>
        <div className="flex items-center gap-2">
          <button
            onClick={onReset}
            className="text-xs px-2.5 py-1 rounded-md border border-neutral-200 dark:border-neutral-700
                       text-neutral-500 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            Zurücksetzen
          </button>
          <button
            onClick={onClose}
            className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors p-1"
            aria-label="Filter schließen"
          >
            ✕
          </button>
        </div>
      </div>

      {/* Presets */}
      {presets.length > 0 && (
        <div>
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1.5">Gespeicherte Filter</div>
          <div className="flex flex-wrap gap-1.5">
            {presets.map(p => (
              <div key={p.name} className="flex items-center gap-1 rounded-full bg-neutral-100 dark:bg-neutral-800 pl-2.5 pr-1 py-0.5">
                <button
                  onClick={() => onFilterChange(p.filters)}
                  className="text-xs text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100"
                >
                  {p.name}
                </button>
                <button
                  onClick={() => handleDeletePreset(p.name)}
                  className="text-neutral-400 hover:text-rose-500 transition-colors text-xs leading-none"
                  aria-label="Preset löschen"
                >
                  ✕
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {/* Date range */}
        <div className="space-y-2">
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Zeitraum</div>
          <div className="flex flex-wrap gap-1">
            {DATE_PRESETS.map(p => (
              <button
                key={p.label}
                onClick={() => applyDatePreset(p.days)}
                className="text-xs px-2 py-1 rounded-md border border-neutral-200 dark:border-neutral-700
                           text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
              >
                {p.label}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-1.5">
            <input
              type="date"
              value={filters.from}
              onChange={e => onFilterChange({ from: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
            <span className="text-xs text-neutral-400">–</span>
            <input
              type="date"
              value={filters.to}
              onChange={e => onFilterChange({ to: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
          </div>
        </div>

        {/* Distance & elevation */}
        <div className="space-y-2">
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Distanz (km)</div>
          <div className="flex items-center gap-1.5">
            <input
              type="number"
              placeholder="Von"
              value={filters.distMin}
              onChange={e => onFilterChange({ distMin: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
            <span className="text-xs text-neutral-400">–</span>
            <input
              type="number"
              placeholder="Bis"
              value={filters.distMax}
              onChange={e => onFilterChange({ distMax: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
          </div>
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mt-2">Höhenmeter (m)</div>
          <div className="flex items-center gap-1.5">
            <input
              type="number"
              placeholder="Von"
              value={filters.elevMin}
              onChange={e => onFilterChange({ elevMin: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
            <span className="text-xs text-neutral-400">–</span>
            <input
              type="number"
              placeholder="Bis"
              value={filters.elevMax}
              onChange={e => onFilterChange({ elevMax: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
          </div>
        </div>

        {/* HR & Power */}
        <div className="space-y-2">
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Herzfrequenz (bpm)</div>
          <div className="flex items-center gap-1.5">
            <input
              type="number"
              placeholder="Von"
              value={filters.hrMin}
              onChange={e => onFilterChange({ hrMin: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
            <span className="text-xs text-neutral-400">–</span>
            <input
              type="number"
              placeholder="Bis"
              value={filters.hrMax}
              onChange={e => onFilterChange({ hrMax: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
          </div>
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mt-2">Leistung (W)</div>
          <div className="flex items-center gap-1.5">
            <input
              type="number"
              placeholder="Von"
              value={filters.powerMin}
              onChange={e => onFilterChange({ powerMin: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
            <span className="text-xs text-neutral-400">–</span>
            <input
              type="number"
              placeholder="Bis"
              value={filters.powerMax}
              onChange={e => onFilterChange({ powerMax: e.target.value })}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
            />
          </div>
        </div>
      </div>

      {/* Seasons & Weekdays — full width row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Jahreszeit</div>
          <div className="flex gap-1.5">
            {SEASON_OPTIONS.map(s => (
              <button
                key={s.value}
                onClick={() => toggleSeason(s.value)}
                className={`text-xs px-2.5 py-1 rounded-md border transition-colors ${
                  filters.seasons.includes(s.value)
                    ? 'bg-[var(--color-accent-DEFAULT)] text-white border-transparent font-medium'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                }`}
              >
                {s.label}
              </button>
            ))}
          </div>
        </div>
        <div className="space-y-1.5">
          <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Wochentag</div>
          <div className="flex gap-1">
            {WEEKDAY_OPTIONS.map(d => (
              <button
                key={d.value}
                onClick={() => toggleWeekday(d.value)}
                className={`text-xs w-7 h-7 rounded-md border transition-colors font-medium ${
                  filters.weekdays.includes(d.value)
                    ? 'bg-[var(--color-accent-DEFAULT)] text-white border-transparent'
                    : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
                }`}
              >
                {d.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Activity flags */}
      <div className="space-y-1.5">
        <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Aktivitätstyp</div>
        <div className="flex gap-2">
          <button
            onClick={() => onFilterChange({ isCommute: filters.isCommute === 'true' ? '' : 'true' })}
            className={`text-xs px-3 py-1 rounded-md border transition-colors ${
              filters.isCommute === 'true'
                ? 'bg-blue-500 text-white border-transparent font-medium'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            Pendeln
          </button>
          <button
            onClick={() => onFilterChange({ isRace: filters.isRace === 'true' ? '' : 'true' })}
            className={`text-xs px-3 py-1 rounded-md border transition-colors ${
              filters.isRace === 'true'
                ? 'bg-amber-500 text-white border-transparent font-medium'
                : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            Wettkampf
          </button>
        </div>
      </div>

      {/* Sport Type Selector */}
      <div className="space-y-1.5">
        <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</div>
        <SportTypeSelector
          selected={filters.sportTypes}
          onChange={types => onFilterChange({ sportTypes: types })}
        />
      </div>

      {/* Tag Filter */}
      <TagFilterInput
        value={tagFilters}
        onChange={handleTagFiltersChange}
      />

      {/* Gear Filter */}
      <GearFilterInput
        value={gearFilters}
        onChange={handleGearFiltersChange}
      />

      {/* Save preset */}
      <div className="pt-2 border-t border-neutral-100 dark:border-neutral-800">
        {showSavePreset ? (
          <div className="flex items-center gap-2">
            <input
              type="text"
              placeholder="Filter-Name…"
              value={presetName}
              onChange={e => setPresetName(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleSavePreset()}
              className="flex-1 text-xs rounded-md border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800
                         text-neutral-900 dark:text-neutral-100 px-2 py-1 focus:outline-none focus:ring-1 focus:ring-[var(--color-accent-DEFAULT)]"
              autoFocus
            />
            <button
              onClick={handleSavePreset}
              className="text-xs px-2.5 py-1 rounded-md bg-[var(--color-accent-DEFAULT)] text-white font-medium hover:opacity-90 transition-opacity"
            >
              Speichern
            </button>
            <button
              onClick={() => setShowSavePreset(false)}
              className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
            >
              Abbrechen
            </button>
          </div>
        ) : (
          <button
            onClick={() => setShowSavePreset(true)}
            className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 transition-colors"
          >
            + Als Filter-Preset speichern
          </button>
        )}
      </div>
    </div>
  )
}

// ── AnalyticsPage ─────────────────────────────────────────────────────────────

export default function AnalyticsPage() {
  const { filters, setFilters, toQueryString } = useAnalyticsFilters()
  const [filterOpen, setFilterOpen] = useState(false)

  const filterCount = activeFilterCount(filters)

  const resetFilters = () => {
    setFilters({
      from: '', to: '',
      sportTypes: [], tags: [],
      weather: '', tempMin: '', tempMax: '',
      timeOfDay: [], intensity: [],
      distMin: '', distMax: '',
      elevMin: '', elevMax: '',
      gearId: '', isCommute: '', isRace: '',
      paceMin: '', paceMax: '',
      countries: [], seasons: [], weekdays: [],
      hrMin: '', hrMax: '', powerMin: '', powerMax: '',
      includeTags: [], excludeTags: [],
      includeGear: [], excludeGear: [],
    })
  }

  // Build a short human-readable active filter summary
  const filterSummary: string[] = []
  if (filters.from || filters.to) {
    const parts: string[] = []
    if (filters.from) parts.push(filters.from)
    if (filters.to) parts.push(filters.to)
    filterSummary.push(parts.join(' – '))
  }
  if (filters.sportTypes.length > 0) {
    filterSummary.push(filters.sportTypes.length === 1 ? filters.sportTypes[0] : `${filters.sportTypes.length} Sportarten`)
  }
  if (filters.isCommute === 'true') filterSummary.push('Pendeln')
  if (filters.isRace === 'true') filterSummary.push('Wettkampf')

  void toQueryString // used by child components via the hook

  return (
    <div className="space-y-4">
      {/* Tab Navigation + Filter Toggle */}
      <div className="flex items-center gap-2 flex-wrap">
        <div className="flex gap-1 border-b border-neutral-200 dark:border-neutral-800 flex-1 min-w-0">
          {tabs.map(tab => (
            <NavLink
              key={tab.to}
              to={tab.to}
              className={({ isActive }) =>
                `px-4 py-2 text-sm font-medium border-b-2 transition-colors whitespace-nowrap ${
                  isActive
                    ? 'border-[var(--color-accent-DEFAULT)] text-[var(--color-accent-DEFAULT)]'
                    : 'border-transparent text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300'
                }`
              }
            >
              {tab.label}
            </NavLink>
          ))}
        </div>
        {/* Filter toggle button */}
        <button
          onClick={() => setFilterOpen(o => !o)}
          className={`flex items-center gap-1.5 text-xs px-3 py-1.5 rounded-lg border transition-colors whitespace-nowrap ${
            filterCount > 0
              ? 'border-[var(--color-accent-DEFAULT)] bg-[var(--color-accent-DEFAULT)]/10 text-[var(--color-accent-DEFAULT)] font-medium'
              : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
          }`}
          aria-expanded={filterOpen}
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3 4h18M7 8h10M10 12h4" />
          </svg>
          Filter
          {filterCount > 0 && (
            <span className="rounded-full bg-[var(--color-accent-DEFAULT)] text-white text-xs font-bold w-4 h-4 flex items-center justify-center leading-none">
              {filterCount}
            </span>
          )}
        </button>
      </div>

      {/* Active filter chips summary */}
      {filterCount > 0 && !filterOpen && filterSummary.length > 0 && (
        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-xs text-neutral-500 dark:text-neutral-400">Aktiv:</span>
          {filterSummary.map((s, i) => (
            <span
              key={i}
              className="text-xs px-2 py-0.5 rounded-full bg-[var(--color-accent-DEFAULT)]/10 text-[var(--color-accent-DEFAULT)] border border-[var(--color-accent-DEFAULT)]/20"
            >
              {s}
            </span>
          ))}
          <button
            onClick={resetFilters}
            className="text-xs text-neutral-400 hover:text-rose-500 transition-colors"
          >
            Alles löschen
          </button>
        </div>
      )}

      {/* Collapsible filter panel */}
      {filterOpen && (
        <FilterPanel
          filters={filters}
          onFilterChange={setFilters}
          onReset={resetFilters}
          onClose={() => setFilterOpen(false)}
        />
      )}

      {/* Tab Content */}
      <Outlet />
    </div>
  )
}
