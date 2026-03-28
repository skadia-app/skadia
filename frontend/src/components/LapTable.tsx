import type { Lap } from '@/api/types'
import { fmtDuration, fmtDist } from '@/lib/format'
import { formatSpeedValue, fieldLabel, fieldUnit } from '@/lib/sportFieldLabels'
import { getSegmentGroup } from '@/lib/sportTaxonomy'

interface Props {
  laps: Lap[]
  sportType: string
  speedMode?: 'speed' | 'pace'
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

export default function LapTable({ laps, sportType, speedMode }: Props) {
  const segGroup = getSegmentGroup(sportType)
  const effectiveMode = speedMode ?? (segGroup === 'foot' || segGroup === 'swim' ? 'pace' : 'speed')
  const showPace = effectiveMode === 'pace'
  const showPower = segGroup === 'bike' && laps.some(l => l.avg_power_w != null)
  const showNP = showPower && laps.some(l => l.normalized_power_w != null)
  const showCadence = (segGroup === 'bike' || segGroup === 'foot') && laps.some(l => l.avg_cadence != null)
  const showRunDynamics = segGroup === 'foot' && laps.some(l => l.avg_vertical_oscillation_mm != null || l.avg_stance_time_ms != null || l.avg_step_length_mm != null)
  const showSwim = segGroup === 'swim'
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
              {showCadence && <th className={thClass}>{fieldLabel('cadence', sportType)}</th>}
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
                <td className={tdClass}>{formatSpeedValue(lap.avg_speed_ms, sportType, speedMode)}</td>
                {showHR && <td className={tdClass}>{lap.avg_heart_rate != null ? `${Math.round(lap.avg_heart_rate)} bpm` : '--'}</td>}
                {showPower && <td className={tdClass}>{lap.avg_power_w != null ? `${Math.round(lap.avg_power_w)} W` : '--'}</td>}
                {showNP && <td className={tdClass}>{lap.normalized_power_w != null ? `${Math.round(lap.normalized_power_w)} W` : '--'}</td>}
                {showCadence && <td className={tdClass}>{lap.avg_cadence != null ? `${Math.round(lap.avg_cadence)} ${fieldUnit('cadence', sportType)}` : '--'}</td>}
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
