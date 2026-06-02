import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Trash2, Database, XCircle, ChevronDown, RefreshCw, AlertTriangle, Sparkles, Layers, Route, Grid3x3, ArrowDownToLine } from 'lucide-react'
import { useState, useMemo, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
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
import { fmtBytes, fmtDateCompact } from '@/lib/format'
import OSMImportModal from '@/components/OSMImportModal'
import RegionSelector from '@/components/RegionSelector'
import PipelinePanel from '@/components/PipelinePanel'

export interface OSMRegionManagerBatchUpdateProps {
  /** Trigger batch-update; opens the rebuild-tiles confirmation dialog. */
  onClick: () => void
  /** True while a batch update / single update / OSM import is running or the mutation is pending. */
  disabled: boolean
  /** Localized label, switches while a batch update is active. */
  label: string
  /** True only when the button should be visible at all (≥1 imported region with replication_url). */
  visible: boolean
}

export function OSMRegionManager({
  hasOsmData,
  fullGeodataAvailable = true,
  renderBatchUpdateButton,
  onBatchUpdateState,
}: {
  hasOsmData?: boolean
  fullGeodataAvailable?: boolean
  /**
   * Optional render-prop that lets the parent place the batch-update
   * button outside the imported-regions table (e.g. in a bottom action row).
   * When supplied, OSMRegionManager hides its own inline button.
   */
  renderBatchUpdateButton?: (props: OSMRegionManagerBatchUpdateProps) => React.ReactNode
  /**
   * Optional callback that pushes the latest batch-update button state to the parent
   * via useEffect. Use this when the parent renders the button on its own — it lets
   * OSMRegionManager hide its inline button without forcing the parent to opt into
   * the render-prop pattern.
   */
  onBatchUpdateState?: (props: OSMRegionManagerBatchUpdateProps) => void
}) {
  const { t } = useTranslation()
  const qc = useQueryClient()
  const { addToast } = useToast()
  const { openModal } = useModal()
  const [diskExpanded, setDiskExpanded] = useState(false)

  const { data: regionData, isFetched: regionsFetched } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
    enabled: fullGeodataAvailable,
    refetchInterval: (query) =>
      fullGeodataAvailable && query.state.data?.regions?.some((r) => r.status === 'importing') ? 5000 : false,
  })

  const deleteMutation = useMutation({
    mutationFn: deleteOSMRegion,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'success', title: t('osm.regions.toasts.regionRemoved') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('osm.regions.toasts.regionRemoveFailed'), duration: 0 })
    },
  })

  const cancelMutation = useMutation({
    mutationFn: cancelOSMImport,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: t('osm.regions.toasts.importCanceled') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('osm.regions.toasts.importCancelFailed'), duration: 0 })
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

  function mutOpts<TArgs>(fn: (args: TArgs) => Promise<unknown>, successTitle: string, errorTitle: string) {
    return {
      mutationFn: fn,
      onSuccess: () => { qc.invalidateQueries({ queryKey: ['active-jobs'] }); addToast({ type: 'info' as const, title: successTitle }) },
      onError: () => { addToast({ type: 'error' as const, title: errorTitle, duration: 0 }) },
    }
  }

  const enrichMut = useMutation(mutOpts((regions: string[]) => retryOSMEnrichment(regions), t('osm.regions.toasts.enrichmentStarted'), t('osm.regions.toasts.enrichmentStartFailed')))
  const tilesMut = useMutation(mutOpts((regions: string[]) => rebuildTiles({ regions, force: true }), t('osm.regions.toasts.tileBuildStarted'), t('osm.regions.toasts.tileBuildStartFailed')))
  const brouterLocalMut = useMutation(mutOpts((regions: string[]) => buildBRouterSegments(regions), t('osm.regions.toasts.brouterBuildStarted'), t('osm.regions.toasts.brouterBuildStartFailed')))
  const brouterDlMut = useMutation(mutOpts((regions: string[]) => downloadBRouterSegments(regions), t('osm.regions.toasts.brouterDownloadStarted'), t('osm.regions.toasts.brouterDownloadStartFailed')))
  const coverageMut = useMutation(mutOpts((regions: string[]) => buildCoverageGrids(regions), t('osm.regions.toasts.coverageBuildStarted'), t('osm.regions.toasts.coverageBuildStartFailed')))

  const updateMut = useMutation({
    mutationFn: (regionID: string) => triggerOSMRegionUpdate(regionID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: t('osm.regions.toasts.incrementalUpdateStarted') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('osm.regions.toasts.updateStartFailed'), duration: 0 })
    },
  })

  const batchUpdateMut = useMutation({
    mutationFn: (rebuildTiles: boolean) => batchUpdateAllRegions(rebuildTiles),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['osm-regions-status'] })
      addToast({ type: 'info', title: t('osm.regions.toasts.batchUpdateStarted') })
    },
    onError: () => {
      addToast({ type: 'error', title: t('osm.regions.toasts.batchUpdateStartFailed'), duration: 0 })
    },
  })

  const { jobs } = useActiveJobs()
  const activeJobs = useMemo(() => {
    const map = new Map<string, typeof jobs[0]>()
    for (const j of jobs) {
      if (j.status === 'running') map.set(j.job_type, j)
    }
    return map
  }, [jobs])
  const activeOsmJob = activeJobs.get('osm_import')
  const activeEnrichmentJob = activeJobs.get('osm_enrichment')
  const activeTileBuild = activeJobs.get('tile_build')
  const activeBrouterBuild = activeJobs.get('brouter_segment_build')
  const activeBrouterDl = activeJobs.get('brouter_segment_download')
  const activeCoverageBuild = activeJobs.get('coverage_build')
  const activeOsmUpdate = activeJobs.get('osm_update')
  const activeBatchUpdate = activeJobs.get('osm_update_all')

  const { data: diskData } = useQuery({
    queryKey: ['osm-disk-usage'],
    queryFn: fetchOSMDiskUsage,
    enabled: fullGeodataAvailable,
    refetchInterval: activeJobs.size > 0 ? 30000 : false,
  })

  const [updateTarget, setUpdateTarget] = useState<OSMImportRegion | null>(null)

  const regions = regionData?.regions ?? []
  const importedRegions = regions.filter(r => r.status === 'imported')

  // Batch-update entry point — shared between the inline button and the
  // optional `renderBatchUpdateButton` slot rendered by the parent.
  function triggerBatchUpdate() {
    openModal({
      title: t('osm.regions.batchUpdate.title'),
      message: t('osm.regions.batchUpdate.message'),
      buttons: [
        { label: t('osm.regions.batchUpdate.no'), variant: 'default', onClick: () => batchUpdateMut.mutate(false) },
        { label: t('osm.regions.batchUpdate.yesBuildTiles'), variant: 'default', onClick: () => batchUpdateMut.mutate(true) },
      ],
    })
  }
  const batchUpdateVisible =
    importedRegions.length > 0 && importedRegions.some(r => r.replication_url)
  const batchUpdateDisabled =
    !!activeOsmJob || !!activeOsmUpdate || !!activeBatchUpdate || batchUpdateMut.isPending
  const batchUpdateLabel = activeBatchUpdate ? t('osm.regions.batchUpdate.running') : t('osm.regions.batchUpdate.allRegions')

  // Push the latest batch-update state up so the parent can render the button
  // wherever it visually belongs (e.g. a bottom action row).
  useEffect(() => {
    if (!onBatchUpdateState) return
    onBatchUpdateState({
      onClick: triggerBatchUpdate,
      disabled: batchUpdateDisabled,
      label: batchUpdateLabel,
      visible: batchUpdateVisible,
    })
    // triggerBatchUpdate is stable enough for our purposes (depends on openModal +
    // batchUpdateMut which keep stable identities). Including it would re-fire on
    // every render; we deliberately omit it.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [batchUpdateDisabled, batchUpdateLabel, batchUpdateVisible, onBatchUpdateState])

  // When either escape hatch is supplied, suppress the inline button so the parent
  // owns placement. Render-prop wins when both are supplied.
  const inlineBatchUpdateHidden = !!renderBatchUpdateButton || !!onBatchUpdateState

  if (!fullGeodataAvailable) {
    return (
      <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4">
        <div className="flex items-start gap-2 mb-3">
          <Route className="w-4 h-4 mt-0.5 text-neutral-500 dark:text-neutral-400 shrink-0" />
          <div>
            <h4 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">{t('osm.regions.brouter.title')}</h4>
            <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('osm.regions.brouter.downloadCatalogDescription')}</p>
          </div>
        </div>
        <div className="pl-6">
          <div className="flex items-start justify-between mb-2">
            <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mt-0.5">{t('osm.regions.brouter.rd5Download')}</span>
            <button
              onClick={() => brouterDlMut.mutate(brouterDlRegions)}
              disabled={brouterDlMut.isPending || !!activeBrouterDl}
              className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
            >
              {brouterDlMut.isPending || activeBrouterDl ? t('common.loading') : t('osm.regions.actions.download')}
            </button>
          </div>
          <RegionSelector
            regions={catalogData?.regions?.map(r => ({ id: r.id, label: r.label, continent: r.continent })) ?? []}
            selected={brouterDlRegions}
            onChange={setBrouterDlRegions}
            showContinentTabs
            hint={t('osm.regions.hints.allCatalogRegions')}
          />
        </div>
      </div>
    )
  }

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
            {t('osm.regions.importRunning')}
          </span>
          <button
            onClick={() => cancelMutation.mutate(activeOsmJob.id)}
            disabled={cancelMutation.isPending}
            className="cursor-pointer flex items-center gap-1 text-xs text-red-600 dark:text-red-400 hover:text-red-700 dark:hover:text-red-300 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <XCircle className="w-3.5 h-3.5" />
            {t('common.cancel')}
          </button>
        </div>
      )}

      {hasOsmData && regionsFetched && regions.length === 0 && !activeOsmJob && (
        <div className="flex items-start gap-2 rounded-lg border border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-900/20 px-3 py-2">
          <AlertTriangle className="w-4 h-4 text-yellow-600 dark:text-yellow-400 mt-0.5 shrink-0" />
          <p className="text-xs text-yellow-700 dark:text-yellow-300">
            {t('osm.regions.incompleteImportWarning')}
          </p>
        </div>
      )}

      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
          {t('osm.regions.importedRegions')}
        </h3>
        {diskData && (
          <button
            onClick={() => setDiskExpanded(!diskExpanded)}
            className="cursor-pointer flex items-center gap-1 text-xs text-neutral-500 dark:text-neutral-400 hover:text-neutral-900 dark:hover:text-neutral-100 transition-colors"
          >
            <Database className="w-3 h-3" />
            {t('osm.regions.osmDataSize', { size: fmtBytes(diskData.total_bytes) })}
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
              <span className="text-neutral-700 dark:text-neutral-300">{fmtBytes(t.total_bytes)}</span>
            </div>
          ))}
          <div className="flex justify-between border-t border-neutral-200 dark:border-neutral-700 pt-1 mt-1 font-medium">
            <span className="text-neutral-600 dark:text-neutral-300">{t('osm.regions.total')}</span>
            <span className="text-neutral-800 dark:text-neutral-200">{fmtBytes(diskData.total_bytes)}</span>
          </div>
        </div>
      )}

      {regions.length === 0 ? (
        <p className="text-sm text-neutral-500 dark:text-neutral-400">
          {t('osm.regions.empty')}
        </p>
      ) : (
        <div className="overflow-x-auto">
          {batchUpdateVisible && !inlineBatchUpdateHidden && (
            <div className="flex justify-end mb-3">
              <button
                onClick={triggerBatchUpdate}
                disabled={batchUpdateDisabled}
                className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90"
              >
                {batchUpdateLabel}
              </button>
            </div>
          )}
          {batchUpdateVisible && renderBatchUpdateButton && (
            <div className="flex justify-end mb-3">
              {renderBatchUpdateButton({
                onClick: triggerBatchUpdate,
                disabled: batchUpdateDisabled,
                label: batchUpdateLabel,
                visible: batchUpdateVisible,
              })}
            </div>
          )}
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-700 text-neutral-500 dark:text-neutral-400 text-xs">
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.region')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.continent')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.imported')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.dataDate')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.categories')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.size')}</th>
                <th className="text-left pb-2 pr-3 font-medium">{t('osm.regions.table.status')}</th>
                <th className="text-right pb-2 font-medium">{t('osm.regions.table.actions')}</th>
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
                    {fmtDateCompact(reg.imported_at)}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {reg.data_date ? fmtDateCompact(reg.data_date) : '—'}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400 text-xs">
                    {reg.categories.join(', ')}
                  </td>
                  <td className="py-2 pr-3 text-neutral-500 dark:text-neutral-400">
                    {fmtBytes(reg.pbf_size_bytes)}
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
                        ? t('osm.regions.status.imported')
                        : reg.status === 'importing'
                          ? t('osm.regions.status.importing')
                          : reg.status === 'needs_redownload'
                            ? t('osm.regions.status.needsRedownload')
                            : t('osm.regions.status.error')}
                    </span>
                  </td>
                  <td className="py-2 text-right">
                    <div className="flex justify-end gap-1">
                      {reg.status === 'imported' && reg.replication_url && (
                        <button
                          onClick={() => updateMut.mutate(reg.region_id)}
                          disabled={!!activeOsmJob || !!activeOsmUpdate || updateMut.isPending}
                          className="cursor-pointer p-1 text-neutral-400 hover:text-blue-500 dark:hover:text-blue-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                          title={t('osm.regions.actions.incrementalUpdate')}
                        >
                          <ArrowDownToLine className="w-4 h-4" />
                        </button>
                      )}
                      <button
                        onClick={() => setUpdateTarget(reg)}
                        disabled={!!activeOsmJob}
                        className="cursor-pointer p-1 text-neutral-400 hover:text-accent-DEFAULT dark:hover:text-accent-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                        title={t('osm.regions.actions.reimportRegion')}
                      >
                        <RefreshCw className="w-4 h-4" />
                      </button>
                      <button
                        onClick={() => {
                          openModal({
                            title: t('osm.regions.delete.title'),
                            message: t('osm.regions.delete.message', { label: reg.label }),
                            buttons: [
                              {
                                label: t('common.cancel'),
                                variant: 'default',
                                onClick: () => {},
                              },
                              {
                                label: t('osm.regions.actions.remove'),
                                variant: 'destructive',
                                onClick: () => deleteMutation.mutate(reg.region_id),
                              },
                            ],
                          })
                        }}
                        disabled={deleteMutation.isPending}
                        className="cursor-pointer p-1 text-neutral-400 hover:text-red-500 dark:hover:text-red-400 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                        title={t('osm.regions.actions.removeFromTracking')}
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
            {t('osm.regions.pipelineActions')}
          </h3>

          <PipelinePanel
            title={t('osm.regions.pipeline.routeMembershipTitle')}
            description={t('osm.regions.pipeline.routeMembershipDescription')}
            icon={Sparkles}
            buttonLabel={t('osm.regions.actions.calculate')}
            onStart={() => enrichMut.mutate(enrichRegions)}
            isPending={enrichMut.isPending || !!activeEnrichmentJob}
            disabled={importedRegions.length === 0}
            disabledHint={t('osm.regions.hints.noImportedRegions')}
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={enrichRegions}
              onChange={setEnrichRegions}
              hint={t('osm.regions.hints.onlyImportedRegions')}
            />
          </PipelinePanel>

          <PipelinePanel
            title={t('osm.regions.pipeline.routingTilesTitle')}
            description={t('osm.regions.pipeline.routingTilesDescription')}
            icon={Layers}
            buttonLabel={t('osm.regions.actions.buildTiles')}
            onStart={() => tilesMut.mutate(tilesRegions)}
            isPending={tilesMut.isPending || !!activeTileBuild}
            disabled={importedRegions.length === 0}
            disabledHint={t('osm.regions.hints.noImportedRegions')}
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={tilesRegions}
              onChange={setTilesRegions}
              hint={t('osm.regions.hints.onlyImportedRegions')}
            />
          </PipelinePanel>

          <PipelinePanel
            title={t('osm.regions.pipeline.coverageTitle')}
            description={t('osm.regions.pipeline.coverageDescription')}
            icon={Grid3x3}
            buttonLabel={t('osm.regions.actions.build')}
            onStart={() => coverageMut.mutate(coverageRegions)}
            isPending={coverageMut.isPending || !!activeCoverageBuild}
            disabled={importedRegions.length === 0}
            disabledHint={t('osm.regions.hints.noImportedRegions')}
          >
            <RegionSelector
              regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
              selected={coverageRegions}
              onChange={setCoverageRegions}
              hint={t('osm.regions.hints.onlyImportedRegions')}
            />
          </PipelinePanel>

          {/* BRouter: combined panel with two sections */}
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4">
            <div className="flex items-start gap-2 mb-3">
              <Route className="w-4 h-4 mt-0.5 text-neutral-500 dark:text-neutral-400 shrink-0" />
              <div>
                <h4 className="text-sm font-semibold text-neutral-800 dark:text-neutral-200">{t('osm.regions.brouter.title')}</h4>
                <p className="text-xs text-neutral-500 dark:text-neutral-400">{t('osm.regions.brouter.description')}</p>
              </div>
            </div>

            {/* Local generation */}
            <div className="mb-3 pl-6">
              <div className="flex items-start justify-between mb-2">
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mt-0.5">{t('osm.regions.brouter.localGenerate')}</span>
                <button
                  onClick={() => brouterLocalMut.mutate(brouterLocalRegions)}
                  disabled={importedRegions.length === 0 || brouterLocalMut.isPending || !!activeBrouterBuild}
                  className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
                >
                  {brouterLocalMut.isPending || activeBrouterBuild ? t('common.loading') : t('osm.regions.actions.generate')}
                </button>
              </div>
              <RegionSelector
                regions={importedRegions.map(r => ({ id: r.region_id, label: r.label }))}
                selected={brouterLocalRegions}
                onChange={setBrouterLocalRegions}
                hint={t('osm.regions.hints.onlyRegionsWithPbf')}
              />
            </div>

            {/* Download from brouter.de */}
            <div className="pl-6 pt-3 border-t border-neutral-100 dark:border-neutral-800/60">
              <div className="flex items-start justify-between mb-2">
                <span className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mt-0.5">{t('osm.regions.brouter.downloadFromBrouter')}</span>
                <button
                  onClick={() => brouterDlMut.mutate(brouterDlRegions)}
                  disabled={brouterDlMut.isPending || !!activeBrouterDl}
                  className="cursor-pointer text-sm font-medium text-white px-4 py-2 rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 shrink-0 ml-3"
                >
                  {brouterDlMut.isPending || activeBrouterDl ? t('common.loading') : t('osm.regions.actions.download')}
                </button>
              </div>
              <RegionSelector
                regions={catalogData?.regions?.map(r => ({ id: r.id, label: r.label, continent: r.continent })) ?? []}
                selected={brouterDlRegions}
                onChange={setBrouterDlRegions}
                showContinentTabs
                hint={t('osm.regions.hints.allCatalogRegions')}
              />
            </div>
          </div>
        </div>
      )}
    </div>
    </>
  )
}
