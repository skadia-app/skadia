// Admin sub-tab bundling all map/tiles admin sections.
// Renders in order:
//   1. MapLayersAdminSection        — system layer CRUD
//   2. SystemLayerVisibilityAdminSection — 14 overlay key toggles
//   3. DEMSettings                  — DEM height data
//   4. OfflineMapSettings           — Protomaps / Martin
//   5. OSMRegionManager             — OSM data management
import { MapLayersAdminSection } from '@/components/admin/MapLayersAdminSection'
import { SystemLayerVisibilityAdminSection } from '@/components/admin/SystemLayerVisibilityAdminSection'
import { DEMSettings } from '@/components/DEMSettings'
import { OfflineMapSettings } from '@/components/OfflineMapSettings'
import { OSMRegionManager } from '@/components/OSMRegionManager'
import { useQuery, useMutation } from '@tanstack/react-query'
import { fetchOSMStatus } from '@/api/osm'
import { importLifts } from '@/api/osm'

export default function AdminMapTilesSubTab() {
  // OSM data presence drives whether lift-import button is available
  const { data: osmStatus } = useQuery({
    queryKey: ['osm-status'],
    queryFn: fetchOSMStatus,
  })
  const hasOsmData = osmStatus?.has_data ?? false

  const importLiftsMut = useMutation({ mutationFn: importLifts })

  return (
    <div className="space-y-6">
      {/* 1. System map layer CRUD */}
      <MapLayersAdminSection />

      {/* 2. System overlay visibility */}
      <SystemLayerVisibilityAdminSection />

      {/* 3. DEM height data */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            DEM-Höhendaten
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Digitales Höhenmodell für Höhenkorrektur und Routing.
          </p>
        </div>
        <DEMSettings />
      </section>

      {/* 4. Offline map settings */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            Offline-Karte (Protomaps / Martin)
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Lokaler Tile-Server und PMTiles-Dateien.
          </p>
        </div>
        <OfflineMapSettings />
      </section>

      {/* 5. OSM region manager */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            OSM-Geodaten
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Regionen importieren und Lift-Segmente aktualisieren.
          </p>
        </div>
        <OSMRegionManager hasOsmData={hasOsmData} />
        <div className="flex gap-2 pt-2">
          <button
            type="button"
            onClick={() => importLiftsMut.mutate()}
            disabled={importLiftsMut.isPending || !hasOsmData}
            className="px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
          >
            {importLiftsMut.isPending ? 'Importiere…' : 'Lift-Segmente importieren'}
          </button>
        </div>
        {importLiftsMut.isSuccess && (
          <p className="text-xs text-emerald-600 dark:text-emerald-400">
            {importLiftsMut.data.imported} Lift-Segmente importiert.
          </p>
        )}
      </section>
    </div>
  )
}
