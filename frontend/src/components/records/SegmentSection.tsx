import { useState } from 'react'
import { Link } from 'react-router-dom'
import { MapPin } from 'lucide-react'
import { fmtDateMedium, fmtDuration } from '@/lib/format'
import type { SegmentEffort, GlobalTopSegmentEffort } from '@/api/segments'
import { cn } from '@/lib/utils'

const RANK_STYLES: Record<number, { bg: string; text: string }> = {
  1: { bg: 'bg-yellow-500/20 dark:bg-yellow-500/20', text: 'text-yellow-600 dark:text-yellow-400' },
  2: { bg: 'bg-neutral-200 dark:bg-neutral-700', text: 'text-neutral-500 dark:text-neutral-400' },
  3: { bg: 'bg-orange-100 dark:bg-orange-500/15', text: 'text-orange-600 dark:text-orange-400' },
}

const DEFAULT_RANK_STYLE = { bg: 'bg-neutral-100 dark:bg-neutral-800', text: 'text-neutral-400 dark:text-neutral-500' }

function rankStyle(rank: number | undefined) {
  if (rank == null) return DEFAULT_RANK_STYLE
  return RANK_STYLES[rank] ?? DEFAULT_RANK_STYLE
}

interface SegmentSectionProps {
  personalPRs: SegmentEffort[]
  globalTop: GlobalTopSegmentEffort[]
  sportFilter?: string[]
}

export function SegmentSection({ personalPRs, globalTop, sportFilter }: SegmentSectionProps) {
  const [tab, setTab] = useState<'personal' | 'global'>('personal')

  const filteredPersonal = sportFilter
    ? personalPRs.filter((e) => sportFilter.includes(e.sport_type))
    : personalPRs

  const filteredGlobal = sportFilter
    ? globalTop.filter((e) => sportFilter.includes(e.sport_type))
    : globalTop

  if (filteredPersonal.length === 0 && filteredGlobal.length === 0) return null

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 overflow-hidden">
      <div className="px-5 py-3 bg-neutral-50 dark:bg-neutral-800/60 border-b border-neutral-200 dark:border-neutral-800 flex items-center gap-3">
        <MapPin className="w-4 h-4 text-neutral-500" />
        <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Segmente</span>
        <div className="flex gap-1 ml-3">
          <button
            onClick={() => setTab('personal')}
            className={cn(
              'px-3 py-1 rounded-md text-xs font-medium border',
              tab === 'personal'
                ? 'bg-orange-500/15 text-orange-500 dark:text-orange-400 border-orange-500/25'
                : 'text-neutral-500 border-neutral-300 dark:border-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
            )}
          >
            Persönlich
          </button>
          <button
            onClick={() => setTab('global')}
            className={cn(
              'px-3 py-1 rounded-md text-xs font-medium border',
              tab === 'global'
                ? 'bg-orange-500/15 text-orange-500 dark:text-orange-400 border-orange-500/25'
                : 'text-neutral-500 border-neutral-300 dark:border-neutral-700 hover:text-neutral-700 dark:hover:text-neutral-300',
            )}
          >
            Global Top 10
          </button>
        </div>
      </div>
      <div className="divide-y divide-neutral-100 dark:divide-neutral-800/50">
        {tab === 'personal' && (filteredPersonal.length === 0
          ? <div className="px-4 py-6 text-center text-sm text-neutral-500">Keine Segment-Daten</div>
          : filteredPersonal.map((effort) => {
              const style = rankStyle(effort.personal_rank)
              return (
                <Link key={effort.id} to={`/activities/${effort.activity_id}`}
                  className="px-4 py-2.5 flex items-center gap-3 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors">
                  <span className={cn('w-6 h-6 rounded-full text-[10px] font-bold flex items-center justify-center', style.bg, style.text)}>
                    {effort.personal_rank ? `#${effort.personal_rank}` : '—'}
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200 truncate">{effort.segment_name ?? 'Segment'}</div>
                    <div className="text-[10px] text-neutral-500">{fmtDateMedium(effort.started_at)}</div>
                  </div>
                  <span className="text-sm font-bold tabular-nums">{fmtDuration(effort.elapsed_time_s)}</span>
                </Link>
              )
            })
        )}
        {tab === 'global' && (filteredGlobal.length === 0
          ? <div className="px-4 py-6 text-center text-sm text-neutral-500">Keine Segment-Daten</div>
          : filteredGlobal.map((effort) => {
              const style = rankStyle(effort.rank_overall)
              return (
                <Link key={effort.id} to={`/activities/${effort.activity_id}`}
                  className="px-4 py-2.5 flex items-center gap-3 hover:bg-neutral-50 dark:hover:bg-neutral-800/30 transition-colors">
                  <span className={cn('w-6 h-6 rounded-full text-[10px] font-bold flex items-center justify-center', style.bg, style.text)}>
                    {effort.rank_overall ? `#${effort.rank_overall}` : '—'}
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200 truncate">{effort.segment_name ?? 'Segment'}</div>
                    <div className="text-[10px] text-neutral-500">
                      {fmtDuration(effort.elapsed_time_s)} · #{effort.rank_overall} von {effort.athlete_count} Athleten
                    </div>
                  </div>
                  <span className="text-sm font-bold tabular-nums">{fmtDuration(effort.elapsed_time_s)}</span>
                  {effort.rank_overall === 1 && (
                    <span className="px-2 py-0.5 rounded text-[10px] font-bold bg-yellow-500/15 text-yellow-600 dark:text-yellow-400 border border-yellow-500/20">
                      Rekord
                    </span>
                  )}
                </Link>
              )
            })
        )}
      </div>
    </div>
  )
}
