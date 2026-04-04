import type { Activity } from '@/api/types'

// ── Label constants (i18n-ready) ──────────────────────────────────────────────

const LABELS = {
  title: 'Laufform',
  dutyFactor: 'Duty Factor',
  verticalRatio: 'Vertical Ratio',
  gctAsymmetry: 'GCT-Asymmetrie',
  fatigueIndex: 'Fatigue Index',
  fatigueVR: 'Fatigue VR',
  tier: (n: number) => `Tier ${n}`,
}

// ── Color helpers ─────────────────────────────────────────────────────────────

function dutyFactorColor(val: number): string {
  if (val < 0.30) return 'text-green-600 dark:text-green-400'
  if (val <= 0.40) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function verticalRatioColor(val: number): string {
  if (val < 0.08) return 'text-green-600 dark:text-green-400'
  if (val <= 0.10) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function gctAsymmetryColor(val: number): string {
  if (val < 3) return 'text-green-600 dark:text-green-400'
  if (val <= 6) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function fatigueColor(val: number): string {
  const abs = Math.abs(val)
  if (abs < 5) return 'text-green-600 dark:text-green-400'
  if (abs <= 15) return 'text-amber-500 dark:text-amber-400'
  return 'text-red-500 dark:text-red-400'
}

function fatigueForcedRed(val: number): string {
  // Positive fatigue = deterioration → always red regardless of magnitude
  if (val > 0) return 'text-red-500 dark:text-red-400'
  return fatigueColor(val)
}

// ── Tier badge ────────────────────────────────────────────────────────────────

function TierBadge({ tier }: { tier: number }) {
  const styles: Record<number, string> = {
    1: 'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950/40 dark:text-blue-400 dark:border-blue-800',
    2: 'bg-violet-50 text-violet-700 border-violet-200 dark:bg-violet-950/40 dark:text-violet-400 dark:border-violet-800',
    3: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950/40 dark:text-emerald-400 dark:border-emerald-800',
  }
  return (
    <span className={`inline-flex items-center rounded border px-1.5 py-0.5 text-[10px] font-semibold tracking-wide ${styles[tier] ?? ''}`}>
      {LABELS.tier(tier)}
    </span>
  )
}

// ── Individual metric tile ────────────────────────────────────────────────────

interface MetricTileProps {
  label: string
  value: string
  colorClass: string
}

function MetricTile({ label, value, colorClass }: MetricTileProps) {
  return (
    <div className="flex flex-col gap-0.5 rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 px-3 py-2 min-w-[100px]">
      <span className="text-[10px] font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide whitespace-nowrap">
        {label}
      </span>
      <span className={`text-sm font-semibold tabular-nums ${colorClass}`}>
        {value}
      </span>
    </div>
  )
}

// ── Main component ────────────────────────────────────────────────────────────

interface RunningFormCardProps {
  activity: Activity
}

export function RunningFormCard({ activity }: RunningFormCardProps) {
  const tier = activity.running_form_tier
  if (!tier || tier < 1) return null

  const dutyFactor = activity.running_form_duty_factor
  const verticalRatio = activity.running_form_vertical_ratio
  const gctAsymmetry = activity.running_form_gct_asymmetry_pct
  const fatigueDuty = activity.running_form_fatigue_duty
  const fatigueVR = activity.running_form_fatigue_vr

  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-3 space-y-2">
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-semibold text-neutral-700 dark:text-neutral-300 uppercase tracking-wide">
          {LABELS.title}
        </span>
        <TierBadge tier={tier} />
      </div>

      {/* Tiles — responsive wrap */}
      <div className="flex flex-wrap gap-2">
        {/* Tier 1: Duty Factor — always shown */}
        {dutyFactor != null && (
          <MetricTile
            label={LABELS.dutyFactor}
            value={dutyFactor.toFixed(2)}
            colorClass={dutyFactorColor(dutyFactor)}
          />
        )}

        {/* Tier 2+: Vertical Ratio */}
        {tier >= 2 && verticalRatio != null && (
          <MetricTile
            label={LABELS.verticalRatio}
            value={verticalRatio.toFixed(3)}
            colorClass={verticalRatioColor(verticalRatio)}
          />
        )}

        {/* Tier 3: GCT Asymmetry */}
        {tier >= 3 && gctAsymmetry != null && (
          <MetricTile
            label={LABELS.gctAsymmetry}
            value={`${gctAsymmetry.toFixed(1)}%`}
            colorClass={gctAsymmetryColor(gctAsymmetry)}
          />
        )}

        {/* Fatigue Index (Duty Factor) — show when available */}
        {fatigueDuty != null && (
          <MetricTile
            label={LABELS.fatigueIndex}
            value={fatigueDuty >= 0 ? `+${fatigueDuty.toFixed(1)}%` : `${fatigueDuty.toFixed(1)}%`}
            colorClass={fatigueForcedRed(fatigueDuty)}
          />
        )}

        {/* Fatigue VR — show at Tier 2+ when available */}
        {tier >= 2 && fatigueVR != null && (
          <MetricTile
            label={LABELS.fatigueVR}
            value={fatigueVR >= 0 ? `+${fatigueVR.toFixed(1)}%` : `${fatigueVR.toFixed(1)}%`}
            colorClass={fatigueForcedRed(fatigueVR)}
          />
        )}
      </div>
    </div>
  )
}
