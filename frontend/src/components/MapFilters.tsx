import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { DayPicker } from 'react-day-picker'
import 'react-day-picker/style.css'
import type { DateRange } from 'react-day-picker'
import { de } from 'date-fns/locale'
import type { MapFilterState } from '@/pages/MapPage'
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

const ALL_SPORT_KEYS = getAllSportKeys()
const USER_DEFAULT_KEY = 'skadia-sport-colors-user-default'

function loadUserDefaultColors(): Record<string, string> | null {
  try {
    const saved = localStorage.getItem(USER_DEFAULT_KEY)
    if (saved) return JSON.parse(saved)
  } catch { /* ignore */ }
  return null
}

interface Props {
  filters: MapFilterState
  onChange: (filters: MapFilterState) => void
  forceDark?: boolean
  viewMode?: 'tracks' | 'heatmap'
}

export default function MapFilters({ filters, onChange, forceDark, viewMode = 'tracks' }: Props) {
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
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (datePickerRef.current && !datePickerRef.current.contains(e.target as Node)) {
        setDatePickerOpen(false)
      }
    }
    if (datePickerOpen) document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [datePickerOpen])

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
                dateFrom: range?.from ? range.from.toISOString().split('T')[0] : '',
                dateTo: range?.to ? range.to.toISOString().split('T')[0] : '',
              })
              if (range?.from && range?.to) setDatePickerOpen(false)
            }}
            locale={de}
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
                    ? 'border-accent-DEFAULT bg-accent-50 dark:bg-accent-900/20 text-accent-DEFAULT'
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
                          'inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium border transition-all',
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
                          'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium border transition-all',
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
                className="w-full h-1.5 rounded-full appearance-none bg-neutral-200 dark:bg-neutral-700 accent-orange-500 cursor-pointer"
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

            {/* Reset */}
            {(!allSelected || filters.dateFrom || filters.dateTo || filters.opacity !== 70 || filters.colorConfig.mode !== 'uniform' || filters.colorConfig.uniformColor !== '#F97316' || !filters.showLocationTags || filters.heatmapColor !== '#F97316' || (filters.tagFilters?.length ?? 0) > 0 || (filters.gearFilters?.length ?? 0) > 0) && (
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
