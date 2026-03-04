import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router-dom'
import { fetchPersonalRecords } from '@/api/analytics'
import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { useMemo } from 'react'

const RECORD_TYPE_LABELS: Record<string, string> = {
  longest_distance: 'Längste Distanz',
  highest_elevation: 'Meiste Höhenmeter',
  longest_duration: 'Längste Dauer',
  fastest_pace: 'Schnellste Pace',
  highest_speed: 'Höchste Geschwindigkeit',
  max_power: 'Max. Leistung',
  max_hr: 'Max. Herzfrequenz',
  max_cadence: 'Max. Kadenz',
  highest_tss: 'Höchster TSS',
  most_calories: 'Meiste Kalorien',
  best_1km: 'Bestzeit 1 km',
  best_5km: 'Bestzeit 5 km',
  best_10km: 'Bestzeit 10 km',
  best_half_marathon: 'Halbmarathon',
  best_marathon: 'Marathon',
  best_5min_power: '5-Min-Leistung',
  best_20min_power: '20-Min-Leistung',
  best_60min_power: '60-Min-Leistung',
}

function TrendIcon({ trend }: { trend?: 'up' | 'down' | 'same' }) {
  if (trend === 'up') {
    return (
      <span className="inline-flex items-center text-emerald-500" title="Verbesserung">
        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 10l7-7m0 0l7 7m-7-7v18" />
        </svg>
      </span>
    )
  }
  if (trend === 'down') {
    return (
      <span className="inline-flex items-center text-rose-500" title="Verschlechterung">
        <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 14l-7 7m0 0l-7-7m7 7V3" />
        </svg>
      </span>
    )
  }
  return (
    <span className="inline-flex items-center text-neutral-400" title="Unverändert">
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
        <path strokeLinecap="round" strokeLinejoin="round" d="M5 12h14" />
      </svg>
    </span>
  )
}

function formatDate(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleDateString('de-DE', { year: 'numeric', month: 'short', day: 'numeric' })
  } catch {
    return dateStr
  }
}

export default function PersonalRecordsTable() {
  const { toQueryString } = useAnalyticsFilters()
  const navigate = useNavigate()

  const queryParams = useMemo(() => toQueryString(), [toQueryString])

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'personal-records', queryParams],
    queryFn: () => fetchPersonalRecords(queryParams),
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  const records = data ?? []

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
        Persönliche Bestleistungen
      </h3>

      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
        {isLoading ? (
          <div className="flex items-center justify-center h-[200px] text-sm text-neutral-400">
            Lade Bestleistungen…
          </div>
        ) : isError ? (
          <div className="flex items-center justify-center h-[160px] text-sm text-neutral-400">
            Fehler beim Laden
          </div>
        ) : records.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-[160px] gap-2 text-neutral-400">
            <svg className="w-8 h-8 opacity-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5}
                d="M9 12l2 2 4-4M7.835 4.697a3.42 3.42 0 001.946-.806 3.42 3.42 0 014.438 0 3.42 3.42 0 001.946.806 3.42 3.42 0 013.138 3.138 3.42 3.42 0 00.806 1.946 3.42 3.42 0 010 4.438 3.42 3.42 0 00-.806 1.946 3.42 3.42 0 01-3.138 3.138 3.42 3.42 0 00-1.946.806 3.42 3.42 0 01-4.438 0 3.42 3.42 0 00-1.946-.806 3.42 3.42 0 01-3.138-3.138 3.42 3.42 0 00-.806-1.946 3.42 3.42 0 010-4.438 3.42 3.42 0 00.806-1.946 3.42 3.42 0 013.138-3.138z" />
            </svg>
            <span className="text-sm">Keine Bestleistungen gefunden</span>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-neutral-100 dark:border-neutral-800">
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider">
                    Rekordtyp
                  </th>
                  <th className="text-right px-4 py-2.5 text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider">
                    Wert
                  </th>
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider hidden sm:table-cell">
                    Aktivität
                  </th>
                  <th className="text-left px-4 py-2.5 text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider">
                    Datum
                  </th>
                  <th className="text-center px-4 py-2.5 text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wider">
                    Trend
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-neutral-50 dark:divide-neutral-800/50">
                {records.map((record, idx) => (
                  <tr
                    key={`${record.type}-${idx}`}
                    onClick={() => record.activity_id && navigate(`/activities/${record.activity_id}`)}
                    className={`transition-colors ${record.activity_id ? 'cursor-pointer hover:bg-neutral-50 dark:hover:bg-neutral-800/60' : ''}`}
                  >
                    <td className="px-4 py-3">
                      <span className="font-medium text-neutral-800 dark:text-neutral-200">
                        {RECORD_TYPE_LABELS[record.type] ?? record.type}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-right tabular-nums">
                      <span className="font-semibold text-neutral-900 dark:text-neutral-100">
                        {record.value.toLocaleString('de-DE', { maximumFractionDigits: 2 })}
                      </span>
                      <span className="ml-1 text-xs text-neutral-400">{record.unit}</span>
                    </td>
                    <td className="px-4 py-3 hidden sm:table-cell">
                      {record.activity_name ? (
                        <span className="text-neutral-600 dark:text-neutral-300 truncate max-w-[200px] block">
                          {record.activity_name}
                        </span>
                      ) : (
                        <span className="text-neutral-400">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-neutral-500 dark:text-neutral-400 whitespace-nowrap">
                      {record.date ? formatDate(record.date) : '—'}
                    </td>
                    <td className="px-4 py-3 text-center">
                      <TrendIcon trend={(record as PersonalRecordWithTrend).trend} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

// Local extension to handle optional trend field from API
interface PersonalRecordWithTrend {
  trend?: 'up' | 'down' | 'same'
}
