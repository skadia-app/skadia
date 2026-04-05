import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getDEMRegionStatus, downloadDEM, type DEMStatusResponse } from '@/api/osm'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'

export function DEMSettings() {
  const queryClient = useQueryClient()
  const { addToast } = useToast()
  const { jobs } = useActiveJobs()
  const activeDEMJob = jobs.find(j => j.job_type === 'dem_download' && j.status === 'running')

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

  const regions = data?.regions ?? []
  const hasImportedRegions = regions.length > 0
  const hgtPresent = regions.reduce((s, r) => s + r.tiles_present, 0)
  const allComplete = regions.every(r => r.complete)
  const incompleteRegions = regions.filter(r => !r.complete)

  return (
    <div className="space-y-3">
      {/* Per-region DEM coverage */}
      {hasImportedRegions ? (
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-sm text-neutral-700 dark:text-neutral-300">
              <span className="font-medium">{hgtPresent.toLocaleString()}</span> HGT-Kacheln
              <span className="text-neutral-400 ml-1">(GLO-30, 30m)</span>
            </p>
            <button
              onClick={() => demDownloadMut.mutate()}
              disabled={!!activeDEMJob || demDownloadMut.isPending}
              className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
            >
              {activeDEMJob ? 'Download läuft…' : allComplete ? 'DEM-Kacheln aktualisieren' : 'DEM-Kacheln herunterladen'}
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {regions.map(r => {
              // tiles_needed = -1 means no manifest yet (unknown completeness)
              const hasManifest = r.tiles_needed >= 0
              const statusColor = !hasManifest
                ? (r.tiles_present > 0
                    ? 'bg-neutral-500/10 text-neutral-600 dark:text-neutral-400'
                    : 'bg-red-500/10 text-red-600 dark:text-red-400')
                : r.complete
                  ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                  : 'bg-red-500/10 text-red-600 dark:text-red-400'
              const dotColor = !hasManifest
                ? (r.tiles_present > 0 ? 'bg-neutral-400' : 'bg-red-500')
                : r.complete ? 'bg-green-500' : 'bg-red-500'

              return (
                <span key={r.id} className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${statusColor}`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${dotColor}`} />
                  {r.label}
                  {hasManifest
                    ? <span className="opacity-60">({r.tiles_present}/{r.tiles_needed})</span>
                    : r.tiles_present > 0
                      ? <span className="opacity-60">({r.tiles_present})</span>
                      : null
                  }
                </span>
              )
            })}
          </div>
          {!allComplete && (
            <p className="text-xs text-neutral-500 dark:text-neutral-400">
              {regions.some(r => r.tiles_needed < 0)
                ? 'DEM-Status unbekannt — einmal herunterladen um Abdeckung zu ermitteln'
                : 'DEM-Daten unvollständig — für Routing-Tiles erforderlich'}
            </p>
          )}
        </div>
      ) : (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Keine importierten Regionen. DEM-Kacheln werden nach dem OSM-Import verfügbar.
        </p>
      )}

      {/* DB raster tiles — secondary info in collapsible */}
      {sources.length > 0 && (
        <details className="text-xs">
          <summary className="cursor-pointer text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100">
            Elevation-Datenquellen ({data?.total_tiles.toLocaleString()} DB-Raster-Kacheln)
          </summary>
          <div className="mt-2 overflow-x-auto">
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
          </div>
        </details>
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
