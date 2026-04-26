import type { ReactNode } from 'react'
import type {
  ActivityTrackSummary,
  TrackSummaryGeoTagItem,
  TrackSummaryWaterItem,
} from '@/api/trackSummary'
import StatsRow, { type ScaleAggregate, type StatsRowProps, type SummaryItem, type VfAggregate, type ScaleKind } from './StatsRow'
import { computeScaleBreakdown } from '@/lib/trackSummaryScales'

const SAC_ORDER = ['T1', 'T2', 'T3', 'T4', 'T5', 'T6'] as const
const MTB_ORDER = ['S0', 'S1', 'S2', 'S3', 'S4', 'S5'] as const
const VF_ORDER = ['K1', 'K2', 'K3', 'K4', 'K5', 'K6'] as const

// Re-export ScaleKind for consumers that import from this module.
export type { ScaleKind }

interface TrackSummaryProps {
  summary: ActivityTrackSummary | undefined
  sportType?: string
  onSymbolHover?: StatsRowProps['onSymbolHover']
  onSymbolLeave?: StatsRowProps['onSymbolLeave']
  /** Scale hover/pin state — forwarded to StatsRow for ScaleBadge interaction. */
  pinnedScale?: ScaleKind | null
  onScaleHover?: (kind: ScaleKind | null) => void
  onScalePin?: (kind: ScaleKind) => void
  /**
   * When true the StatsRow pills row is NOT rendered at the top of this section.
   * Use when the caller wants to position the pills elsewhere (e.g. as the
   * `pills` prop of SurfacesSection, placed below its header).
   * Call buildStatsRowProps() to get the computed props for your own StatsRow.
   */
  suppressPillsAtTop?: boolean
  children?: ReactNode
}

function pickMaxScale(
  ranges: Record<string, unknown> | null,
  order: readonly string[],
): ScaleAggregate | null {
  if (!ranges) return null
  let max: string | null = null
  for (const level of order) {
    const entries = ranges[level]
    if (Array.isArray(entries) && entries.length > 0) {
      max = level
    }
  }
  return max ? { max } : null
}

function pickVfAggregate(summary: ActivityTrackSummary | undefined): VfAggregate | null {
  if (!summary) return null

  const max = pickMaxScale(summary.vf_ranges, VF_ORDER)?.max ?? null
  const hasUnknown = (summary.vf_unknown_ranges?.length ?? 0) > 0

  if (!max && !hasUnknown) return null

  return {
    max,
    hasUnknown,
  }
}

function toSummaryItem(item: TrackSummaryGeoTagItem | TrackSummaryWaterItem): SummaryItem {
  return {
    osm_id: item.osm_id,
    name: item.name,
    track_pos: item.track_pos,
    ele_m: item.ele_m,
  }
}

/**
 * Derives all StatsRow props from a loaded ActivityTrackSummary.
 * Call this when you need the pills row somewhere other than the
 * default top position (e.g. as `pills` prop of SurfacesSection).
 */
export function buildStatsRowProps(summary: ActivityTrackSummary): Pick<
  StatsRowProps,
  | 'forestPct' | 'protectedPct' | 'summits' | 'waters' | 'settlements'
  | 'sacAggregate' | 'mtbAggregate' | 'vfAggregate'
  | 'sacBreakdown' | 'mtbBreakdown' | 'vfBreakdown'
  | 'sacMaxLevel' | 'mtbMaxLevel' | 'vfMaxLevel'
> {
  const sacResult = computeScaleBreakdown(summary.sac_ranges, summary.resample_spacing_m, SAC_ORDER)
  const mtbResult = computeScaleBreakdown(summary.mtb_ranges, summary.resample_spacing_m, MTB_ORDER)
  const vfResult = computeScaleBreakdown(summary.vf_ranges, summary.resample_spacing_m, VF_ORDER)

  return {
    forestPct: summary.forest_pct,
    protectedPct: summary.protected_pct,
    summits: (summary.summits ?? []).map(toSummaryItem),
    waters: (summary.waters ?? []).map(toSummaryItem),
    settlements: (summary.settlements ?? []).map(toSummaryItem),
    sacAggregate: pickMaxScale(summary.sac_ranges, SAC_ORDER),
    mtbAggregate: pickMaxScale(summary.mtb_ranges, MTB_ORDER),
    vfAggregate: pickVfAggregate(summary),
    sacBreakdown: sacResult.breakdown,
    mtbBreakdown: mtbResult.breakdown,
    vfBreakdown: vfResult.breakdown,
    sacMaxLevel: sacResult.maxLevel,
    mtbMaxLevel: mtbResult.maxLevel,
    vfMaxLevel: vfResult.maxLevel,
  }
}

function LoadingShell() {
  return (
    <section data-testid="track-summary" className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="h-7 w-28 rounded-md bg-neutral-100 animate-pulse dark:bg-neutral-800" />
        <div className="h-7 w-24 rounded-md bg-neutral-100 animate-pulse dark:bg-neutral-800" />
        <div className="h-7 w-32 rounded-md bg-neutral-100 animate-pulse dark:bg-neutral-800" />
      </div>
    </section>
  )
}

export function TrackSummary({
  summary,
  sportType,
  onSymbolHover,
  onSymbolLeave,
  pinnedScale,
  onScaleHover,
  onScalePin,
  suppressPillsAtTop = false,
  children,
}: TrackSummaryProps) {
  if (summary === undefined) {
    return <LoadingShell />
  }

  const statsRowProps = buildStatsRowProps(summary)

  return (
    <section data-testid="track-summary" className="space-y-3">
      {!suppressPillsAtTop && (
        <StatsRow
          {...statsRowProps}
          sportType={sportType}
          onSymbolHover={onSymbolHover}
          onSymbolLeave={onSymbolLeave}
          pinnedScale={pinnedScale}
          onScaleHover={onScaleHover}
          onScalePin={onScalePin}
        />
      )}
      {children}
    </section>
  )
}

export default TrackSummary
