import { useState } from 'react'
import type { MapFilterState } from '@/pages/MapPage'
import type { SportOption } from '@/lib/sportOptions'
import { SPORT_OPTIONS, getAllSportKeys } from '@/lib/sportOptions'

const ALL_SPORT_KEYS = getAllSportKeys()

interface Props {
  filters: MapFilterState
  onChange: (filters: MapFilterState) => void
}

export default function MapFilters({ filters, onChange }: Props) {
  const [collapsed, setCollapsed] = useState(false)
  const [cyclingExpanded, setCyclingExpanded] = useState(false)

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

  function toggleAll() {
    if (allSelected) {
      onChange({ ...filters, sportTypes: new Set() })
    } else {
      onChange({ ...filters, sportTypes: new Set(ALL_SPORT_KEYS) })
    }
  }

  return (
    <div className="absolute top-4 right-4 z-10 w-64">
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
                    {opt.children ? (
                      /* Parent with children (Radfahren) */
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
                            onClick={() => setCyclingExpanded(!cyclingExpanded)}
                            className="flex items-center gap-1 text-sm text-neutral-700 dark:text-neutral-300 hover:text-neutral-900 dark:hover:text-neutral-100 cursor-pointer"
                          >
                            <svg
                              className={`w-3 h-3 text-neutral-400 transition-transform ${cyclingExpanded ? '' : '-rotate-90'}`}
                              viewBox="0 0 20 20"
                              fill="currentColor"
                            >
                              <path fillRule="evenodd" d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z" clipRule="evenodd" />
                            </svg>
                            {opt.label}
                          </button>
                        </div>
                        {cyclingExpanded && (
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

            {/* Reset */}
            {(!allSelected || filters.dateFrom || filters.dateTo || filters.opacity !== 70) && (
              <button
                onClick={() => onChange({
                  sportTypes: new Set(ALL_SPORT_KEYS),
                  dateFrom: '',
                  dateTo: '',
                  opacity: 70,
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
