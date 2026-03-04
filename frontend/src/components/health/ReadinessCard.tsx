import type { HealthReadinessScore } from '@/api/health'

interface Props {
  data: HealthReadinessScore
}

function scoreColor(score: number): string {
  if (score < 40) return '#ef4444'
  if (score < 70) return '#f97316'
  return '#22c55e'
}

function levelLabel(level: string | null): string {
  if (!level) return ''
  const labels: Record<string, string> = {
    LOW: 'Niedrig',
    MODERATE: 'Moderat',
    HIGH: 'Hoch',
  }
  return labels[level] ?? level
}

interface Factor {
  label: string
  value: number | null
  unit?: string
}

export default function ReadinessCard({ data }: Props) {
  const score = data.score ?? 0
  const color = scoreColor(score)

  const factors: Factor[] = [
    { label: 'HRV (wöchentl. Ø)', value: data.hrv_weekly_avg_ms != null ? Math.round(data.hrv_weekly_avg_ms) : null, unit: 'ms' },
    { label: 'Schlaf-Score', value: data.sleep_score },
    { label: 'HRV-Faktor', value: data.hrv_factor_pct != null ? Math.round(data.hrv_factor_pct) : null, unit: '%' },
    { label: 'Schlafhistorie', value: data.sleep_history_factor_pct != null ? Math.round(data.sleep_history_factor_pct) : null, unit: '%' },
    { label: 'Stresshistorie', value: data.stress_history_factor_pct != null ? Math.round(data.stress_history_factor_pct) : null, unit: '%' },
    { label: 'Erholungszeit', value: data.recovery_time_factor_pct != null ? Math.round(data.recovery_time_factor_pct) : null, unit: '%' },
  ].filter(f => f.value != null)

  // Gradient background based on score
  const gradientStyle = {
    background: `linear-gradient(135deg, ${color}15, ${color}08)`,
    borderColor: `${color}40`,
  }

  return (
    <div className="rounded-xl border p-5" style={gradientStyle}>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Bereitschaft heute
        </h3>
        {data.level && (
          <span className="text-xs px-2 py-0.5 rounded-full font-medium" style={{ backgroundColor: `${color}20`, color }}>
            {levelLabel(data.level)}
          </span>
        )}
      </div>

      <div className="flex items-center gap-6">
        {/* Score circle */}
        <div className="flex-shrink-0 flex flex-col items-center">
          <div
            className="text-5xl font-bold tabular-nums leading-none"
            style={{ color }}
          >
            {data.score ?? '—'}
          </div>
          <div className="text-xs text-neutral-400 mt-1">von 100</div>
        </div>

        {/* Score bar */}
        <div className="flex-1">
          <div className="h-3 w-full rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden mb-1">
            <div
              className="h-full rounded-full transition-all duration-500"
              style={{ width: `${score}%`, backgroundColor: color }}
            />
          </div>
          <div className="flex justify-between text-xs text-neutral-400">
            <span>0</span>
            <span>50</span>
            <span>100</span>
          </div>
        </div>

        {/* Factors */}
        {factors.length > 0 && (
          <div className="flex-shrink-0 space-y-1 text-sm">
            {factors.map(f => (
              <div key={f.label} className="flex items-center gap-2">
                <span className="text-neutral-500 dark:text-neutral-400 text-xs">{f.label}:</span>
                <span className="font-medium text-neutral-900 dark:text-neutral-100 text-xs tabular-nums">
                  {f.value}{f.unit ?? ''}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {data.acwr_pct != null && (
        <div className="mt-3 text-xs text-neutral-400">
          ACWR: {data.acwr_pct.toFixed(1)}%
          {data.recovery_time_min != null && ` · Erholungszeit: ${Math.round(data.recovery_time_min / 60)}h`}
        </div>
      )}
    </div>
  )
}
