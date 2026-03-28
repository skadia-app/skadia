import { useState, useRef, useEffect } from 'react'
import type { Split, SplitSummary } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { formatSpeedKmh } from '@/lib/sportFieldLabels'

interface Props {
  splits: Split[]
  summaries: SplitSummary[]
  sportType: string
  subType?: string
  activityId: string
}

function fmtDist(m?: number): string {
  if (m == null || isNaN(m)) return '--'
  if (m >= 1000) return `${(m / 1000).toFixed(2)} km`
  return `${Math.round(m)} m`
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

function splitTypeLabel(type: string, subType?: string): string {
  if (type === 'climb_active') {
    return subType === 'bouldering' ? 'Bouldern' : 'Klettern'
  }
  const labels: Record<string, string> = {
    climb_rest: 'Pause',
    ski_run: 'Abfahrt',
    ski_lift: 'Lift',
    ascent: 'Aufstieg',
    descent: 'Abstieg',
    flat: 'Flach',
  }
  return labels[type] ?? type
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

/** Inline-Edit-Zelle für Kletter-Schwierigkeitsgrade */
function GradeCell({ splitId, activityId, initialGrade }: { splitId: string; activityId: string; initialGrade?: string }) {
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
      // Fehler still ignorieren — Wert bleibt im State
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
        placeholder="z.B. 6b"
        className="w-16 px-1 py-0.5 text-xs border border-accent-DEFAULT rounded bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
      />
    )
  }

  return (
    <button
      onClick={() => setEditing(true)}
      title="Schwierigkeitsgrad bearbeiten"
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
  return <GenericSplits splits={splits} />
}

function ClimbingSplits({ splits, summaries, subType, activityId }: { splits: Split[]; summaries: SplitSummary[]; subType?: string; activityId: string }) {
  const isBouldering = subType === 'bouldering'
  const activeSplits = splits.filter(s => s.split_type === 'climb_active')
  const totalActiveTime = activeSplits.reduce((sum, s) => sum + (s.timer_time_s ?? s.elapsed_time_s ?? 0), 0)
  const totalCalories = splits.reduce((sum, s) => sum + (s.total_calories ?? 0), 0)

  const summaryForClimb = summaries.find(s => s.split_type === 'climb_active')
  const routenVerb = isBouldering ? 'geboulbert' : 'geklettert'
  const sectionTitle = isBouldering ? 'Boulder-Routen' : 'Routen'

  // Nur climb_active-Splits numerieren (separate Route-Zählung)
  let routeCounter = 0

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">{sectionTitle}</h2>

      {/* Summary card */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 mb-3 flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span className="text-neutral-700 dark:text-neutral-300">
          <span className="font-medium">{summaryForClimb?.num_splits ?? activeSplits.length}</span> Routen {routenVerb}
        </span>
        <span className="text-neutral-700 dark:text-neutral-300">
          <span className="font-medium">{fmtDuration(totalActiveTime)}</span> aktiv
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
              <th className={thClass}>Typ</th>
              <th className={thClass}>Grad</th>
              <th className={thClass}>Dauer</th>
              <th className={thClass}>HR</th>
              <th className={thClass}>kcal</th>
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
                      {splitTypeLabel(s.split_type, subType)}
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
        Grad-Spalte: Klicken zum Eintragen (z.B. 6b, V4, 7A+)
      </p>
    </div>
  )
}

function SkiSplits({ splits }: { splits: Split[] }) {
  const showElevation = splits.some(s => s.total_ascent_m != null || s.total_descent_m != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Splits</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>Typ</th>
              <th className={thClass}>Dauer</th>
              <th className={thClass}>Distanz</th>
              <th className={thClass}>Geschw.</th>
              {showElevation && <th className={thClass}>Aufstieg</th>}
              {showElevation && <th className={thClass}>Abstieg</th>}
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
                    {splitTypeLabel(s.split_type)}
                  </span>
                </td>
                <td className={tdClass}>{fmtDuration(s.timer_time_s ?? s.elapsed_time_s)}</td>
                <td className={tdClass}>{fmtDist(s.distance_m)}</td>
                <td className={tdClass}>{formatSpeedKmh(s.avg_speed_ms ?? 0)}</td>
                {showElevation && <td className={tdClass}>{s.total_ascent_m != null ? `+${Math.round(s.total_ascent_m)} m` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_descent_m != null ? `-${Math.round(s.total_descent_m)} m` : '--'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function GenericSplits({ splits }: { splits: Split[] }) {
  const showHR = splits.some(s => s.avg_hr != null)
  const showElevation = splits.some(s => s.total_ascent_m != null || s.total_descent_m != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Splits</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>Typ</th>
              <th className={thClass}>Dauer</th>
              <th className={thClass}>Distanz</th>
              <th className={thClass}>Geschw.</th>
              {showHR && <th className={thClass}>HR</th>}
              {showElevation && <th className={thClass}>Aufstieg</th>}
              {showElevation && <th className={thClass}>Abstieg</th>}
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
                    {splitTypeLabel(s.split_type)}
                  </span>
                </td>
                <td className={tdClass}>{fmtDuration(s.timer_time_s ?? s.elapsed_time_s)}</td>
                <td className={tdClass}>{fmtDist(s.distance_m)}</td>
                <td className={tdClass}>{formatSpeedKmh(s.avg_speed_ms ?? 0)}</td>
                {showHR && <td className={tdClass}>{s.avg_hr != null ? `${Math.round(s.avg_hr)} bpm` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_ascent_m != null ? `+${Math.round(s.total_ascent_m)} m` : '--'}</td>}
                {showElevation && <td className={tdClass}>{s.total_descent_m != null ? `-${Math.round(s.total_descent_m)} m` : '--'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
