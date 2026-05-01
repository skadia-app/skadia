// ReadinessCard.tsx — Readiness score card (compact for Dashboard, full for Insights).
// UI strings in German; comments in English.
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Lightbulb, AlertTriangle, AlertCircle, RefreshCw, HeartPulse } from 'lucide-react'
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
  insufficient_factors:
    'Noch nicht berechenbar — aktuelle HRV-, Schlaf- und Trainingslast-Daten werden benötigt.',
  no_data:
    'Noch keine Gesundheitsdaten vorhanden — verbinde ein Wearable oder importiere FIT-Dateien.',
  service_unavailable:
    'Berechnung läuft im Hintergrund. Seite in einer Minute aktualisieren.',
}

// Single-line refusal hints for the compact/dashboard variant — must fit in ~1 line.
const REFUSAL_TEXTS_COMPACT: Record<string, string> = {
  insufficient_factors: 'Datenbasis fehlt',
  no_data:              'Keine Daten',
  service_unavailable:  'Berechnung läuft…',
}

// Reason labels for per-factor missing explanations.
const MISSING_REASON_LABELS: Record<string, string> = {
  no_data:       'Noch keine Daten',
  no_data_today: 'Heute keine Messung',
  stale:         'Letzte Messung zu alt',
  no_baseline:   'Basislinie noch nicht ausreichend',
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

// ── Level scale thresholds — defines the boundary positions on the 0-100 bar ──
// Levels: critical <20, low <40, moderate <60, good <80, optimal ≤100
const LEVEL_THRESHOLDS: { level: ReadinessLevel; pct: number; label: string }[] = [
  { level: 'critical', pct: 20, label: 'Kritisch' },
  { level: 'low',      pct: 40, label: 'Niedrig'  },
  { level: 'moderate', pct: 60, label: 'Moderat'  },
  { level: 'good',     pct: 80, label: 'Gut'      },
]

// ── ScoreRing ─────────────────────────────────────────────────────────────────

interface ScoreRingProps {
  score: number | null
  level: ReadinessLevel | null
  size?: number
}

// stroke-width scales with ring diameter so the arc weight stays proportional.
function ringStroke(size: number): number {
  return Math.round(size * 0.09)
}

// Font size: three tiers — small (<56 → 10), medium (<70 → 16), large (≥70 → 18).
// At the dashboard size (64 px) the score number renders at 16 pt, clearly readable.
// The full-card size (72 px) gets 18 pt for stronger visual hierarchy.
function ringFontSize(size: number): number {
  if (size < 56) return 10
  if (size < 70) return 16
  return 18
}

function ScoreRing({ score, level, size = 64 }: ScoreRingProps) {
  const stroke = ringStroke(size)
  const r = size / 2 - stroke
  const cx = size / 2
  const cy = size / 2
  const circumference = 2 * Math.PI * r
  const pct = score !== null ? Math.max(0, Math.min(100, score)) : 0
  const offset = circumference * (1 - pct / 100)
  const color = level ? LEVEL_COLORS[level] : '#6b7280'
  const fontSize = ringFontSize(size)

  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${size} ${size}`}
      aria-label={score !== null ? `Bereitschaftswert ${score} von 100` : 'Kein Bereitschaftswert'}
      role="img"
      style={{ filter: level ? `drop-shadow(0 1px 3px ${color}55)` : undefined }}
    >
      <circle
        cx={cx} cy={cy} r={r}
        fill="none" stroke="#e5e7eb" strokeWidth={stroke}
        className="dark:[stroke:#374151]"
      />
      <circle
        cx={cx} cy={cy} r={r}
        fill="none" stroke={color} strokeWidth={stroke}
        strokeDasharray={circumference} strokeDashoffset={offset}
        strokeLinecap="round"
        transform={`rotate(-90 ${cx} ${cy})`}
        style={{ transition: 'stroke-dashoffset 0.6s ease' }}
      />
      {score !== null ? (
        <text
          x={cx} y={cy} textAnchor="middle" dominantBaseline="middle"
          fontSize={fontSize} fontWeight="bold" fill={color}
          fontFamily="inherit"
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

// ── GhostRing — visual anchor for empty/error states ─────────────────────────
// Dashed neutral outline with optional center icon; mirrors ScoreRing dimensions.
// Uses a relative-positioned wrapper so the icon can be centered via absolute positioning
// without requiring foreignObject (which has inconsistent support in test renderers).

interface GhostRingProps {
  size?: number
}

function GhostRing({ size = 64 }: GhostRingProps) {
  const stroke = ringStroke(size)
  const r = size / 2 - stroke
  const cx = size / 2
  const cy = size / 2
  const iconSize = size < 56 ? 14 : 18

  return (
    <div className="relative flex-shrink-0" style={{ width: size, height: size }}>
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        aria-hidden="true"
        style={{ display: 'block' }}
      >
        {/* Dashed ghost outline — same track geometry as ScoreRing (stroke-width via ringStroke) */}
        <circle
          cx={cx} cy={cy} r={r}
          fill="none"
          stroke="#d1d5db"
          strokeWidth={stroke}
          strokeDasharray="4 4"
          className="dark:[stroke:#404040]"
        />
      </svg>
      {/* Center icon — absolutely positioned over the SVG */}
      <div
        className="absolute inset-0 flex items-center justify-center"
        aria-hidden="true"
      >
        <HeartPulse
          size={iconSize}
          className="text-neutral-300 dark:text-neutral-600"
        />
      </div>
    </div>
  )
}

// ── ScaleBar — horizontal Schlecht→Optimal progress bar ───────────────────────

interface ScaleBarProps {
  score: number | null
  level: ReadinessLevel | null
}

function ScaleBar({ score, level }: ScaleBarProps) {
  const pct = score !== null ? Math.max(0, Math.min(100, score)) : 0
  const color = level ? LEVEL_COLORS[level] : '#6b7280'

  return (
    <div className="space-y-1.5">
      {/* Track + fill */}
      <div
        className="relative h-2.5 w-full rounded-full bg-neutral-200 dark:bg-neutral-700 overflow-visible"
        role="progressbar"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-label={`Bereitschaftswert ${pct} von 100`}
      >
        {/* Coloured fill */}
        <div
          className="absolute inset-y-0 left-0 rounded-full"
          style={{
            width: `${pct}%`,
            backgroundColor: color,
            transition: 'width 0.6s ease',
          }}
        />
        {/* Level boundary tick marks */}
        {LEVEL_THRESHOLDS.map(t => (
          <div
            key={t.level}
            className="absolute top-[-2px] bottom-[-2px] w-px bg-white dark:bg-neutral-900 opacity-60"
            style={{ left: `${t.pct}%` }}
          />
        ))}
      </div>

      {/* End labels */}
      <div className="flex justify-between text-[10px] text-neutral-400 dark:text-neutral-500 select-none">
        <span>Schlecht</span>
        <span>Optimal</span>
      </div>
    </div>
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
        <p className="font-medium mb-1">Fehlende Faktoren:</p>
        <ul className="space-y-0.5">
          {missingFactors.map(f => (
            <li key={f.key} className="flex items-center justify-between gap-2">
              <span>{FACTOR_LABELS[f.key] ?? f.key}</span>
              <span className="text-neutral-400 dark:text-neutral-500 shrink-0">
                {MISSING_REASON_LABELS[f.reason] ?? f.reason}
              </span>
            </li>
          ))}
        </ul>
      </div>
    )
  }

  return null
}

// ── Loading skeleton ──────────────────────────────────────────────────────────

function CardSkeleton({ compact }: { compact?: boolean }) {
  // Compact skeleton ring: 64 px (matches compact ScoreRing/GhostRing size={64}).
  // Full skeleton ring: 72 px (matches full ScoreRing/GhostRing size={72}).
  const ringSize = compact ? 'w-16 h-16' : 'w-[72px] h-[72px]'
  return (
    <div
      className={`rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 animate-pulse${compact ? ' flex flex-col justify-center h-full' : ''}`}
      aria-busy="true"
      aria-label="Lade Bereitschaftswert"
    >
      <div className="h-3 w-40 bg-neutral-200 dark:bg-neutral-700 rounded mb-3" />
      <div className={`flex items-center gap-4 ${compact ? '' : 'mb-3'}`}>
        <div className={`${ringSize} rounded-full bg-neutral-200 dark:bg-neutral-700 flex-shrink-0`} />
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
      <div
        className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4 flex flex-col justify-center h-full"
        data-testid="readiness-compact-empty"
      >
        <div className="flex items-center gap-3">
          <GhostRing size={64} />
          <div className="min-w-0">
            <p className="text-[10px] uppercase tracking-wide font-medium text-neutral-400 dark:text-neutral-500">
              Bereitschaft
            </p>
            <p className="flex items-center gap-1 text-xs text-neutral-400 dark:text-neutral-500">
              <AlertCircle size={11} aria-hidden="true" className="flex-shrink-0" />
              Nicht verfügbar
            </p>
          </div>
        </div>
      </div>
    )
  }

  const level = data?.level ?? null
  const color = level ? LEVEL_COLORS[level] : '#6b7280'

  return (
    <div
      className="rounded-xl border p-4 flex flex-col justify-center h-full"
      style={{
        background: `linear-gradient(135deg, ${color}12, ${color}06)`,
        borderColor: `${color}30`,
      }}
      data-testid="readiness-compact"
    >
      <div className="flex items-center gap-3">
        <ScoreRing score={data?.score ?? null} level={level} size={64} />
        <div className="min-w-0">
          <p className="text-[10px] uppercase tracking-wide font-medium text-neutral-500 dark:text-neutral-400">
            Bereitschaft
          </p>
          {data?.refused ? (
            // Compact refused: ghost ring layout with single-line hint (no heavy amber banner).
            <p className="text-xs text-neutral-400 dark:text-neutral-500 line-clamp-1">
              {REFUSAL_TEXTS_COMPACT[data.refusal_reason ?? ''] ?? 'Nicht berechenbar'}
            </p>
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
      <div
        className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3"
        data-testid="readiness-full-empty"
      >
        {/* Header mirrors populated layout */}
        <h3 className="text-xs font-medium text-neutral-400 dark:text-neutral-500 uppercase tracking-wide">
          Bereitschaft
        </h3>
        {/* Ghost ring + error message — vertically centred */}
        <div className="flex flex-col items-center gap-3 py-2">
          <GhostRing size={72} />
          <p className="text-sm text-neutral-400 dark:text-neutral-500 text-center leading-snug">
            Bereitschaftswert konnte nicht geladen werden.
          </p>
          <p className="flex items-center gap-1 text-xs text-neutral-400 dark:text-neutral-500">
            <RefreshCw size={11} aria-hidden="true" className="flex-shrink-0" />
            Seite neu laden
          </p>
        </div>
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
          <ScoreRing score={data?.score ?? null} level={level} size={72} />
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

      {/* Schlecht→Optimal scale bar — only when a score is available */}
      {data?.score !== null && data?.score !== undefined && (
        <ScaleBar score={data.score} level={level} />
      )}

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
        <div
          className="flex items-start gap-2 border-t border-neutral-200/50 dark:border-neutral-700/50 pt-3"
          data-testid="recommendation-text"
        >
          <Lightbulb
            className="text-neutral-400 dark:text-neutral-500 flex-shrink-0 mt-px"
            size={13}
            aria-hidden="true"
          />
          <p className="text-xs text-neutral-600 dark:text-neutral-300 leading-relaxed">
            {recText}
          </p>
        </div>
      )}

      {/* Signal modifier notice */}
      {data?.signal_modifier === 'rhr_changepoint_recent' && (
        <div
          className="flex items-start gap-2 rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-3 py-2"
          role="note"
          data-testid="signal-modifier-notice"
        >
          <AlertTriangle
            className="text-amber-500 flex-shrink-0 mt-px"
            size={12}
            aria-hidden="true"
          />
          <p className="text-[10px] text-amber-700 dark:text-amber-300 leading-relaxed">
            Hinweis: Ruhepuls-Veränderung erkannt.
          </p>
        </div>
      )}
    </div>
  )
}
