import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'

// ── Types ─────────────────────────────────────────────────────────────────────

interface ReadinessV2Factor {
  name: string
  value: number
  status: 'good' | 'ok' | 'warning' | 'bad' | string
}

interface ReadinessV2Response {
  score: number
  confidence: 'high' | 'medium' | 'low' | string
  factors: ReadinessV2Factor[]
  recommendation: string
  computed_at?: string
}

// ── Helpers ───────────────────────────────────────────────────────────────────

function scoreColor(score: number): string {
  if (score >= 70) return '#22c55e'
  if (score >= 40) return '#f59e0b'
  return '#ef4444'
}

function factorStatusColor(status: string): string {
  switch (status.toLowerCase()) {
    case 'good':
    case 'gut':
    case 'hoch':
    case 'high':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'ok':
    case 'moderate':
    case 'moderat':
    case 'mittel':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default:
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
  }
}

function confidenceLabel(confidence: string): string {
  const labels: Record<string, string> = {
    high: 'Hoch',
    medium: 'Mittel',
    low: 'Niedrig',
  }
  return labels[confidence.toLowerCase()] ?? confidence
}

function factorLabel(name: string): string {
  const labels: Record<string, string> = {
    hrv_cv: 'HRV-Variabilität',
    hrv_rmssd: 'HRV (RMSSD)',
    acwr: 'ACWR',
    sleep: 'Schlaf',
    changepoints: 'Veränderungspunkte',
    fatigue: 'Ermüdung',
    ctl: 'Fitness (CTL)',
    tsb: 'Form (TSB)',
  }
  return labels[name.toLowerCase()] ?? name
}

// ── SVG Score Ring ─────────────────────────────────────────────────────────────

function ScoreRing({ score }: { score: number }) {
  const r = 20, cx = 24, cy = 24
  const circumference = 2 * Math.PI * r
  const offset = circumference * (1 - score / 100)
  const color = scoreColor(score)
  return (
    <svg width="48" height="48" viewBox="0 0 48 48">
      <circle cx={cx} cy={cy} r={r} fill="none" stroke="#e5e7eb" strokeWidth="4" />
      <circle
        cx={cx} cy={cy} r={r} fill="none"
        stroke={color} strokeWidth="4"
        strokeDasharray={circumference} strokeDashoffset={offset}
        strokeLinecap="round" transform={`rotate(-90 ${cx} ${cy})`}
        style={{ transition: 'stroke-dashoffset 0.6s ease' }}
      />
      <text
        x={cx} y={cy} textAnchor="middle" dominantBaseline="middle"
        fontSize="10" fontWeight="bold" fill={color}
      >
        {score}
      </text>
    </svg>
  )
}

// ── Factor row ────────────────────────────────────────────────────────────────

function FactorRow({ factor }: { factor: ReadinessV2Factor }) {
  return (
    <div className="flex items-center justify-between gap-2 text-xs">
      <span className="text-neutral-600 dark:text-neutral-300 truncate">
        {factorLabel(factor.name)}
      </span>
      <div className="flex items-center gap-1.5 flex-shrink-0">
        <span className="tabular-nums text-neutral-700 dark:text-neutral-200 font-medium">
          {typeof factor.value === 'number' ? factor.value.toFixed(1) : factor.value}
        </span>
        <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${factorStatusColor(factor.status)}`}>
          {factor.status}
        </span>
      </div>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

export default function ReadinessV2Card() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'readiness-v2'],
    queryFn: async () => {
      try {
        return await apiClient<ReadinessV2Response>('/analytics/readiness-v2')
      } catch {
        return null
      }
    },
    staleTime: 5 * 60 * 1000,
    retry: 1,
  })

  if (isLoading) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 animate-pulse">
        <div className="h-4 w-48 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
        <div className="flex items-center gap-4">
          <div className="w-12 h-12 rounded-full bg-neutral-200 dark:bg-neutral-700 flex-shrink-0" />
          <div className="flex-1 space-y-2">
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-full" />
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-3/4" />
          </div>
        </div>
      </div>
    )
  }

  if (isError || !data) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">
          Trainingsbereitschaft v2
        </h3>
        <p className="text-sm text-neutral-400 text-center py-4">
          Dein Readiness-Score v2 wird nach der nächsten Analyse berechnet.
        </p>
      </div>
    )
  }

  const score = data.score ?? 0
  const color = scoreColor(score)

  return (
    <div
      className="rounded-xl border p-5 space-y-4"
      style={{
        background: `linear-gradient(135deg, ${color}12, ${color}06)`,
        borderColor: `${color}35`,
      }}
    >
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Trainingsbereitschaft v2
        </h3>
        {data.confidence && (
          <span className="text-xs text-neutral-500 dark:text-neutral-400">
            Konfidenz: <span className="font-medium text-neutral-700 dark:text-neutral-200">{confidenceLabel(data.confidence)}</span>
          </span>
        )}
      </div>

      {/* Score ring + factors */}
      <div className="flex items-start gap-4">
        <div className="flex-shrink-0 flex flex-col items-center gap-1">
          <ScoreRing score={score} />
          <span className="text-xs text-neutral-400">/ 100</span>
        </div>

        {/* Factors */}
        {data.factors && data.factors.length > 0 && (
          <div className="flex-1 min-w-0 space-y-2">
            {data.factors.map((f, i) => (
              <FactorRow key={i} factor={f} />
            ))}
          </div>
        )}
      </div>

      {/* Recommendation */}
      {data.recommendation && (
        <p className="text-sm text-neutral-600 dark:text-neutral-300 leading-relaxed border-t border-neutral-200/50 dark:border-neutral-700/50 pt-3">
          {data.recommendation}
        </p>
      )}
    </div>
  )
}
