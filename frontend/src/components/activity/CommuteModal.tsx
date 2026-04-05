import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { createCommuteRule } from '@/api/rules'
import { listPrivacyZones } from '@/api/privacy'
import { useToast } from '@/lib/toast'
import { ModalShell } from '@/components/ui/ModalShell'

interface CommuteModalProps {
  activityId: string
  isRoundTrip?: boolean
  onClose: () => void
}

export default function CommuteModal({ activityId, isRoundTrip, onClose }: CommuteModalProps) {
  const { addToast } = useToast()
  const [selectedZone, setSelectedZone] = useState('')

  const { data: zones } = useQuery({
    queryKey: ['privacy-zones'],
    queryFn: listPrivacyZones,
    enabled: !!isRoundTrip,
  })

  const createMut = useMutation({
    mutationFn: () =>
      isRoundTrip && selectedZone
        ? createCommuteRule(activityId, { zoneId: selectedZone })
        : createCommuteRule(activityId),
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
      {isRoundTrip ? (
        <>
          <h3 className="text-lg font-semibold text-neutral-900 dark:text-neutral-100">
            Pendel-Rundtour — Wo liegt dein Ziel?
          </h3>
          <p className="text-sm text-neutral-600 dark:text-neutral-400 mt-3">
            Diese Aktivität startet und endet am selben Ort. Wähle dein Ziel (z.B. Arbeitsplatz), damit Rundtouren mit einem Aufenthalt dort als Pendeln erkannt werden:
          </p>
          {zones && zones.length > 0 ? (
            <select
              value={selectedZone}
              onChange={e => setSelectedZone(e.target.value)}
              className="mt-3 w-full px-3 py-2 text-sm rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 outline-none focus:border-accent-400"
            >
              <option value="">Zone wählen...</option>
              {zones.map(z => (
                <option key={z.id} value={z.id}>{z.name}</option>
              ))}
            </select>
          ) : (
            <>
              <p className="mt-3 text-sm text-neutral-500 dark:text-neutral-400">
                Erstelle zuerst Zonen unter{' '}
                <a href="/settings/privacy" className="underline hover:text-neutral-700 dark:hover:text-neutral-300">
                  Einstellungen → Privatsphäre
                </a>
              </p>
              <button
                onClick={onClose}
                className="w-full mt-4 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
              >
                Abbrechen
              </button>
            </>
          )}
          {zones && zones.length > 0 && (
            <div className="flex gap-3 mt-4">
              <button
                onClick={() => createMut.mutate()}
                disabled={!selectedZone || createMut.isPending}
                className="flex-1 px-4 py-2 text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 rounded-lg cursor-pointer transition-colors disabled:opacity-50"
              >
                {createMut.isPending ? 'Wird erstellt...' : 'Pendel-Regel erstellen'}
              </button>
              <button
                onClick={onClose}
                className="flex-1 px-4 py-2 text-sm font-medium text-neutral-700 dark:text-neutral-300 bg-neutral-100 dark:bg-neutral-800 hover:bg-neutral-200 dark:hover:bg-neutral-700 rounded-lg cursor-pointer transition-colors"
              >
                Abbrechen
              </button>
            </div>
          )}
        </>
      ) : (
        <>
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
        </>
      )}
    </ModalShell>
  )
}
