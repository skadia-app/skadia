import { useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import type { MapFilterState } from '@/pages/MapPage'
import type { SportOption } from '@/lib/sportOptions'
import { SPORT_OPTIONS, getAllSportKeys, DEFAULT_SPORT_COLORS } from '@/lib/sportOptions'
import { CATEGORIES } from '@/lib/sportTaxonomy'

const ALL_SPORT_KEYS = getAllSportKeys()

const USER_DEFAULT_KEY = 'skadia-sport-colors-user-default'

function loadUserDefaultColors(): Record<string, string> | null {
  try {
    const saved = localStorage.getItem(USER_DEFAULT_KEY)
    if (saved) return JSON.parse(saved)
  } catch { /* ignore */ }
  return null
}

function ColorSwatch({ value, onChange }: { value: string; onChange: (c: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  const handleClick = () => {
    if (btnRef.current && inputRef.current) {
      const rect = btnRef.current.getBoundingClientRect()
      const pickerW = 220
      const pickerH = 250
      let left = rect.left - pickerW - 4
      if (left < 8) left = rect.right + 4
      let top = rect.top
      if (top + pickerH > window.innerHeight - 8) top = window.innerHeight - pickerH - 8
      if (top < 8) top = 8
      inputRef.current.style.top = `${top}px`
      inputRef.current.style.left = `${left}px`
    }
    inputRef.current?.click()
  }

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        onClick={handleClick}
        className="w-7 h-5 rounded border border-neutral-300 dark:border-neutral-600 cursor-pointer flex-shrink-0"
        style={{ backgroundColor: value }}
        title={value}
      />
      {createPortal(
        <input
          ref={inputRef}
          type="color"
          value={value}
          onChange={e => onChange(e.target.value)}
          style={{ position: 'fixed', top: 0, left: 0, width: 1, height: 1, opacity: 0, pointerEvents: 'none' }}
          tabIndex={-1}
        />,
        document.body,
      )}
    </>
  )
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

  const allSelected = filters.sportTypes.size === ALL_SPORT_KEYS.length

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
      onChange({ ...filters, sportTypes: new Set(ALL_SPORT_KEYS) })
    }
  }

  return (
    <div className={`absolute top-4 right-4 z-10 w-64${forceDark ? ' dark' : ''}`}>
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">
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
          <div className="px-4 pb-4 space-y-4 border-t border-neutral-100 dark:border-neutral-800">
            {/* Sport type checkboxes */}
            <div className="pt-3">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-neutral-500 dark:text-neutral-400">Sportart</span>
                <button
                  onClick={toggleAll}
                  className="text-xs text-orange-500 hover:text-orange-600 cursor-pointer"
                >
                  {allSelected ? 'Alle abwählen' : 'Alle auswählen'}
                </button>
              </div>
              <div className="space-y-1.5">
                {SPORT_OPTIONS.map((opt) => (
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
                            className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
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
                                    className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
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
                          className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
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
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
                  placeholder="Von"
                />
                <input
                  type="date"
                  value={filters.dateTo}
                  onChange={(e) => onChange({ ...filters, dateTo: e.target.value })}
                  className="w-full border border-neutral-300 dark:border-neutral-700 rounded-lg px-3 py-1.5 text-sm bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 focus:ring-2 focus:ring-orange-500 focus:border-transparent"
                  placeholder="Bis"
                />
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
                  <ColorSwatch
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
                      className={`flex-1 py-1 transition-colors cursor-pointer ${filters.colorConfig.mode === 'uniform' ? 'bg-orange-500 text-white' : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'}`}
                    >
                      Einheitlich
                    </button>
                    <button
                      onClick={() => onChange({ ...filters, colorConfig: { ...filters.colorConfig, mode: 'per-type' } })}
                      className={`flex-1 py-1 transition-colors cursor-pointer ${filters.colorConfig.mode === 'per-type' ? 'bg-orange-500 text-white' : 'text-neutral-600 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-700'}`}
                    >
                      Nach Sportart
                    </button>
                  </div>

                  {filters.colorConfig.mode === 'uniform' && (
                    <div className="flex items-center gap-2">
                      <label className="text-xs text-neutral-600 dark:text-neutral-400 flex-1">Track-Farbe</label>
                      <ColorSwatch
                        value={filters.colorConfig.uniformColor}
                        onChange={(c) => onChange({ ...filters, colorConfig: { ...filters.colorConfig, uniformColor: c } })}
                      />
                    </div>
                  )}

                  {filters.colorConfig.mode === 'per-type' && (
                    <div className="space-y-1">
                      {CATEGORIES.map(({ key, label, color }) => (
                        <div key={key} className="flex items-center gap-2">
                          <span className="text-xs text-neutral-600 dark:text-neutral-400 flex-1">{label}</span>
                          <ColorSwatch
                            value={filters.colorConfig.colors[key] ?? color}
                            onChange={(c) => onChange({
                              ...filters,
                              colorConfig: {
                                ...filters.colorConfig,
                                colors: { ...filters.colorConfig.colors, [key]: c },
                              },
                            })}
                          />
                        </div>
                      ))}
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
                          className="flex-1 text-xs text-orange-400 hover:text-orange-600 dark:hover:text-orange-300 cursor-pointer"
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
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={filters.showLocationTags ?? false}
                  onChange={(e) => onChange({ ...filters, showLocationTags: e.target.checked })}
                  className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
                />
                <span className="text-sm text-neutral-700 dark:text-neutral-300">Standort-Pins einblenden</span>
              </label>
            </div>

            {/* Reset */}
            {(!allSelected || filters.dateFrom || filters.dateTo || filters.opacity !== 70 || filters.colorConfig.mode !== 'uniform' || filters.colorConfig.uniformColor !== '#F97316' || filters.showLocationTags || filters.heatmapColor !== '#F97316') && (
              <button
                onClick={() => onChange({
                  sportTypes: new Set(ALL_SPORT_KEYS),
                  dateFrom: '',
                  dateTo: '',
                  opacity: 70,
                  colorConfig: { mode: 'uniform', uniformColor: '#F97316', colors: { ...DEFAULT_SPORT_COLORS } },
                  showLocationTags: false,
                  heatmapColor: '#F97316',
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
