import type { SwimLength } from '@/api/types'
import { fmtDuration } from '@/lib/format'
import { fieldLabel, swimStrokeLabel } from '@/lib/sportFieldLabels'

interface Props {
  lengths: SwimLength[]
}

// swimStrokeLabel imported from sportFieldLabels

function strokeColor(stroke?: string): string {
  switch (stroke) {
    case 'freestyle': return 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-400'
    case 'backstroke': return 'bg-green-100 dark:bg-green-900/30 text-green-700 dark:text-green-400'
    case 'breaststroke': return 'bg-purple-100 dark:bg-purple-900/30 text-purple-700 dark:text-purple-400'
    case 'butterfly': return 'bg-accent-100 dark:bg-orange-900/30 text-orange-700 dark:text-orange-400'
    default: return 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400'
  }
}

function lengthTypeLabel(type: string): string {
  return type === 'active' ? 'Aktiv' : type === 'idle' ? 'Pause' : type
}

const thClass = 'px-2 py-1.5 text-left text-xs font-medium text-neutral-500 dark:text-neutral-400 whitespace-nowrap'
const tdClass = 'px-2 py-1.5 text-sm text-neutral-900 dark:text-neutral-100 whitespace-nowrap'

export default function SwimLengthTable({ lengths }: Props) {
  const showCadence = lengths.some(l => l.avg_swimming_cadence != null)

  return (
    <div>
      <h2 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100 mb-3">Bahnen</h2>
      <div className="overflow-x-auto rounded-lg border border-neutral-200 dark:border-neutral-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-neutral-200 dark:border-neutral-700 bg-neutral-50 dark:bg-neutral-800/50">
              <th className={thClass}>#</th>
              <th className={thClass}>Typ</th>
              <th className={thClass}>Stil</th>
              <th className={thClass}>Dauer</th>
              <th className={thClass}>Züge</th>
              <th className={thClass}>SWOLF</th>
              {showCadence && <th className={thClass}>{fieldLabel('cadence', 'swimming')}</th>}
              <th className={thClass}>kcal</th>
            </tr>
          </thead>
          <tbody>
            {lengths.map((len, i) => {
              const swolf = (len.elapsed_time_s != null && len.total_strokes != null)
                ? Math.round(len.elapsed_time_s + len.total_strokes)
                : null

              return (
                <tr
                  key={len.id}
                  className={`border-b border-neutral-100 dark:border-neutral-800 ${i % 2 !== 0 ? 'bg-neutral-50/50 dark:bg-neutral-800/20' : ''}`}
                >
                  <td className={tdClass}>{len.length_index + 1}</td>
                  <td className={tdClass}>
                    <span className={`text-xs px-1.5 py-0.5 rounded ${len.length_type === 'idle' ? 'bg-neutral-100 dark:bg-neutral-800 text-neutral-500 dark:text-neutral-400' : 'bg-blue-50 dark:bg-blue-900/20 text-blue-600 dark:text-blue-400'}`}>
                      {lengthTypeLabel(len.length_type)}
                    </span>
                  </td>
                  <td className={tdClass}>
                    {len.swim_stroke ? (
                      <span className={`text-xs px-1.5 py-0.5 rounded ${strokeColor(len.swim_stroke)}`}>
                        {swimStrokeLabel(len.swim_stroke)}
                      </span>
                    ) : '--'}
                  </td>
                  <td className={tdClass}>{fmtDuration(len.elapsed_time_s ?? len.timer_time_s)}</td>
                  <td className={tdClass}>{len.total_strokes ?? '--'}</td>
                  <td className={tdClass}>{swolf ?? '--'}</td>
                  {showCadence && <td className={tdClass}>{len.avg_swimming_cadence != null ? Math.round(len.avg_swimming_cadence) : '--'}</td>}
                  <td className={tdClass}>{len.total_calories != null ? Math.round(len.total_calories) : '--'}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
