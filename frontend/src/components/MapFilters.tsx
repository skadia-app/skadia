import { memo, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useClickOutside } from '@/hooks/useClickOutside'
import { DayPicker } from 'react-day-picker'
import 'react-day-picker/style.css'
import type { DateRange } from 'react-day-picker'
import { MonthYearCaption } from '@/components/DateRangePicker'
import { de } from 'date-fns/locale'
import type { MapFilterState } from '@/pages/MapPage'
import type { SportOption } from '@/lib/sportOptions'
import { filterToExistingSports, getAllSportKeys, getOptionKey, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import { sportFilterOptionsForPreset } from '@/lib/sportFilterPresets'
import { fetchSportTypeStats } from '@/api/stats'
import ColorPicker from '@/components/ColorPicker'
import TagFilterInput from '@/components/TagFilterInput'
import GearFilterInput, { gearTypeIcon } from '@/components/GearFilterInput'
import type { TagFilter } from '@/components/TagFilterInput'
import type { GearFilter, GearFilterState } from '@/components/GearFilterInput'
import { toLocalISO } from '@/lib/dateUtils'
import { SportFilter } from '@/components/sport/SportFilter'
import { HeatmapApplicableModifierKeys } from '@/lib/generatedHeatmapApplicableModifiers'
import type { SegmentColorConfig, SegmentsOverlayFilters } from '@/components/map/SegmentsOverlay'
import { DEFAULT_SEGMENT_COLORS } from '@/components/map/SegmentsOverlay'

const MAP_FILTER_OPTIONS = sportFilterOptionsForPreset('MAP_PANEL_GPS')
const ALL_SPORT_KEYS = getAllSportKeys(MAP_FILTER_OPTIONS)

function uniqueSportTypes(options: SportOption[]): string[] {
  const types = new Set<string>()
  for (const opt of options) {
    if (opt.children) {
      for (const child of opt.children) types.add(child.value)
    } else {
      types.add(opt.value)
    }
  }
  return [...types]
}

function optionLeaves(opt: SportOption): SportOption[] {
  return opt.children ?? [opt]
}

function parseCountryCodes(value: string): string[] {
  return [...new Set(value
    .split(',')
    .map(v => v.trim().toUpperCase())
    .filter(v => /^[A-Z]{2}$/.test(v)))]
}

interface Props {
  filters: MapFilterState
  onChange: (filters: MapFilterState) => void
  forceDark?: boolean
  viewMode?: 'off' | 'tracks' | 'heatmap' | 'terra_incognita'
  /** When true, renders as a flex column child (no absolute positioning). */
  docked?: boolean
  showSegmentsOverlay?: boolean
  segmentFilters?: SegmentsOverlayFilters
  onSegmentFiltersChange?: (filters: SegmentsOverlayFilters) => void
  segmentColors?: Partial<SegmentColorConfig>
  onSegmentColorsChange?: (colors: Partial<SegmentColorConfig>) => void
}

function MapFilters({ filters, onChange, forceDark, viewMode = 'tracks', docked = false, showSegmentsOverlay, segmentFilters, onSegmentFiltersChange, segmentColors, onSegmentColorsChange }: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const [expandedColorGroups, setExpandedColorGroups] = useState<Set<string>>(new Set())
  const [datePickerOpen, setDatePickerOpen] = useState(false)
  const countryCodesValue = filters.countryCodes.join(', ')
  const [countryInput, setCountryInput] = useState(countryCodesValue)
  const previousCountryCodesValue = useRef(countryCodesValue)
  const datePickerRef = useRef<HTMLDivElement>(null)
  const { getSportColor, setSportColor, resetSportColor, hideUnused } = useGlobalSportSettings()
  const { data: statsData } = useQuery({
    queryKey: ['sport-type-stats', ''],
    queryFn: () => fetchSportTypeStats(),
    enabled: hideUnused,
  })
  const existingSportKeys = useMemo(
    () => new Set((statsData ?? []).map((s) => s.sub_type ? `${s.sport_type}:${s.sub_type}` : s.sport_type)),
    [statsData],
  )
  const visibleOptions = useMemo(() => {
    if (hideUnused && existingSportKeys.size > 0) {
      return filterToExistingSports(MAP_FILTER_OPTIONS, existingSportKeys)
    }
    return MAP_FILTER_OPTIONS
  }, [existingSportKeys, hideUnused])
  const allSelected = filters.sportTypes === null || ALL_SPORT_KEYS.every(k => filters.sportTypes!.has(k))

  function setOptionColor(opt: SportOption, hex: string) {
    const sportTypes = uniqueSportTypes(optionLeaves(opt))
    for (const sportType of sportTypes) setSportColor(sportType, hex)
    onChange({
      ...filters,
      colorConfig: {
        ...filters.colorConfig,
        colors: {
          ...filters.colorConfig.colors,
          ...Object.fromEntries(sportTypes.map((sportType) => [sportType, hex])),
        },
      },
    })
  }

  function resetVisibleColors() {
    const deleteKeys = new Set<string>()
    for (const opt of visibleOptions) {
      if (opt.groupKey) deleteKeys.add(opt.groupKey)
      deleteKeys.add(opt.value)
      for (const leaf of optionLeaves(opt)) {
        deleteKeys.add(leaf.value)
        deleteKeys.add(getOptionKey(leaf))
        resetSportColor(leaf.value)
      }
    }
    const colors = { ...filters.colorConfig.colors }
    for (const key of deleteKeys) delete colors[key]
    onChange({ ...filters, colorConfig: { ...filters.colorConfig, colors } })
  }

  // Close date picker on click outside
  useClickOutside(datePickerRef, () => setDatePickerOpen(false), datePickerOpen)

  useEffect(() => {
    if (previousCountryCodesValue.current !== countryCodesValue) {
      previousCountryCodesValue.current = countryCodesValue
      setCountryInput(countryCodesValue)
    }
  }, [countryCodesValue])

  return (
    <div className={docked ? `h-full w-full${forceDark ? ' dark' : ''}` : `absolute top-3 right-4 z-10 w-64${forceDark ? ' dark' : ''}`} ref={datePickerRef}>
      {/* DayPicker-Popup – links vom Panel, außerhalb overflow-hidden */}
      {datePickerOpen && (
        <div className="absolute top-0 right-full mr-2 z-50 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-xl shadow-xl overflow-hidden">
          <DayPicker
            mode="range"
            numberOfMonths={2}
            selected={{
              from: filters.dateFrom ? new Date(filters.dateFrom) : undefined,
              to: filters.dateTo ? new Date(filters.dateTo) : undefined,
            }}
            onSelect={(range: DateRange | undefined) => {
              onChange({
                ...filters,
                dateFrom: range?.from ? toLocalISO(range.from) : '',
                dateTo:   range?.to   ? toLocalISO(range.to)   : '',
              })
              if (range?.from && range?.to) setDatePickerOpen(false)
            }}
            locale={de}
            components={{ MonthCaption: MonthYearCaption as any }}
            className="p-3 text-sm"
          />
        </div>
      )}
      <div className={docked
        ? 'h-full bg-white dark:bg-neutral-900 overflow-y-auto flex flex-col'
        : 'rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden max-h-[calc(100vh-6rem)] flex flex-col'
      }>
        {/* Header */}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-neutral-900 dark:text-neutral-100 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
        >
          <span>Filter</span>
          <svg
            className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '-rotate-90' : ''}`}
            viewBox="0 0 20 20"
            fill="currentColor"
          >
            <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
          </svg>
        </button>

        {!collapsed && (
          <div className="overflow-y-auto flex-1 px-4 pb-4 space-y-4 border-t border-neutral-100 dark:border-neutral-800">
            {/* Aktivitäts-Filter — in Terra Incognita nicht relevant */}
            {viewMode !== 'terra_incognita' && <>
            <div className="pt-3">
              <SportFilter
                preset="MAP_PANEL_GPS"
                modifierKeys={MAP_MODIFIER_KEYS}
                value={{
                  sportTypes: filters.sportTypes === null ? null : [...filters.sportTypes],
                  modifiers: {
                    is_electric: filters.isElectric ?? undefined,
                    is_commute: filters.isCommute ?? undefined,
                  },
                }}
                onChange={(next) => onChange({
                  ...filters,
                  sportTypes: next.sportTypes === null ? null : new Set(next.sportTypes),
                  isElectric: next.modifiers?.is_electric ?? null,
                  isCommute: next.modifiers?.is_commute ?? null,
                })}
              />
            </div>

            {/* Date range */}
            <div>
              <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">Zeitraum</div>
              <div className="space-y-2">
                <input
                  type="date"
                  value={filters.dateFrom}
                  onChange={(e) => onChange({ ...filters, dateFrom: e.target.value })}
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                />
                <input
                  type="date"
                  value={filters.dateTo}
                  onChange={(e) => onChange({ ...filters, dateTo: e.target.value })}
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
                />
              </div>
              <button
                onClick={() => setDatePickerOpen(!datePickerOpen)}
                className={`mt-2 w-full flex items-center justify-between px-2.5 py-1.5 text-xs rounded-lg border cursor-pointer transition-colors ${
                  datePickerOpen
                    ? 'border-accent-DEFAULT bg-accent-100 dark:bg-accent-DEFAULT/15 text-accent-DEFAULT'
                    : 'border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-700 dark:text-neutral-300 hover:border-neutral-300 dark:hover:border-neutral-600'
                }`}
              >
                <span>Zeitraum wählen...</span>
                {(filters.dateFrom || filters.dateTo) && (
                  <span
                    onClick={e => {
                      e.stopPropagation()
                      onChange({ ...filters, dateFrom: '', dateTo: '' })
                      setDatePickerOpen(false)
                    }}
                    className="text-neutral-400 hover:text-red-500 cursor-pointer ml-1"
                  >&#x2715;</span>
                )}
              </button>
            </div>

            <div>
              <label className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-1 block">Länder</label>
              <input
                value={countryInput}
                onChange={e => {
                  const value = e.target.value
                  setCountryInput(value)
                  onChange({ ...filters, countryCodes: parseCountryCodes(value) })
                }}
                placeholder="CH, IT"
                className="w-full rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 px-2 py-1 text-xs text-neutral-800 dark:text-neutral-200 focus:outline-none focus:ring-1 focus:ring-accent-400"
              />
            </div>

            {/* Tag & Gear filter — combined chips + compact search fields */}
            <div className="space-y-2">
              {/* Combined selected chips (tags + gear) */}
              {((filters.tagFilters?.length ?? 0) > 0 || (filters.gearFilters?.length ?? 0) > 0) && (
                <div>
                  <div className="flex flex-wrap gap-1.5 mb-1">
                    {(filters.tagFilters ?? []).filter(f => f.state !== undefined).map((chip) => (
                      <button
                        key={`tag-${chip.id}`}
                        type="button"
                        onClick={() => {
                          const tags = filters.tagFilters ?? []
                          if (chip.state === 'include') {
                            onChange({ ...filters, tagFilters: tags.map(f => f.id === chip.id ? { ...f, state: 'exclude' as const } : f) })
                          } else {
                            onChange({ ...filters, tagFilters: tags.filter(f => f.id !== chip.id) })
                          }
                        }}
                        className={[
                          'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium border transition-all cursor-pointer',
                          chip.state === 'include'
                            ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                            : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through',
                        ].join(' ')}
                      >
                        <span className="w-2 h-2 rounded-full flex-shrink-0" style={{ backgroundColor: chip.color }} />
                        {chip.name}
                        <span className="ml-0.5 opacity-60 text-[10px]">
                          {chip.state === 'include' ? '✓' : '✕'}
                        </span>
                      </button>
                    ))}
                    {(filters.gearFilters ?? []).filter(f => f.state !== undefined).map((chip) => (
                      <button
                        key={`gear-${chip.id}`}
                        type="button"
                        onClick={() => {
                          const gear = filters.gearFilters ?? []
                          if (chip.state === 'include') {
                            onChange({ ...filters, gearFilters: gear.map(f => f.id === chip.id ? { ...f, state: 'exclude' as GearFilterState } : f) })
                          } else {
                            onChange({ ...filters, gearFilters: gear.filter(f => f.id !== chip.id) })
                          }
                        }}
                        className={[
                          'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border transition-all cursor-pointer',
                          chip.state === 'include'
                            ? 'bg-green-100 dark:bg-green-900/40 border-green-400 dark:border-green-600 text-green-800 dark:text-green-200'
                            : 'bg-red-100 dark:bg-red-900/40 border-red-400 dark:border-red-600 text-red-700 dark:text-red-300 line-through',
                        ].join(' ')}
                      >
                        <span className="flex-shrink-0" aria-hidden="true">{gearTypeIcon(chip.type)}</span>
                        {chip.name}
                        <span className="ml-0.5 opacity-60 text-[10px]">
                          {chip.state === 'include' ? '✓' : '✕'}
                        </span>
                      </button>
                    ))}
                  </div>
                  <p className="text-[10px] text-neutral-400 dark:text-neutral-500">
                    Klick: einschließen → ausschließen → entfernen
                  </p>
                </div>
              )}
              <TagFilterInput
                value={filters.tagFilters ?? []}
                onChange={(v: TagFilter[]) => onChange({ ...filters, tagFilters: v })}
                compact
              />
              <GearFilterInput
                value={filters.gearFilters ?? []}
                onChange={(v: GearFilter[]) => onChange({ ...filters, gearFilters: v })}
                compact
              />
            </div>

            {/* Opacity slider */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Deckkraft</span>
                <span className="text-xs text-neutral-400 tabular-nums">{filters.opacity}%</span>
              </div>
              <input
                type="range"
                min={0}
                max={100}
                value={filters.opacity}
                onChange={(e) => onChange({ ...filters, opacity: Number(e.target.value) })}
                className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-accent-DEFAULT cursor-pointer"
              />
            </div>

            {/* Track-Farben / Heatmap-Farbe */}
            <div>
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Farben</span>
              </div>

              {viewMode === 'heatmap' ? (
                <div className="flex items-center gap-2">
                  <label className="text-xs text-neutral-600 dark:text-neutral-400 flex-1">Heatmap-Farbe</label>
                  {filters.heatmapColor !== '#F97316' && (
                    <button
                      onClick={() => onChange({ ...filters, heatmapColor: '#F97316' })}
                      className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
                      title="Auf Standard zurücksetzen"
                    >
                      ↺
                    </button>
                  )}
                  <ColorPicker
                    value={filters.heatmapColor}
                    onChange={(c) => onChange({ ...filters, heatmapColor: c })}
                  />
                </div>
              ) : (
                <>
                  {/* Toggle: Einheitlich / Nach Sportart */}
                  <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs mb-2">
                    <button
                      onClick={() => onChange({ ...filters, colorConfig: { ...filters.colorConfig, mode: 'uniform' } })}
                      className={`flex-1 py-1 transition-colors cursor-pointer ${filters.colorConfig.mode === 'uniform' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'}`}
                    >
                      Einheitlich
                    </button>
                    <button
                      onClick={() => onChange({ ...filters, colorConfig: { ...filters.colorConfig, mode: 'per-type' } })}
                      className={`flex-1 py-1 transition-colors cursor-pointer ${filters.colorConfig.mode === 'per-type' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'}`}
                    >
                      Nach Sportart
                    </button>
                  </div>

                  {filters.colorConfig.mode === 'uniform' && (
                    <div className="flex items-center gap-2">
                      <label className="text-xs text-neutral-600 dark:text-neutral-400 flex-1">Track-Farbe</label>
                      <ColorPicker
                        value={filters.colorConfig.uniformColor}
                        onChange={(c) => onChange({ ...filters, colorConfig: { ...filters.colorConfig, uniformColor: c } })}
                      />
                    </div>
                  )}

                  {filters.colorConfig.mode === 'per-type' && (
                    <div className="space-y-1">
                      {visibleOptions.map(opt => {
                        const key = getOptionKey(opt)
                        const isActive = opt.children
                          ? (filters.sportTypes === null || opt.children.some(c => filters.sportTypes!.has(getOptionKey(c))))
                          : (filters.sportTypes === null || filters.sportTypes.has(key))

                        if (!isActive) return null

                        const hasChildren = !!opt.children && opt.children.length > 0
                        const isExpanded = expandedColorGroups.has(opt.value)
                        const activeLeaves = filters.sportTypes === null
                          ? optionLeaves(opt)
                          : optionLeaves(opt).filter((leaf) => filters.sportTypes!.has(getOptionKey(leaf)))
                        const groupSportTypes = uniqueSportTypes(activeLeaves.length ? activeLeaves : optionLeaves(opt))
                        const firstGroupColor = groupSportTypes.length > 0 ? getSportColor(groupSportTypes[0]) : '#888'
                        const groupColor = groupSportTypes.every((sportType) => getSportColor(sportType) === firstGroupColor)
                          ? firstGroupColor
                          : filters.colorConfig.colors[opt.groupKey ?? opt.value] ?? '#888'

                        return (
                          <div key={opt.value}>
                            <div className="flex items-center gap-1.5">
                              <ColorPicker
                                value={groupColor}
                                onChange={hex => setOptionColor(opt, hex)}
                              />
                              <span className="flex-1 text-xs text-neutral-700 dark:text-neutral-300 truncate">{opt.label}</span>
                              {hasChildren && (
                                <button
                                  onClick={() => setExpandedColorGroups(prev => {
                                    const next = new Set(prev)
                                    next.has(opt.value) ? next.delete(opt.value) : next.add(opt.value)
                                    return next
                                  })}
                                  aria-label={isExpanded ? 'Farbgruppe einklappen' : 'Farbgruppe ausklappen'}
                                  aria-expanded={isExpanded}
                                  className="p-0.5 text-neutral-400 hover:text-neutral-600 cursor-pointer"
                                >
                                  <svg className={`w-3 h-3 transition-transform ${isExpanded ? 'rotate-180' : ''}`} viewBox="0 0 20 20" fill="currentColor">
                                    <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                                  </svg>
                                </button>
                              )}
                            </div>
                            {hasChildren && isExpanded && (
                              <div className="ml-4 mt-1 space-y-1">
                                {(filters.sportTypes === null ? opt.children! : opt.children!.filter(c => filters.sportTypes!.has(getOptionKey(c)))).map(child => {
                                  const childKey = getOptionKey(child)
                                  return (
                                    <div key={childKey} className="flex items-center gap-1.5">
                                      <ColorPicker
                                        value={getSportColor(child.value)}
                                        onChange={hex => {
                                          setSportColor(child.value, hex)
                                          const next = { ...filters, colorConfig: { ...filters.colorConfig, colors: { ...filters.colorConfig.colors, [child.value]: hex } } }
                                          onChange(next)
                                        }}
                                      />
                                      <span className="text-xs text-neutral-500 dark:text-neutral-400 truncate">{child.label}</span>
                                    </div>
                                  )
                                })}
                              </div>
                            )}
                          </div>
                        )
                      })}
                      <div className="flex gap-2 mt-1">
                        <button
                          onClick={resetVisibleColors}
                          className="flex-1 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
                        >
                          Zurücksetzen
                        </button>
                      </div>
                    </div>
                  )}
                </>
              )}
            </div>

            {/* Standorte anzeigen */}
            <div className="border-t border-neutral-100 dark:border-neutral-800 pt-3">
              <label className="flex items-start gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={filters.showLocationTags ?? true}
                  onChange={(e) => onChange({ ...filters, showLocationTags: e.target.checked })}
                  className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1 mt-0.5 flex-shrink-0"
                />
                <div>
                  <span className="text-sm text-neutral-700 dark:text-neutral-300">Standort-Pins</span>
                  <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">Manuell zugewiesene Standorte</p>
                </div>
              </label>
            </div>
            </> /* Ende: Aktivitäts-Filter */}

            {/* Segment-Konfiguration */}
            {showSegmentsOverlay && segmentFilters && onSegmentFiltersChange && segmentColors && onSegmentColorsChange && (
              <div className="border-t border-neutral-100 dark:border-neutral-800 pt-3 space-y-2">
                <span className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">Segment-Konfiguration</span>
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Typ:</div>
                  <div className="flex flex-wrap gap-1">
                    {([
                      { id: 'standard', label: 'Standard' },
                      { id: 'trail', label: 'Trail' },
                      { id: 'lift', label: 'Lift' },
                      { id: 'wintersport', label: 'Winter' },
                      { id: 'wassersport', label: 'Wasser' },
                    ] as const).map(({ id, label }) => (
                      <button
                        key={id}
                        onClick={() => onSegmentFiltersChange({
                          ...segmentFilters,
                          terrainTypes: segmentFilters.terrainTypes.includes(id)
                            ? segmentFilters.terrainTypes.filter(t => t !== id)
                            : [...segmentFilters.terrainTypes, id],
                        })}
                        className={`px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${
                          segmentFilters.terrainTypes.includes(id)
                            ? 'bg-accent-DEFAULT text-white border-transparent'
                            : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400'
                        }`}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Kurse:</div>
                  <div className="flex flex-wrap gap-1">
                    {([
                      { id: 'line', label: 'Linien' },
                      { id: 'checkpoint_sequence', label: 'Checkpoint-Kurse' },
                    ] as const).map(({ id, label }) => (
                      <button
                        key={id}
                        onClick={() => {
                          const nextCourseKinds = segmentFilters.courseKinds.includes(id)
                            ? segmentFilters.courseKinds.filter(k => k !== id)
                            : [...segmentFilters.courseKinds, id]
                          if (nextCourseKinds.length === 0) return
                          onSegmentFiltersChange({
                            ...segmentFilters,
                            courseKinds: nextCourseKinds,
                          })
                        }}
                        className={`px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${
                          segmentFilters.courseKinds.includes(id)
                            ? 'bg-accent-DEFAULT text-white border-transparent'
                            : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400'
                        }`}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
                <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                  <input type="checkbox" checked={segmentFilters.riddenOnly} onChange={e => onSegmentFiltersChange({ ...segmentFilters, riddenOnly: e.target.checked })} className="rounded" />
                  <span>Nur gefahrene</span>
                </label>
                <label className="flex items-center gap-1.5 text-xs cursor-pointer">
                  <input type="checkbox" checked={segmentFilters.favoritesOnly} onChange={e => onSegmentFiltersChange({ ...segmentFilters, favoritesOnly: e.target.checked })} className="rounded" />
                  <span>Nur Favoriten</span>
                </label>
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Sportart:</div>
                  <div className="flex flex-wrap gap-1">
                    {([
                      { id: 'bike', label: 'Rad' },
                      { id: 'foot', label: 'Lauf' },
                      { id: 'ski', label: 'Winter' },
                      { id: 'paddle', label: 'Wasser' },
                      { id: 'generic', label: 'Sonstig' },
                    ] as const).map(({ id, label }) => (
                      <button
                        key={id}
                        onClick={() => onSegmentFiltersChange({
                          ...segmentFilters,
                          sportGroups: segmentFilters.sportGroups.includes(id)
                            ? segmentFilters.sportGroups.filter(g => g !== id)
                            : [...segmentFilters.sportGroups, id],
                        })}
                        className={`px-1.5 py-0.5 text-xs rounded border cursor-pointer transition-colors ${
                          segmentFilters.sportGroups.includes(id)
                            ? 'bg-accent-DEFAULT text-white border-transparent'
                            : 'border-neutral-300 dark:border-neutral-600 text-neutral-600 dark:text-neutral-300 hover:border-neutral-400'
                        }`}
                      >
                        {label}
                      </button>
                    ))}
                  </div>
                </div>
                <div>
                  <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Farben:</div>
                  <div className="grid grid-cols-2 gap-x-3 gap-y-1.5">
                    {([
                      { key: 'standard', label: 'Standard' },
                      { key: 'trail', label: 'Trail' },
                      { key: 'lift', label: 'Lift' },
                      { key: 'wintersport', label: 'Winter' },
                      { key: 'wassersport', label: 'Wasser' },
                      { key: 'checkpoint', label: 'Checkpoint' },
                      { key: 'ridden', label: 'Gefahren' },
                      { key: 'favorite', label: 'Favoriten' },
                    ] as { key: keyof SegmentColorConfig; label: string }[]).map(({ key, label }) => (
                      <div key={key} className="flex items-center gap-1.5 text-xs">
                        <ColorPicker
                          value={segmentColors[key] ?? DEFAULT_SEGMENT_COLORS[key]}
                          onChange={hex => {
                            const next = { ...segmentColors, [key]: hex }
                            onSegmentColorsChange(next)
                            localStorage.setItem('skadia-segment-colors', JSON.stringify(next))
                          }}
                        />
                        <span className="text-neutral-600 dark:text-neutral-300">{label}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {/* Reset */}

            {(!allSelected || filters.dateFrom || filters.dateTo || filters.opacity !== 70 || filters.colorConfig.mode !== 'uniform' || filters.colorConfig.uniformColor !== '#F97316' || !filters.showLocationTags || filters.heatmapColor !== '#F97316' || (filters.tagFilters?.length ?? 0) > 0 || (filters.gearFilters?.length ?? 0) > 0 || filters.countryCodes.length > 0 || filters.isElectric !== null || filters.isCommute !== null) && (
              <button
                onClick={() => onChange({
                  sportTypes: null,
                  dateFrom: '',
                  dateTo: '',
                  opacity: 70,
                  colorConfig: { mode: 'uniform', uniformColor: '#F97316', colors: { ...DEFAULT_SPORT_COLORS } },
                  showLocationTags: true,
                  heatmapColor: '#F97316',
                  tagFilters: [],
                  gearFilters: [],
                  countryCodes: [],
                  isElectric: null,
                  isCommute: null,
                })}
                className="w-full text-xs text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 cursor-pointer"
              >
                Filter zurücksetzen
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

export default memo(MapFilters)
const MAP_MODIFIER_KEYS = [...HeatmapApplicableModifierKeys]
