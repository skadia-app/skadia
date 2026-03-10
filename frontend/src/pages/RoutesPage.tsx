import { useQuery } from '@tanstack/react-query'
import { listRoutes } from '@/api/routes'
import { Link } from 'react-router-dom'

export default function RoutesPage() {
  const { data: routes = [], isLoading } = useQuery({
    queryKey: ['routes'],
    queryFn: () => listRoutes(),
  })

  return (
    <div className="max-w-4xl mx-auto px-4 py-8 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Gespeicherte Routen
        </h1>
        <Link
          to="/map"
          className="px-4 py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors"
        >
          Neue Route planen
        </Link>
      </div>

      {isLoading ? (
        <div className="text-sm text-neutral-500 dark:text-neutral-400">Lade Routen…</div>
      ) : routes.length === 0 ? (
        <div className="text-center py-16 text-neutral-500 dark:text-neutral-400 space-y-2">
          <p className="text-lg font-medium">Noch keine Routen gespeichert</p>
          <p className="text-sm">
            Öffne den{' '}
            <Link to="/map" className="text-accent-DEFAULT hover:underline">
              Routenplaner auf der Karte
            </Link>{' '}
            und speichere deine erste Route.
          </p>
        </div>
      ) : (
        <ul className="divide-y divide-neutral-200 dark:divide-neutral-800">
          {routes.map(route => (
            <li key={route.id} className="py-4 flex items-center justify-between gap-4">
              <div className="flex-1 min-w-0">
                <p className="font-medium text-neutral-900 dark:text-neutral-100 truncate">
                  {route.name || 'Unbenannte Route'}
                </p>
                <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
                  {route.profile} · {route.waypoints?.length ?? 0} Wegpunkte
                </p>
              </div>
              <Link
                to={`/map?route=${route.id}`}
                className="shrink-0 px-3 py-1.5 text-xs font-medium rounded-lg border border-neutral-300 dark:border-neutral-700 hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
              >
                Öffnen
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
