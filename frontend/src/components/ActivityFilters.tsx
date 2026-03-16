import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { SportOption } from '@/lib/sportOptions'
import { generateActivityFilterOptions, getOptionKey, getAllSportKeys, filterToExistingSports } from '@/lib/sportOptions'
import { fetchSportTypeStats } from '@/api/stats'
import { sportLabel } from '@/lib/utils'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import TagFilterInput from '@/components/TagFilterInput'
import GearFilterInput, { gearTypeIcon } from '@/components/GearFilterInput'
import type { TagFilter, TagFilterState } from '@/components/TagFilterInput'
import type { GearFilter, GearFilterState } from '@/components/GearFilterInput'

const ACTIVITY_SPORT_OPTIONS = generateActivityFilterOptions()
const ALL_SPORT_KEYS = getAllSportKeys(ACTIVITY_SPORT_OPTIONS)

export interface ActivityFilterState {
  sportTypes: Set<string>
  dateFrom: string
  dateTo: string
  isIndoor: boolean | null
  hasGPS: boolean | null
  isEbike: boolean | null
  gearId: string
  tagFilters: TagFilter[]
  gearFilters: GearFilter[]
  search: string
}

export const EMPTY_FILTERS: ActivityFilterState = {
  sportTypes: new Set<string>(),
  dateFrom: '',
  dateTo: '',
  isIndoor: null,
  hasGPS: null,
  isEbike: null,
  gearId: '',
  tagFilters: [],
  gearFilters: [],
  search: '',
}

/** Serialize filter state to URL search params */
export function filtersToParams(f: ActivityFilterState): URLSearchParams {
  const p = new URLSearchParams()
  if (f.sportTypes.size > 0 && f.sportTypes.size < ALL_SPORT_KEYS.length) {
    p.set('sports', [...f.sportTypes].join(','))
  }
  if (f.dateFrom) p.set('from', f.dateFrom)
  if (f.dateTo) p.set('to', f.dateTo)
  if (f.isIndoor !== null) p.set('indoor', String(f.isIndoor))
  if (f.hasGPS !== null) p.set('gps', String(f.hasGPS))
  if (f.isEbike !== null) p.set('ebike', String(f.isEbike))
  if (f.gearId) p.set('gear', f.gearId)
  if (f.search) p.set('q', f.search)
  const includeTags = f.tagFilters.filter(tf => tf.state === 'include').map(tf => tf.id)
  const excludeTags = f.tagFilters.filter(tf => tf.state === 'exclude').map(tf => tf.id)
  const includeGear = f.gearFilters.filter(gf => gf.state === 'include').map(gf => gf.id)
  const excludeGear = f.gearFilters.filter(gf => gf.state === 'exclude').map(gf => gf.id)
  if (includeTags.length) p.set('include_tags', includeTags.join(','))
  if (excludeTags.length) p.set('exclude_tags', excludeTags.join(','))
  if (includeGear.length) p.set('include_gear', includeGear.join(','))
  if (excludeGear.length) p.set('exclude_gear', excludeGear.join(','))
  return p
}

/** Deserialize URL search params to filter state */
export function paramsToFilters(p: URLSearchParams): ActivityFilterState {
  const f: ActivityFilterState = { ...EMPTY_FILTERS }
  const sports = p.get('sports')
  if (sports) f.sportTypes = new Set(sports.split(','))
  f.dateFrom = p.get('from') ?? ''
  f.dateTo = p.get('to') ?? ''
  const indoor = p.get('indoor')
  if (indoor !== null) f.isIndoor = indoor === 'true'
  const gps = p.get('gps')
  if (gps !== null) f.hasGPS = gps === 'true'
  const ebike = p.get('ebike')
  if (ebike !== null) f.isEbike = ebike === 'true'
  f.gearId = p.get('gear') ?? ''
  f.search = p.get('q') ?? ''
  // tagFilters and gearFilters require full objects and cannot be restored from URL IDs alone
  f.tagFilters = []
  f.gearFilters = []
  return f
}

/** Convert ActivityFilterState to API query params */
export function filtersToApiParams(f: ActivityFilterState) {
  const includeTags = f.tagFilters.filter(tf => tf.state === 'include').map(tf => tf.id)
  const excludeTags = f.tagFilters.filter(tf => tf.state === 'exclude').map(tf => tf.id)
  const includeGear = f.gearFilters.filter(gf => gf.state === 'include').map(gf => gf.id)
  const excludeGear = f.gearFilters.filter(gf => gf.state === 'exclude').map(gf => gf.id)
  const base = {
    dateFrom: f.dateFrom,
    dateTo: f.dateTo,
    isIndoor: f.isIndoor,
    hasGPS: f.hasGPS,
    isEbike: f.isEbike,
    gearId: f.gearId,
    search: f.search,
    includeTags: includeTags.length ? includeTags : undefined,
    excludeTags: excludeTags.length ? excludeTags : undefined,
    includeGear: includeGear.length ? includeGear : undefined,
    excludeGear: excludeGear.length ? excludeGear : undefined,
  }
  if (f.sportTypes.size === 0 || f.sportTypes.size === ALL_SPORT_KEYS.length) {
    return { sportTypes: undefined, ...base }
  }
  return { sportTypes: [...f.sportTypes], ...base }
}

interface Props {
  filters: ActivityFilterState
  onChange: (filters: ActivityFilterState) => void
}

export default function ActivityFilters({ filters, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [sportSearch, setSportSearch] = useState('')
  const popoverRef = useRef<HTMLDivElement>(null)

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
      return filterToExistingSports(ACTIVITY_SPORT_OPTIONS, existingSportTypes)
    }
    return ACTIVITY_SPORT_OPTIONS
  }, [hideUnused, existingSportTypes])
  const visibleKeys = useMemo(() => getAllSportKeys(visibleOptions), [visibleOptions])

  // Close popover on outside click
  useEffect(() => {
    if (!open) return
    function handleClick(e: MouseEvent) {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  // Count active filters
  const activeCount = countActive(filters)

  function countActive(f: ActivityFilterState): number {
    let n = 0
    if (f.sportTypes.size > 0 && !visibleKeys.every(k => f.sportTypes.has(k))) n++
    if (f.dateFrom) n++
    if (f.dateTo) n++
    if (f.isIndoor !== null) n++
    if (f.hasGPS !== null) n++
    if (f.isEbike !== null) n++
    if (f.gearId) n++
    if (f.tagFilters.length) n++
    if (f.gearFilters.length) n++
    return n
  }

  // Sport toggle helpers (same pattern as MapFilters)
  function toggleSport(key: string) {
    const next = new Set(filters.sportTypes)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    onChange({ ...filters, sportTypes: next })
  }

  function toggleParent(parent: SportOption) {
    if (!parent.children) return
    const childKeys = parent.children.map(getOptionKey)
    const allSelected = childKeys.every(k => filters.sportTypes.has(k))
    const next = new Set(filters.sportTypes)
    if (allSelected) childKeys.forEach(k => next.delete(k))
    else childKeys.forEach(k => next.add(k))
    onChange({ ...filters, sportTypes: next })
  }

  function isParentChecked(parent: SportOption): boolean {
    if (!parent.children) return false
    return parent.children.every(c => filters.sportTypes.has(getOptionKey(c)))
  }

  function isParentIndeterminate(parent: SportOption): boolean {
    if (!parent.children) return false
    const checked = parent.children.filter(c => filters.sportTypes.has(getOptionKey(c))).length
    return checked > 0 && checked < parent.children.length
  }

  const allSportsSelected = filters.sportTypes.size === 0 || visibleKeys.every(k => filters.sportTypes.has(k))

  function toggleAllSports() {
    if (allSportsSelected) {
      onChange({ ...filters, sportTypes: new Set<string>() })
    } else {
      onChange({ ...filters, sportTypes: new Set(visibleKeys) })
    }
  }

  function toggleGroup(groupKey: string) {
    setExpandedGroups(prev => {
      const next = new Set(prev)
      if (next.has(groupKey)) next.delete(groupKey)
      else next.add(groupKey)
      return next
    })
  }

  function resetFilters() {
    onChange({ ...EMPTY_FILTERS, search: filters.search })
    setSportSearch('')
  }

  // Filter sport options by search
  const sportSearchLower = sportSearch.toLowerCase().trim()
  function matchesSportSearch(opt: SportOption): boolean {
    if (!sportSearchLower) return true
    if (opt.label.toLowerCase().includes(sportSearchLower)) return true
    if (opt.children) return opt.children.some(c => c.label.toLowerCase().includes(sportSearchLower))
    return false
  }

  // Active filter chips
  const chips: { label: string; onRemove: () => void }[] = []

  if (filters.sportTypes.size > 0 && filters.sportTypes.size < ALL_SPORT_KEYS.length) {
    const labels = [...filters.sportTypes].map(k => {
      const [sport, sub] = k.split(':')
      return sportLabel(sport, sub || undefined)
    })
    const display = labels.length <= 3 ? labels.join(', ') : `${labels.length} Sportarten`
    chips.push({ label: display, onRemove: () => onChange({ ...filters, sportTypes: new Set<string>() }) })
  }
  if (filters.dateFrom) {
    chips.push({ label: `Ab ${filters.dateFrom}`, onRemove: () => onChange({ ...filters, dateFrom: '' }) })
  }
  if (filters.dateTo) {
    chips.push({ label: `Bis ${filters.dateTo}`, onRemove: () => onChange({ ...filters, dateTo: '' }) })
  }
  if (filters.isIndoor !== null) {
    chips.push({ label: filters.isIndoor ? 'Indoor' : 'Outdoor', onRemove: () => onChange({ ...filters, isIndoor: null }) })
  }
  if (filters.hasGPS !== null) {
    chips.push({ label: filters.hasGPS ? 'Mit GPS' : 'Ohne GPS', onRemove: () => onChange({ ...filters, hasGPS: null }) })
  }
  if (filters.isEbike !== null) {
    chips.push({ label: filters.isEbike ? 'Nur E-Bike' : 'Ohne E-Bike', onRemove: () => onChange({ ...filters, isEbike: null }) })
  }
  if (filters.gearId) {
    chips.push({ label: 'Ausrüstung', onRemove: () => onChange({ ...filters, gearId: '' }) })
  }
  // Tag and gear chips are rendered separately with tri-state cycling

  return (
    <div className="relative" ref={popoverRef}>
      {/* Trigger button */}
      <button
        onClick={() => setOpen(!open)}
        className={`inline-flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border transition-colors cursor-pointer ${
          activeCount > 0
            ? 'border-accent-400 bg-orange-50 dark:bg-orange-950/30 text-accent-600 dark:text-orange-400'
            : 'border-neutral-200 dark:border-neutral-700 text-neutral-600 dark:text-neutral-300 hover:bg-neutral-50 dark:hover:bg-neutral-800'
        }`}
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
        </svg>
        Filter
        {activeCount > 0 && (
          <span className="inline-flex items-center justify-center w-4 h-4 text-[10px] font-bold rounded-full bg-accent-DEFAULT text-white">
            {activeCount}
          </span>
        )}
      </button>

      {/* Popover */}
      {open && (
        <div
          onMouseDown={e => e.stopPropagation()}
          className="absolute right-0 top-full mt-1 z-20 w-80 bg-white dark:bg-neutral-900 rounded-xl shadow-lg border border-neutral-200 dark:border-neutral-800"
          style={{ maxHeight: 'calc(100vh - 80px)' }}
        >
          <div className="overflow-y-auto p-4 space-y-4" style={{ maxHeight: 'calc(100vh - 82px)' }}>

            {/* Sportarten */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportarten</span>
                <button
                  onClick={toggleAllSports}
                  className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
                >
                  {allSportsSelected ? 'Keine' : 'Alle'}
                </button>
              </div>
              <input
                type="text"
                value={sportSearch}
                onChange={e => setSportSearch(e.target.value)}
                placeholder="Sportart suchen..."
                className="w-full mb-2 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-800 dark:text-neutral-200 placeholder:text-neutral-400 focus:outline-none focus:ring-1 focus:ring-accent-400"
              />
              <div className="space-y-1.5 max-h-48 overflow-y-auto">
                {visibleOptions.filter(matchesSportSearch).map(opt => (
                  <div key={opt.label}>
                    {opt.children && opt.groupKey ? (
                      <div>
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            checked={isParentChecked(opt)}
                            ref={el => { if (el) el.indeterminate = isParentIndeterminate(opt) }}
                            onChange={() => toggleParent(opt)}
                            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
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
                              .map(child => {
                                const key = getOptionKey(child)
                                return (
                                  <label key={key} className="flex items-center gap-2 cursor-pointer group">
                                    <input
                                      type="checkbox"
                                      checked={filters.sportTypes.has(key)}
                                      onChange={() => toggleSport(key)}
                                      className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
                                    />
                                    <span className="text-sm text-neutral-600 dark:text-neutral-400 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                                      {child.label}
                                    </span>
                                  </label>
                                )
                              })}
                          </div>
                        )}
                      </div>
                    ) : (
                      <label className="flex items-center gap-2 cursor-pointer group">
                        <input
                          type="checkbox"
                          checked={filters.sportTypes.has(opt.value)}
                          onChange={() => toggleSport(opt.value)}
                          className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
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

            {/* Zeitraum */}
            <div>
              <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">Zeitraum</div>
              <div className="grid grid-cols-2 gap-2">
                <div>
                  <label className="text-[10px] text-neutral-400 mb-0.5 block">Von</label>
                  <input
                    type="date"
                    value={filters.dateFrom}
                    onChange={e => onChange({ ...filters, dateFrom: e.target.value })}
                    className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-accent-400"
                  />
                </div>
                <div>
                  <label className="text-[10px] text-neutral-400 mb-0.5 block">Bis</label>
                  <input
                    type="date"
                    value={filters.dateTo}
                    onChange={e => onChange({ ...filters, dateTo: e.target.value })}
                    className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-sm px-2.5 py-1.5 text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-accent-400"
                  />
                </div>
              </div>
            </div>

            {/* Indoor / GPS / E-Bike */}
            <div className="grid grid-cols-3 gap-3">
              <div>
                <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">Indoor</div>
                <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                  {([
                    [null, 'Alle'],
                    [true, 'Ja'],
                    [false, 'Nein'],
                  ] as [boolean | null, string][]).map(([val, label]) => (
                    <button
                      key={String(val)}
                      onClick={() => onChange({ ...filters, isIndoor: val })}
                      className={`flex-1 px-2 py-1.5 transition-colors cursor-pointer ${
                        filters.isIndoor === val
                          ? 'bg-accent-DEFAULT text-white font-medium'
                          : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
              <div>
                <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">GPS</div>
                <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                  {([
                    [null, 'Alle'],
                    [true, 'Ja'],
                    [false, 'Nein'],
                  ] as [boolean | null, string][]).map(([val, label]) => (
                    <button
                      key={String(val)}
                      onClick={() => onChange({ ...filters, hasGPS: val })}
                      className={`flex-1 px-2 py-1.5 transition-colors cursor-pointer ${
                        filters.hasGPS === val
                          ? 'bg-accent-DEFAULT text-white font-medium'
                          : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
              <div>
                <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">E-Bike</div>
                <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                  {([
                    [null, 'Alle'],
                    [true, 'Nur'],
                    [false, 'Ohne'],
                  ] as [boolean | null, string][]).map(([val, label]) => (
                    <button
                      key={String(val)}
                      onClick={() => onChange({ ...filters, isEbike: val })}
                      className={`flex-1 px-2 py-1.5 transition-colors cursor-pointer ${
                        filters.isEbike === val
                          ? 'bg-accent-DEFAULT text-white font-medium'
                          : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                      }`}
                    >
                      {label}
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Tags (tri-state filter) */}
            <div>
              <TagFilterInput
                value={filters.tagFilters}
                onChange={(v: TagFilter[]) => onChange({ ...filters, tagFilters: v })}
              />
            </div>

            {/* Gear (tri-state filter) */}
            <div>
              <GearFilterInput
                value={filters.gearFilters}
                onChange={(v: GearFilter[]) => onChange({ ...filters, gearFilters: v })}
              />
            </div>

            {/* Reset */}
            {activeCount > 0 && (
              <button
                onClick={resetFilters}
                className="w-full text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer py-1"
              >
                Filter zurücksetzen
              </button>
            )}
          </div>
        </div>
      )}

      {/* Active filter chips (rendered inline below button) */}
      {(chips.length > 0 || filters.tagFilters.length > 0 || filters.gearFilters.length > 0) && (
        <div className="flex flex-wrap gap-1.5 mt-2">
          {chips.map(chip => (
            <span
              key={chip.label}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-[11px] rounded-full bg-accent-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-300 border border-orange-200 dark:border-orange-800"
            >
              {chip.label}
              <button
                onClick={chip.onRemove}
                className="hover:text-orange-900 dark:hover:text-orange-100 cursor-pointer"
              >
                &times;
              </button>
            </span>
          ))}
          {filters.tagFilters.map(tf => (
            <button
              key={`tag-${tf.id}`}
              onClick={() => {
                if (tf.state === 'include') {
                  onChange({ ...filters, tagFilters: filters.tagFilters.map(f => f.id === tf.id ? { ...f, state: 'exclude' as TagFilterState } : f) })
                } else {
                  onChange({ ...filters, tagFilters: filters.tagFilters.filter(f => f.id !== tf.id) })
                }
              }}
              className={`inline-flex items-center gap-1 px-2 py-0.5 text-[11px] rounded-full border cursor-pointer transition-colors ${
                tf.state === 'include'
                  ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                  : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through'
              }`}
            >
              <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: tf.color }} />
              {tf.name}
              <span className="opacity-60 text-[10px]">{tf.state === 'include' ? '\u2713' : '\u2715'}</span>
            </button>
          ))}
          {filters.gearFilters.map(gf => (
            <button
              key={`gear-${gf.id}`}
              onClick={() => {
                if (gf.state === 'include') {
                  onChange({ ...filters, gearFilters: filters.gearFilters.map(f => f.id === gf.id ? { ...f, state: 'exclude' as GearFilterState } : f) })
                } else {
                  onChange({ ...filters, gearFilters: filters.gearFilters.filter(f => f.id !== gf.id) })
                }
              }}
              className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] rounded-full border cursor-pointer transition-colors ${
                gf.state === 'include'
                  ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                  : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through'
              }`}
            >
              <span className="flex-shrink-0">{gearTypeIcon(gf.type)}</span>
              {gf.name}
              <span className="opacity-60 text-[10px]">{gf.state === 'include' ? '\u2713' : '\u2715'}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
