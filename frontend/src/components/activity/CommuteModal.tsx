import { useMutation } from '@tanstack/react-query'
import { createCommuteRule } from '@/api/rules'
import { useToast } from '@/lib/toast'
import { ModalShell } from '@/components/ui/ModalShell'

interface CommuteModalProps {
  activityId: string
  onClose: () => void
}

export default function CommuteModal({ activityId, onClose }: CommuteModalProps) {
  const { addToast } = useToast()

  const createMut = useMutation({
    mutationFn: () => createCommuteRule(activityId),
    onSuccess: (result) => {
      addToast({
        type: 'success',
        title: result.created ? 'Pendel-Regel erstellt' : 'Pendel-Route hinzugefügt',
      })
      onClose()
    },
    onError: (error: unknown) => {
      const apiErr = error as { status?: number } | null
      if (apiErr?.status === 409) {
        addToast({ type: 'info', title: 'Strecke bereits in Pendel-Regel vorhanden' })
        onClose()
        return
      }
      addToast({ type: 'error', title: 'Fehler beim Erstellen der Regel', duration: 0 })
      onClose()
    },
  })

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-md">
      <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
        Pendel-Route erstellen?
      </h3>
      <p className="text-sm text-neutral-600 dark:text-neutral-400 mt-3">
        Sollen Aktivitäten mit diesem Start- und Endpunkt zukünftig automatisch als Pendeln getaggt werden?
      </p>
      <div className="flex gap-3 mt-4">
        <button
          onClick={() => createMut.mutate()}
          disabled={createMut.isPending}
          className="flex-1 px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 rounded-lg cursor-pointer transition-colors disabled:opacity-50"
        >
          {createMut.isPending ? 'Wird erstellt...' : 'Ja, automatisch taggen'}
        </button>
        <button
          onClick={onClose}
          className="flex-1 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
        >
          Nein, nur dieses Mal
        </button>
      </div>
      <p className="text-xs text-neutral-400 dark:text-neutral-500 mt-3">
        Du kannst unter{' '}
        <a href="/workshop/rules" className="underline hover:text-neutral-600 dark:hover:text-neutral-300">
          Regeln
        </a>{' '}
        bei der Regel &quot;Pendeln&quot; den Backfill-Button drücken, um bestehende Aktivitäten nachträglich zu taggen.
      </p>
    </ModalShell>
  )
}
