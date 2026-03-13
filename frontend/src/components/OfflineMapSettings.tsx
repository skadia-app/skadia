import { useQuery } from '@tanstack/react-query'
import { getCapabilities, type Capabilities } from '@/api/routes'

export function OfflineMapSettings() {
  const { data: caps, isLoading } = useQuery<Capabilities>({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 60_000,
  })

  if (isLoading) {
    return (
      <div className="text-xs text-neutral-400 dark:text-neutral-500">Prüfe Verfügbarkeit…</div>
    )
  }

  return (
    <div className="space-y-3">
      {/* Status indicator */}
      <div className="flex items-center gap-2">
        <div
          className={`w-2 h-2 rounded-full flex-shrink-0 ${
            caps?.offline_map ? 'bg-green-500' : 'bg-neutral-400'
          }`}
        />
        <span className="text-sm text-neutral-700 dark:text-neutral-300">
          {caps?.offline_map
            ? 'Offline-Basemap verfügbar'
            : 'Keine PMTiles-Basemap vorhanden'}
        </span>
      </div>

      {/* Instructions when unavailable */}
      {!caps?.offline_map && (
        <div className="text-xs text-neutral-500 dark:text-neutral-400 space-y-2 pl-4">
          <p>Für Offline-Karten PMTiles herunterladen:</p>
          <code className="block bg-neutral-100 dark:bg-neutral-800 px-2 py-1.5 rounded font-mono text-xs">
            ./scripts/download-pmtiles.sh
          </code>
          <p>
            DACH-Region (~2–4 GB). Benötigt:{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono">
              pmtiles
            </code>{' '}
            CLI
          </p>
          <p>
            <a
              href="https://protomaps.com"
              target="_blank"
              rel="noopener noreferrer"
              className="text-accent-DEFAULT hover:underline underline-offset-2"
            >
              Protomaps Projekt →
            </a>
          </p>
        </div>
      )}

      {/* Info when available */}
      {caps?.offline_map && (
        <p className="text-xs text-neutral-500 dark:text-neutral-400 pl-4">
          Der Kartenstil <strong className="text-neutral-700 dark:text-neutral-300">Offline (Protomaps)</strong>{' '}
          ist in der Kartenansicht auswählbar.
        </p>
      )}
    </div>
  )
}
