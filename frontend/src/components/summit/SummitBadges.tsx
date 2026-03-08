import React from 'react'

interface Peak {
  elevation_m: number | null
  name: string
  osm_id: number | null
}

interface SummitBadgesProps {
  peaks: Peak[]
}

const HEIGHT_CLASSES = [
  { label: '1000er', min: 1000, max: 1999, color: '#22C55E' },
  { label: '2000er', min: 2000, max: 2999, color: '#3B82F6' },
  { label: '3000er', min: 3000, max: 3999, color: '#8B5CF6' },
  { label: '4000er', min: 4000, max: 99999, color: '#EF4444' },
]

const MILESTONES = [
  { count: 10, label: '10 Gipfel' },
  { count: 25, label: '25 Gipfel' },
  { count: 50, label: '50 Gipfel' },
  { count: 100, label: '100 Gipfel' },
  { count: 250, label: '250 Gipfel' },
]

function MountainIcon({ color }: { color: string }) {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
    </svg>
  )
}

export default function SummitBadges({ peaks }: SummitBadgesProps) {
  const total = peaks.length

  if (total === 0) return null

  return (
    <div className="space-y-4">
      {/* Höhenklassen-Badges */}
      <div>
        <p className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
          Höhenklassen
        </p>
        <div className="flex flex-wrap gap-3">
          {HEIGHT_CLASSES.map(cls => {
            const count = peaks.filter(
              p => p.elevation_m != null && p.elevation_m >= cls.min && p.elevation_m <= cls.max
            ).length
            const achieved = count > 0
            return (
              <div
                key={cls.label}
                className={`flex items-center gap-2 px-3 py-2 rounded-lg border transition-all ${
                  achieved ? '' : 'opacity-40'
                }`}
                style={
                  achieved
                    ? { borderColor: cls.color, backgroundColor: cls.color + '18' }
                    : { borderColor: '#d1d5db' }
                }
              >
                <MountainIcon color={achieved ? cls.color : '#9ca3af'} />
                <div>
                  <p
                    className="text-sm font-semibold leading-tight"
                    style={{ color: achieved ? cls.color : '#9ca3af' }}
                  >
                    {cls.label}
                  </p>
                  <p className="text-xs text-neutral-500">
                    {count} {count === 1 ? 'Gipfel' : 'Gipfel'}
                  </p>
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {/* Meilenstein-Badges */}
      <div>
        <p className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
          Meilensteine — {total} Gipfel gesamt
        </p>
        <div className="flex flex-wrap gap-2">
          {MILESTONES.map(ms => {
            const achieved = total >= ms.count
            return (
              <span
                key={ms.count}
                className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium transition-all ${
                  achieved
                    ? 'bg-accent-100 text-accent-700 dark:bg-accent-900/40 dark:text-accent-300'
                    : 'bg-neutral-100 text-neutral-400 dark:bg-neutral-800 dark:text-neutral-600'
                }`}
              >
                {achieved ? (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <polyline points="20 6 9 17 4 12" />
                  </svg>
                ) : (
                  <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <circle cx="12" cy="12" r="9" />
                  </svg>
                )}
                {ms.label}
              </span>
            )
          })}
        </div>
      </div>
    </div>
  )
}
