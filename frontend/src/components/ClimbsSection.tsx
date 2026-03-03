import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchActivityClimbs } from '@/api/enrichment'
import type { Climb } from '@/api/types'

interface Props {
  activityId: string
}

const categoryStyles: Record<string, { label: string; bg: string; text: string }> = {
  HC: { label: 'HC', bg: 'bg-purple-100 dark:bg-purple-900/30', text: 'text-purple-700 dark:text-purple-300' },
  '1': { label: 'Kat. 1', bg: 'bg-red-100 dark:bg-red-900/30', text: 'text-red-700 dark:text-red-300' },
  '2': { label: 'Kat. 2', bg: 'bg-orange-100 dark:bg-orange-900/30', text: 'text-orange-700 dark:text-orange-300' },
  '3': { label: 'Kat. 3', bg: 'bg-yellow-100 dark:bg-yellow-900/30', text: 'text-yellow-700 dark:text-yellow-300' },
  '4': { label: 'Kat. 4', bg: 'bg-green-100 dark:bg-green-900/30', text: 'text-green-700 dark:text-green-300' },
}

function gradientColor(pct: number): string {
  if (pct >= 10) return 'bg-red-500'
  if (pct >= 6) return 'bg-yellow-500'
  if (pct >= 3) return 'bg-green-500'
  return 'bg-neutral-400'
}

function CategoryBadge({ category }: { category: string }) {
  const style = categoryStyles[category]
  if (!style) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400">
        --
      </span>
    )
  }
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-bold ${style.bg} ${style.text}`}>
      {style.label}
    </span>
  )
}

function GradientBar({ pct }: { pct: number }) {
  const width = Math.min(Math.max(pct / 15 * 100, 8), 100)
  return (
    <div className="flex items-center gap-2">
      <div className="w-16 h-2 rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
        <div
          className={`h-full rounded-full ${gradientColor(pct)}`}
          style={{ width: `${width}%` }}
        />
      </div>
      <span className="tabular-nums text-neutral-700 dark:text-neutral-300">
        {pct.toFixed(1)}%
      </span>
    </div>
  )
}

export default function ClimbsSection({ activityId }: Props) {
  const [collapsed, setCollapsed] = useState(false)

  const { data: climbs = [], isLoading, isError } = useQuery({
    queryKey: ['activity-climbs', activityId],
    queryFn: () => fetchActivityClimbs(activityId),
  })

  if (isLoading) return null
  if (isError) return null
  if (climbs.length === 0) return null

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="flex items-center gap-2 mb-3 group"
      >
        <svg
          className={`w-4 h-4 text-neutral-400 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 group-hover:text-orange-500 transition-colors">
          Anstiege
        </h2>
        <span className="text-sm text-neutral-400">({climbs.length})</span>
      </button>

      {!collapsed && (
        <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">#</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Länge</th>
                <th className="text-right px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Höhengewinn</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Steigung</th>
                <th className="text-left px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Max. Steigung</th>
                <th className="text-center px-3 py-2 text-xs font-medium text-neutral-500 dark:text-neutral-400">Kategorie</th>
              </tr>
            </thead>
            <tbody>
              {climbs.map((climb, i) => (
                <tr
                  key={climb.id}
                  className={`border-b border-neutral-100 dark:border-neutral-800 transition-colors ${
                    i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''
                  }`}
                >
                  <td className="px-3 py-2 text-center text-xs text-neutral-600 dark:text-neutral-400">
                    {climb.climb_number}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-900 dark:text-neutral-100 font-medium">
                    {climb.length_m >= 1000
                      ? `${(climb.length_m / 1000).toFixed(1)} km`
                      : `${Math.round(climb.length_m)} m`}
                  </td>
                  <td className="px-3 py-2 text-right tabular-nums text-neutral-700 dark:text-neutral-300">
                    {Math.round(climb.elevation_gain_m)} m
                  </td>
                  <td className="px-3 py-2">
                    <GradientBar pct={climb.avg_gradient_pct} />
                  </td>
                  <td className="px-3 py-2">
                    <GradientBar pct={climb.max_gradient_pct} />
                  </td>
                  <td className="px-3 py-2 text-center">
                    <CategoryBadge category={climb.category} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
