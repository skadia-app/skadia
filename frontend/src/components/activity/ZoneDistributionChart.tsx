import { useState } from 'react'
import type { ZoneDistribution } from '@/api/types'

interface ZoneDistributionChartProps {
  distribution: ZoneDistribution
}

const ZONE_COLORS = ['#94a3b8', '#60a5fa', '#4ade80', '#facc15', '#f97316', '#ef4444', '#a855f7']

export default function ZoneDistributionChart({ distribution }: ZoneDistributionChartProps) {
  const [activeType, setActiveType] = useState<'hr' | 'power'>(
    distribution.hr_zones.length > 0 ? 'hr' : 'power'
  )

  const zones = activeType === 'hr' ? distribution.hr_zones : distribution.power_zones
  if (zones.length === 0) return null

  const totalSeconds = zones.reduce((sum, z) => sum + z.seconds, 0)

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-neutral-700 dark:text-neutral-300">Zonen-Verteilung</h3>
        {distribution.hr_zones.length > 0 && distribution.power_zones.length > 0 && (
          <div className="flex gap-2 text-xs">
            <button
              onClick={() => setActiveType('hr')}
              className={`px-2 py-0.5 rounded cursor-pointer ${activeType === 'hr' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-500 hover:text-neutral-700'}`}
            >HR</button>
            <button
              onClick={() => setActiveType('power')}
              className={`px-2 py-0.5 rounded cursor-pointer ${activeType === 'power' ? 'bg-accent-DEFAULT text-white' : 'text-neutral-500 hover:text-neutral-700'}`}
            >Power</button>
          </div>
        )}
      </div>
      <div className="flex h-8 rounded-full overflow-hidden">
        {zones.map((z, i) => {
          const pct = totalSeconds > 0 ? (z.seconds / totalSeconds) * 100 : 0
          const color = ZONE_COLORS[z.zone_index] ?? ZONE_COLORS[ZONE_COLORS.length - 1]
          return (
            <div
              key={i}
              style={{ width: `${pct}%`, backgroundColor: color }}
              title={`Z${z.zone_index + 1}: ${Math.round(z.seconds / 60)}min (${pct.toFixed(1)}%)`}
            />
          )
        })}
      </div>
      <div className="flex flex-wrap gap-2">
        {zones.map((z, i) => {
          const pct = totalSeconds > 0 ? (z.seconds / totalSeconds) * 100 : 0
          const color = ZONE_COLORS[z.zone_index] ?? ZONE_COLORS[ZONE_COLORS.length - 1]
          return (
            <div key={i} className="flex items-center gap-1 text-xs text-neutral-600 dark:text-neutral-400">
              <span className="inline-block w-3 h-3 rounded-sm" style={{ backgroundColor: color }} />
              Z{z.zone_index + 1}: {Math.round(z.seconds / 60)}min ({pct.toFixed(0)}%)
            </div>
          )
        })}
      </div>
    </div>
  )
}
