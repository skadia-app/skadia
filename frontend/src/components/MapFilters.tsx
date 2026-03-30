import { useEffect, useMemo, useRef, useState } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import type { CoverageOverlayState, CoverageCompositeState } from '@/hooks/useCoverageOverlays'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { DayPicker } from 'react-day-picker'
import 'react-day-picker/style.css'
import type { DateRange } from 'react-day-picker'
import { MonthYearCaption } from '@/components/DateRangePicker'
import { de } from 'date-fns/locale'
import type { MapFilterState, MapOverlayState } from '@/pages/MapPage'
import type { SportOption } from '@/lib/sportOptions'
import { SPORT_OPTIONS, getAllSportKeys, DEFAULT_SPORT_COLORS, filterToExistingSports } from '@/lib/sportOptions'
import { CATEGORIES } from '@/lib/sportTaxonomy'
import ColorPicker from '@/components/ColorPicker'
import { fetchSportTypeStats } from '@/api/stats'
import { useGlobalSportSettings } from '@/lib/globalSportSettings'
import TagFilterInput from '@/components/TagFilterInput'
import GearFilterInput, { gearTypeIcon } from '@/components/GearFilterInput'
import type { TagFilter } from '@/components/TagFilterInput'
import type { GearFilter, GearFilterState } from '@/components/GearFilterInput'
import { toLocalISO } from '@/lib/dateUtils'
import type { SegmentColorConfig, SegmentsOverlayFilters } from '@/components/Map/SegmentsOverlay'
import { DEFAULT_SEGMENT_COLORS } from '@/components/Map/SegmentsOverlay'

const ALL_SPORT_KEYS = getAllSportKeys()
const USER_DEFAULT_KEY = 'skadia-sport-colors-user-default'

function loadUserDefaultColors(): Record<string, string> | null {
  try {
    const saved = localStorage.getItem(USER_DEFAULT_KEY)
    if (saved) return JSON.parse(saved)
  } catch { /* ignore */ }
  return null
}

function CoverageCompositeToggle({
  label,
  color,
  state,
  compositeKey,
  layerLabels,
  onToggle,
  onToggleLayer,
}: {
  label: string
  color: string
  state: CoverageCompositeState
  compositeKey: keyof CoverageOverlayState
  layerLabels: Record<string, string>
  onToggle: (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => void
  onToggleLayer: (composite: keyof CoverageOverlayState, layer: string, enabled: boolean) => void
}) {
  const [expanded, setExpanded] = useState(false)
  const hasSubLayers = Object.keys(layerLabels).length > 0

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        {hasSubLayers && (
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-xs text-neutral-400 cursor-pointer w-3 text-center"
          >
            <svg
              className={`w-3 h-3 transition-transform ${expanded ? '' : '-rotate-90'}`}
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fillRule="evenodd"
                d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
                clipRule="evenodd"
              />
            </svg>
          </button>
        )}
        {!hasSubLayers && <span className="w-3" />}
        <label className="flex items-center gap-2 text-xs cursor-pointer">
          <input
            type="checkbox"
            checked={state.enabled}
            onChange={(e) => onToggle(compositeKey, { enabled: e.target.checked })}
            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
          />
          <span className={`${color} font-medium`}>{label}</span>
        </label>
      </div>
      {expanded && state.enabled && (
        <div className="ml-7 space-y-1.5">
          <label className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
            <span className="whitespace-nowrap">Deckkraft</span>
            <input
              type="range"
              min={10}
              max={100}
              value={state.opacity}
              onChange={(e) => onToggle(compositeKey, { opacity: Number(e.target.value) })}
              className="flex-1 h-1 accent-neutral-500"
            />
            <span className="w-8 text-right tabular-nums">{state.opacity}%</span>
          </label>
          {Object.entries(layerLabels).map(([key, name]) => (
            <label key={key} className="flex items-center gap-2 text-xs cursor-pointer">
              <input
                type="checkbox"
                checked={state.layers[key] ?? true}
                onChange={(e) => onToggleLayer(compositeKey, key, e.target.checked)}
                className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
              />
              <span className="text-neutral-600 dark:text-neutral-400">{name}</span>
            </label>
          ))}
        </div>
      )}
    </div>
  )
}

interface Props {
  filters: MapFilterState
  onChange: (filters: MapFilterState) => void
  forceDark?: boolean
  viewMode?: 'tracks' | 'heatmap' | 'terra_incognita'
  overlayState?: MapOverlayState
  onOverlayChange?: (key: keyof MapOverlayState, value: boolean) => void
  segmentFilters?: SegmentsOverlayFilters
  onSegmentFiltersChange?: (filters: SegmentsOverlayFilters) => void
  segmentColors?: Partial<SegmentColorConfig>
  onSegmentColorsChange?: (colors: Partial<SegmentColorConfig>) => void
  coverageState?: CoverageOverlayState
  onCoverageUpdate?: (key: keyof CoverageOverlayState, update: Partial<CoverageCompositeState>) => void
  onCoverageToggleLayer?: (composite: keyof CoverageOverlayState, layer: string, enabled: boolean) => void
}

export default function MapFilters({ filters, onChange, forceDark, viewMode = 'tracks', overlayState, onOverlayChange, segmentFilters, onSegmentFiltersChange, segmentColors, onSegmentColorsChange, coverageState, onCoverageUpdate, onCoverageToggleLayer }: Props) {
  const navigate = useNavigate()
  const [collapsed, setCollapsed] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [expandedColorGroups, setExpandedColorGroups] = useState<Set<string>>(new Set())
  const [datePickerOpen, setDatePickerOpen] = useState(false)
  const datePickerRef = useRef<HTMLDivElement>(null)

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
      return filterToExistingSports(SPORT_OPTIONS, existingSportTypes)
    }
    return SPORT_OPTIONS
  }, [hideUnused, existingSportTypes])
  const visibleKeys = useMemo(() => getAllSportKeys(visibleOptions), [visibleOptions])

  // Close date picker on click outside
  useClickOutside(datePickerRef, () => setDatePickerOpen(false), datePickerOpen)

  const allSelected = visibleKeys.every(k => filters.sportTypes.has(k))

  function getKey(opt: SportOption): string {
    return opt.subType ? `${opt.value}:${opt.subType}` : opt.value
  }

  function toggleSport(key: string) {
    const next = new Set(filters.sportTypes)
    if (next.has(key)) {
      next.delete(key)
    } else {
      next.add(key)
    }
    onChange({ ...filters, sportTypes: next })
  }

  function toggleParent(parent: SportOption) {
    if (!parent.children) return
    const childKeys = parent.children.map(getKey)
    const allChildrenSelected = childKeys.every(k => filters.sportTypes.has(k))
    const next = new Set(filters.sportTypes)
    if (allChildrenSelected) {
      childKeys.forEach(k => next.delete(k))
    } else {
      childKeys.forEach(k => next.add(k))
    }
    onChange({ ...filters, sportTypes: next })
  }

  function isParentChecked(parent: SportOption): boolean {
    if (!parent.children) return false
    return parent.children.every(c => filters.sportTypes.has(getKey(c)))
  }

  function isParentIndeterminate(parent: SportOption): boolean {
    if (!parent.children) return false
    const checked = parent.children.filter(c => filters.sportTypes.has(getKey(c))).length
    return checked > 0 && checked < parent.children.length
  }

  function toggleGroup(groupKey: string) {
    setExpandedGroups(prev => {
      const next = new Set(prev)
      if (next.has(groupKey)) {
        next.delete(groupKey)
      } else {
        next.add(groupKey)
      }
      return next
    })
  }

  function toggleAll() {
    if (allSelected) {
      onChange({ ...filters, sportTypes: new Set() })
    } else {
      onChange({ ...filters, sportTypes: new Set(visibleKeys) })
    }
  }

  return (
    <div className={`absolute top-4 right-4 z-10 w-64${forceDark ? ' dark' : ''}`} ref={datePickerRef}>
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
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden max-h-[calc(100vh-6rem)] flex flex-col">
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
            {/* Sport type checkboxes */}
            <div className="pt-3">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</span>
                <button
                  onClick={toggleAll}
                  className="text-xs text-accent-DEFAULT hover:text-accent-600 cursor-pointer"
                >
                  {allSelected ? 'Alle abwählen' : 'Alle auswählen'}
                </button>
              </div>
              <div className="space-y-1.5">
                {visibleOptions.map((opt) => (
                  <div key={opt.label}>
                    {opt.children && opt.groupKey ? (
                      /* Parent with children */
                      <div>
                        <div className="flex items-center gap-2">
                          <input
                            type="checkbox"
                            checked={isParentChecked(opt)}
                            ref={(el) => {
                              if (el) el.indeterminate = isParentIndeterminate(opt)
                            }}
                            onChange={() => toggleParent(opt)}
                            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-accent-DEFAULT focus:ring-accent-DEFAULT focus:ring-1"
                          />
                          <button
                            onClick={() => toggleGroup(opt.groupKey!)}
                            className="flex items-center gap-1 text-sm text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100 cursor-pointer"
                          >
                            <svg
                              className={`w-3 h-3 text-neutral-400 transition-transform ${expandedGroups.has(opt.groupKey!) ? '' : '-rotate-90'}`}
                              viewBox="0 0 20 20"
                              fill="currentColor"
                            >
                              <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                            </svg>
                            {opt.label}
                          </button>
                        </div>
                        {expandedGroups.has(opt.groupKey!) && (
                          <div className="ml-6 mt-1 space-y-1">
                            {opt.children.map((child) => {
                              const key = getKey(child)
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
                      /* Simple option */
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

            {/* E-Bike tristate filter */}
            {/* TODO(electric): When additional electric sport tags are added (e.g. E-Scooter),
                generalize this filter to "Elektrisch" with sport-specific sub-options,
                or add separate per-sport electric filters. */}
            <div>
              <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">🔌 E-Bike</div>
              <div className="flex rounded-lg overflow-hidden border border-neutral-200 dark:border-neutral-700 text-xs">
                {([
                  [null, 'Alle'],
                  [true, 'Nur'],
                  [false, 'Ohne'],
                ] as [boolean | null, string][]).map(([val, label]) => (
                  <button
                    key={String(val)}
                    onClick={() => onChange({ ...filters, isElectric: val })}
                    className={`flex-1 px-2 py-1.5 transition-colors cursor-pointer ${
                      filters.isElectric === val
                        ? 'bg-accent-DEFAULT text-white font-medium'
                        : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
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
                        const key = opt.subType ? `${opt.value}:${opt.subType}` : opt.value
                        const isActive = opt.children
                          ? opt.children.some(c => filters.sportTypes.has(getKey(c)))
                          : filters.sportTypes.has(key)

                        if (!isActive) return null

                        const hasChildren = !!opt.children && opt.children.length > 0
                        const isExpanded = expandedColorGroups.has(opt.value)
                        // Resolve category color for group options
                        const catColor = opt.groupKey
                          ? CATEGORIES.find(c => c.key === opt.groupKey)?.color ?? '#888'
                          : DEFAULT_SPORT_COLORS[opt.value] ?? '#888'

                        return (
                          <div key={opt.value}>
                            <div className="flex items-center gap-1.5">
                              <ColorPicker
                                value={filters.colorConfig.colors[opt.groupKey ?? opt.value] ?? catColor}
                                onChange={hex => {
                                  const colorKey = opt.groupKey ?? opt.value
                                  const next = { ...filters, colorConfig: { ...filters.colorConfig, colors: { ...filters.colorConfig.colors, [colorKey]: hex } } }
                                  onChange(next)
                                }}
                              />
                              <span className="flex-1 text-xs text-neutral-700 dark:text-neutral-300 truncate">{opt.label}</span>
                              {hasChildren && (
                                <button
                                  onClick={() => setExpandedColorGroups(prev => {
                                    const next = new Set(prev)
                                    next.has(opt.value) ? next.delete(opt.value) : next.add(opt.value)
                                    return next
                                  })}
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
                                {opt.children!.filter(c => filters.sportTypes.has(getKey(c))).map(child => {
                                  const childKey = getKey(child)
                                  return (
                                    <div key={childKey} className="flex items-center gap-1.5">
                                      <ColorPicker
                                        value={filters.colorConfig.colors[childKey] ?? filters.colorConfig.colors[child.value] ?? DEFAULT_SPORT_COLORS[child.value] ?? '#888'}
                                        onChange={hex => {
                                          const next = { ...filters, colorConfig: { ...filters.colorConfig, colors: { ...filters.colorConfig.colors, [childKey]: hex } } }
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
                          onClick={() => {
                            const userDefault = loadUserDefaultColors()
                            onChange({ ...filters, colorConfig: { ...filters.colorConfig, colors: userDefault ?? Object.fromEntries(CATEGORIES.map(c => [c.key, c.color])) } })
                          }}
                          className="flex-1 text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 cursor-pointer"
                        >
                          Zurücksetzen
                        </button>
                        <button
                          onClick={() => localStorage.setItem(USER_DEFAULT_KEY, JSON.stringify(filters.colorConfig.colors))}
                          className="flex-1 text-xs text-orange-400 hover:text-accent-600 dark:hover:text-orange-300 cursor-pointer"
                        >
                          Als Standard speichern
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
                  <span className="text-sm text-neutral-700 dark:text-neutral-300">Indoor-Standort-Pins</span>
                  <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-0.5">Standorte für Aktivitäten ohne GPS-Daten (Indoor-Tagging)</p>
                </div>
              </label>
            </div>
            </> /* Ende: Aktivitäts-Filter */}

            {/* Kartenebenen — Overlay-Toggles (Gipfel, Gewässer, Wälder, etc.) */}
            {overlayState && onOverlayChange && (
              <div className="border-t border-neutral-100 dark:border-neutral-800 pt-3 space-y-2">
                <span className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">Kartenebenen</span>
                <button
                  onClick={() => navigate('/map/summits')}
                  className="cursor-pointer w-full text-left text-xs text-accent-DEFAULT hover:underline mb-2"
                >
                  Gipfelsammlung öffnen →
                </button>
                <div className="space-y-1">
                  <label className="flex items-center gap-2 text-xs cursor-pointer">
                    <input type="checkbox" checked={overlayState.showSummits} onChange={e => onOverlayChange('showSummits', e.target.checked)} className="rounded" />
                    <span>Gipfel</span>
                  </label>
                  {overlayState.showSummits && (
                    <label className="flex items-center gap-2 text-xs cursor-pointer ml-5 text-neutral-500 dark:text-neutral-400">
                      <input type="checkbox" checked={overlayState.summitsOnlyVisited} onChange={e => onOverlayChange('summitsOnlyVisited', e.target.checked)} className="rounded" />
                      <span>Nur besuchte</span>
                    </label>
                  )}
                </div>
                <label className="flex items-center gap-2 text-xs cursor-pointer">
                  <input type="checkbox" checked={overlayState.showWaters} onChange={e => onOverlayChange('showWaters', e.target.checked)} className="rounded" />
                  <span>Gewässer</span>
                </label>
                <label className="flex items-center gap-2 text-xs cursor-pointer">
                  <input type="checkbox" checked={overlayState.showForests} onChange={e => onOverlayChange('showForests', e.target.checked)} className="rounded" />
                  <span>Wälder</span>
                </label>
                <label className="flex items-center gap-2 text-xs cursor-pointer">
                  <input type="checkbox" checked={overlayState.showVisitedPois} onChange={e => onOverlayChange('showVisitedPois', e.target.checked)} className="rounded" />
                  <span>Besuchte Einkehr</span>
                </label>
                <label className="flex items-center gap-2 text-xs cursor-pointer">
                  <input type="checkbox" checked={overlayState.showAmenities} onChange={e => onOverlayChange('showAmenities', e.target.checked)} className="rounded" />
                  <span>Einkehr (Tiles)</span>
                </label>
                <div className="space-y-1">
                  <label className="flex items-center gap-2 text-xs cursor-pointer">
                    <input type="checkbox" checked={overlayState.showSegmentsOverlay} onChange={e => onOverlayChange('showSegmentsOverlay', e.target.checked)} className="rounded" />
                    <span>Segmente (Linien)</span>
                  </label>
                  {overlayState.showSegmentsOverlay && segmentFilters && onSegmentFiltersChange && segmentColors && onSegmentColorsChange && (
                    <div className="ml-5 space-y-2">
                      <div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">Typ:</div>
                        <div className="flex flex-wrap gap-1">
                          {([
                            { id: 'standard', label: 'Standard' },
                            { id: 'trail', label: 'Trail' },
                            { id: 'lift', label: 'Lift' },
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
                </div>

                {/* Coverage-Overlays */}
                {coverageState && onCoverageUpdate && onCoverageToggleLayer && (
                  <>
                    <div className="border-t border-neutral-200 dark:border-neutral-700 my-2" />
                    <CoverageCompositeToggle
                      label="Natur-Overlay"
                      color="text-green-600 dark:text-green-400"
                      state={coverageState.nature}
                      compositeKey="nature"
                      layerLabels={{
                        forest: 'Wald',
                        water: 'Gewaesser',
                        riverProximity: 'Fluss-Naehe',
                        protected: 'Schutzgebiete',
                        landuse: 'Landnutzung',
                      }}
                      onToggle={onCoverageUpdate}
                      onToggleLayer={onCoverageToggleLayer}
                    />
                    <CoverageCompositeToggle
                      label="Lärm-Overlay"
                      color="text-purple-500 dark:text-purple-400"
                      state={coverageState.noise}
                      compositeKey="noise"
                      layerLabels={{}}
                      onToggle={onCoverageUpdate}
                      onToggleLayer={onCoverageToggleLayer}
                    />
                    <CoverageCompositeToggle
                      label="Verkehr-Overlay"
                      color="text-orange-500 dark:text-orange-400"
                      state={coverageState.trafficUrban}
                      compositeKey="trafficUrban"
                      layerLabels={{
                        traffic: 'Verkehr',
                        urbanization: 'Urbanisierung',
                      }}
                      onToggle={onCoverageUpdate}
                      onToggleLayer={onCoverageToggleLayer}
                    />
                  </>
                )}
              </div>
            )}

            {/* Reset */}

            {(!allSelected || filters.dateFrom || filters.dateTo || filters.opacity !== 70 || filters.colorConfig.mode !== 'uniform' || filters.colorConfig.uniformColor !== '#F97316' || !filters.showLocationTags || filters.heatmapColor !== '#F97316' || (filters.tagFilters?.length ?? 0) > 0 || (filters.gearFilters?.length ?? 0) > 0 || filters.isElectric !== null) && (
              <button
                onClick={() => onChange({
                  sportTypes: new Set(ALL_SPORT_KEYS),
                  dateFrom: '',
                  dateTo: '',
                  opacity: 70,
                  colorConfig: { mode: 'uniform', uniformColor: '#F97316', colors: { ...DEFAULT_SPORT_COLORS } },
                  showLocationTags: true,
                  heatmapColor: '#F97316',
                  tagFilters: [],
                  gearFilters: [],
                  isElectric: null,
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
