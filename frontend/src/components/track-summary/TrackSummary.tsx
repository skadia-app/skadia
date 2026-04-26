import type { ReactNode } from 'react'
import type {
  ActivityTrackSummary,
  TrackSummaryGeoTagItem,
  TrackSummaryWaterItem,
} from '@/api/trackSummary'
import StatsRow, { type ScaleAggregate, type StatsRowProps, type SummaryItem, type VfAggregate } from './StatsRow'

const SAC_ORDER = ['T1', 'T2', 'T3', 'T4', 'T5', 'T6'] as const
const MTB_ORDER = ['S0', 'S1', 'S2', 'S3', 'S4', 'S5'] as const
const VF_ORDER = ['K1', 'K2', 'K3', 'K4', 'K5', 'K6'] as const

interface TrackSummaryProps {
  summary: ActivityTrackSummary | undefined
  sportType?: string
  onSymbolHover?: StatsRowProps['onSymbolHover']
  onSymbolLeave?: StatsRowProps['onSymbolLeave']
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
  children,
}: TrackSummaryProps) {
  if (summary === undefined) {
    return <LoadingShell />
  }

  const summits = (summary.summits ?? []).map(toSummaryItem)
  const waters = (summary.waters ?? []).map(toSummaryItem)
  const settlements = (summary.settlements ?? []).map(toSummaryItem)

  return (
    <section data-testid="track-summary" className="space-y-3">
      <StatsRow
        forestPct={summary.forest_pct}
        protectedPct={summary.protected_pct}
        summits={summits}
        waters={waters}
        settlements={settlements}
        sacAggregate={pickMaxScale(summary.sac_ranges, SAC_ORDER)}
        mtbAggregate={pickMaxScale(summary.mtb_ranges, MTB_ORDER)}
        vfAggregate={pickVfAggregate(summary)}
        sportType={sportType}
        onSymbolHover={onSymbolHover}
        onSymbolLeave={onSymbolLeave}
      />
      {children}
    </section>
  )
}

export default TrackSummary
