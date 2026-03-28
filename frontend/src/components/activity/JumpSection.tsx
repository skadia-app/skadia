import { useState } from 'react'
import type { ActivityJump } from '@/api/types'
import { formatSpeedKmh } from '@/lib/sportFieldLabels'

interface Props {
  jumpCount: number
  jumps: ActivityJump[] | undefined
}

export function JumpSection({ jumpCount, jumps }: Props) {
  const [expanded, setExpanded] = useState(false)

  if (jumpCount <= 0) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center justify-between cursor-pointer"
      >
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          Sprünge: {jumpCount}
        </h3>
        <span className="text-xs text-neutral-400">{expanded ? '▲' : '▼'}</span>
      </button>
      {expanded && jumps && jumps.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-xs text-neutral-500 dark:text-neutral-400 border-b border-neutral-100 dark:border-neutral-800">
                <th className="px-4 py-2 text-left">#</th>
                <th className="px-4 py-2 text-right">Höhe</th>
                <th className="px-4 py-2 text-right">Hangtime</th>
                <th className="px-4 py-2 text-right">Speed</th>
                <th className="px-4 py-2 text-right">Score</th>
              </tr>
            </thead>
            <tbody>
              {jumps.map((j, i) => (
                <tr key={j.timestamp} className="text-neutral-700 dark:text-neutral-300 border-b border-neutral-50 dark:border-neutral-800/50">
                  <td className="px-4 py-2 tabular-nums">{i + 1}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.height_m != null ? `${j.height_m.toFixed(1)} m` : '—'}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.hang_time_s != null ? `${j.hang_time_s.toFixed(1)} s` : '—'}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.speed_ms != null ? formatSpeedKmh(j.speed_ms) : '—'}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.score ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
