import { useInfiniteQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { fetchWorkouts } from '@/api/workouts'

const PAGE_SIZE = 20

export default function WorkoutsPage() {
  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } = useInfiniteQuery({
    queryKey: ['workouts'],
    queryFn: ({ pageParam = 0 }) => fetchWorkouts(PAGE_SIZE, pageParam as number),
    getNextPageParam: (lastPage, allPages) =>
      lastPage.length === PAGE_SIZE ? allPages.flat().length : undefined,
    initialPageParam: 0,
  })

  const workouts = data?.pages.flat() ?? []

  if (isLoading) {
    return <div className="p-6 text-neutral-500">Lade Workouts...</div>
  }

  return (
    <div className="max-w-4xl mx-auto p-6">
      <h1 className="text-2xl font-bold mb-6">Workouts</h1>
      {workouts.length === 0 ? (
        <p className="text-neutral-500">
          Keine Workouts importiert. Workouts werden automatisch aus FIT-Dateien extrahiert.
        </p>
      ) : (
        <div className="space-y-2">
          {workouts.map(w => (
            <Link
              key={w.id}
              to={`/workouts/${w.id}`}
              className="flex items-center gap-3 p-3 rounded-lg border border-neutral-200 dark:border-neutral-800 hover:bg-neutral-50 dark:hover:bg-neutral-900 transition-colors"
            >
              <div className="flex-1 min-w-0">
                <div className="font-medium truncate">
                  {w.name || 'Unbenanntes Workout'}
                </div>
                {w.description && (
                  <div className="text-sm text-neutral-500 truncate">{w.description}</div>
                )}
              </div>
              <div className="flex items-center gap-3 text-sm text-neutral-400 shrink-0">
                {(w.activity_count ?? 0) > 0 && (
                  <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-medium bg-blue-50 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400">
                    {w.activity_count}× absolviert
                  </span>
                )}
                <span>{w.num_valid_steps ?? 0} Schritte</span>
              </div>
            </Link>
          ))}
          {hasNextPage && (
            <button
              onClick={() => fetchNextPage()}
              disabled={isFetchingNextPage}
              className="w-full py-2 text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 disabled:opacity-50"
            >
              {isFetchingNextPage ? 'Lade...' : 'Weitere laden'}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
