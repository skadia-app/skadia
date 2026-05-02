import { useRef, useState } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { generateActivityFilterOptions, getAllSportKeys } from '@/lib/sportOptions'
import { sportLabel } from '@/lib/sportTaxonomy'
import { Pill } from '@/components/ui/Pill'
import { SportFilter } from '@/components/sport/SportFilter'
import TagFilterInput from '@/components/TagFilterInput'
import GearFilterInput, { gearTypeIcon } from '@/components/GearFilterInput'
import type { TagFilter, TagFilterState } from '@/components/TagFilterInput'
import type { GearFilter, GearFilterState, DeviceFilter } from '@/components/GearFilterInput'

const ACTIVITY_SPORT_OPTIONS = generateActivityFilterOptions()
const ALL_SPORT_KEYS = getAllSportKeys(ACTIVITY_SPORT_OPTIONS)

export interface ActivityFilterState {
  sportTypes: Set<string> | null
  dateFrom: string
  dateTo: string
  isIndoor: boolean | null
  hasGPS: boolean | null
  isElectric: boolean | null
  isVirtual: boolean | null
  isCommute: boolean | null
  gearId: string
  tagFilters: TagFilter[]
  gearFilters: GearFilter[]
  deviceFilters: DeviceFilter[]
  search: string
}

export const EMPTY_FILTERS: ActivityFilterState = {
  sportTypes: null,
  dateFrom: '',
  dateTo: '',
  isIndoor: null,
  hasGPS: null,
  isElectric: null,
  isVirtual: null,
  isCommute: null,
  gearId: '',
  tagFilters: [],
  gearFilters: [],
  deviceFilters: [],
  search: '',
}

/** Serialize filter state to URL search params */
export function filtersToParams(f: ActivityFilterState): URLSearchParams {
  const p = new URLSearchParams()
  if (f.sportTypes !== null && f.sportTypes.size > 0 && f.sportTypes.size < ALL_SPORT_KEYS.length) {
    p.set('sports', [...f.sportTypes].join(','))
  }
  if (f.dateFrom) p.set('from', f.dateFrom)
  if (f.dateTo) p.set('to', f.dateTo)
  if (f.isIndoor !== null) p.set('indoor', String(f.isIndoor))
  if (f.hasGPS !== null) p.set('gps', String(f.hasGPS))
  if (f.isElectric !== null) p.set('ebike', String(f.isElectric))
  if (f.isVirtual !== null) p.set('virtual', String(f.isVirtual))
  if (f.isCommute !== null) p.set('commute', String(f.isCommute))
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
  const includeDevices = f.deviceFilters.filter(df => df.state === 'include').map(df => df.id)
  const excludeDevices = f.deviceFilters.filter(df => df.state === 'exclude').map(df => df.id)
  if (includeDevices.length) p.set('include_devices', includeDevices.join(','))
  if (excludeDevices.length) p.set('exclude_devices', excludeDevices.join(','))
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
  if (ebike !== null) f.isElectric = ebike === 'true'
  const virtual_ = p.get('virtual')
  if (virtual_ !== null) f.isVirtual = virtual_ === 'true'
  const commute = p.get('commute')
  if (commute !== null) f.isCommute = commute === 'true'
  f.gearId = p.get('gear') ?? ''
  f.search = p.get('q') ?? ''
  // tagFilters, gearFilters and deviceFilters require full objects and cannot be restored from URL IDs alone
  f.tagFilters = []
  f.gearFilters = []
  f.deviceFilters = []
  return f
}

/** Convert ActivityFilterState to API query params */
export function filtersToApiParams(f: ActivityFilterState) {
  const includeTags = f.tagFilters.filter(tf => tf.state === 'include').map(tf => tf.id)
  const excludeTags = f.tagFilters.filter(tf => tf.state === 'exclude').map(tf => tf.id)
  const includeGear = f.gearFilters.filter(gf => gf.state === 'include').map(gf => gf.id)
  const excludeGear = f.gearFilters.filter(gf => gf.state === 'exclude').map(gf => gf.id)
  const includeDevices = f.deviceFilters.filter(df => df.state === 'include').map(df => df.id)
  const excludeDevices = f.deviceFilters.filter(df => df.state === 'exclude').map(df => df.id)
  const base = {
    dateFrom: f.dateFrom,
    dateTo: f.dateTo,
    isIndoor: f.isIndoor,
    hasGPS: f.hasGPS,
    isElectric: f.isElectric,
    isVirtual: f.isVirtual,
    isCommute: f.isCommute,
    gearId: f.gearId,
    search: f.search,
    includeTags: includeTags.length ? includeTags : undefined,
    excludeTags: excludeTags.length ? excludeTags : undefined,
    includeGear: includeGear.length ? includeGear : undefined,
    excludeGear: excludeGear.length ? excludeGear : undefined,
    includeDevices: includeDevices.length ? includeDevices : undefined,
    excludeDevices: excludeDevices.length ? excludeDevices : undefined,
  }
  if (f.sportTypes === null || f.sportTypes.size === ALL_SPORT_KEYS.length) {
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
  const popoverRef = useRef<HTMLDivElement>(null)

  // Close popover on outside click
  useClickOutside(popoverRef, () => setOpen(false), open)

  // Count active filters
  const activeCount = countActive(filters)

  function countActive(f: ActivityFilterState): number {
    let n = 0
    if (f.sportTypes !== null && !ALL_SPORT_KEYS.every(k => f.sportTypes!.has(k))) n++
    if (f.dateFrom) n++
    if (f.dateTo) n++
    if (f.isIndoor !== null) n++
    if (f.hasGPS !== null) n++
    if (f.isElectric !== null) n++
    if (f.isVirtual !== null) n++
    if (f.isCommute !== null) n++
    if (f.gearId) n++
    if (f.tagFilters.length) n++
    if (f.gearFilters.length) n++
    if (f.deviceFilters.length) n++
    return n
  }

  function resetFilters() {
    onChange({ ...EMPTY_FILTERS, search: filters.search })
  }

  // Active filter chips
  const chips: { label: string; onRemove: () => void }[] = []

  if (filters.sportTypes !== null && filters.sportTypes.size < ALL_SPORT_KEYS.length) {
    const labels = [...filters.sportTypes].map(k => {
      const [sport, sub] = k.split(':')
      return sportLabel(sport, sub || undefined)
    })
    const display = labels.length <= 3 ? labels.join(', ') : `${labels.length} Sportarten`
    chips.push({ label: display, onRemove: () => onChange({ ...filters, sportTypes: null }) })
  }
  if (filters.dateFrom) {
    chips.push({ label: `Ab ${filters.dateFrom}`, onRemove: () => onChange({ ...filters, dateFrom: '' }) })
  }
  if (filters.dateTo) {
    chips.push({ label: `Bis ${filters.dateTo}`, onRemove: () => onChange({ ...filters, dateTo: '' }) })
  }
  if (filters.hasGPS !== null) {
    chips.push({ label: filters.hasGPS ? 'Mit GPS' : 'Ohne GPS', onRemove: () => onChange({ ...filters, hasGPS: null }) })
  }
  if (filters.isIndoor !== null) {
    chips.push({ label: filters.isIndoor ? 'Nur Indoor' : 'Ohne Indoor', onRemove: () => onChange({ ...filters, isIndoor: null }) })
  }
  if (filters.isElectric !== null) {
    chips.push({ label: filters.isElectric ? 'Nur E-Bike' : 'Ohne E-Bike', onRemove: () => onChange({ ...filters, isElectric: null }) })
  }
  if (filters.isVirtual !== null) {
    chips.push({ label: filters.isVirtual ? 'Nur Virtuell' : 'Ohne Virtuell', onRemove: () => onChange({ ...filters, isVirtual: null }) })
  }
  if (filters.isCommute !== null) {
    chips.push({ label: filters.isCommute ? 'Nur Pendeln' : 'Ohne Pendeln', onRemove: () => onChange({ ...filters, isCommute: null }) })
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

            <SportFilter
              preset="ACTIVITY_ALL"
              modifierKeys={['is_electric', 'is_indoor', 'is_virtual', 'is_commute']}
              value={{
                sportTypes: filters.sportTypes === null ? null : [...filters.sportTypes],
                modifiers: {
                  is_electric: filters.isElectric ?? undefined,
                  is_indoor: filters.isIndoor ?? undefined,
                  is_virtual: filters.isVirtual ?? undefined,
                  is_commute: filters.isCommute ?? undefined,
                },
              }}
              onChange={(next) => onChange({
                ...filters,
                sportTypes: next.sportTypes === null ? null : new Set(next.sportTypes),
                isElectric: next.modifiers?.is_electric ?? null,
                isIndoor: next.modifiers?.is_indoor ?? null,
                isVirtual: next.modifiers?.is_virtual ?? null,
                isCommute: next.modifiers?.is_commute ?? null,
              })}
            />

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

            {/* GPS */}
            <div>
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
            </div>

            {/* Tags (tri-state filter) */}
            <div>
              <TagFilterInput
                value={filters.tagFilters}
                onChange={(v: TagFilter[]) => onChange({ ...filters, tagFilters: v })}
              />
            </div>

            {/* Gear & Device (tri-state filter) */}
            <div>
              <GearFilterInput
                value={filters.gearFilters}
                onChange={(v: GearFilter[]) => onChange({ ...filters, gearFilters: v })}
                deviceValue={filters.deviceFilters}
                onDeviceChange={(v: DeviceFilter[]) => onChange({ ...filters, deviceFilters: v })}
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
      {(chips.length > 0 || filters.tagFilters.length > 0 || filters.gearFilters.length > 0 || filters.deviceFilters.length > 0) && (
        <div className="flex flex-wrap gap-1.5 mt-2">
          {chips.map(chip => (
            <Pill
              key={chip.label}
              size="sm"
              active={true}
              onClick={chip.onRemove}
              ariaLabel={`Filter entfernen: ${chip.label}`}
            >
              {chip.label}
              <span className="ml-1 opacity-60 text-xs">&times;</span>
            </Pill>
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
          {filters.deviceFilters.map(df => (
            <button
              key={`device-${df.id}`}
              onClick={() => {
                if (df.state === 'include') {
                  onChange({ ...filters, deviceFilters: filters.deviceFilters.map(f => f.id === df.id ? { ...f, state: 'exclude' as GearFilterState } : f) })
                } else {
                  onChange({ ...filters, deviceFilters: filters.deviceFilters.filter(f => f.id !== df.id) })
                }
              }}
              className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-[11px] rounded-full border cursor-pointer transition-colors ${
                df.state === 'include'
                  ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                  : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through'
              }`}
            >
              <span className="flex-shrink-0">{df.deviceType === 'recorder' ? '📡' : '📶'}</span>
              {df.name}
              <span className="opacity-60 text-[10px]">{df.state === 'include' ? '\u2713' : '\u2715'}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
