import { NavLink, Outlet } from 'react-router-dom'
import { cn } from '@/lib/utils'
import { useTheme } from '@/lib/theme'
import ImportHub from '@/components/ImportHub'
import ActiveJobsIndicator from '@/components/ActiveJobsIndicator'
import NotificationCenter from '@/components/NotificationCenter'

const navItems = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/activities', label: 'Aktivitäten' },
  { to: '/map', label: 'Karte' },
  { to: '/calendar', label: 'Kalender' },
  { to: '/analytics', label: 'Analyse' },
  { to: '/chat', label: 'Assistent' },
  { to: '/routes', label: 'Routen' },
  { to: '/segments', label: 'Segmente' },
  { to: '/workshop', label: 'Werkstatt' },
  { to: '/settings', label: 'Einstellungen' },
]

export default function Layout() {
  const { theme, toggle } = useTheme()

  return (
    <div className="min-h-screen bg-white dark:bg-neutral-950 text-neutral-900 dark:text-neutral-100 flex flex-col">
      <header className="border-b border-neutral-200 dark:border-neutral-800 bg-white/90 dark:bg-neutral-900/80 backdrop-blur-sm sticky top-0 z-50">
        <div className="max-w-7xl mx-auto px-4 h-14 flex items-center gap-6">
          <span className="font-bold text-lg text-accent-DEFAULT tracking-tight">skadia</span>
          <nav className="flex gap-1 flex-1">
            {navItems.map(({ to, label }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  cn(
                    'px-3 py-1.5 rounded-md text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-neutral-100 dark:bg-neutral-700 text-neutral-900 dark:text-neutral-100'
                      : 'text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-800',
                  )
                }
              >
                {label}
              </NavLink>
            ))}
          </nav>

          <ImportHub />

          <ActiveJobsIndicator />

          <NotificationCenter />

          <button
            onClick={toggle}
            aria-label="Farbschema umschalten"
            className="p-2 rounded-lg text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
          >
            {theme === 'light' ? <MoonIcon /> : <SunIcon />}
          </button>
        </div>
      </header>

      <main className="flex-1 max-w-7xl mx-auto w-full px-4 py-6">
        <Outlet />
      </main>
    </div>
  )
}

function SunIcon() {
  return (
    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41" />
    </svg>
  )
}

function MoonIcon() {
  return (
    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9z" />
    </svg>
  )
}
