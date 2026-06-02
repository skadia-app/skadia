import { useState, useRef, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import type { Split, SplitSummary } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { formatSpeedValue } from '@/lib/sportFieldLabels'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

interface Props {
  splits: Split[]
  summaries: SplitSummary[]
  sportType: string
  subType?: string
  activityId: string
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

function splitTypeLabel(type: string, t: TFunction, subType?: string): string {
  if (type === 'climb_active') {
    return subType === 'bouldering'
      ? t('activityTables.splits.types.bouldering')
      : t('activityTables.splits.types.climbing')
  }
  const labels: Record<string, string> = {
    climb_rest: t('activityTables.splits.types.rest'),
    ski_run: t('activityTables.splits.types.skiRun'),
    ski_lift: t('activityTables.splits.types.skiLift'),
    ascent: t('activityTables.splits.types.ascent'),
    descent: t('activityTables.splits.types.descent'),
    flat: t('activityTables.splits.types.flat'),
  }
  return labels[type] ?? t('activityTables.splits.types.unknown')
}

function splitTypeBadgeColor(type: string): string {
  switch (type) {
    case 'climb_active': return 'bg-accent-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400'
    case 'climb_rest': return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400'
    case 'ski_run': return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
    case 'ski_lift': return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400'
    case 'ascent': return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
    case 'descent': return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
    default: return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
  }
}

function GradeCell({ splitId, activityId, initialGrade }: { splitId: string; activityId: string; initialGrade?: string }) {
  const { t } = useTranslation()
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(initialGrade ?? '')
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus()
      inputRef.current.select()
    }
  }, [editing])

  async function save() {
    setSaving(true)
    try {
      await fetch(`/api/v1/activities/${activityId}/splits/${splitId}`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ climbing_grade: value.trim() || null }),
      })
    } catch {
      // Ignore save failures silently; the edited value remains in local state.
    } finally {
      setSaving(false)
      setEditing(false)
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter') save()
    if (e.key === 'Escape') {
      setValue(initialGrade ?? '')
      setEditing(false)
    }
  }

  if (editing) {
    return (
      <input
        ref={inputRef}
        value={value}
        onChange={e => setValue(e.target.value)}
        onBlur={save}
        onKeyDown={handleKeyDown}
        disabled={saving}
        placeholder={t('activityTables.splits.gradePlaceholder')}
        className="w-16 px-1 py-0.5 text-xs border border-accent-DEFAULT rounded bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
      />
    )
  }

  return (
    <button
      onClick={() => setEditing(true)}
      title={t('activityTables.splits.editGradeTitle')}
      className="group flex items-center gap-1 text-left cursor-pointer"
    >
      {value ? (
        <span className="font-medium text-accent-DEFAULT">{value}</span>
      ) : (
        <span className="text-neutral-400 dark:text-neutral-600 italic text-xs">—</span>
      )}
      <svg
        className="w-3 h-3 text-neutral-300 dark:text-neutral-600 group-hover:text-accent-DEFAULT opacity-0 group-hover:opacity-100 transition-opacity"
        fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
      >
        <path strokeLinecap="round" strokeLinejoin="round" d="M15.232 5.232l3.536 3.536M9 11l6.586-6.586a2 2 0 112.828 2.828L11.828 13.828a4 4 0 01-1.414.828l-3 1 1-3a4 4 0 01.828-1.414z" />
      </svg>
    </button>
  )
}

export default function SplitTable({ splits, summaries, sportType, subType, activityId }: Props) {
  if (sportType === 'rock_climbing') return <ClimbingSplits splits={splits} summaries={summaries} subType={subType} activityId={activityId} />
  if (sportType === 'skiing') return <SkiSplits splits={splits} />
  return <GenericSplits splits={splits} sportType={sportType} />
}

function ClimbingSplits({ splits, summaries, subType, activityId }: { splits: Split[]; summaries: SplitSummary[]; subType?: string; activityId: string }) {
  const { t } = useTranslation()
  const isBouldering = subType === 'bouldering'
  const activeSplits = splits.filter(s => s.split_type === 'climb_active')
  const totalActiveTime = activeSplits.reduce((sum, s) => sum + (s.timer_time_s ?? s.elapsed_time_s ?? 0), 0)
  const totalCalories = splits.reduce((sum, s) => sum + (s.total_calories ?? 0), 0)

  const summaryForClimb = summaries.find(s => s.split_type === 'climb_active')
  const routeVerb = isBouldering
    ? t('activityTables.splits.climbingSummary.bouldered')
    : t('activityTables.splits.climbingSummary.climbed')
  const sectionTitle = isBouldering
    ? t('activityTables.splits.boulderRoutesTitle')
    : t('activityTables.splits.routesTitle')

  // Number climb_active splits separately from rest periods.
  let routeCounter = 0

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">{sectionTitle}</h2>

      {/* Summary card */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 mb-3 flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span className="text-neutral-700 dark:text-neutral-300">
          <span className="font-medium">{summaryForClimb?.num_splits ?? activeSplits.length}</span> {t('activityTables.splits.climbingSummary.routes')} {routeVerb}
        </span>
        <span className="text-neutral-700 dark:text-neutral-300">
          <span className="font-medium">{fmtDuration(totalActiveTime)}</span> {t('activityTables.splits.climbingSummary.active')}
        </span>
        {totalCalories > 0 && (
          <span className="text-neutral-700 dark:text-neutral-300">
            <span className="font-medium">{Math.round(totalCalories)}</span> kcal
          </span>
        )}
      </div>

      {/* Table */}
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>{t('activityTables.splits.headers.type')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.grade')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.duration')}</th>
              <th className={thClass}>HR</th>
              <th className={thClass}>{t('activityStats.calories')} (kcal)</th>
            </tr>
          </thead>
          <tbody>
            {splits.map((s, i) => {
              const isActive = s.split_type === 'climb_active'
              if (isActive) routeCounter++
              const routeNum = isActive ? routeCounter : null
              return (
                <tr
                  key={s.id}
                  className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
                >
                  <td className={`${tdClass} text-neutral-400`}>{routeNum ?? ''}</td>
                  <td className={tdClass}>
                    <span className={`text-xs px-1.5 py-0.5 rounded ${splitTypeBadgeColor(s.split_type)}`}>
                      {splitTypeLabel(s.split_type, t, subType)}
                    </span>
                  </td>
                  <td className={tdClass}>
                    {isActive ? (
                      <GradeCell splitId={s.id} activityId={activityId} initialGrade={s.climbing_grade} />
                    ) : (
                      <span className="text-neutral-300 dark:text-neutral-700">—</span>
                    )}
                  </td>
                  <td className={tdClass}>{fmtDuration(s.timer_time_s ?? s.elapsed_time_s)}</td>
                  <td className={tdClass}>{s.avg_hr != null ? `${Math.round(s.avg_hr)} bpm` : '--'}</td>
                  <td className={tdClass}>{s.total_calories != null ? Math.round(s.total_calories) : '--'}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className="mt-2 text-xs text-neutral-400 dark:text-neutral-600">
        {t('activityTables.splits.gradeHint')}
      </p>
    </div>
  )
}

function SkiSplits({ splits }: { splits: Split[] }) {
  const { t } = useTranslation()
  const { options, formatDistance, formatElevation } = useUnitFormatters()
  const showElevation = splits.some(s => s.total_ascent_m != null || s.total_descent_m != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">{t('activityTables.splits.title')}</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>{t('activityTables.splits.headers.type')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.duration')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.distance')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.speed')}</th>
              {showElevation && <th className={thClass}>{t('activityTables.splits.headers.ascent')}</th>}
              {showElevation && <th className={thClass}>{t('activityTables.splits.headers.descent')}</th>}
            </tr>
          </thead>
          <tbody>
            {splits.map((s, i) => (
              <tr
                key={s.id}
                className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
              >
                <td className={tdClass}>{s.split_index + 1}</td>
                <td className={tdClass}>
                  <span className={`text-xs px-1.5 py-0.5 rounded ${splitTypeBadgeColor(s.split_type)}`}>
                    {splitTypeLabel(s.split_type, t)}
                  </span>
                </td>
                <td className={tdClass}>{fmtDuration(s.timer_time_s ?? s.elapsed_time_s)}</td>
                <td className={tdClass}>{formatDistance(s.distance_m)}</td>
                <td className={tdClass}>{formatSpeedValue(s.avg_speed_ms ?? 0, 'skiing', 'speed', options)}</td>
                {showElevation && <td className={tdClass}>{s.total_ascent_m != null ? `+${formatElevation(s.total_ascent_m)}` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_descent_m != null ? `-${formatElevation(s.total_descent_m)}` : '--'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function GenericSplits({ splits, sportType }: { splits: Split[]; sportType: string }) {
  const { t } = useTranslation()
  const { options, formatDistance, formatElevation } = useUnitFormatters()
  const showHR = splits.some(s => s.avg_hr != null)
  const showElevation = splits.some(s => s.total_ascent_m != null || s.total_descent_m != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">{t('activityTables.splits.title')}</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>{t('activityTables.splits.headers.type')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.duration')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.distance')}</th>
              <th className={thClass}>{t('activityTables.splits.headers.speed')}</th>
              {showHR && <th className={thClass}>HR</th>}
              {showElevation && <th className={thClass}>{t('activityTables.splits.headers.ascent')}</th>}
              {showElevation && <th className={thClass}>{t('activityTables.splits.headers.descent')}</th>}
            </tr>
          </thead>
          <tbody>
            {splits.map((s, i) => (
              <tr
                key={s.id}
                className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
              >
                <td className={tdClass}>{s.split_index + 1}</td>
                <td className={tdClass}>
                  <span className={`text-xs px-1.5 py-0.5 rounded ${splitTypeBadgeColor(s.split_type)}`}>
                    {splitTypeLabel(s.split_type, t)}
                  </span>
                </td>
                <td className={tdClass}>{fmtDuration(s.timer_time_s ?? s.elapsed_time_s)}</td>
                <td className={tdClass}>{formatDistance(s.distance_m)}</td>
                <td className={tdClass}>{formatSpeedValue(s.avg_speed_ms ?? 0, sportType, undefined, options)}</td>
                {showHR && <td className={tdClass}>{s.avg_hr != null ? `${Math.round(s.avg_hr)} bpm` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_ascent_m != null ? `+${formatElevation(s.total_ascent_m)}` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_descent_m != null ? `-${formatElevation(s.total_descent_m)}` : '--'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
