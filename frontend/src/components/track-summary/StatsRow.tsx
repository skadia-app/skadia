import type { ReactNode } from 'react'
import { Building2, Droplet, Mountain, Shield, TreePine } from 'lucide-react'
import { labels } from '@/lib/trackSummaryLabels'
import { SymbolListPopover } from './SymbolListPopover'
import { ScaleBadge } from '@/components/shared/scale-badge/ScaleBadge'
import type { ScaleBadgeLevel } from '@/lib/scaleAggregation'
import { normalizeSportTuple } from '@/lib/sportTaxonomy'

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

const SCALE_VISIBLE_SPORTS = new Set([
  'hiking',
  'mountaineering',
  'snowshoeing',
  'backcountry_skiing',
  'rock_climbing',
  'via_ferrata',
])

export type ScaleKind = 'sac' | 'mtb' | 'vf'

export interface StatsRowProps {
  forestPct?: number | null
  protectedPct?: number | null
  summits?: SummaryItem[]
  waters?: SummaryItem[]
  settlements?: SummaryItem[]
  /** Legacy aggregate — still used by contexts without coord data (e.g. list views). */
  sacAggregate?: ScaleAggregate | null
  mtbAggregate?: ScaleAggregate | null
  vfAggregate?: VfAggregate | null
  /** Rich breakdown for ScaleBadge with hover-popover. When provided, overrides sacAggregate display. */
  sacBreakdown?: ScaleBadgeLevel[]
  mtbBreakdown?: ScaleBadgeLevel[]
  vfBreakdown?: ScaleBadgeLevel[]
  sacMaxLevel?: string | null
  mtbMaxLevel?: string | null
  vfMaxLevel?: string | null
  sportType?: string
  subType?: string | null
  onSymbolHover?: (kind: StatsKind, items: SummaryItem[]) => void
  onSymbolLeave?: () => void
  /** Scale hover/pin callbacks — used when ScaleBadge breakdowns are provided. */
  pinnedScale?: ScaleKind | null
  onScaleHover?: (kind: ScaleKind | null) => void
  onScalePin?: (kind: ScaleKind) => void
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
  sacBreakdown,
  mtbBreakdown,
  vfBreakdown,
  sacMaxLevel,
  mtbMaxLevel,
  vfMaxLevel,
  sportType,
  subType,
  onSymbolHover,
  onSymbolLeave,
  pinnedScale,
  onScaleHover,
  onScalePin,
}: StatsRowProps) {
  const t = labels.trackSummary
  const tuple = sportType ? normalizeSportTuple(sportType, subType) : null
  const showScales = tuple != null && (
    SCALE_VISIBLE_SPORTS.has(tuple.sportType) ||
    (tuple.sportType === 'running' && tuple.subType === 'trail') ||
    (tuple.sportType === 'cycling' && (tuple.subType === 'mountain' || tuple.subType === 'gravel_cycling'))
  )

  // Item 2: hide forest/protected at 0% (Math.round-aware: <0.5 rounds to 0).
  // Activity API returns 0..100 range; threshold mirrors route-planner
  // (RouteProfileChart: >0 for forest, >0.05 for protected on 0..1 scale →
  // activity equivalent: ≥1 for forest (i.e. Math.round ≥ 1), ≥1 for protected).
  const showForest = forestPct != null && Math.round(forestPct) >= 1
  const showProtected = protectedPct != null && Math.round(protectedPct) >= 1

  return (
    <div
      data-testid="stats-row"
      className="flex flex-wrap items-center gap-2 text-xs text-neutral-600 dark:text-neutral-300"
    >
      {/* Item 3: tooltip text matches route-planner (RouteProfileChart.tsx:432,435). */}
      {showForest && (
        <span title="Waldanteil auf der Route">
          <ItemChip>
            <TreePine className="h-3.5 w-3.5" aria-hidden="true" />
            <span>{t.forestPctTpl(forestPct!)}</span>
          </ItemChip>
        </span>
      )}

      {showProtected && (
        <span title="Anteil durch Schutzgebiet auf der Route">
          <ItemChip>
            <Shield className="h-3.5 w-3.5" aria-hidden="true" />
            <span>{t.protectedPctTpl(protectedPct!)}</span>
          </ItemChip>
        </span>
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
          {/* Item 4: use ScaleBadge (with colored dot + hover popover) when breakdown data is
              available; fall back to plain ItemChip for contexts without coord streams. */}
          {sacBreakdown && sacBreakdown.length > 0 && sacMaxLevel ? (
            <ScaleBadge
              kind="sac"
              breakdown={sacBreakdown}
              maxLevel={sacMaxLevel}
              isPinned={pinnedScale === 'sac'}
              onTogglePin={() => onScalePin?.('sac')}
              onHover={(active) => onScaleHover?.(active ? 'sac' : null)}
            />
          ) : (
            sacAggregate?.max && (
              <ItemChip>
                <span>{t.sacUpToTpl(sacAggregate.max)}</span>
              </ItemChip>
            )
          )}

          {mtbBreakdown && mtbBreakdown.length > 0 && mtbMaxLevel ? (
            <ScaleBadge
              kind="mtb"
              breakdown={mtbBreakdown}
              maxLevel={mtbMaxLevel}
              isPinned={pinnedScale === 'mtb'}
              onTogglePin={() => onScalePin?.('mtb')}
              onHover={(active) => onScaleHover?.(active ? 'mtb' : null)}
            />
          ) : (
            mtbAggregate?.max && (
              <ItemChip>
                <span>{t.mtbUpToTpl(mtbAggregate.max)}</span>
              </ItemChip>
            )
          )}

          {vfBreakdown && vfBreakdown.length > 0 && vfMaxLevel ? (
            <ScaleBadge
              kind="vf"
              breakdown={vfBreakdown}
              maxLevel={vfMaxLevel}
              isPinned={pinnedScale === 'vf'}
              onTogglePin={() => onScalePin?.('vf')}
              onHover={(active) => onScaleHover?.(active ? 'vf' : null)}
            />
          ) : (
            vfAggregate?.max && (
              <ItemChip>
                <span>{t.vfUpToTpl(vfAggregate.max)}</span>
              </ItemChip>
            )
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
