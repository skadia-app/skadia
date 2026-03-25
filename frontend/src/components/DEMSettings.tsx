import { useQuery } from '@tanstack/react-query'
import { apiClient } from '@/api/client'

interface DEMSource {
  source: string
  resolution_m: number
  tile_count: number
}

interface DEMStatus {
  sources: DEMSource[]
  total_tiles: number
}

export function DEMSettings() {
  const { data } = useQuery({
    queryKey: ['dem-status'],
    queryFn: () => apiClient<DEMStatus>('/admin/dem-status'),
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
