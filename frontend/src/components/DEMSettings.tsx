import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getDEMRegionStatus, downloadDEM, type DEMStatusResponse } from '@/api/osm'
import { useToast } from '@/lib/toast'

export function DEMSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()

  const { data } = useQuery<DEMStatusResponse>({
    queryKey: ['dem-region-status'],
    queryFn: getDEMRegionStatus,
  })

  const demDownloadMut = useMutation({
    mutationFn: () => downloadDEM(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['dem-region-status'] })
      addToast({ type: 'info', title: 'DEM-Download gestartet' })
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'DEM-Download fehlgeschlagen', message: err.message, duration: 0 })
    },
  })

  const sources = data?.sources ?? []

  return (
    <div className="space-y-3">
      {sources.length === 0 ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">Keine DEM-Höhendaten geladen.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b text-neutral-500 dark:text-neutral-400 text-xs">
                <th className="text-left pb-2 pr-4">Quelle</th>
                <th className="text-left pb-2 pr-4">Auflösung</th>
                <th className="text-left pb-2">Kacheln</th>
              </tr>
            </thead>
            <tbody>
              {sources.map(s => (
                <tr key={s.source} className="border-b last:border-0">
                  <td className="py-2 pr-4 font-medium">{s.source}</td>
                  <td className="py-2 pr-4 text-neutral-500 dark:text-neutral-400">{s.resolution_m}m</td>
                  <td className="py-2 text-neutral-500 dark:text-neutral-400">{s.tile_count.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-2">
            Gesamt: {data?.total_tiles.toLocaleString()} Kacheln
          </p>
        </div>
      )}

      {data?.regions && data.regions.length > 0 && (
        <div className="mt-3 space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm text-neutral-600 dark:text-neutral-400">
              {data.regions.reduce((s, r) => s + r.tiles_present, 0)}/{data.regions.reduce((s, r) => s + r.tiles_needed, 0)} Kacheln vorhanden
              {data.missing_tiles > 0 && (
                <span className="text-amber-500 ml-1">
                  ({data.regions.filter(r => !r.complete).map(r => r.id).join(', ')} fehlen)
                </span>
              )}
            </span>
            <button
              onClick={() => demDownloadMut.mutate()}
              disabled={!data.missing_tiles || demDownloadMut.isPending}
              className="cursor-pointer px-3 py-1.5 rounded-lg text-xs font-medium bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 text-white disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Fehlende DEM-Kacheln herunterladen
            </button>
          </div>
        </div>
      )}

      <details className="text-xs">
        <summary className="cursor-pointer text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100">
          Datenquellen &amp; Import-Anleitung
        </summary>
        <div className="mt-2 space-y-3 pl-2 border-l-2 border-neutral-200 dark:border-neutral-800">
          <div>
            <p className="font-medium">Copernicus EEA-10 (10m — empfohlen für Alpen)</p>
            <p className="text-neutral-500 dark:text-neutral-400">Zugang beantragen: dataspace.copernicus.eu</p>
            <code className="block bg-neutral-100 dark:bg-neutral-800 px-2 py-1 rounded mt-1">
              ./scripts/dem-import.sh eea10 ./data/dem/eea10/ 10
            </code>
          </div>
          <div>
            <p className="font-medium">LiDAR Österreich (0.5-1m)</p>
            <p className="text-neutral-500 dark:text-neutral-400">data.bev.gv.at (kostenlos)</p>
            <code className="block bg-neutral-100 dark:bg-neutral-800 px-2 py-1 rounded mt-1">
              ./scripts/dem-import.sh lidar_at ./data/dem/lidar_at/ 1
            </code>
          </div>
          <div>
            <p className="font-medium">Sonny's 4lima LiDAR .hgt (DACH ~1.5 GB, BRouter-kompatibel)</p>
            <p className="text-neutral-500 dark:text-neutral-400">
              <a href="https://sonny.4lima.de" target="_blank" rel="noopener noreferrer" className="text-accent-DEFAULT hover:underline">
                sonny.4lima.de
              </a> — direkt als srtm-dir für BRouter-Segmentbuild nutzbar
            </p>
          </div>
          <div>
            <p className="font-medium">Copernicus GLO-30 (30m — Fallback)</p>
            <p className="text-neutral-500 dark:text-neutral-400">Sofort verfügbar (AWS S3), zu grob für Alpenregionen</p>
            <code className="block bg-neutral-100 dark:bg-neutral-800 px-2 py-1 rounded mt-1">
              ./scripts/dem-import.sh copernicus30 ./data/dem/copernicus30/ 30
            </code>
          </div>
          <p className="text-neutral-500 dark:text-neutral-400 mt-2">
            DEM verbessert Höhenprofile, Routing-Kosten und Stage-1-Enrichment.
          </p>
        </div>
      </details>
    </div>
  )
}
