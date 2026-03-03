import type { Lap } from '@/api/types'

interface Props {
  laps: Lap[]
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

function fmtPace(speedMs?: number): string {
  if (speedMs == null || speedMs <= 0) return '--'
  const secPerKm = 1000 / speedMs
  const m = Math.floor(secPerKm / 60)
  const s = Math.floor(secPerKm % 60)
  return `${m}:${s.toString().padStart(2, '0')} /km`
}

function fmtSpeed(speedMs?: number): string {
  if (speedMs == null || speedMs <= 0) return '--'
  return `${(speedMs * 3.6).toFixed(1)} km/h`
}

function isRunLike(sport: string): boolean {
  return ['running', 'hiking', 'walking'].includes(sport)
}

function isSwimLike(sport: string): boolean {
  return sport === 'swimming'
}

function isCycleLike(sport: string): boolean {
  return ['cycling', 'e_biking'].includes(sport)
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

export default function LapTable({ laps, sportType }: Props) {
  const showPace = isRunLike(sportType)
  const showPower = isCycleLike(sportType) && laps.some(l => l.avg_power_w != null)
  const showNP = showPower && laps.some(l => l.normalized_power_w != null)
  const showCadence = (isCycleLike(sportType) || isRunLike(sportType)) && laps.some(l => l.avg_cadence != null)
  const showRunDynamics = isRunLike(sportType) && laps.some(l => l.avg_vertical_oscillation_mm != null || l.avg_stance_time_ms != null || l.avg_step_length_mm != null)
  const showSwim = isSwimLike(sportType)
  const showHR = laps.some(l => l.avg_heart_rate != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Runden</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>Dauer</th>
              <th className={thClass}>Distanz</th>
              <th className={thClass}>{showPace ? 'Tempo' : 'Geschw.'}</th>
              {showHR && <th className={thClass}>HR</th>}
              {showPower && <th className={thClass}>Power</th>}
              {showNP && <th className={thClass}>NP</th>}
              {showCadence && <th className={thClass}>Kadenz</th>}
              {showRunDynamics && <th className={thClass}>Vert. Osz.</th>}
              {showRunDynamics && <th className={thClass}>Bodenkontakt</th>}
              {showRunDynamics && <th className={thClass}>Schrittl.</th>}
              {showSwim && <th className={thClass}>Bahnen</th>}
              {showSwim && <th className={thClass}>Stil</th>}
              {showSwim && <th className={thClass}>Schlagdist.</th>}
            </tr>
          </thead>
          <tbody>
            {laps.map((lap, i) => (
              <tr
                key={lap.id}
                className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
              >
                <td className={tdClass}>{lap.lap_index != null ? lap.lap_index + 1 : '--'}</td>
                <td className={tdClass}>{fmtDuration(lap.timer_time_s ?? lap.elapsed_time_s)}</td>
                <td className={tdClass}>{fmtDist(lap.distance_m)}</td>
                <td className={tdClass}>{showPace ? fmtPace(lap.avg_speed_ms) : fmtSpeed(lap.avg_speed_ms)}</td>
                {showHR && <td className={tdClass}>{lap.avg_heart_rate != null ? `${Math.round(lap.avg_heart_rate)} bpm` : '--'}</td>}
                {showPower && <td className={tdClass}>{lap.avg_power_w != null ? `${Math.round(lap.avg_power_w)} W` : '--'}</td>}
                {showNP && <td className={tdClass}>{lap.normalized_power_w != null ? `${Math.round(lap.normalized_power_w)} W` : '--'}</td>}
                {showCadence && <td className={tdClass}>{lap.avg_cadence != null ? `${Math.round(lap.avg_cadence)}` : '--'}</td>}
                {showRunDynamics && <td className={tdClass}>{lap.avg_vertical_oscillation_mm != null ? `${(lap.avg_vertical_oscillation_mm / 10).toFixed(1)} cm` : '--'}</td>}
                {showRunDynamics && <td className={tdClass}>{lap.avg_stance_time_ms != null ? `${Math.round(lap.avg_stance_time_ms)} ms` : '--'}</td>}
                {showRunDynamics && <td className={tdClass}>{lap.avg_step_length_mm != null ? `${(lap.avg_step_length_mm / 10).toFixed(0)} cm` : '--'}</td>}
                {showSwim && <td className={tdClass}>{lap.num_lengths ?? '--'}</td>}
                {showSwim && <td className={tdClass}>{swimStrokeLabel(lap.swim_stroke)}</td>}
                {showSwim && <td className={tdClass}>{lap.avg_stroke_distance_m != null ? `${lap.avg_stroke_distance_m.toFixed(2)} m` : '--'}</td>}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function swimStrokeLabel(stroke?: string): string {
  if (!stroke) return '--'
  const labels: Record<string, string> = {
    freestyle: 'Kraul',
    backstroke: 'Ruecken',
    breaststroke: 'Brust',
    butterfly: 'Schmetterling',
    drill: 'Drill',
    mixed: 'Gemischt',
    im: 'Lagen',
  }
  return labels[stroke] ?? stroke
}
