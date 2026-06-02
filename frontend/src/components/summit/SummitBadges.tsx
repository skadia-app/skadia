import { useTranslation } from 'react-i18next'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { elevationInputUnit, elevationToDisplayValue, type UnitFormatOptions } from '@/lib/units'

interface Summit {
  elevation_m: number | null
  name: string
  osm_id: number | null
}

interface SummitBadgesProps {
  summits: Summit[]
}

const HEIGHT_CLASSES = [
  { min: 1000, max: 1999, color: '#22C55E' },
  { min: 2000, max: 2999, color: '#3B82F6' },
  { min: 3000, max: 3999, color: '#8B5CF6' },
  { min: 4000, max: null, color: '#EF4444' },
]

const MILESTONES = [
  { count: 10 },
  { count: 25 },
  { count: 50 },
  { count: 100 },
  { count: 250 },
]

function MountainIcon({ color }: { color: string }) {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
    </svg>
  )
}

function formatHeightClassLabel(
  min: number,
  max: number | null,
  options: UnitFormatOptions,
): string {
  const formatter = new Intl.NumberFormat(options.locale, { maximumFractionDigits: 0 })
  const unit = elevationInputUnit(options.units)
  const minDisplay = formatter.format(Math.round(elevationToDisplayValue(min, options.units)))
  if (max == null) return `${minDisplay}+ ${unit}`
  const maxDisplay = formatter.format(Math.round(elevationToDisplayValue(max, options.units)))
  return `${minDisplay}-${maxDisplay} ${unit}`
}

export default function SummitBadges({ summits }: SummitBadgesProps) {
  const { t } = useTranslation()
  const { options } = useUnitFormatters()
  const total = summits.length

  if (total === 0) return null

  return (
    <div className="space-y-4">
      {/* Elevation class badges */}
      <div>
        <p className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
          {t('summitCollection.badges.elevationClasses')}
        </p>
        <div className="flex flex-wrap gap-3">
          {HEIGHT_CLASSES.map(cls => {
            const count = summits.filter(
              s => s.elevation_m != null && s.elevation_m >= cls.min && (cls.max == null || s.elevation_m <= cls.max)
            ).length
            const achieved = count > 0
            const label = formatHeightClassLabel(cls.min, cls.max, options)
            return (
              <div
                key={label}
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
                    {label}
                  </p>
                  <p className="text-xs text-neutral-500">
                    {t('summitCollection.badges.summitCount', { count })}
                  </p>
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {/* Milestone badges */}
      <div>
        <p className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
          {t('summitCollection.badges.milestones', { total })}
        </p>
        <div className="flex flex-wrap gap-2">
          {MILESTONES.map(ms => {
            const achieved = total >= ms.count
            return (
              <span
                key={ms.count}
                className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-medium transition-all ${
                  achieved
                    ? 'bg-accent-100 text-accent-600 dark:bg-accent-DEFAULT/25 dark:text-accent-400'
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
                {t('summitCollection.badges.milestoneLabel', { count: ms.count })}
              </span>
            )
          })}
        </div>
      </div>
    </div>
  )
}
