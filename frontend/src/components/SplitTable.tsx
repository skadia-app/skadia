import type { Split, SplitSummary } from '@/api/types'

interface Props {
  splits: Split[]
  summaries: SplitSummary[]
  sportType: string
}

function fmtDuration(s?: number): string {
  if (s == null || isNaN(s)) return '--'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
  return `${m}:${sec.toString().padStart(2, '0')}`
}

function fmtDist(m?: number): string {
  if (m == null || isNaN(m)) return '--'
  if (m >= 1000) return `${(m / 1000).toFixed(2)} km`
  return `${Math.round(m)} m`
}

function fmtSpeed(speedMs?: number): string {
  if (speedMs == null || speedMs <= 0) return '--'
  return `${(speedMs * 3.6).toFixed(1)} km/h`
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

function splitTypeLabel(type: string): string {
  const labels: Record<string, string> = {
    climb_active: 'Klettern',
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
    case 'climb_active': return 'bg-orange-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400'
    case 'climb_rest': return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400'
    case 'ski_run': return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
    case 'ski_lift': return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400'
    case 'ascent': return 'bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-400'
    case 'descent': return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
    default: return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
  }
}

export default function SplitTable({ splits, summaries, sportType }: Props) {
  if (sportType === 'rock_climbing') return <ClimbingSplits splits={splits} summaries={summaries} />
  if (sportType === 'skiing') return <SkiSplits splits={splits} />
  return <GenericSplits splits={splits} />
}

function ClimbingSplits({ splits, summaries }: { splits: Split[]; summaries: SplitSummary[] }) {
  const activeSplits = splits.filter(s => s.split_type === 'climb_active')
  const totalActiveTime = activeSplits.reduce((sum, s) => sum + (s.timer_time_s ?? s.elapsed_time_s ?? 0), 0)
  const totalCalories = splits.reduce((sum, s) => sum + (s.total_calories ?? 0), 0)

  const summaryForClimb = summaries.find(s => s.split_type === 'climb_active')

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Routen</h2>

      {/* Summary card */}
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3 mb-3 flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span className="text-neutral-700 dark:text-neutral-300">
          <span className="font-medium">{summaryForClimb?.num_splits ?? activeSplits.length}</span> Routen geklettert
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
              <th className={thClass}>Dauer</th>
              <th className={thClass}>HR</th>
              <th className={thClass}>kcal</th>
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
                <td className={tdClass}>{s.avg_hr != null ? `${Math.round(s.avg_hr)} bpm` : '--'}</td>
                <td className={tdClass}>{s.total_calories != null ? Math.round(s.total_calories) : '--'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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
                <td className={tdClass}>{fmtSpeed(s.avg_speed_ms)}</td>
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
                <td className={tdClass}>{fmtSpeed(s.avg_speed_ms)}</td>
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
