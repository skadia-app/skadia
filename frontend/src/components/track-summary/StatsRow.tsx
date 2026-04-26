import type { ReactNode } from 'react'
import { Building2, Droplet, Mountain, Shield, TreePine } from 'lucide-react'
import { labels } from '@/lib/trackSummaryLabels'

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

export interface StatsRowProps {
  ascentM?: number | null
  descentM?: number | null
  forestPct?: number | null
  protectedPct?: number | null
  summits?: SummaryItem[]
  waters?: SummaryItem[]
  settlements?: SummaryItem[]
  sacAggregate?: ScaleAggregate | null
  mtbAggregate?: ScaleAggregate | null
  vfAggregate?: VfAggregate | null
  scalesDisabled?: boolean
  onSymbolHover?: (kind: StatsKind, items: SummaryItem[]) => void
  onSymbolLeave?: () => void
}

interface SymbolChipProps {
  kind: StatsKind
  icon: ReactNode
  label: string
  items: SummaryItem[]
  onHover?: (kind: StatsKind, items: SummaryItem[]) => void
  onLeave?: () => void
}

function ItemChip({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-md bg-neutral-100 px-2 py-1 text-neutral-700 dark:bg-neutral-800 dark:text-neutral-200">
      {children}
    </span>
  )
}

function InteractiveSymbolChip({ kind, icon, label, items, onHover, onLeave }: SymbolChipProps) {
  const handleEnter = () => onHover?.(kind, items)
  const handleLeave = () => onLeave?.()

  if (!onHover || items.length === 0) {
    return (
      <ItemChip>
        {icon}
        <span>{label}</span>
      </ItemChip>
    )
  }

  return (
    <button
      type="button"
      className="inline-flex items-center gap-1.5 rounded-md bg-neutral-100 px-2 py-1 text-neutral-700 transition-colors hover:bg-neutral-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT/60 dark:bg-neutral-800 dark:text-neutral-200 dark:hover:bg-neutral-700"
      onMouseEnter={handleEnter}
      onMouseLeave={handleLeave}
      onFocus={handleEnter}
      onBlur={handleLeave}
    >
      {icon}
      <span>{label}</span>
    </button>
  )
}

export function StatsRow({
  ascentM,
  descentM,
  forestPct,
  protectedPct,
  summits = [],
  waters = [],
  settlements = [],
  sacAggregate,
  mtbAggregate,
  vfAggregate,
  scalesDisabled = false,
  onSymbolHover,
  onSymbolLeave,
}: StatsRowProps) {
  const t = labels.trackSummary

  return (
    <div
      data-testid="stats-row"
      className="flex flex-wrap items-center gap-2 text-xs text-neutral-600 dark:text-neutral-300"
    >
      {ascentM != null && (
        <ItemChip>
          <span aria-hidden="true" className="text-emerald-600 dark:text-emerald-400">↑</span>
          <span>{t.ascent}</span>
          <span className="font-medium tabular-nums text-neutral-900 dark:text-neutral-100">
            {Math.round(ascentM)} m
          </span>
        </ItemChip>
      )}

      {descentM != null && (
        <ItemChip>
          <span aria-hidden="true" className="text-sky-600 dark:text-sky-400">↓</span>
          <span>{t.descent}</span>
          <span className="font-medium tabular-nums text-neutral-900 dark:text-neutral-100">
            {Math.round(descentM)} m
          </span>
        </ItemChip>
      )}

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
        <InteractiveSymbolChip
          kind="summit"
          icon={<Mountain className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.summitsCountTpl(summits.length)}
          items={summits}
          onHover={onSymbolHover}
          onLeave={onSymbolLeave}
        />
      )}

      {waters.length > 0 && (
        <InteractiveSymbolChip
          kind="water"
          icon={<Droplet className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.watersCountTpl(waters.length)}
          items={waters}
          onHover={onSymbolHover}
          onLeave={onSymbolLeave}
        />
      )}

      {settlements.length > 0 && (
        <InteractiveSymbolChip
          kind="settlement"
          icon={<Building2 className="h-3.5 w-3.5" aria-hidden="true" />}
          label={t.settlementsCountTpl(settlements.length)}
          items={settlements}
          onHover={onSymbolHover}
          onLeave={onSymbolLeave}
        />
      )}

      {scalesDisabled ? (
        <ItemChip>
          <span title={t.scalesDisabledHint}>{t.scalesDisabled}</span>
        </ItemChip>
      ) : (
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
