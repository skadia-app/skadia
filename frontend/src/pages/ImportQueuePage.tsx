import ImportQueueTab from '@/components/ImportQueueTab'

export default function ImportQueuePage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">
          Zu prüfen
        </h1>
        <p className="mt-1 text-sm text-neutral-500 dark:text-neutral-400">
          Importierte Dateien mit manueller Nacharbeit prüfen, zuordnen oder verwerfen.
        </p>
      </div>
      <ImportQueueTab />
    </div>
  )
}
