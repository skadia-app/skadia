import { useQuery } from '@tanstack/react-query'
import { useState, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Wrench } from 'lucide-react'
import { sportLabel } from '@/lib/sportTaxonomy'
import { apiClient } from '@/api/client'
import { fmtDuration, fmtDateCompact } from '@/lib/format'
import type { Segment } from '@/api/segments'
import type { ActivityFilterState } from '@/components/ActivityFilters'
import { isSupportedLanguage } from '@/i18n/languages'
import { supportedLanguageFrom } from '@/i18n/localizedString'

interface Performance {
  id: string
  activity_id: string
  elapsed_time_s: number
  moving_time_s: number
  started_at: string
  sport_type: string
  sub_type?: string | null
  title: string
  is_reverse: boolean
  gear_id?: string | null
  gear_name?: string | null
  gear_type?: string | null
  timezone?: string | null
}

function getDirectionLabels(segment: Segment | undefined, t: TFunction) {
  if (segment?.segment_type === 'lift') {
    return {
      standard: t('segmentDetail.activityList.directions.liftUp'),
      reverse: t('segmentDetail.activityList.directions.liftDown'),
      all: t('common.all'),
    }
  }
  // Track-Orientierung beim Erstellen = climb_type. Also ist "standard"-Richtung
  // exakt der climb_type des Segments, "reverse" die Gegenrichtung.
  if (segment?.climb_type === 'bergab') {
    return {
      standard: t('segmentsList.elevationKinds.downhill'),
      reverse: t('segmentsList.elevationKinds.uphill'),
      all: t('common.all'),
    }
  }
  if (segment?.climb_type === 'bergauf') {
    return {
      standard: t('segmentsList.elevationKinds.uphill'),
      reverse: t('segmentsList.elevationKinds.downhill'),
      all: t('common.all'),
    }
  }
  return {
    standard: t('segmentDetail.activityList.directions.standard'),
    reverse: t('segmentDetail.activityList.directions.reverse'),
    all: t('common.all'),
  }
}

function getAvailableModes(segment?: Segment): Array<'standard' | 'reverse' | 'all'> {
  // Nicht-bidirektionale Segmente haben nur eine sinnvolle Richtung — die gespeicherte.
  if (segment && !segment.bidirectional) return ['standard']
  return ['standard', 'reverse', 'all']
}

interface SegmentActivityListProps {
  segmentId: string
  segment?: Segment
  filters?: ActivityFilterState
}


export function SegmentActivityList({ segmentId, segment, filters }: SegmentActivityListProps) {
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const language = supportedLanguageFrom(isSupportedLanguage(i18n.language) ? i18n.language : 'de')
  const availableModes = getAvailableModes(segment)
  const [directionMode, setDirectionMode] = useState<'standard' | 'reverse' | 'all'>('standard')
  const directionLabels = getDirectionLabels(segment, t)

  const isReverse = directionMode === 'all' ? undefined : directionMode === 'reverse'

  const { data, isLoading } = useQuery({
    queryKey: ['segment-performances', segmentId, directionMode],
    queryFn: () => {
      const qs = isReverse !== undefined ? `?is_reverse=${isReverse}` : ''
      return apiClient<{ performances: Performance[] }>(`/segments/${segmentId}/performances${qs}`)
    },
  })

  const all: Performance[] = data?.performances ?? []

  const filtered = useMemo(() => {
    return all.filter(p => {
      const date = new Date(p.started_at)

      if (filters?.dateFrom) {
        const from = new Date(filters.dateFrom)
        from.setHours(0, 0, 0, 0)
        if (date < from) return false
      }
      if (filters?.dateTo) {
        const to = new Date(filters.dateTo)
        to.setHours(23, 59, 59, 999)
        if (date > to) return false
      }

      if (filters?.sportTypes && filters.sportTypes.size > 0) {
        // sportTypes uses "sport" or "sport:subtype" keys.
        const matches = [...filters.sportTypes].some(key => {
          const [sport, sub] = key.split(':')
          return sport === p.sport_type && (!sub || sub === (p.sub_type ?? ''))
        })
        if (!matches) return false
      }

      if (filters?.gearFilters && filters.gearFilters.length > 0) {
        const includes = filters.gearFilters.filter(gf => gf.state === 'include')
        const excludes = filters.gearFilters.filter(gf => gf.state === 'exclude')
        if (includes.length > 0 && (!p.gear_id || !includes.some(gf => gf.id === p.gear_id))) return false
        if (excludes.length > 0 && p.gear_id && excludes.some(gf => gf.id === p.gear_id)) return false
      }

      return true
    })
  }, [all, filters])

  return (
    <div className="flex flex-col gap-3">
      {/* Direction controls — bei nicht-bidirektionalen Segmenten nur eine Richtung anzeigen */}
      {availableModes.length > 1 && <div className="flex gap-2">
        {availableModes.map(mode => (
          <button
            key={mode}
            onClick={() => setDirectionMode(mode)}
            className={`px-3 py-1 rounded text-sm cursor-pointer transition-colors ${
              directionMode === mode
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {directionLabels[mode as 'standard' | 'reverse' | 'all']}
          </button>
        ))}
      </div>}

      {/* Activity list */}
      <div className="flex flex-col gap-2 max-h-72 overflow-y-auto">
        {isLoading && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">{t('segmentDetail.activityList.loading')}</div>
        )}
        {!isLoading && filtered.length === 0 && (
          <div className="text-sm text-neutral-500 dark:text-neutral-400">
            {t('segmentDetail.activityList.empty')}
          </div>
        )}
        {filtered.map(perf => (
          <button
            key={perf.id}
            onClick={() => navigate(`/activities/${perf.activity_id}`)}
            className="text-left p-3 border border-neutral-200 dark:border-neutral-700 rounded-lg hover:border-neutral-300 dark:hover:border-neutral-600 hover:bg-neutral-50 dark:hover:bg-neutral-800/50 transition-colors cursor-pointer"
          >
            <div className="font-medium text-sm truncate text-neutral-900 dark:text-neutral-100">
              {perf.title || sportLabel(perf.sport_type, perf.sub_type, { language })}
            </div>
            <div className="text-xs text-neutral-500 dark:text-neutral-400 flex flex-wrap gap-2 mt-1">
              <span>{fmtDateCompact(perf.started_at, perf.timezone)}</span>
              <span>·</span>
              <span>{fmtDuration(perf.elapsed_time_s)}</span>
              <span>·</span>
              <span>{sportLabel(perf.sport_type, perf.sub_type, { language })}</span>
              {perf.gear_name && (
                <>
                  <span>·</span>
                  <span className="inline-flex items-center gap-1 text-neutral-600 dark:text-neutral-400">
                    <Wrench className="w-3 h-3" />
                    {perf.gear_name}
                  </span>
                </>
              )}
            </div>
          </button>
        ))}
      </div>

      {/* Result count */}
      {!isLoading && all.length > 0 && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400">
          {filtered.length === all.length
            ? t('segmentDetail.activityList.count', { count: all.length })
            : t('segmentDetail.activityList.filteredCount', { count: filtered.length, filtered: filtered.length, total: all.length })}
        </div>
      )}
    </div>
  )
}
