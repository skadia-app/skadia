import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { ActivityJump } from '@/api/types'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface Props {
  jumpCount: number
  jumps: ActivityJump[] | undefined
}

export function JumpSection({ jumpCount, jumps }: Props) {
  const { t } = useTranslation()
  const { options, formatSpeed } = useUnitFormatters()
  const [expanded, setExpanded] = useState(false)

  if (jumpCount <= 0) return null

  const formatJumpHeight = (meters: number) => {
    const value = options.units === 'imperial' ? meters * 3.280839895 : meters
    const unit = options.units === 'imperial' ? 'ft' : 'm'
    return `${new Intl.NumberFormat(options.locale, {
      minimumFractionDigits: 1,
      maximumFractionDigits: 1,
    }).format(value)} ${unit}`
  }

  const formatSeconds = (seconds: number) => `${new Intl.NumberFormat(options.locale, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(seconds)} s`

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center justify-between cursor-pointer"
      >
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          {t('activityDetail.jumps.title', { count: jumpCount })}
        </h3>
        <span className="text-xs text-neutral-400">{expanded ? '▲' : '▼'}</span>
      </button>
      {expanded && jumps && jumps.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-xs text-neutral-500 dark:text-neutral-400 border-b border-neutral-100 dark:border-neutral-800">
                <th className="px-4 py-2 text-left">#</th>
                <th className="px-4 py-2 text-right">{t('activityDetail.jumps.height')}</th>
                <th className="px-4 py-2 text-right">{t('activityDetail.jumps.hangTime')}</th>
                <th className="px-4 py-2 text-right">{t('activityDetail.jumps.speed')}</th>
                <th className="px-4 py-2 text-right">{t('activityDetail.jumps.score')}</th>
              </tr>
            </thead>
            <tbody>
              {jumps.map((j, i) => (
                <tr key={j.timestamp} className="text-neutral-700 dark:text-neutral-300 border-b border-neutral-50 dark:border-neutral-800/50">
                  <td className="px-4 py-2 tabular-nums">{i + 1}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.height_m != null ? formatJumpHeight(j.height_m) : '—'}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.hang_time_s != null ? formatSeconds(j.hang_time_s) : '—'}</td>
                  <td className="px-4 py-2 text-right tabular-nums">{j.speed_ms != null ? formatSpeed(j.speed_ms) : '—'}</td>
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
