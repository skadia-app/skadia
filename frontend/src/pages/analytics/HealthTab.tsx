import { useQuery } from '@tanstack/react-query'
import { fetchHRVRecovery } from '@/api/analytics'
import HRVAnalysisChart from '@/components/analytics/health/HRVAnalysis'
import SleepAnalysisChart from '@/components/analytics/health/SleepAnalysis'
import WeightChart from '@/components/analytics/health/WeightChart'
import HealthMetricsGrid from '@/components/analytics/health/HealthMetricsGrid'

// ── Recovery Score colours ────────────────────────────────────────────────────

function scoreColor(score: number): string {
  if (score >= 70) return '#22c55e'
  if (score >= 40) return '#f59e0b'
  return '#ef4444'
}

function levelLabel(level: string): string {
  const labels: Record<string, string> = {
    high: 'Hoch',
    good: 'Gut',
    moderate: 'Moderat',
    low: 'Niedrig',
    poor: 'Schlecht',
  }
  return labels[level.toLowerCase()] ?? level
}

// ── Inline RecoveryScoreCard ──────────────────────────────────────────────────

function RecoveryScoreCard() {
  const { data, isLoading } = useQuery({
    queryKey: ['analytics', 'hrv-recovery'],
    queryFn: fetchHRVRecovery,
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="flex items-center gap-6">
          <div className="w-20 h-20 rounded-full bg-neutral-200 dark:bg-neutral-700" />
          <div className="flex-1 space-y-2">
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-full" />
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-3/4" />
          </div>
        </div>
      </div>
    )
  }

  if (!data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 text-sm text-neutral-400 text-center">
        Keine Erholungsdaten verfügbar
      </div>
    )
  }

  const score = data.score ?? 0
  const color = scoreColor(score)

  // SVG circle progress ring
  const radius = 36
  const circumference = 2 * Math.PI * radius
  const offset = circumference - (score / 100) * circumference

  // Gradient background
  const gradientStyle: React.CSSProperties = {
    background: `linear-gradient(135deg, ${color}12, ${color}06)`,
    borderColor: `${color}35`,
  }

  const factors = [
    { label: 'HRV', value: data.hrv_factor, unit: '%' },
    { label: 'Schlaf', value: data.sleep_factor, unit: '%' },
    { label: 'Ermüdung', value: data.fatigue_factor, unit: '%' },
  ].filter(f => f.value != null)

  return (
    <div className="rounded-xl border p-5" style={gradientStyle}>
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Erholungs-Score
        </h3>
        {data.level && (
          <span
            className="text-xs px-2.5 py-0.5 rounded-full font-medium"
            style={{ backgroundColor: `${color}20`, color }}
          >
            {levelLabel(data.level)}
          </span>
        )}
      </div>

      <div className="flex items-center gap-6 flex-wrap">
        {/* SVG ring gauge */}
        <div className="flex-shrink-0 flex flex-col items-center">
          <svg width="96" height="96" className="-rotate-90">
            <circle
              cx="48" cy="48" r={radius}
              fill="none"
              stroke="currentColor"
              className="text-neutral-200 dark:text-neutral-700"
              strokeWidth="8"
            />
            <circle
              cx="48" cy="48" r={radius}
              fill="none"
              stroke={color}
              strokeWidth="8"
              strokeDasharray={circumference}
              strokeDashoffset={offset}
              strokeLinecap="round"
              style={{ transition: 'stroke-dashoffset 0.6s ease' }}
            />
          </svg>
          <div className="mt-[-64px] flex flex-col items-center">
            <span
              className="text-2xl font-bold tabular-nums leading-none"
              style={{ color }}
            >
              {score}
            </span>
            <span className="text-xs text-neutral-400 mt-0.5">/ 100</span>
          </div>
          <div className="mt-8" />
        </div>

        {/* Progress bar + factors */}
        <div className="flex-1 min-w-0 space-y-3">
          <div>
            <div className="h-2.5 w-full rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-hidden">
              <div
                className="h-full rounded-full transition-all duration-500"
                style={{ width: `${score}%`, backgroundColor: color }}
              />
            </div>
            <div className="flex justify-between text-xs text-neutral-400 mt-0.5">
              <span>Schlecht</span>
              <span>Optimal</span>
            </div>
          </div>

          {factors.length > 0 && (
            <div className="grid grid-cols-3 gap-2">
              {factors.map(f => (
                <div key={f.label} className="text-center">
                  <div className="text-xs text-neutral-500 dark:text-neutral-400">{f.label}</div>
                  <div className="text-sm font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
                    {f.value != null ? `${(f.value * 100).toFixed(0)}${f.unit}` : '—'}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {data.recommendation && (
        <p className="mt-4 text-sm text-neutral-600 dark:text-neutral-300 leading-relaxed border-t border-neutral-200/50 dark:border-neutral-700/50 pt-3">
          {data.recommendation}
        </p>
      )}
    </div>
  )
}

// ── HealthTab ─────────────────────────────────────────────────────────────────

export default function HealthTab() {
  return (
    <div className="space-y-6">
      {/* HRV Analysis — full width */}
      <HRVAnalysisChart />

      {/* Recovery Score — full width */}
      <RecoveryScoreCard />

      {/* 2-column grid: Sleep | Weight */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <SleepAnalysisChart />
        <WeightChart />
      </div>

      {/* Health Metrics Grid — full width */}
      <HealthMetricsGrid />
    </div>
  )
}
