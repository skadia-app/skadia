import { useState, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { fetchOSMRegions, startOSMImport, getDEMRegionStatus, type OSMRegion, type DEMStatusResponse } from '@/api/osm'
import { useToast } from '@/lib/toast'
import { fmtBytes } from '@/lib/format'
import { ModalShell } from '@/components/ui/ModalShell'

const ALL_CATEGORIES = [
  // POI-Kategorien
  { id: 'summits',        label: 'Gipfel & Sättel',          group: 'POIs' },
  { id: 'passes',         label: 'Passstraßen',              group: 'POIs' },
  { id: 'settlements',    label: 'Ortschaften',              group: 'POIs' },
  { id: 'gastronomy',     label: 'Gastronomie',              group: 'POIs' },
  { id: 'accommodation',  label: 'Unterkünfte & Hütten',     group: 'POIs' },
  { id: 'infrastructure', label: 'Infrastruktur (WC/Wasser/P)', group: 'POIs' },
  { id: 'transport',      label: 'Öffentlicher Verkehr',     group: 'POIs' },
  { id: 'viewpoints',     label: 'Aussichtspunkte',          group: 'POIs' },
  { id: 'shops',          label: 'Geschäfte & Versorgung',   group: 'POIs' },
  { id: 'sport',          label: 'Sportanlagen',             group: 'POIs' },
  { id: 'camping',        label: 'Camping & Feuerstellen',   group: 'POIs' },
  { id: 'information',    label: 'Wegweiser & Info',         group: 'POIs' },
  { id: 'landmarks',      label: 'Landmarks & Natur',        group: 'POIs' },
  { id: 'religion',       label: 'Kapellen & Wegkreuze',     group: 'POIs' },
  { id: 'wind_turbines', label: 'Windkraftanlagen',         group: 'POIs' },
  // Wege
  { id: 'ways',           label: 'Wegenetz (Routing)',       group: 'Wege' },
  { id: 'lifts',          label: 'Lifte & Seilbahnen',       group: 'Wege' },
  { id: 'pistes',         label: 'Skipisten',                group: 'Wege' },
  { id: 'climbing',       label: 'Kletterrouten & Via Ferrata', group: 'Wege' },
  { id: 'ferries',        label: 'Fährrouten',               group: 'Wege' },
  { id: 'routes',         label: 'Benannte Routen (Fernwege/Radrouten)', group: 'Wege' },
  // Flächen
  { id: 'waterways',      label: 'Gewässer',                 group: 'Flächen' },
  { id: 'forests',        label: 'Wälder',                   group: 'Flächen' },
  { id: 'glaciers',       label: 'Gletscher',                group: 'Flächen' },
  { id: 'wetlands',       label: 'Feuchtgebiete & Moore',    group: 'Flächen' },
  { id: 'boundaries',     label: 'Grenzen & Schutzgebiete',   group: 'Flächen' },
  { id: 'landuse',        label: 'Landnutzung (Felder, Industrie, Militär)', group: 'Flächen' },
  // Wege (additional)
  { id: 'railways',       label: 'Eisenbahnlinien',            group: 'Wege' },
] as const

type CategoryId = (typeof ALL_CATEGORIES)[number]['id']
type CategoryGroup = (typeof ALL_CATEGORIES)[number]['group']

const GROUPS: CategoryGroup[] = ['POIs', 'Wege', 'Flächen']

interface Props {
  onClose: () => void
  initialRegionId?: string
  initialContinent?: string
  initialCategories?: string[]
}

export default function OSMImportModal({ onClose, initialRegionId, initialContinent, initialCategories }: Props) {
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
  const hasDEM = (demStatus?.total_tiles ?? 0) > 0 || (demStatus?.regions?.some(r => r.tiles_present > 0) ?? false)

  const tilesDisabled = !runEnrichment || (!hasDEM && !downloadDEM)
  const tilesDisabledReason = !runEnrichment
    ? '(benötigt Enrichment)'
    : (!hasDEM && !downloadDEM)
      ? '(benötigt DEM — aktiviere "DEM herunterladen")'
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
      addToast({ type: 'success', title: 'Import gestartet', message: 'Status im Berechnungs-Indikator (oben rechts)' })
      queryClient.invalidateQueries({ queryKey: ['active-jobs'] })
      onClose()
    },
    onError: (err: Error) => {
      addToast({ type: 'error', title: 'Fehler beim Starten', message: err.message, duration: 0 })
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
        return demStatus.regions
          .filter(r => selectedRegions.includes(r.id))
          .reduce((sum, r) => sum + Math.max(0, r.tiles_needed - r.tiles_present), 0)
      }
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
    <ModalShell onClose={onClose} maxWidth="max-w-4xl">
      <div className="bg-white dark:bg-neutral-900 rounded-2xl border border-border w-full max-h-[92vh] flex flex-col shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <h2 className="text-base font-semibold">OSM Import-Assistent</h2>
          <button onClick={onClose} className="cursor-pointer text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200">
            <X size={18} />
          </button>
        </div>

        <div className="overflow-y-auto flex-1 px-6 py-5 space-y-6">
          {/* Sektion 1: Regionen */}
          <div>
            <h3 className="text-sm font-medium mb-3">Regionen</h3>
            {/* Kontinent-Tabs */}
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
              <div className="text-sm text-neutral-400">Lade Regionen...</div>
            ) : (
              <div className="flex flex-wrap gap-2">
                {visibleRegions.map(r => {
                  const selected = selectedRegions.includes(r.id)
                  const large = isLargeRegion(r)
                  return (
                    <button
                      key={r.id}
                      onClick={() => toggleRegion(r.id)}
                      title={`${fmtMB(r.size_mb)}${large ? ' — Import kann mehrere Stunden dauern' : ''}`}
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

          {/* Sektion 2: Kategorien */}
          <div>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-sm font-medium">Datenkategorien</h3>
              <button
                onClick={() =>
                  allSelected
                    ? setSelectedCategories([])
                    : setSelectedCategories(ALL_CATEGORIES.map(c => c.id))
                }
                className="cursor-pointer text-xs text-accent-DEFAULT hover:underline"
              >
                {allSelected ? 'Alle abwählen' : 'Alle auswählen'}
              </button>
            </div>

            {GROUPS.map(group => {
              const groupCats = ALL_CATEGORIES.filter(c => c.group === group)
              const groupAllSelected = groupCats.every(c => selectedCategories.includes(c.id))
              return (
                <div key={group}>
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="text-xs font-semibold text-neutral-400 uppercase tracking-wide">{group}</h4>
                    <button
                      type="button"
                      className="cursor-pointer text-xs text-accent-DEFAULT hover:text-accent-700"
                      onClick={() => selectGroup(group)}
                    >
                      {groupAllSelected ? 'Gruppe abwählen' : 'Alle'}
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
                        <span className="text-neutral-700 dark:text-neutral-300">{cat.label}</span>
                      </label>
                    ))}
                  </div>
                </div>
              )
            })}
          </div>

          {/* Sektion 3: Nach dem Import */}
          <div>
            <h3 className="text-sm font-medium mb-3">Nach dem Import</h3>
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
                <span className="text-neutral-700 dark:text-neutral-300">Route-Membership berechnen</span>
                <span className="text-xs text-neutral-400">(Radrouten/Wanderrouten-Zugehörigkeit)</span>
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={downloadDEM}
                  onChange={e => setDownloadDEM(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">DEM-Höhendaten herunterladen</span>
                <span className="text-xs text-neutral-400">(GLO-30, 30m — für Routing-Tiles)</span>
                {hasDEM && <span className="text-xs text-green-500">(bereits vorhanden)</span>}
              </label>
              <label className={`flex items-center gap-2 text-sm ${coverageDisabled ? 'opacity-40' : ''}`}>
                <input
                  type="checkbox"
                  checked={buildCoverage}
                  disabled={coverageDisabled}
                  onChange={e => setBuildCoverage(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">Coverage-Grids bauen</span>
                <span className="text-xs text-neutral-400">(Wald, Lärm, Verkehr — für Karten-Overlays)</span>
                {coverageDisabled && <span className="text-xs text-red-500">(benötigt Enrichment)</span>}
              </label>
              <label className={`flex items-center gap-2 text-sm ${tilesDisabled ? 'opacity-40' : ''}`}>
                <input
                  type="checkbox"
                  checked={buildTiles}
                  disabled={tilesDisabled}
                  onChange={e => setBuildTiles(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">Routing-Tiles bauen</span>
                <span className="text-xs text-neutral-400">(BadRouter Graph mit DEM)</span>
                {tilesDisabled && <span className="text-xs text-red-500">{tilesDisabledReason}</span>}
              </label>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={buildBRouter}
                  onChange={e => setBuildBRouter(e.target.checked)}
                  className="accent-accent-DEFAULT"
                />
                <span className="text-neutral-700 dark:text-neutral-300">BRouter rd5-Segmente lokal generieren</span>
                <span className="text-xs text-neutral-400">(aus PBF-Daten)</span>
              </label>
            </div>
          </div>

          {/* Sektion 3b: Speicherbedarf */}
          {storageSummary && (
            <div className="rounded-lg bg-neutral-50 dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 px-4 py-3 text-xs space-y-1">
              <p className="font-medium text-neutral-700 dark:text-neutral-300 mb-1">Geschätzter Speicherbedarf</p>
              <p className="text-neutral-500 dark:text-neutral-400">
                PBF-Download: <span className="font-mono">{(storageSummary.pbfMB / 1024).toFixed(1)} GB</span>
              </p>
              {downloadDEM && (
                <p className="text-neutral-500 dark:text-neutral-400">
                  DEM-Höhendaten: <span className="font-mono">~{(storageSummary.demMB / 1024).toFixed(1)} GB</span>
                  <span className="text-neutral-400"> ({storageSummary.demTiles} Kacheln × 25 MB)</span>
                </p>
              )}
              <p className="text-neutral-500 dark:text-neutral-400">
                PostgreSQL: <span className="font-mono">~{(storageSummary.pgMB / 1024).toFixed(1)} GB</span>
                <span className="text-neutral-400"> (ca. 4× PBF)</span>
              </p>
              {(buildTiles || buildCoverage) && (
                <p className="text-neutral-500 dark:text-neutral-400">
                  Routing + Coverage: <span className="font-mono">~{(storageSummary.routingMB / 1024).toFixed(1)} GB</span>
                  <span className="text-neutral-400"> (ca. 4× PBF)</span>
                </p>
              )}
              <div className="border-t border-neutral-200 dark:border-neutral-700 mt-1 pt-1 flex justify-between">
                <span className="font-medium text-neutral-600 dark:text-neutral-300">
                  Gesamt: ~{(storageSummary.totalMB / 1024).toFixed(1)} GB
                </span>
                <span className={`font-medium ${storageSummary.freeMB < storageSummary.totalMB ? 'text-red-500' : 'text-neutral-500 dark:text-neutral-400'}`}>
                  Verfügbar: {(storageSummary.freeMB / 1024).toFixed(0)} GB
                </span>
              </div>
              {storageSummary.totalMB > 0 && (
                <p className="text-neutral-400 mt-1">
                  In Summe bis zu 12× der PBF-Größe wenn alle Optionen aktiviert.
                </p>
              )}
            </div>
          )}

          {/* Sektion 4: Hinweise */}
          <div className="rounded-lg bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-4 py-3 text-xs text-amber-700 dark:text-amber-400 space-y-1">
            <p>Die gewählten Datenkategorien werden vor dem Import vollständig geleert und neu befüllt.</p>
            <p>Große Regionen (z.B. Europa gesamt, China) können mehrere Stunden dauern.</p>
            <p>Während des Imports — insbesondere beim Einlesen der Daten — können CPU-Auslastung, RAM-Verbrauch und benötigter Speicherplatz zeitweise sehr hoch sein.</p>
          </div>
        </div>

        {/* Footer */}
        <div className="px-6 py-4 border-t border-border flex items-center justify-between">
          <span className="text-xs text-neutral-400">
            {selectedRegions.length} Region(en), {selectedCategories.length} Kategorie(n) gewählt
          </span>
          <div className="flex gap-3">
            <button
              onClick={onClose}
              disabled={mutation.isPending}
              className="cursor-pointer px-4 py-2 rounded-lg text-sm font-medium border border-border text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              Abbrechen
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
              {mutation.isPending ? 'Wird gestartet…' : 'Import starten'}
            </button>
          </div>
        </div>
      </div>
    </ModalShell>
  )
}
