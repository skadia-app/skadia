import { NavLink, Outlet, Navigate, useLocation } from 'react-router-dom'

const tabs = [
  { to: '/analytics/training', label: 'Training' },
  { to: '/analytics/health', label: 'Gesundheit' },
  { to: '/analytics/stats', label: 'Statistiken' },
  { to: '/analytics/insights', label: 'Insights' },
]

export default function AnalyticsPage() {
  const location = useLocation()

  if (location.pathname === '/analytics') {
    return <Navigate to="/analytics/training" replace />
  }

  return (
    <div className="space-y-4">
      {/* Tab Navigation */}
      <div className="flex gap-1 border-b border-neutral-200 dark:border-neutral-800">
        {tabs.map(tab => (
          <NavLink
            key={tab.to}
            to={tab.to}
            className={({ isActive }) =>
              `px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                isActive
                  ? 'border-[var(--color-accent-DEFAULT)] text-[var(--color-accent-DEFAULT)]'
                  : 'border-transparent text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300'
              }`
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </div>

      {/* Tab Content */}
      <Outlet />
    </div>
  )
}
