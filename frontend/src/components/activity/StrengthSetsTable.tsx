import type { ActivitySet } from '@/api/types'
import { fmtDuration } from '@/lib/format'

const EXERCISE_LABELS: Record<number, string> = {
  0: 'Bankdrücken', 1: 'Wadenheben', 2: 'Cardio', 3: 'Tragen',
  4: 'Bauchmuskel-Crunch',
  5: 'Core / Rumpf', 6: 'Crunches', 7: 'Curls', 8: 'Kreuzheben',
  9: 'Fliegende', 10: 'Hüftheben', 11: 'Fliegende (Maschine)', 12: 'Hüftheben',
  13: 'Rückenstrecker', 14: 'Seitheben',
  15: 'Beinbeuger', 16: 'Beinheben', 17: 'Ausfallschritt',
  18: 'Olympisches Heben', 19: 'Plank', 20: 'Plyometrie',
  21: 'Klimmzug', 22: 'Liegestütz', 23: 'Rudern (Übung)',
  24: 'Schulterdrücken', 25: 'Schulterstabilität', 26: 'Schulterheben', 27: 'Sit-Ups',
  28: 'Kniebeuge', 29: 'Ganzkörper', 30: 'Trizepsdrücken', 31: 'Aufwärmen',
  32: 'Suspension', 33: 'Kettlebell', 34: 'Langhantel', 35: 'Kurzhantel',
}

function exerciseLabel(categories: number[] | undefined): string {
  if (!categories?.length) return 'Unbekannt'
  return EXERCISE_LABELS[categories[0]] ?? `Übung #${categories[0]}`
}

function setTypeLabel(type: number | undefined): string {
  if (type === 0) return 'Arbeitssatz'
  if (type === 1) return 'Pause'
  return `Typ ${type ?? '?'}`
}

interface Props { sets: ActivitySet[] }

export function StrengthSetsTable({ sets }: Props) {
  if (sets.length === 0) return null

  const groups: { label: string; sets: (ActivitySet & { index: number })[] }[] = []
  let currentLabel = ''
  let currentGroup: (ActivitySet & { index: number })[] = []

  sets.forEach((s, i) => {
    const label = exerciseLabel(s.exercise_categories)
    if (label !== currentLabel) {
      if (currentGroup.length > 0) groups.push({ label: currentLabel, sets: currentGroup })
      currentLabel = label
      currentGroup = []
    }
    currentGroup.push({ ...s, index: i + 1 })
  })
  if (currentGroup.length > 0) groups.push({ label: currentLabel, sets: currentGroup })

  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden">
      <div className="px-4 py-3 bg-neutral-50 dark:bg-neutral-900/60 border-b border-neutral-200 dark:border-neutral-800">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Sets</h3>
      </div>
      <div className="divide-y divide-neutral-100 dark:divide-neutral-800">
        {groups.map(g => (
          <div key={g.label} className="px-4 py-3">
            <div className="text-sm font-medium text-neutral-800 dark:text-neutral-200 mb-2">{g.label}</div>
            <table className="w-full text-sm">
              <thead>
                <tr className="text-xs text-neutral-500 dark:text-neutral-400">
                  <th className="text-left w-8">#</th>
                  <th className="text-left">Typ</th>
                  <th className="text-right">Wdh.</th>
                  <th className="text-right">Gewicht</th>
                  <th className="text-right">Dauer</th>
                </tr>
              </thead>
              <tbody>
                {g.sets.map(s => (
                  <tr key={s.index} className="text-neutral-700 dark:text-neutral-300">
                    <td className="tabular-nums">{s.index}</td>
                    <td>{setTypeLabel(s.set_type)}</td>
                    <td className="text-right tabular-nums">{s.repetitions ?? '—'}×</td>
                    <td className="text-right tabular-nums">
                      {s.weight_kg != null ? `${s.weight_kg.toFixed(1)} ${s.weight_display_unit === 1 ? 'lbs' : 'kg'}` : '—'}
                    </td>
                    <td className="text-right tabular-nums">{s.duration_s != null ? fmtDuration(s.duration_s, 'short') : '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ))}
      </div>
    </div>
  )
}
