import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { fetchOSMRegions, startOSMImport, getDEMRegionStatus, type OSMRegion } from '@/api/osm'
import { useToast } from '@/lib/toast'
import { fmtBytes } from '@/lib/format'
import { ModalShell } from '@/components/ui/ModalShell'

const ALL_CATEGORIES = [
  { id: 'summits',        labelKey: 'summits',        group: 'pois' },
  { id: 'passes',         labelKey: 'passes',         group: 'pois' },
  { id: 'settlements',    labelKey: 'settlements',    group: 'pois' },
  { id: 'gastronomy',     labelKey: 'gastronomy',     group: 'pois' },
  { id: 'accommodation',  labelKey: 'accommodation',  group: 'pois' },
  { id: 'infrastructure', labelKey: 'infrastructure', group: 'pois' },
  { id: 'transport',      labelKey: 'transport',      group: 'pois' },
  { id: 'viewpoints',     labelKey: 'viewpoints',     group: 'pois' },
  { id: 'shops',          labelKey: 'shops',          group: 'pois' },
  { id: 'sport',          labelKey: 'sport',          group: 'pois' },
  { id: 'camping',        labelKey: 'camping',        group: 'pois' },
  { id: 'information',    labelKey: 'information',    group: 'pois' },
  { id: 'landmarks',      labelKey: 'landmarks',      group: 'pois' },
  { id: 'religion',       labelKey: 'religion',       group: 'pois' },
  { id: 'wind_turbines',  labelKey: 'windTurbines',   group: 'pois' },
  { id: 'ways',           labelKey: 'ways',           group: 'ways' },
  { id: 'lifts',          labelKey: 'lifts',          group: 'ways' },
  { id: 'pistes',         labelKey: 'pistes',         group: 'ways' },
  { id: 'climbing',       labelKey: 'climbing',       group: 'ways' },
  { id: 'ferries',        labelKey: 'ferries',        group: 'ways' },
  { id: 'routes',         labelKey: 'routes',         group: 'ways' },
  { id: 'waterways',      labelKey: 'waterways',      group: 'areas' },
  { id: 'forests',        labelKey: 'forests',        group: 'areas' },
  { id: 'glaciers',       labelKey: 'glaciers',       group: 'areas' },
  { id: 'wetlands',       labelKey: 'wetlands',       group: 'areas' },
  { id: 'boundaries',     labelKey: 'boundaries',     group: 'areas' },
  { id: 'landuse',        labelKey: 'landuse',        group: 'areas' },
  { id: 'railways',       labelKey: 'railways',       group: 'ways' },
] as const

type CategoryId = (typeof ALL_CATEGORIES)[number]['id']
type CategoryGroup = (typeof ALL_CATEGORIES)[number]['group']

const GROUPS: CategoryGroup[] = ['pois', 'ways', 'areas']

interface Props {
  onClose: () => void
  initialRegionId?: string
  initialContinent?: string
  initialCategories?: string[]
}

export default function OSMImportModal({ onClose, initialRegionId, initialContinent, initialCategories }: Props) {
  const { t } = useTranslation()
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const [selectedRegions, setSelectedRegions] = useState<string[]>(
    initialRegionId ? [initialRegionId] : []
  )
  const [selectedCategories, setSelectedCategories] = useState<CategoryId[]>(
    initialCategories
      ? (initialCategories.filter(c => ALL_CATEGORIES.some(a => a.id === c)) as CategoryId[])
      : ALL_CATEGORIES.map(c => c.id)
  )
  const [activeContinent, setActiveContinent] = useState(initialContinent ?? 'Europa')
  const [buildBRouter, setBuildBRouter] = useState(true)
  const [runEnrichment, setRunEnrichment] = useState(true)
  const [buildCoverage, setBuildCoverage] = useState(false)
  const [buildTiles, setBuildTiles] = useState(false)
  const [downloadDEM, setDownloadDEM] = useState(false)

  // DEM status for dependency checks
  const { data: demStatus } = useQuery({
    queryKey: ['dem-region-status'],
    queryFn: getDEMRegionStatus,
  })
  // Check if ALL selected regions have complete DEM coverage (not just any region globally)
  const hasDEM = selectedRegions.length > 0 && selectedRegions.every(id => {
    const reg = demStatus?.regions?.find(r => r.id === id)
    return reg != null && reg.complete
  })

  const tilesDisabled = !runEnrichment || (!hasDEM && !downloadDEM)
  const tilesDisabledReason = !runEnrichment
    ? t('osm.import.dependencies.requiresEnrichment')
    : (!hasDEM && !downloadDEM)
      ? t('osm.import.dependencies.requiresDemDownload')
      : ''
  const coverageDisabled = !runEnrichment
  // buildTopology removed: pgRouting 4.0+ removed pgr_createTopology

  const { data: regionsData, isLoading } = useQuery({
    queryKey: ['osm-regions'],
    queryFn: fetchOSMRegions,
  })

  const mutation = useMutation({
    mutationFn: startOSMImport,
    onSuccess: () => {
      addToast({ type: 'success', title: t('osm.import.toasts.started'), message: t('osm.import.toasts.statusHint') })
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
      onClose()
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: t('osm.import.toasts.startFailed'), message: err.message, duration: 0 })
    },
  })

  const continents = [...new Set(regionsData?.regions.map(r => r.continent) ?? [])]
  const visibleRegions = regionsData?.regions.filter(r => r.continent === activeContinent) ?? []

  function toggleRegion(id: string) {
    setSelectedRegions(prev =>
      prev.includes(id) ? prev.filter(r => r !== id) : [...prev, id]
    )
  }

  function toggleCategory(id: CategoryId) {
    setSelectedCategories(prev =>
      prev.includes(id) ? prev.filter(c => c !== id) : [...prev, id]
    )
  }

  function selectGroup(group: CategoryGroup) {
    const groupIds = ALL_CATEGORIES.filter(c => c.group === group).map(c => c.id)
    const allSelected = groupIds.every(id => selectedCategories.includes(id))
    if (allSelected) {
      setSelectedCategories(prev => prev.filter(id => !groupIds.includes(id)))
    } else {
      setSelectedCategories(prev => [...new Set([...prev, ...groupIds])])
    }
  }

  const fmtMB = (mb: number) => fmtBytes(mb * 1024 * 1024)

  const isLargeRegion = (r: OSMRegion) => r.size_mb >= 5000
  const canStart = selectedRegions.length > 0 && selectedCategories.length > 0 && !mutation.isPending
  const allSelected = selectedCategories.length === ALL_CATEGORIES.length

  const storageSummary = useMemo(() => {
    if (selectedRegions.length === 0) return null
    const pbfMB = selectedRegions.reduce((sum, id) => {
      const reg = regionsData?.regions.find(r => r.id === id)
      return sum + (reg?.size_mb ?? 0)
    }, 0)
    const demTiles = (() => {
      if (demStatus?.regions && demStatus.regions.length > 0) {
        const matched = demStatus.regions.filter(r => selectedRegions.includes(r.id))
        if (matched.length > 0) {
          return matched.reduce((sum, r) => sum + Math.max(0, r.tiles_needed - r.tiles_present), 0)
        }
      }
      // Fallback estimate for regions not yet in DEM status (not imported yet)
      return Math.max(1, Math.round(pbfMB / 25))
    })()
    const demMB = downloadDEM ? demTiles * 25 : 0
    const pgMB = pbfMB * 4
    const routingMB = (buildTiles || buildCoverage) ? pbfMB * 4 : 0
    const totalMB = pbfMB + demMB + pgMB + routingMB
    const freeMB = (demStatus?.disk_free_bytes ?? 0) / 1024 / 1024
    return { pbfMB, demMB, demTiles, pgMB, routingMB, totalMB, freeMB }
  }, [selectedRegions, regionsData, demStatus, downloadDEM, buildTiles, buildCoverage])

  return (
    <ModalShell onClose={onClose} maxWidth="max-w-4xl" className="p-0">
      <div className="w-full max-h-[92vh] flex flex-col">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-neutral-200 dark:border-neutral-800">
          <h2 className="text-base font-semibold">{t('osm.import.title')}</h2>
          <button onClick={onClose} className="cursor-pointer text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200">
            <X size={18} />
          </button>
        </div>

        <div className="overflow-y-auto flex-1 px-6 py-5 space-y-6">
          {/* Section 1: regions */}
          <div>
            <h3 className="text-sm font-medium mb-3">{t('osm.import.regions')}</h3>
            {/* Continent tabs */}
            <div className="flex flex-wrap gap-1 mb-3">
              {continents.map(c => (
                <button
                  key={c}
                  onClick={() => setActiveContinent(c)}
                  className={`cursor-pointer px-3 py-1 rounded-full text-xs font-medium transition-colors ${
                    activeContinent === c
                      ? 'bg-accent-DEFAULT text-white'
                      : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
                  }`}
                >
                  {c}
                </button>
              ))}
            </div>

            {isLoading ? (
              <div className="text-sm text-neutral-400">{t('osm.import.loadingRegions')}</div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {visibleRegions.map(r => {
                  const selected = selectedRegions.includes(r.id)
                  const large = isLargeRegion(r)
                  return (
                    <button
                      key={r.id}
                      onClick={() => toggleRegion(r.id)}
                      title={`${fmtMB(r.size_mb)}${large ? t('osm.import.largeRegionTitleSuffix') : ''}`}
                      className={`cursor-pointer px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ${
                        selected
                          ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                          : 'bg-white dark:bg-neutral-800 border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:border-accent-DEFAULT'
                      }`}
                    >
                      {r.label}
                      <span className={`ml-1.5 text-[10px] ${selected ? 'opacity-80' : 'text-neutral-400'}`}>
                        {fmtMB(r.size_mb)}
                      </span>
                    </button>
                  )
                })}
              </div>
            )}
          </div>

          {/* Section 2: categories */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-medium">{t('osm.import.dataCategories')}</h3>
              <button
                onClick={() =>
                  allSelected
                    ? setSelectedCategories([])
                    : setSelectedCategories(ALL_CATEGORIES.map(c => c.id))
                }
                className="cursor-pointer text-xs text-accent-DEFAULT hover:underline"
              >
                {allSelected ? t('osm.import.deselectAll') : t('osm.import.selectAll')}
              </button>
            </div>

            {GROUPS.map(group => {
              const groupCats = ALL_CATEGORIES.filter(c => c.group === group)
              const groupAllSelected = groupCats.every(c => selectedCategories.includes(c.id))
              return (
                <div key={group}>
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="text-xs font-semibold text-neutral-400 uppercase tracking-wide">{t(`osm.import.groups.${group}`)}</h4>
                    <button
                      type="button"
                      className="cursor-pointer text-xs text-accent-DEFAULT hover:text-accent-700"
                      onClick={() => selectGroup(group)}
                    >
                      {groupAllSelected ? t('osm.import.deselectGroup') : t('common.all')}
                    </button>
                  </div>
                  <div className="grid grid-cols-3 gap-1 mb-4">
                    {groupCats.map(cat => (
                      <label key={cat.id} className="flex items-center gap-2 text-sm cursor-pointer">
                        <input
                          type="checkbox"
                          checked={selectedCategories.includes(cat.id)}
                          onChange={() => toggleCategory(cat.id)}
                          className="accent-accent-DEFAULT"
                        />
                        <span className="text-neutral-700 dark:text-neutral-300">{t(`osm.import.categories.${cat.labelKey}`)}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>

          {/* Section 3: after import */}
          <div>
            <h3 className="text-sm font-medium mb-3">{t('osm.import.afterImport')}</h3>
            <div className="space-y-2.5">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={runEnrichment}
                  onChange={e => {
                    setRunEnrichment(e.target.checked)
                    if (!e.target.checked) {
                      setBuildTiles(false)
                      setBuildCoverage(false)
                    }
                  }}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">{t('osm.import.options.routeMembership')}</span>
                <span className="text-xs text-neutral-400">{t('osm.import.options.routeMembershipHint')}</span>
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={downloadDEM}
                  onChange={e => setDownloadDEM(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">{t('osm.import.options.downloadDem')}</span>
                <span className="text-xs text-neutral-400">{t('osm.import.options.downloadDemHint')}</span>
                {hasDEM && <span className="text-xs text-green-500">{t('osm.import.options.alreadyAvailable')}</span>}
              </label>
              <label className={`flex items-center gap-2 text-sm ${coverageDisabled ? 'opacity-40' : ''}`}>
                <input
                  type="checkbox"
                  checked={buildCoverage}
                  disabled={coverageDisabled}
                  onChange={e => setBuildCoverage(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">{t('osm.import.options.buildCoverage')}</span>
                <span className="text-xs text-neutral-400">{t('osm.import.options.buildCoverageHint')}</span>
                {coverageDisabled && <span className="text-xs text-red-500">{t('osm.import.dependencies.requiresEnrichment')}</span>}
              </label>
              <label className={`flex items-center gap-2 text-sm ${tilesDisabled ? 'opacity-40' : ''}`}>
                <input
                  type="checkbox"
                  checked={buildTiles}
                  disabled={tilesDisabled}
                  onChange={e => setBuildTiles(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">{t('osm.import.options.buildRoutingTiles')}</span>
                <span className="text-xs text-neutral-400">{t('osm.import.options.buildRoutingTilesHint')}</span>
                {tilesDisabled && <span className="text-xs text-red-500">{tilesDisabledReason}</span>}
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={buildBRouter}
                  onChange={e => setBuildBRouter(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">{t('osm.import.options.buildBrouter')}</span>
                <span className="text-xs text-neutral-400">{t('osm.import.options.buildBrouterHint')}</span>
              </label>
            </div>
          </div>

          {/* Section 3b: storage estimate */}
          {storageSummary && (
            <div className="rounded-lg bg-neutral-50 dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-xs space-y-1">
              <p className="font-medium text-neutral-700 dark:text-neutral-300 mb-1">{t('osm.import.storage.title')}</p>
              <p className="text-neutral-500 dark:text-neutral-400">
                {t('osm.import.storage.pbfDownload')}: <span className="font-mono">{fmtMB(storageSummary.pbfMB)}</span>
              </p>
              {downloadDEM && (
                <p className="text-neutral-500 dark:text-neutral-400">
                  {t('osm.import.storage.demElevation')}: <span className="font-mono">~{fmtMB(storageSummary.demMB)}</span>
                  <span className="text-neutral-400"> {t('osm.import.storage.tileEstimate', { count: storageSummary.demTiles })}</span>
                </p>
              )}
              <p className="text-neutral-500 dark:text-neutral-400">
                PostgreSQL: <span className="font-mono">~{fmtMB(storageSummary.pgMB)}</span>
              </p>
              {(buildTiles || buildCoverage) && (
                <p className="text-neutral-500 dark:text-neutral-400">
                  {t('osm.import.storage.routingCoverage')}: <span className="font-mono">~{fmtMB(storageSummary.routingMB)}</span>
                </p>
              )}
              <div className="border-t border-neutral-200 dark:border-neutral-700 mt-1 pt-1 flex justify-between">
                <span className="font-medium text-neutral-600 dark:text-neutral-300">
                  {t('osm.import.storage.total')}: ~{fmtMB(storageSummary.totalMB)}
                </span>
                <span className={`font-medium ${storageSummary.freeMB < storageSummary.totalMB ? 'text-red-500' : 'text-neutral-500 dark:text-neutral-400'}`}>
                  {t('osm.import.storage.available')}: {fmtMB(storageSummary.freeMB)}
                </span>
              </div>
              {storageSummary.totalMB > 0 && (
                <p className="text-neutral-400 mt-1">
                  {t('osm.import.storage.multiplierHint')}
                </p>
              )}
            </div>
          )}

          {/* Section 4: notes */}
          <div className="rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-4 py-3 text-xs text-amber-700 dark:text-amber-400 space-y-1">
            <p>{t('osm.import.warnings.replaceCategories')}</p>
            <p>{t('osm.import.warnings.largeRegions')}</p>
            <p>{t('osm.import.warnings.resourceUsage')}</p>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-neutral-200 dark:border-neutral-800 flex items-center justify-between">
          <span className="text-xs text-neutral-400">
            {t('osm.import.selectionSummary', { regions: selectedRegions.length, categories: selectedCategories.length })}
          </span>
          <div className="flex gap-3">
            <button
              onClick={onClose}
              disabled={mutation.isPending}
              className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium border border-neutral-200 dark:border-neutral-800 text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {t('common.cancel')}
            </button>
            <button
              onClick={() => {
                if ('Notification' in window && Notification.permission === 'default') {
                  Notification.requestPermission()
                }
                mutation.mutate({
                  regions: selectedRegions,
                  categories: selectedCategories,
                  build_brouter: buildBRouter,
                  run_enrichment: runEnrichment,
                  build_coverage: buildCoverage && runEnrichment,
                  build_tiles: buildTiles && runEnrichment,
                  download_dem: downloadDEM,
                })
              }}
              disabled={!canStart}
              className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {mutation.isPending ? t('osm.import.starting') : t('osm.import.start')}
            </button>
          </div>
        </div>
      </div>
    </ModalShell>
  )
}
