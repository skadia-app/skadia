import type { ReactNode } from 'react'
import { Building2, Droplet, Mountain, Shield, TreePine } from 'lucide-react'
import { labels } from '@/lib/trackSummaryLabels'
import { SymbolListPopover } from './SymbolListPopover'

export type SummaryItem = {
  osm_id: number | null
  name: string
  track_pos: number | null
  ele_m: number | null
}

export type StatsKind = 'summit' | 'water' | 'settlement'

export type ScaleAggregate = {
  max: string | null
}

export type VfAggregate = ScaleAggregate & {
  hasUnknown: boolean
}

// Sports for which SAC / MTB / VF difficulty scale pills are meaningful.
// Mirrors the backend enrichment gate — only outdoor/mountain sports produce these scales.
const SCALE_VISIBLE_SPORTS = new Set([
  'hiking',
  'mountaineering',
  'snowshoeing',
  'trail_running',
  'mountain_biking',
  'backcountry_skiing',
  'rock_climbing',
])

export interface StatsRowProps {
  forestPct?: number | null
  protectedPct?: number | null
  summits?: SummaryItem[]
  waters?: SummaryItem[]
  settlements?: SummaryItem[]
  sacAggregate?: ScaleAggregate | null
  mtbAggregate?: ScaleAggregate | null
  vfAggregate?: VfAggregate | null
  sportType?: string
  onSymbolHover?: (kind: StatsKind, items: SummaryItem[]) => void
  onSymbolLeave?: () => void
}

function ItemChip({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md bg-neutral-100 px-2 py-1 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-200">
      {children}
    </span>
  )
}

export function StatsRow({
  forestPct,
  protectedPct,
  summits = [],
  waters = [],
  settlements = [],
  sacAggregate,
  mtbAggregate,
  vfAggregate,
  sportType,
  onSymbolHover,
  onSymbolLeave,
}: StatsRowProps) {
  const t = labels.trackSummary
  const showScales = sportType != null && SCALE_VISIBLE_SPORTS.has(sportType)

  return (
    <div
      data-testid="stats-row"
      className="flex flex-wrap items-center gap-2 text-xs text-neutral-600 dark:text-neutral-300"
    >
      {forestPct != null && (
        <ItemChip>
          <TreePine className="h-3.5 w-3.5" aria-hidden="true" />
          <span>{t.forestPctTpl(forestPct)}</span>
        </ItemChip>
      )}

      {protectedPct != null && (
        <ItemChip>
          <Shield className="h-3.5 w-3.5" aria-hidden="true" />
          <span>{t.protectedPctTpl(protectedPct)}</span>
        </ItemChip>
      )}

      {summits.length > 0 && (
        <SymbolListPopover
          kind="summit"
          icon={<Mountain className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.summitsCountTpl(summits.length)}
          items={summits}
          onHover={onSymbolHover}
          onLeave={onSymbolLeave}
        />
      )}

      {waters.length > 0 && (
        <SymbolListPopover
          kind="water"
          icon={<Droplet className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.watersCountTpl(waters.length)}
          items={waters}
        />
      )}

      {settlements.length > 0 && (
        <SymbolListPopover
          kind="settlement"
          icon={<Building2 className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.settlementsCountTpl(settlements.length)}
          items={settlements}
        />
      )}

      {showScales && (
        <>
          {sacAggregate?.max && (
            <ItemChip>
              <span>{t.sacUpToTpl(sacAggregate.max)}</span>
            </ItemChip>
          )}

          {mtbAggregate?.max && (
            <ItemChip>
              <span>{t.mtbUpToTpl(mtbAggregate.max)}</span>
            </ItemChip>
          )}

          {vfAggregate?.max && (
            <ItemChip>
              <span>{t.vfUpToTpl(vfAggregate.max)}</span>
            </ItemChip>
          )}

          {vfAggregate?.hasUnknown && (
            <ItemChip>
              <span>{t.vfUnknownPresent}</span>
            </ItemChip>
          )}
        </>
      )}
    </div>
  )
}

export default StatsRow
