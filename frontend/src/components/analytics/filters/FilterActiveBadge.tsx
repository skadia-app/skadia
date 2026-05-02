import { useAnalyticsFilters } from '@/lib/analyticsFilters'
import { activeFilterCount } from '@/lib/analyticsFilters'

export default function FilterActiveBadge() {
  const { filters } = useAnalyticsFilters()
  const count = activeFilterCount(filters)

  if (count === 0) return null

  function handleClick() {
    document.getElementById('analytics-filter-panel')?.scrollIntoView({ behavior: 'smooth' })
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 text-blue-700 dark:text-blue-300 text-xs font-medium"
    >
      🔍 {count} Filter aktiv
    </button>
  )
}
