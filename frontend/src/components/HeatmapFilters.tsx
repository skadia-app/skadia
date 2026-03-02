import { useState } from 'react'
import type { HeatmapFilterState } from '@/pages/HeatmapPage'

const SPORT_OPTIONS = [
  { value: 'cycling', label: 'Radfahren' },
  { value: 'running', label: 'Laufen' },
  { value: 'hiking', label: 'Wandern' },
  { value: 'walking', label: 'Gehen' },
  { value: 'swimming', label: 'Schwimmen' },
  { value: 'e_biking', label: 'E-Bike' },
  { value: 'skiing', label: 'Ski' },
  { value: 'paddling', label: 'Paddeln' },
]

interface Props {
  filters: HeatmapFilterState
  onChange: (filters: HeatmapFilterState) => void
}

export default function HeatmapFilters({ filters, onChange }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  function toggleSport(sport: string) {
    const next = new Set(filters.sportTypes)
    if (next.has(sport)) {
      next.delete(sport)
    } else {
      next.add(sport)
    }
    onChange({ ...filters, sportTypes: next })
  }

  return (
    <div className="absolute top-4 right-4 z-10 w-64">
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white/95 dark:bg-neutral-900/95 backdrop-blur-sm shadow-lg overflow-hidden">
        {/* Header */}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="w-full flex items-center justify-between px-4 py-2.5 text-sm font-medium text-neutral-900 dark:text-neutral-100 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors"
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
              <div className="text-xs font-medium text-neutral-500 dark:text-neutral-400 mb-2">Sportart</div>
              <div className="space-y-1.5">
                {SPORT_OPTIONS.map(({ value, label }) => (
                  <label key={value} className="flex items-center gap-2 cursor-pointer group">
                    <input
                      type="checkbox"
                      checked={filters.sportTypes.has(value)}
                      onChange={() => toggleSport(value)}
                      className="w-3.5 h-3.5 rounded border-neutral-300 dark:border-neutral-600 text-orange-500 focus:ring-orange-500 focus:ring-1"
                    />
                    <span className="text-sm text-neutral-700 dark:text-neutral-300 group-hover:text-neutral-900 dark:group-hover:text-neutral-100">
                      {label}
                    </span>
                  </label>
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
          </div>
        )}
      </div>
    </div>
  )
}
