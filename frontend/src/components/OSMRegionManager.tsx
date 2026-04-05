import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2, Database, XCircle, ChevronDown, RefreshCw, AlertTriangle, Sparkles, Layers, Route, Grid3x3, ArrowDownToLine } from 'lucide-react'
import { useState } from 'react'
import {
  fetchOSMRegionStatus,
  fetchOSMDiskUsage,
  deleteOSMRegion,
  cancelOSMImport,
  retryOSMEnrichment,
  rebuildTiles,
  buildBRouterSegments,
  downloadBRouterSegments,
  fetchOSMRegions,
  buildCoverageGrids,
  triggerOSMRegionUpdate,
  batchUpdateAllRegions,
  type OSMImportRegion,
} from '@/api/osm'
import { useToast } from '@/lib/toast'
import { useActiveJobs } from '@/lib/useActiveJobs'
import { useModal } from '@/lib/modal'
import OSMImportModal from '@/components/OSMImportModal'
import RegionSelector from '@/components/RegionSelector'
import PipelinePanel from '@/components/PipelinePanel'

function formatBytes(bytes: number | null): string {
  if (!bytes) return '—'
  if (bytes > 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
  return `${(bytes / 1024 / 1024).toFixed(0)} MB`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('de-AT', {
    day: '2-digit',
    month: '2-digit',
    year: 'numeric',
  })
}

export function OSMRegionManager({ hasOsmData }: { hasOsmData?: boolean }) {
  const qc = useQueryClient()
  const { addToast } = useToast()
  const { openModal } = useModal()
  const [diskExpanded, setDiskExpanded] = useState(false)

  const { data: regionData, isFetched: regionsFetched } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
    refetchInterval: (query) =>
      query.state.data?.regions?.some((r) => r.status === 'importing') ? 5000 : false,
  })

  const { data: diskData } = useQuery({
    queryKey: ['osm-disk-usage'],
    queryFn: fetchOSMDiskUsage,
    refetchInterval: 30000,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteOSMRegion,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'success', title: 'Region entfernt' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Entfernen der Region', duration: 0 })
    },
  })

  const cancelMutation = useMutation({
    mutationFn: cancelOSMImport,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: 'Import abgebrochen' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Fehler beim Abbrechen des Imports', duration: 0 })
    },
  })

  const [enrichRegions, setEnrichRegions] = useState<string[]>([])
  const [tilesRegions, setTilesRegions] = useState<string[]>([])
  const [brouterLocalRegions, setBrouterLocalRegions] = useState<string[]>([])
  const [brouterDlRegions, setBrouterDlRegions] = useState<string[]>([])
  const [coverageRegions, setCoverageRegions] = useState<string[]>([])

  const { data: catalogData } = useQuery({
    queryKey: ['osm-regions'],
    queryFn: fetchOSMRegions,
  })

  const enrichMut = useMutation({
    mutationFn: (regions: string[]) => retryOSMEnrichment(regions),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info', title: 'Enrichment gestartet' }) },
    onError: () => { addToast({ type: 'error', title: 'Enrichment konnte nicht gestartet werden', duration: 0 }) },
  })
  const tilesMut = useMutation({
    mutationFn: (regions: string[]) => rebuildTiles({ regions, force: true }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info', title: 'Tile-Build gestartet' }) },
    onError: () => { addToast({ type: 'error', title: 'Tile-Build konnte nicht gestartet werden', duration: 0 }) },
  })
  const brouterLocalMut = useMutation({
    mutationFn: (regions: string[]) => buildBRouterSegments(regions),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info', title: 'BRouter-Build gestartet' }) },
    onError: () => { addToast({ type: 'error', title: 'BRouter-Build konnte nicht gestartet werden', duration: 0 }) },
  })
  const brouterDlMut = useMutation({
    mutationFn: (regions: string[]) => downloadBRouterSegments(regions),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info', title: 'BRouter-Download gestartet' }) },
    onError: () => { addToast({ type: 'error', title: 'BRouter-Download konnte nicht gestartet werden', duration: 0 }) },
  })
  const coverageMut = useMutation({
    mutationFn: (regions: string[]) => buildCoverageGrids(regions),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info', title: 'Coverage-Build gestartet' }) },
    onError: () => { addToast({ type: 'error', title: 'Coverage-Build konnte nicht gestartet werden', duration: 0 }) },
  })

  const updateMut = useMutation({
    mutationFn: (regionID: string) => triggerOSMRegionUpdate(regionID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: 'Inkrementelles Update gestartet' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Update konnte nicht gestartet werden', duration: 0 })
    },
  })

  const batchUpdateMut = useMutation({
    mutationFn: (rebuildTiles: boolean) => batchUpdateAllRegions(rebuildTiles),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: 'Batch-Update gestartet' })
    },
    onError: () => {
      addToast({ type: 'error', title: 'Batch-Update konnte nicht gestartet werden', duration: 0 })
    },
  })

  const { jobs } = useActiveJobs()
  const activeOsmJob = jobs.find((j) => j.job_type === 'osm_import' && j.status === 'running')
  const activeEnrichmentJob = jobs.find((j) => j.job_type === 'osm_enrichment' && j.status === 'running')
  const activeTileBuild = jobs.find(j => j.job_type === 'tile_build' && j.status === 'running')
  const activeBrouterBuild = jobs.find(j => j.job_type === 'brouter_segment_build' && j.status === 'running')
  const activeBrouterDl = jobs.find(j => j.job_type === 'brouter_segment_download' && j.status === 'running')
  const activeCoverageBuild = jobs.find(j => j.job_type === 'coverage_build' && j.status === 'running')
  const activeOsmUpdate = jobs.find(j => j.job_type === 'osm_update' && j.status === 'running')
  const activeBatchUpdate = jobs.find(j => j.job_type === 'osm_update_all' && j.status === 'running')

  const [updateTarget, setUpdateTarget] = useState<OSMImportRegion | null>(null)

  const regions = regionData?.regions ?? []
  const importedRegions = regions.filter(r => r.status === 'imported')

  return (
    <>
    {updateTarget && (
      <OSMImportModal
        onClose={() => setUpdateTarget(null)}
        initialRegionId={updateTarget.region_id}
        initialContinent={updateTarget.continent}
        initialCategories={updateTarget.categories}
      />
    )}
    <div className="space-y-4">
      {activeOsmJob && (
        <div className="flex items-center justify-between rounded-lg border border-yellow-200 dark:border-yellow-800 bg-yellow-50 dark:bg-yellow-900/20 px-3 py-2">
          <span className="text-xs text-yellow-700 dark:text-yellow-400">
            OSM-Import läuft…
          </span>
          <button
            onClick={() => cancelMutation.mutate(activeOsmJob.id)}
            disabled={cancelMutation.isPending}
            className="cursor-pointer flex items-center gap-1 text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <XCircle className="w-3.5 h-3.5" />
            Abbrechen
          </button>
        </div>
      )}

      {hasOsmData && regionsFetched && regions.length === 0 && !activeOsmJob && (
        <div className="flex items-start gap-2 rounded-lg border border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-900/20 px-3 py-2">
          <AlertTriangle className="w-4 h-4 text-yellow-600 dark:text-yellow-400 mt-0.5 shrink-0" />
          <p className="text-xs text-yellow-700 dark:text-yellow-300">
            OSM-Daten vorhanden, aber kein vollständiger Import verzeichnet — die Daten könnten unvollständig sein. Import neu starten um sauberen Stand zu erhalten.
          </p>
        </div>
      )}

      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          Importierte Regionen
        </h3>
        {diskData && (
          <button
            onClick={() => setDiskExpanded(!diskExpanded)}
            className="cursor-pointer flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors"
          >
            <Database className="w-3 h-3" />
            {formatBytes(diskData.total_bytes)} OSM-Daten
            <ChevronDown
              className={`w-3 h-3 transition-transform ${diskExpanded ? 'rotate-180' : ''}`}
            />
          </button>
        )}
      </div>

      {diskExpanded && diskData && (
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-3 text-xs space-y-1 bg-neutral-50 dark:bg-neutral-800/30">
          {diskData.tables.map((t) => (
            <div key={t.tablename} className="flex justify-between">
              <span className="text-neutral-500 dark:text-neutral-400 font-mono">{t.tablename}</span>
              <span className="text-neutral-700 dark:text-neutral-300">{formatBytes(t.total_bytes)}</span>
            </div>
          ))}
          <div className="flex justify-between border-t border-neutral-200 dark:border-neutral-700 pt-1 mt-1 font-medium">
            <span className="text-neutral-600 dark:text-neutral-300">Gesamt</span>
            <span className="text-neutral-800 dark:text-neutral-200">{formatBytes(diskData.total_bytes)}</span>
          </div>
        </div>
      )}

      {regions.length === 0 ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          Noch keine OSM-Regionen importiert.
        </p>
      ) : (
        <div className="overflow-x-auto">
          {importedRegions.length > 0 && importedRegions.some(r => r.replication_url) && (
            <div className="flex justify-end mb-3">
              <button
                onClick={() => {
                  const rebuild = window.confirm('Sollen nach dem Update auch Routing-Tiles neu gebaut werden?')
                  batchUpdateMut.mutate(rebuild)
                }}
                disabled={!!activeOsmJob || !!activeOsmUpdate || !!activeBatchUpdate || batchUpdateMut.isPending}
                className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90"
              >
                {activeBatchUpdate ? 'Batch-Update läuft…' : 'Alle Regionen aktualisieren'}
              </button>
            </div>
          )}
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 text-xs">
                <th className="text-left pb-2 pr-3 font-medium">Region</th>
                <th className="text-left pb-2 pr-3 font-medium">Kontinent</th>
                <th className="text-left pb-2 pr-3 font-medium">Importiert</th>
                <th className="text-left pb-2 pr-3 font-medium">Daten vom</th>
                <th className="text-left pb-2 pr-3 font-medium">Kategorien</th>
                <th className="text-left pb-2 pr-3 font-medium">Größe</th>
                <th className="text-left pb-2 pr-3 font-medium">Status</th>
                <th className="text-right pb-2 font-medium">Aktionen</th>
              </tr>
            </thead>
            <tbody>
              {regions.map((reg) => (
                <tr
                  key={reg.region_id}
                  className="border-b border-neutral-100 dark:border-neutral-800 last:border-0"
                >
                  <td className="py-2 pr-3 font-medium text-neutral-900 dark:text-neutral-100">
                    {reg.label}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {reg.continent}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {formatDate(reg.imported_at)}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {reg.data_date ? formatDate(reg.data_date) : '—'}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400 text-xs">
                    {reg.categories.join(', ')}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {formatBytes(reg.pbf_size_bytes)}
                  </td>
                  <td className="py-2 pr-3">
                    <span
                      className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                        reg.status === 'imported'
                          ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                          : reg.status === 'importing'
                            ? 'bg-yellow-500/10 text-yellow-600 dark:text-yellow-400'
                            : reg.status === 'needs_redownload'
                              ? 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
                              : 'bg-red-500/10 text-red-600 dark:text-red-400'
                      }`}
                    >
                      {reg.status === 'imported'
                        ? 'Importiert'
                        : reg.status === 'importing'
                          ? 'Wird importiert…'
                          : reg.status === 'needs_redownload'
                            ? 'Neu herunterladen'
                            : 'Fehler'}
                    </span>
                  </td>
                  <td className="py-2 text-right">
                    <div className="flex justify-end gap-1">
                      {reg.status === 'imported' && reg.replication_url && (
                        <button
                          onClick={() => updateMut.mutate(reg.region_id)}
                          disabled={!!activeOsmJob || !!activeOsmUpdate || updateMut.isPending}
                          className="cursor-pointer p-1 text-neutral-400 hover:text-blue-500 dark:hover:text-blue-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                          title="Inkrementelles Update (Diffs anwenden)"
                        >
                          <ArrowDownToLine className="w-4 h-4" />
                        </button>
                      )}
                      <button
                        onClick={() => setUpdateTarget(reg)}
                        disabled={!!activeOsmJob}
                        className="cursor-pointer p-1 text-neutral-400 hover:text-accent-DEFAULT dark:hover:text-accent-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                        title="Region neu importieren"
                      >
                        <RefreshCw className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => {
                          openModal({
                            title: 'Region entfernen?',
                            message: `"${reg.label}" und alle zugehörigen OSM-Daten werden unwiderruflich gelöscht.`,
                            buttons: [
                              {
                                label: 'Abbrechen',
                                variant: 'default',
                                onClick: () => {},
                              },
                              {
                                label: 'Entfernen',
                                variant: 'destructive',
                                onClick: () => deleteMutation.mutate(reg.region_id),
                              },
                            ],
                          })
                        }}
                        disabled={deleteMutation.isPending}
                        className="cursor-pointer p-1 text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                        title="Aus Tracking entfernen"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {regions.length > 0 && (
        <div className="space-y-3 pt-3">
          <h3 className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide">
            Pipeline-Aktionen
          </h3>

          <PipelinePanel
            title="Route-Membership"
            description="Berechnet Rad- und Wanderrouten-Zugehörigkeit. Coverage-Werte (Wald, Lärm) werden beim Tile-Build automatisch berechnet."
            icon={Sparkles}
            buttonLabel="Berechnen"
            onStart={() => enrichMut.mutate(enrichRegions)}
            isPending={enrichMut.isPending || !!activeEnrichmentJob}
            disabled={importedRegions.length === 0}
            disabledHint="Keine importierten Regionen vorhanden."
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={enrichRegions}
              onChange={setEnrichRegions}
              hint="Nur importierte Regionen werden angezeigt."
            />
          </PipelinePanel>

          <PipelinePanel
            title="Routing-Tiles"
            description="BadRouter Graph-Tiles mit DEM-Elevation neu bauen"
            icon={Layers}
            buttonLabel="Tiles bauen"
            onStart={() => tilesMut.mutate(tilesRegions)}
            isPending={tilesMut.isPending || !!activeTileBuild}
            disabled={importedRegions.length === 0}
            disabledHint="Keine importierten Regionen vorhanden."
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={tilesRegions}
              onChange={setTilesRegions}
              hint="Nur importierte Regionen werden angezeigt."
            />
          </PipelinePanel>

          <PipelinePanel
            title="Coverage-Grids"
            description="Wald-, Lärm-, Verkehrs- und weitere Coverage-Grids neu bauen (für Karten-Overlays und Graph-Build)"
            icon={Grid3x3}
            buttonLabel="Bauen"
            onStart={() => coverageMut.mutate(coverageRegions)}
            isPending={coverageMut.isPending || !!activeCoverageBuild}
            disabled={importedRegions.length === 0}
            disabledHint="Keine importierten Regionen vorhanden."
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={coverageRegions}
              onChange={setCoverageRegions}
              hint="Nur importierte Regionen werden angezeigt."
            />
          </PipelinePanel>

          {/* BRouter — combined panel with two sections */}
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4">
            <div className="flex items-start gap-2 mb-3">
              <Route className="w-4 h-4 mt-0.5 text-neutral-500 dark:text-neutral-400 shrink-0" />
              <div>
                <h4 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">BRouter-Segmente</h4>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">rd5-Routing-Daten lokal generieren oder von brouter.de herunterladen</p>
              </div>
            </div>

            {/* Lokal generieren */}
            <div className="mb-3 pl-6">
              <div className="flex items-start justify-between mb-2">
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mt-0.5">Lokal generieren (aus PBF-Dateien)</span>
                <button
                  onClick={() => brouterLocalMut.mutate(brouterLocalRegions)}
                  disabled={importedRegions.length === 0 || brouterLocalMut.isPending || !!activeBrouterBuild}
                  className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
                >
                  {brouterLocalMut.isPending || activeBrouterBuild ? 'Läuft…' : 'Generieren'}
                </button>
              </div>
              <RegionSelector
                regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
                selected={brouterLocalRegions}
                onChange={setBrouterLocalRegions}
                hint="Nur Regionen mit vorhandener PBF-Datei."
              />
            </div>

            {/* Download von brouter.de */}
            <div className="pl-6 pt-3 border-t border-neutral-100 dark:border-neutral-800/60">
              <div className="flex items-start justify-between mb-2">
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mt-0.5">Download von brouter.de</span>
                <button
                  onClick={() => brouterDlMut.mutate(brouterDlRegions)}
                  disabled={brouterDlMut.isPending || !!activeBrouterDl}
                  className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
                >
                  {brouterDlMut.isPending || activeBrouterDl ? 'Läuft…' : 'Downloaden'}
                </button>
              </div>
              <RegionSelector
                regions={catalogData?.regions?.map(r => ({ id: r.id, label: r.label, continent: r.continent })) ?? []}
                selected={brouterDlRegions}
                onChange={setBrouterDlRegions}
                showContinentTabs
                hint="Alle Regionen aus dem Katalog verfügbar."
              />
            </div>
          </div>
        </div>
      )}
    </div>
    </>
  )
}
