import type { Gear } from '@/api/types'
import { formatDistance, formatDuration, formatElevationGain, formatElevationLoss } from '@/lib/utils'

interface GearWearTabProps {
  item: Gear
}

export default function GearWearTab({ item }: GearWearTabProps) {
  return (
    <div className="space-y-6">
      {/* Nutzungs-Aggregate — Ski / Downhill-MTB / Tourenski care about
          descent volume, not ascent, so Tiefenmeter sits next to Höhenmeter. */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">
          Nutzung gesamt
        </h3>
        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-3">
          <WearCard label="Distanz" value={formatDistance(item.total_distance_m)} />
          <WearCard
            label="Höhenmeter"
            value={formatElevationGain(item.total_elevation_gain_m) ?? '—'}
          />
          <WearCard
            label="Tiefenmeter"
            value={formatElevationLoss(item.total_elevation_loss_m) ?? '—'}
          />
          <WearCard label="Zeit" value={formatDuration(item.total_time_s)} />
          <WearCard label="Aktivitäten" value={String(item.total_activities)} />
        </div>
      </section>

      <section
        data-testid="service-events-component-note"
        className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-4 space-y-2"
      >
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          Serviceereignisse
        </h3>
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Serviceeinträge und Wartungsintervalle werden bei den einzelnen Komponenten verwaltet.
        </p>
      </section>
    </div>
  )
}

function WearCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-neutral-200 dark:border-neutral-800 p-3">
      <div className="text-xs text-neutral-500 dark:text-neutral-400 mb-1">{label}</div>
      <div className="text-base font-semibold text-neutral-900 dark:text-neutral-100 tabular-nums">
        {value}
      </div>
    </div>
  )
}
