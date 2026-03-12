import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { fetchOSMRegions, startOSMImport, type OSMRegion } from '@/api/osm'
import { useToast } from '@/lib/toast'

const ALL_CATEGORIES = [
  // POI-Kategorien
  { id: 'peaks',          label: 'Gipfel & Sättel',          group: 'POIs' },
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
  { id: 'landmarks',      label: 'Gipfelkreuze & Landmarks', group: 'POIs' },
  { id: 'religion',       label: 'Kapellen & Wegkreuze',     group: 'POIs' },
  // Wege
  { id: 'ways',           label: 'Wegenetz (Routing)',       group: 'Wege' },
  { id: 'lifts',          label: 'Lifte & Seilbahnen',       group: 'Wege' },
  { id: 'pistes',         label: 'Skipisten',                group: 'Wege' },
  { id: 'ferries',        label: 'Fährrouten',               group: 'Wege' },
  // Flächen
  { id: 'waterways',      label: 'Gewässer',                 group: 'Flächen' },
  { id: 'forests',        label: 'Wälder',                   group: 'Flächen' },
  { id: 'wetlands',       label: 'Feuchtgebiete & Moore',    group: 'Flächen' },
  { id: 'boundaries',     label: 'Grenzen',                  group: 'Flächen' },
] as const

type CategoryId = (typeof ALL_CATEGORIES)[number]['id']
type CategoryGroup = (typeof ALL_CATEGORIES)[number]['group']

const GROUPS: CategoryGroup[] = ['POIs', 'Wege', 'Flächen']

interface Props {
  onClose: () => void
}

export default function OSMImportModal({ onClose }: Props) {
  const { addToast } = useToast()
  const queryClient = useQueryClient()

  const [selectedRegions, setSelectedRegions] = useState<string[]>([])
  const [selectedCategories, setSelectedCategories] = useState<CategoryId[]>(
    ALL_CATEGORIES.map(c => c.id)
  )
  const [activeContinent, setActiveContinent] = useState('Europa')
  const [buildBRouter, setBuildBRouter] = useState(true)
  const [buildTopology, setBuildTopology] = useState(true)

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

  function formatSize(mb: number) {
    if (mb >= 1000) return `~${(mb / 1000).toFixed(0)} GB`
    return `~${mb} MB`
  }

  const isLargeRegion = (r: OSMRegion) => r.size_mb >= 5000
  const canStart = selectedRegions.length > 0 && selectedCategories.length > 0 && !mutation.isPending
  const allSelected = selectedCategories.length === ALL_CATEGORIES.length

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="bg-white dark:bg-neutral-900 rounded-2xl border border-border w-full max-w-4xl max-h-[92vh] flex flex-col shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border">
          <h2 className="text-base font-semibold">OSM Import-Assistent</h2>
          <button onClick={onClose} className="text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200">
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
                  className={`px-3 py-1 rounded-full text-xs font-medium transition-colors ${
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
                      title={`${formatSize(r.size_mb)}${large ? ' — Import kann mehrere Stunden dauern' : ''}`}
                      className={`px-3 py-1.5 rounded-lg text-xs font-medium border transition-colors ${
                        selected
                          ? 'bg-accent-DEFAULT text-white border-accent-DEFAULT'
                          : 'bg-white dark:bg-neutral-800 border-neutral-200 dark:border-neutral-700 text-neutral-700 dark:text-neutral-300 hover:border-accent-DEFAULT'
                      }`}
                    >
                      {r.label}
                      <span className={`ml-1.5 text-[10px] ${selected ? 'opacity-80' : 'text-neutral-400'}`}>
                        {formatSize(r.size_mb)}
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
                className="text-xs text-accent-DEFAULT hover:underline"
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
                      className="text-xs text-accent-DEFAULT hover:text-accent-700"
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
          <div className="border-t border-neutral-700 pt-4 mt-2">
            <h3 className="text-sm font-medium mb-3">Nach dem Import</h3>
            <div className="space-y-2">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={buildBRouter}
                  onChange={() => setBuildBRouter(!buildBRouter)}
                  className="accent-accent-DEFAULT"
                />
                <div>
                  <span className="text-sm">BRouter-Segmente bauen</span>
                  <span className="text-xs text-neutral-400 block">
                    Routing-Daten für Routenplaner (PBF aufbewahren)
                  </span>
                </div>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={buildTopology}
                  onChange={() => setBuildTopology(!buildTopology)}
                  disabled={!selectedCategories.includes('ways')}
                  className="accent-accent-DEFAULT"
                />
                <div>
                  <span className={`text-sm ${!selectedCategories.includes('ways') ? 'text-neutral-500' : ''}`}>
                    Trail-Topologie aufbauen
                  </span>
                  <span className="text-xs text-neutral-400 block">
                    pgRouting-Netzwerk + Kostenberechnung (~10–30 Min)
                  </span>
                </div>
              </label>
            </div>
          </div>

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
              className="px-4 py-2 rounded-lg text-sm font-medium border border-border text-neutral-700 dark:text-neutral-300 hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-50 transition-colors"
            >
              Abbrechen
            </button>
            <button
              onClick={() => mutation.mutate({
                regions: selectedRegions,
                categories: selectedCategories,
                build_brouter: buildBRouter,
                build_topology: buildTopology,
              })}
              disabled={!canStart}
              className="px-4 py-2 rounded-lg text-sm font-medium bg-accent-DEFAULT hover:bg-accent-DEFAULT/90 text-white disabled:opacity-50 transition-colors"
            >
              {mutation.isPending ? 'Wird gestartet…' : 'Import starten'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
