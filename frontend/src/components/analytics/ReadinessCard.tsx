// ReadinessCard.tsx — Readiness score card (compact for Dashboard, full for Insights).
// UI strings in German; comments in English.
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchReadiness,
  fetchReadinessPreferences,
  putReadinessPreferences,
  type ReadinessFactor,
  type ReadinessLevel,
  type ReadinessConfidence,
  type ReadinessSource,
} from '@/api/analytics'
import { recommendationText } from './readinessRecommendations'

// ── Const maps (i18n-prepared, DE strings) ────────────────────────────────────

const LEVEL_LABELS: Record<ReadinessLevel, string> = {
  critical: 'kritisch',
  low: 'niedrig',
  moderate: 'moderat',
  good: 'gut',
  optimal: 'optimal',
}

const LEVEL_COLORS: Record<ReadinessLevel, string> = {
  critical: '#ef4444',
  low: '#f97316',
  moderate: '#eab308',
  good: '#22c55e',
  optimal: '#10b981',
}

const CONFIDENCE_LABELS: Record<ReadinessConfidence, string> = {
  high: 'Hoch',
  medium: 'Mittel',
  low: 'Niedrig',
}

const FACTOR_LABELS: Record<string, string> = {
  hrv_status: 'HRV-Status',
  sleep: 'Schlaf',
  training_load: 'Trainingslast',
  dfa_alpha1: 'DFA-α1',
  sample_entropy: 'Sample Entropy',
  rhr_changepoint: 'Ruhepuls-Veränderung',
  stress_history: 'Stress (Historie)',
  recovery_time: 'Erholungszeit',
}

const SOURCE_LABELS: Record<ReadinessSource, string> = {
  garmin: 'Garmin',
  garmin_fit: 'Garmin (FIT-Datei)',
  manual_override: 'Manuell',
  skadia: 'Skadia',
  strava: 'Strava',
  komoot: 'Komoot',
  manual: 'Manuell (Import)',
  mobile: 'Mobile App',
}

const REFUSAL_TEXTS: Record<string, string> = {
  insufficient_factors: 'Noch nicht berechenbar — zu wenige aktuelle Faktoren.',
  no_data: 'Noch keine Daten verfügbar.',
  service_unavailable: 'Berechnung temporär nicht verfügbar.',
}

// ── Helpers ───────────────────────────────────────────────────────────────────

/** Derive a display status from the factor's 0-100 value. */
function factorStatus(factor: ReadinessFactor): 'gut' | 'moderat' | 'niedrig' | 'fehlend' {
  if (factor.missing || factor.value === null) return 'fehlend'
  if (factor.value >= 70) return 'gut'
  if (factor.value >= 40) return 'moderat'
  return 'niedrig'
}

function statusBadgeClasses(status: ReturnType<typeof factorStatus>): string {
  switch (status) {
    case 'gut':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'moderat':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    case 'niedrig':
      return 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300'
    case 'fehlend':
      return 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
  }
}

function sourceBadgeClasses(source: ReadinessSource): string {
  switch (source) {
    case 'skadia':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    default:
      return 'bg-neutral-100 text-neutral-600 dark:bg-neutral-800 dark:text-neutral-300'
  }
}

// ── ScoreRing ─────────────────────────────────────────────────────────────────

interface ScoreRingProps {
  score: number | null
  level: ReadinessLevel | null
  size?: number
}

function ScoreRing({ score, level, size = 64 }: ScoreRingProps) {
  const r = size / 2 - 6
  const cx = size / 2
  const cy = size / 2
  const circumference = 2 * Math.PI * r
  const pct = score !== null ? Math.max(0, Math.min(100, score)) : 0
  const offset = circumference * (1 - pct / 100)
  const color = level ? LEVEL_COLORS[level] : '#6b7280'
  const fontSize = size < 56 ? 10 : 14

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      aria-label={score !== null ? `Bereitschaftswert ${score} von 100` : 'Kein Bereitschaftswert'}
      role="img"
    >
      <circle
        cx={cx} cy={cy} r={r}
        fill="none" stroke="#e5e7eb" strokeWidth="5"
      />
      <circle
        cx={cx} cy={cy} r={r}
        fill="none" stroke={color} strokeWidth="5"
        strokeDasharray={circumference} strokeDashoffset={offset}
        strokeLinecap="round"
        transform={`rotate(-90 ${cx} ${cy})`}
        style={{ transition: 'stroke-dashoffset 0.6s ease' }}
      />
      {score !== null ? (
        <text
          x={cx} y={cy} textAnchor="middle" dominantBaseline="middle"
          fontSize={fontSize} fontWeight="bold" fill={color}
        >
          {score}
        </text>
      ) : (
        <text
          x={cx} y={cy} textAnchor="middle" dominantBaseline="middle"
          fontSize={fontSize - 2} fill="#9ca3af"
        >
          {'—'}
        </text>
      )}
    </svg>
  )
}

// ── FactorRow ─────────────────────────────────────────────────────────────────

function FactorRow({ factor }: { factor: ReadinessFactor }) {
  const status = factorStatus(factor)
  const label = FACTOR_LABELS[factor.key] ?? factor.key
  const sourceLabel = SOURCE_LABELS[factor.source] ?? factor.source

  return (
    <div
      className="flex items-center justify-between gap-2 text-xs"
      data-testid={`factor-row-${factor.key}`}
    >
      <span className="text-neutral-600 dark:text-neutral-300 truncate min-w-0">
        {label}
      </span>
      <div className="flex items-center gap-1 flex-shrink-0">
        {factor.value !== null && (
          <span className="tabular-nums text-neutral-700 dark:text-neutral-200 font-medium">
            {factor.value.toFixed(0)}
          </span>
        )}
        <span
          className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium ${statusBadgeClasses(status)}`}
          data-testid={`status-badge-${factor.key}`}
        >
          {status}
        </span>
        <span
          className={`inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-medium ${sourceBadgeClasses(factor.source)}`}
          data-testid={`source-badge-${factor.key}`}
        >
          {sourceLabel}
        </span>
      </div>
    </div>
  )
}

// ── ConfidenceBadge ───────────────────────────────────────────────────────────

function ConfidenceBadge({ confidence }: { confidence: ReadinessConfidence }) {
  return (
    <span className="text-xs text-neutral-500 dark:text-neutral-400">
      {'Konfidenz: '}
      <span className="font-medium text-neutral-700 dark:text-neutral-200">
        {CONFIDENCE_LABELS[confidence]}
      </span>
    </span>
  )
}

// ── SourceToggle ──────────────────────────────────────────────────────────────

interface SourceToggleProps {
  preferred: 'manufacturer' | 'skadia' | null
  onChange: (value: 'manufacturer' | 'skadia' | null) => void
  disabled?: boolean
}

function SourceToggle({ preferred, onChange, disabled }: SourceToggleProps) {
  function handleChange(e: React.ChangeEvent<HTMLSelectElement>) {
    const val = e.target.value
    if (val === 'null') onChange(null)
    else if (val === 'manufacturer') onChange('manufacturer')
    else if (val === 'skadia') onChange('skadia')
  }

  return (
    <div className="flex items-center gap-2">
      <label
        htmlFor="readiness-source-toggle"
        className="text-xs text-neutral-500 dark:text-neutral-400 whitespace-nowrap"
      >
        Datenquelle:
      </label>
      <select
        id="readiness-source-toggle"
        value={preferred ?? 'null'}
        onChange={handleChange}
        disabled={disabled}
        className="text-xs rounded border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-800 text-neutral-700 dark:text-neutral-200 px-2 py-0.5 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
        aria-label="Datenquelle für Bereitschaftsberechnung"
        data-testid="source-toggle"
      >
        <option value="null">Automatisch</option>
        <option value="manufacturer">Hersteller (Garmin)</option>
        <option value="skadia">Skadia-Berechnung</option>
      </select>
    </div>
  )
}

// ── MissingFactorsBanner ──────────────────────────────────────────────────────

interface MissingFactorsBannerProps {
  refused: boolean
  refusalReason: string | null
  missingFactors: Array<{ key: string; reason: string; last_seen_at: string | null }>
}

function MissingFactorsBanner({ refused, refusalReason, missingFactors }: MissingFactorsBannerProps) {
  if (refused && refusalReason) {
    const text = REFUSAL_TEXTS[refusalReason] ?? 'Berechnung nicht möglich.'
    return (
      <div
        className="rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-3 py-2 text-xs text-amber-800 dark:text-amber-200"
        role="alert"
        data-testid="refusal-banner"
      >
        {text}
      </div>
    )
  }

  if (missingFactors.length > 0) {
    return (
      <div
        className="rounded-lg bg-neutral-50 dark:bg-neutral-800/50 border border-neutral-200 dark:border-neutral-700 px-3 py-2 text-xs text-neutral-600 dark:text-neutral-400"
        data-testid="missing-factors-banner"
      >
        {'Fehlende Faktoren: '}
        <span className="font-medium">
          {missingFactors.map(f => FACTOR_LABELS[f.key] ?? f.key).join(', ')}
        </span>
      </div>
    )
  }

  return null
}

// ── Loading skeleton ──────────────────────────────────────────────────────────

function CardSkeleton({ compact }: { compact?: boolean }) {
  return (
    <div
      className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 animate-pulse"
      aria-busy="true"
      aria-label="Lade Bereitschaftswert"
    >
      <div className="h-3 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
      <div className={`flex items-center gap-4 ${compact ? '' : 'mb-3'}`}>
        <div className="w-16 h-16 rounded-full bg-neutral-200 dark:bg-neutral-700 flex-shrink-0" />
        {!compact && (
          <div className="flex-1 space-y-2">
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-full" />
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-3/4" />
            <div className="h-3 bg-neutral-200 dark:bg-neutral-700 rounded w-2/3" />
          </div>
        )}
      </div>
    </div>
  )
}

// ── ReadinessCardCompact — Dashboard variant ──────────────────────────────────

export function ReadinessCardCompact() {
  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'readiness'],
    queryFn: fetchReadiness,
    staleTime: 5 * 60_000,
  })

  if (isLoading) return <CardSkeleton compact />

  if (isError) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
        <p className="text-xs text-neutral-400 text-center py-2">
          Bereitschaftswert nicht verfügbar.
        </p>
      </div>
    )
  }

  const level = data?.level ?? null
  const color = level ? LEVEL_COLORS[level] : '#6b7280'

  return (
    <div
      className="rounded-xl border p-4"
      style={{
        background: `linear-gradient(135deg, ${color}12, ${color}06)`,
        borderColor: `${color}30`,
      }}
      data-testid="readiness-compact"
    >
      <div className="flex items-center gap-3">
        <ScoreRing score={data?.score ?? null} level={level} size={52} />
        <div className="min-w-0">
          <p className="text-[10px] uppercase tracking-wide font-medium text-neutral-500 dark:text-neutral-400">
            Bereitschaft
          </p>
          {data?.refused ? (
            <MissingFactorsBanner
              refused={data.refused}
              refusalReason={data.refusal_reason}
              missingFactors={[]}
            />
          ) : level ? (
            <p className="text-sm font-semibold" style={{ color }}>
              {LEVEL_LABELS[level]}
            </p>
          ) : (
            <p className="text-sm text-neutral-400">Keine Daten</p>
          )}
          {data?.confidence && (
            <ConfidenceBadge confidence={data.confidence} />
          )}
        </div>
      </div>
    </div>
  )
}

// ── ReadinessCardFull — Insights variant (default export) ─────────────────────

export default function ReadinessCardFull() {
  const queryClient = useQueryClient()

  const { data, isLoading, isError } = useQuery({
    queryKey: ['analytics', 'readiness'],
    queryFn: fetchReadiness,
    staleTime: 5 * 60_000,
  })

  const { data: prefs } = useQuery({
    queryKey: ['user', 'readiness-preferences'],
    queryFn: fetchReadinessPreferences,
    staleTime: 5 * 60_000,
  })

  const prefMutation = useMutation({
    mutationFn: putReadinessPreferences,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['analytics', 'readiness'] })
      queryClient.invalidateQueries({ queryKey: ['user', 'readiness-preferences'] })
    },
  })

  // Source toggle is visible only when at least one factor has ≥2 available sources.
  // Using factor.source alone would be wrong — it only shows the active choice,
  // not whether an alternative exists (Codex-Finding #6).
  const toggleVisible = (data?.factors ?? []).some(
    f => (f.available_sources?.length ?? 0) >= 2,
  )

  if (isLoading) return <CardSkeleton />

  if (isError) {
    return (
      <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-200 mb-3">
          Bereitschaft
        </h3>
        <p className="text-sm text-neutral-400 text-center py-4">
          Bereitschaftswert konnte nicht geladen werden.
        </p>
      </div>
    )
  }

  const level = data?.level ?? null
  const color = level ? LEVEL_COLORS[level] : '#6b7280'
  const recCode = data?.recommendation_code ?? null
  const recText = recommendationText(recCode)
  const factors = data?.factors ?? []
  const missingFactors = data?.missing_factors ?? []

  return (
    <div
      className="rounded-xl border p-5 space-y-4"
      style={{
        background: `linear-gradient(135deg, ${color}12, ${color}06)`,
        borderColor: `${color}30`,
      }}
      data-testid="readiness-full"
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2 flex-wrap">
        <h3 className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
          Bereitschaft
        </h3>
        {data?.confidence && <ConfidenceBadge confidence={data.confidence} />}
      </div>

      {/* Score ring + factors */}
      <div className="flex items-start gap-4">
        <div className="flex-shrink-0 flex flex-col items-center gap-1">
          <ScoreRing score={data?.score ?? null} level={level} size={64} />
          {level && (
            <span className="text-[10px] font-medium" style={{ color }}>
              {LEVEL_LABELS[level]}
            </span>
          )}
        </div>

        {factors.length > 0 && (
          <div className="flex-1 min-w-0 space-y-2">
            {factors.map(f => (
              <FactorRow key={f.key} factor={f} />
            ))}
          </div>
        )}
      </div>

      {/* Source toggle (only when ≥1 factor has ≥2 available_sources) */}
      {toggleVisible && (
        <SourceToggle
          preferred={prefs?.preferred_source ?? null}
          onChange={source => prefMutation.mutate({ preferred_source: source })}
          disabled={prefMutation.isPending}
        />
      )}

      {/* Refusal / missing factors banner */}
      {(data?.refused || missingFactors.length > 0) && (
        <MissingFactorsBanner
          refused={data?.refused ?? false}
          refusalReason={data?.refusal_reason ?? null}
          missingFactors={missingFactors}
        />
      )}

      {/* Recommendation */}
      {recText && (
        <p
          className="text-xs text-neutral-600 dark:text-neutral-300 leading-relaxed border-t border-neutral-200/50 dark:border-neutral-700/50 pt-3"
          data-testid="recommendation-text"
        >
          {recText}
        </p>
      )}

      {/* Signal modifier notice */}
      {data?.signal_modifier === 'rhr_changepoint_recent' && (
        <p
          className="text-[10px] text-amber-600 dark:text-amber-400"
          data-testid="signal-modifier-notice"
        >
          Hinweis: Ruhepuls-Veränderung erkannt.
        </p>
      )}
    </div>
  )
}
