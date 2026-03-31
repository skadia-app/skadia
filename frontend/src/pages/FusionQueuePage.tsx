import { useQuery } from '@tanstack/react-query'
import { fetchRecordingGroups } from '@/api/recordingGroups'
import { type RecordingGroup } from '@/api/types'
import DuplicateReviewPanel from '@/components/DuplicateReviewPanel'

export default function FusionQueuePage() {
  const { data, isLoading, refetch } = useQuery<RecordingGroup[]>({
    queryKey: ['recording-groups'],
    queryFn: () => fetchRecordingGroups(50),
  })
  const groups = data ?? []

  return (
    <div className="max-w-5xl mx-auto space-y-6 py-8 px-6">
      <div>
        <h1 className="text-2xl font-bold text-neutral-900 dark:text-neutral-100">
          Fusionskandidaten
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Aktivitäten die möglicherweise mit mehreren Geräten aufgezeichnet wurden.
        </p>
        {!isLoading && groups.length > 0 && (
          <p className="mt-0.5 text-sm text-neutral-400 dark:text-neutral-500">
            {groups.length === 1
              ? '1 Aufnahmegruppe erkannt.'
              : `${groups.length} Aufnahmegruppen erkannt.`}{' '}
            Wähle für jede Gruppe ob die Aktivitäten zusammengehören.
          </p>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12 text-neutral-400 text-sm">Lade…</div>
      ) : (
        <DuplicateReviewPanel
          groups={groups}
          onDone={() => refetch()}
        />
      )}
    </div>
  )
}
