import LifetimeStats from '@/components/analytics/stats/LifetimeStats'
import EddingtonChart from '@/components/analytics/stats/EddingtonChart'
import DistributionCharts from '@/components/analytics/stats/DistributionCharts'
import YearlyOverview from '@/components/analytics/stats/YearlyOverview'
import StreaksDisplay from '@/components/analytics/stats/StreaksDisplay'

export default function StatsTab() {
  return (
    <div className="space-y-8">
      {/* Lifetime Stats — full width, highlighted */}
      <section className="rounded-2xl border border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50 p-5">
        <h2 className="text-base font-bold text-neutral-900 dark:text-neutral-100 mb-4">
          Lifetime-Statistiken
        </h2>
        <LifetimeStats />
      </section>

      {/* Eddington + Streaks — 2-column grid */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <section className="rounded-2xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
          <EddingtonChart />
        </section>
        <section className="rounded-2xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
          <StreaksDisplay />
        </section>
      </div>

      {/* Distribution Charts — full width */}
      <section className="rounded-2xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-5">
        <DistributionCharts />
      </section>

      {/* Yearly Overview — full width */}
      <section>
        <h2 className="text-base font-bold text-neutral-900 dark:text-neutral-100 mb-4">
          Jahres-Ubersicht
        </h2>
        <YearlyOverview />
      </section>
    </div>
  )
}
