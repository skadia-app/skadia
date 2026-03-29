import { useQuery } from '@tanstack/react-query'
import { getFusionQueue, type FusionQueueResponse } from '@/api/importWizard'
import DuplicateReviewPanel from '@/components/DuplicateReviewPanel'

export default function FusionQueuePage() {
  const { data, isLoading, refetch } = useQuery<FusionQueueResponse>({
    queryKey: ['fusion-queue'],
    queryFn: () => getFusionQueue(50),
  })
  const entries = data?.items ?? []
  const computing = data?.computing ?? false

  return (
    <div className="max-w-3xl mx-auto space-y-6 py-6 px-4">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Fusionskandidaten
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Aktivitäten die möglicherweise mit zwei Geräten aufgezeichnet wurden und zusammengeführt werden können.
        </p>
      </div>

      {computing && (
        <div className="flex items-center gap-2 rounded-lg bg-accent-50 dark:bg-accent-900/20 border border-accent-200 dark:border-accent-800 px-4 py-2 text-sm text-accent-700 dark:text-accent-300">
          <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          Fusionskandidaten werden berechnet…
        </div>
      )}

      {isLoading ? (
        <div className="flex items-center justify-center py-12 text-neutral-400 text-sm">Lade…</div>
      ) : (
        <DuplicateReviewPanel
          entries={entries}
          onDone={() => refetch()}
          doneLabel="Liste aktualisieren"
        />
      )}
    </div>
  )
}
