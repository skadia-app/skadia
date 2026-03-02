import PMCChart from '@/components/PMCChart'

export default function DashboardPage() {
  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Dashboard</h1>
      <PMCChart />
    </div>
  )
}
