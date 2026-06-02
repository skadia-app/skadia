import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import type { ActivityEvent } from '@/api/types'

interface Props { events: ActivityEvent[] | undefined }

export function RadarSummary({ events }: Props) {
  const { t } = useTranslation()
  const summary = useMemo(() => {
    if (!events?.length) return null
    const radarEvents = events.filter(e => e.radar_threat_count != null && e.radar_threat_count > 0)
    if (radarEvents.length === 0) return null
    const totalCount = radarEvents.reduce((acc, e) => acc + (e.radar_threat_count ?? 0), 0)
    const maxThreat = Math.max(...radarEvents.map(e => e.radar_threat_level_max ?? 0))
    return { totalCount, maxThreat }
  }, [events])

  if (!summary) return null

  return (
    <p className="text-sm text-neutral-600 dark:text-neutral-400 flex items-center gap-1.5">
      <span aria-hidden>&#x1F697;</span>
      {t('activityDetail.radarSummary.detected', {
        count: summary.totalCount >= 500 ? '500+' : summary.totalCount,
      })}
      {summary.maxThreat > 0 && ` (${t('activityDetail.radarSummary.maxThreat', { level: summary.maxThreat })})`}
    </p>
  )
}
