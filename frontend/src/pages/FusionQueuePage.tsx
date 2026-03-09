import { useQuery } from '@tanstack/react-query'
import { getFusionQueue, type FusionQueueEntry } from '@/api/importWizard'
import DuplicateReviewPanel from '@/components/DuplicateReviewPanel'

export default function FusionQueuePage() {
  const { data: entries = [], isLoading, refetch } = useQuery<FusionQueueEntry[]>({
    queryKey: ['fusion-queue'],
    queryFn: () => getFusionQueue(50),
  })

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
