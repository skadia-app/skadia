// Admin sub-tab bundling all map/tiles admin sections.
// Sections in order (grouped by data flow — raw geodata first, derived layers next):
//   1. MapLayersAdminSection             — system layer CRUD
//   2. SystemLayerVisibilityAdminSection — overlay-key toggles
//   3. Routing-Dienste                   — capability badges
//   4. OSM-Geodaten                      — stats, regions, scheduler, primary-action row
//   5. Höhendaten (DEM)                  — coverage + bottom-anchored download action
//   6. Offline-Karte                     — combined Tile-Server + PMTiles card
import { useEffect, useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { MapLayersAdminSection } from '@/components/admin/MapLayersAdminSection'
import { SystemLayerVisibilityAdminSection } from '@/components/admin/SystemLayerVisibilityAdminSection'
import { DEMSettings } from '@/components/DEMSettings'
import { OfflineMapSettings } from '@/components/OfflineMapSettings'
import { OSMRegionManager, type OSMRegionManagerBatchUpdateProps } from '@/components/OSMRegionManager'
import OSMImportModal from '@/components/OSMImportModal'
import { osmCategoryLabel } from '@/lib/osmCategoryLabels'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureConfigured } from '@/lib/deploymentCapabilities'
import { useToast } from '@/lib/toast'
import { getToken } from '@/lib/auth'
import {
  fetchOSMStatus,
  importLifts,
  fetchOSMRegionStatus,
  getOSMSchedule,
  setOSMSchedule,
  type OSMSchedule,
} from '@/api/osm'

// ── Small presentational helpers ─────────────────────────────────────────────

type CapBadgeStatus = 'active' | 'inactive' | 'loading' | 'warning'

function CapBadge({ status, label }: { status: CapBadgeStatus; label: string }) {
  const containerClass =
    status === 'active'
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
      : status === 'warning'
        ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400'
        : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
  const dotClass =
    status === 'active'
      ? 'bg-emerald-500'
      : status === 'warning'
        ? 'bg-amber-500'
        : 'bg-neutral-400'
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${containerClass}`}>
      <span className={`h-1.5 w-1.5 rounded-full flex-shrink-0 ${dotClass}${status === 'loading' ? ' animate-pulse' : ''}`} />
      {label}
    </span>
  )
}

function OsmStatCard({ value, label, labelClassName }: { value: number; label: string; labelClassName?: string }) {
  return (
    <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
      <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{value.toLocaleString('de')}</div>
      <div className={`text-xs text-neutral-500 dark:text-neutral-400${labelClassName ? ` ${labelClassName}` : ''}`}>{label}</div>
    </div>
  )
}

// ── Main component ───────────────────────────────────────────────────────────

export default function AdminMapTilesSubTab() {
  const { addToast } = useToast()

  // Capabilities (powers Routing-Dienste badges + DEM badge)
  const { data: caps, isLoading: capsLoading } = useCapabilities()
  const osmImportConfigured = featureConfigured(caps, 'osm_import')
  const demConfigured = featureConfigured(caps, 'dem_correction')
  const offlineMapConfigured = featureConfigured(caps, 'offline_map')
  const tileProbeConfigured = featureConfigured(caps, 'routing_tiles') || featureConfigured(caps, 'system_layers_vector') || offlineMapConfigured

  // OSM data presence drives whether lift-import button is available
  const { data: osmStatus } = useQuery({
    queryKey: ['osm-status'],
    queryFn: fetchOSMStatus,
    enabled: osmImportConfigured,
  })
  const hasOsmData = osmStatus?.has_data ?? false

  const importLiftsMut = useMutation({ mutationFn: importLifts })

  // Local vector-tile server (configured via VITE_MARTIN_URL)
  const tileBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? '/api/v1/tiles'
  const styleUrl = `${tileBase}/style.json`
  const [tileStatus, setTileStatus] = useState<'loading' | 'ok' | 'unavailable'>('loading')
  const [tileSources, setTileSources] = useState<string[]>([])
  const [tileCheckKey, setTileCheckKey] = useState(0)

  useEffect(() => {
    if (!tileProbeConfigured) {
      setTileStatus('unavailable')
      setTileSources([])
      return
    }
    let cancelled = false
    setTileStatus('loading')
    setTileSources([])
    const token = getToken()
    fetch(styleUrl, {
      signal: AbortSignal.timeout(5000),
      ...(token ? { headers: { Authorization: `Bearer ${token}` } } : {}),
    })
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((json: { sources?: Record<string, unknown> }) => {
        if (cancelled) return
        setTileStatus('ok')
        if (json.sources && typeof json.sources === 'object') {
          setTileSources(Object.keys(json.sources))
        }
      })
      .catch(() => {
        if (!cancelled) setTileStatus('unavailable')
      })
    return () => {
      cancelled = true
    }
  }, [styleUrl, tileCheckKey, tileProbeConfigured])

  // OSM region list + schedule (only meaningful when at least one region is imported)
  const { data: regionData } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
    enabled: osmImportConfigured,
  })
  const importedRegions = regionData?.regions ?? []
  const hasImportedRegions = importedRegions.length > 0

  const [showImportModal, setShowImportModal] = useState(false)

  // OSMRegionManager pushes its "Alle Regionen aktualisieren" state up via useEffect
  // so we can render the button in the bottom action row alongside the other
  // OSM primary actions.
  const [batchUpdateProps, setBatchUpdateProps] = useState<OSMRegionManagerBatchUpdateProps | null>(null)

  const [schedInterval, setSchedInterval] = useState<string>('manual')
  const [schedWeekday, setSchedWeekday] = useState<number>(1)
  const [schedDay, setSchedDay] = useState<number>(1)
  const [schedHour, setSchedHour] = useState<number>(3)
  const [schedLoaded, setSchedLoaded] = useState(false)

  const { data: schedData } = useQuery<OSMSchedule>({
    queryKey: ['osm-schedule'],
    queryFn: getOSMSchedule,
    enabled: osmImportConfigured && hasImportedRegions,
  })

  useEffect(() => {
    if (schedData && !schedLoaded) {
      setSchedInterval(schedData.interval)
      setSchedWeekday(schedData.weekday)
      setSchedDay(schedData.day)
      setSchedHour(schedData.hour)
      setSchedLoaded(true)
    }
  }, [schedData, schedLoaded])

  function computeNextUpdate(interval: string, weekday: number, day: number, hour: number): string {
    const now = new Date()
    const next = new Date()
    next.setMinutes(0, 0, 0)
    next.setHours(hour)
    if (interval === 'weekly') {
      const isoWeekday = now.getDay() === 0 ? 7 : now.getDay()
      let daysUntil = weekday - isoWeekday
      if (daysUntil < 0 || (daysUntil === 0 && now.getHours() >= hour)) {
        daysUntil += 7
      }
      next.setDate(now.getDate() + daysUntil)
    } else if (interval === 'monthly') {
      next.setDate(day)
      if (next <= now) {
        next.setMonth(next.getMonth() + 1)
      }
    }
    return next.toLocaleDateString('de-DE', {
      weekday: 'long', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit',
    })
  }

  function saveSchedule(schedule: OSMSchedule) {
    setOSMSchedule(schedule).catch(() => {
      addToast({ type: 'error', title: 'Fehler beim Speichern des Update-Zeitplans', duration: 0 })
    })
  }

  const tileBadgeStatus: CapBadgeStatus =
    tileStatus === 'loading' ? 'loading' : tileStatus === 'ok' ? 'active' : 'warning'
  const tileBadgeLabel =
    tileStatus === 'loading' ? 'Prüfe…' : tileStatus === 'ok' ? 'Verbunden' : 'Nicht verfügbar'

  return (
    <div className="space-y-6">
      {/* 1. System map layer CRUD */}
      <MapLayersAdminSection />

      {/* 2. System overlay visibility */}
      <SystemLayerVisibilityAdminSection />

      {/* 3. Routing-Dienste */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            Routing-Dienste
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Verfügbare Routing-Engines und abhängige Karten-Features.
          </p>
        </div>
        {capsLoading ? (
          <p className="text-xs text-neutral-400">Prüfe Verfügbarkeit…</p>
        ) : (
          <>
            <div className="flex flex-wrap gap-2">
              <CapBadge status={caps?.brouter ? 'active' : 'inactive'}            label="BRouter" />
              <CapBadge status={caps?.badrouter ? 'active' : 'inactive'}          label="BadRouter" />
              <CapBadge status={caps?.isochrone ? 'active' : 'inactive'}          label="Isochrone" />
              <CapBadge status={caps?.dem_correction ? 'active' : 'inactive'}     label="DEM-Höhenkorrektur" />
              <CapBadge status={caps?.surface_enrichment ? 'active' : 'inactive'} label="Oberflächen-Anreicherung" />
              <CapBadge status={caps?.route_suggestion ? 'active' : 'inactive'}   label="Routen-Vorschläge" />
              <CapBadge status={caps?.offline_map ? 'active' : 'inactive'}        label="Offline-Karte" />
            </div>
            {!caps?.brouter && !caps?.badrouter && (
              <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
                Kein Routing-Dienst verfügbar. BRouter benötigt rd5-Segmente und .brf-Profile. BadRouter benötigt Routing-Tiles (mit DEM).
              </p>
            )}
          </>
        )}
      </section>

      {/* 4. Geodata region manager + BRouter segment download */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {osmImportConfigured ? 'OSM-Geodaten' : 'BRouter-Segmente'}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {osmImportConfigured
              ? 'Regionen importieren, Lift-Segmente aktualisieren und Statistiken einsehen.'
              : 'RD5-Segmente für das Core-Routing herunterladen.'}
          </p>
        </div>

        {/* OSM stats grid (POIs, Wege, Flächen) */}
        {osmImportConfigured && osmStatus && (
          <div className="space-y-3">
            {osmStatus.categories && Object.keys(osmStatus.categories).length > 0 && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">POIs</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {Object.entries(osmStatus.categories).map(([key, count]) => (
                    <OsmStatCard key={key} value={count} label={osmCategoryLabel(key)} />
                  ))}
                </div>
              </div>
            )}
            {(osmStatus.lifts > 0 || osmStatus.pistes > 0 || osmStatus.ways > 0 || osmStatus.routes > 0 || osmStatus.railways > 0) && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Wege</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {(
                    [
                      osmStatus.ways > 0     ? { value: osmStatus.ways,     label: 'Wege' }      : null,
                      osmStatus.routes > 0   ? { value: osmStatus.routes,   label: 'Routen' }    : null,
                      osmStatus.lifts > 0    ? { value: osmStatus.lifts,    label: 'Lifte' }     : null,
                      osmStatus.pistes > 0   ? { value: osmStatus.pistes,   label: 'Pisten' }    : null,
                      osmStatus.railways > 0 ? { value: osmStatus.railways, label: 'Eisenbahn' } : null,
                    ] as const
                  ).filter((item): item is NonNullable<typeof item> => item !== null).map(item => (
                    <OsmStatCard key={item.label} value={item.value} label={item.label} />
                  ))}
                </div>
              </div>
            )}
            {(osmStatus.waterways > 0 || osmStatus.forests > 0 || osmStatus.glaciers > 0 || osmStatus.wetlands > 0 || osmStatus.boundaries > 0 || osmStatus.landuse > 0) && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Flächen</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {(
                    [
                      osmStatus.waterways > 0  ? { value: osmStatus.waterways,  label: 'Gewässer' }      : null,
                      osmStatus.forests > 0    ? { value: osmStatus.forests,    label: 'Wälder' }        : null,
                      osmStatus.glaciers > 0   ? { value: osmStatus.glaciers,   label: 'Gletscher' }     : null,
                      osmStatus.wetlands > 0   ? { value: osmStatus.wetlands,   label: 'Feuchtgebiete' } : null,
                      osmStatus.boundaries > 0 ? { value: osmStatus.boundaries, label: 'Grenzen' }       : null,
                      osmStatus.landuse > 0    ? { value: osmStatus.landuse,    label: 'Landnutzung' }   : null,
                    ] as const
                  ).filter((item): item is NonNullable<typeof item> => item !== null).map(item => (
                    <OsmStatCard key={item.label} value={item.value} label={item.label} />
                  ))}
                </div>
              </div>
            )}
            {!osmStatus.has_data && (
              <p className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-2">Keine OSM-Daten importiert.</p>
            )}
          </div>
        )}

        <OSMRegionManager
          hasOsmData={hasOsmData}
          fullGeodataAvailable={osmImportConfigured}
          onBatchUpdateState={osmImportConfigured ? setBatchUpdateProps : undefined}
        />

        {/* OSM update scheduler — only meaningful with at least one imported region */}
        {osmImportConfigured && hasImportedRegions && (
          <div className="space-y-3 pt-3 border-t border-neutral-200 dark:border-neutral-700">
            <h4 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              Automatische Karten-Updates
            </h4>
            <div className="flex flex-wrap gap-3 items-center text-sm">
              <div className="flex flex-col gap-1">
                <label className="text-xs text-neutral-500 dark:text-neutral-400">Intervall</label>
                <select
                  value={schedInterval}
                  onChange={e => {
                    const v = e.target.value
                    setSchedInterval(v)
                    saveSchedule({ interval: v, weekday: schedWeekday, day: schedDay, hour: schedHour })
                  }}
                  className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                >
                  <option value="manual">Aus (manuell)</option>
                  <option value="weekly">Wöchentlich</option>
                  <option value="monthly">Monatlich</option>
                </select>
              </div>

              {schedInterval === 'weekly' && (
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">Wochentag</label>
                  <select
                    value={schedWeekday}
                    onChange={e => {
                      const v = Number(e.target.value)
                      setSchedWeekday(v)
                      saveSchedule({ interval: schedInterval, weekday: v, day: schedDay, hour: schedHour })
                    }}
                    className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                  >
                    <option value={1}>Montag</option>
                    <option value={2}>Dienstag</option>
                    <option value={3}>Mittwoch</option>
                    <option value={4}>Donnerstag</option>
                    <option value={5}>Freitag</option>
                    <option value={6}>Samstag</option>
                    <option value={7}>Sonntag</option>
                  </select>
                </div>
              )}

              {schedInterval === 'monthly' && (
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">Tag des Monats</label>
                  <select
                    value={schedDay}
                    onChange={e => {
                      const v = Number(e.target.value)
                      setSchedDay(v)
                      saveSchedule({ interval: schedInterval, weekday: schedWeekday, day: v, hour: schedHour })
                    }}
                    className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                  >
                    {Array.from({ length: 28 }, (_, i) => (
                      <option key={i + 1} value={i + 1}>{i + 1}.</option>
                    ))}
                  </select>
                </div>
              )}

              {schedInterval !== 'manual' && (
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">Uhrzeit</label>
                  <select
                    value={schedHour}
                    onChange={e => {
                      const v = Number(e.target.value)
                      setSchedHour(v)
                      saveSchedule({ interval: schedInterval, weekday: schedWeekday, day: schedDay, hour: v })
                    }}
                    className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                  >
                    {Array.from({ length: 24 }, (_, i) => (
                      <option key={i} value={i}>{String(i).padStart(2, '0')}:00</option>
                    ))}
                  </select>
                </div>
              )}
            </div>
            {schedInterval !== 'manual' && (
              <p className="text-sm text-neutral-500">
                Nächstes Update: {computeNextUpdate(schedInterval, schedWeekday, schedDay, schedHour)}
              </p>
            )}
            {schedInterval !== 'manual' && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                Automatische Updates werden in einer zukünftigen Version aktiviert. Aktuell dient diese Einstellung nur zur Planung.
              </p>
            )}
          </div>
        )}

        {/* Bottom-anchored primary-action row — all OSM data-modifying actions live here */}
        {osmImportConfigured && (
          <>
            <div className="space-y-2 pt-3 border-t border-neutral-200 dark:border-neutral-700">
              <button
                type="button"
                onClick={() => setShowImportModal(true)}
                className="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 cursor-pointer transition-colors"
              >
                Import-Assistent
              </button>
              {batchUpdateProps?.visible && (
                <button
                  type="button"
                  onClick={batchUpdateProps.onClick}
                  disabled={batchUpdateProps.disabled}
                  className="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                >
                  {batchUpdateProps.label}
                </button>
              )}
              <button
                type="button"
                onClick={() => importLiftsMut.mutate()}
                disabled={importLiftsMut.isPending || !hasOsmData}
                className="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
              >
                {importLiftsMut.isPending ? 'Importiere…' : 'Lift-Segmente importieren'}
              </button>
            </div>
            {importLiftsMut.isSuccess && (
              <p className="text-xs text-emerald-600 dark:text-emerald-400">
                {importLiftsMut.data.imported} Lift-Segmente importiert.
              </p>
            )}
          </>
        )}
      </section>

      {/* 5. DEM height data — narrative content first, primary action anchored at bottom of DEMSettings */}
      {demConfigured && (
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
              Höhendaten (DEM)
            </h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              Digitales Höhenmodell für Höhenkorrektur und Routing.
            </p>
          </div>
          <CapBadge status={caps?.dem_correction ? 'active' : 'inactive'} label="DEM aktiv" />
        </div>
        <DEMSettings />
      </section>
      )}

      {/* 6. Offline-Karte — combined card: tile-server status + PMTiles basemap as two sub-blocks */}
      {(offlineMapConfigured || tileProbeConfigured) && (
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-5">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            Offline-Karte
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            Quellen für Vektorkacheln und Offline-Basemap.
          </p>
        </div>

        {/* Sub-block A: local vector-tile server (status fetched from style.json) */}
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3 flex-wrap">
            <h4 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              Vektorkachel-Server
            </h4>
            <div className="flex items-center gap-3 flex-wrap">
              <CapBadge status={tileBadgeStatus} label={tileBadgeLabel} />
              {tileStatus !== 'loading' && (
                <button
                  onClick={() => setTileCheckKey(k => k + 1)}
                  className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 underline underline-offset-2 cursor-pointer"
                >
                  Erneut prüfen
                </button>
              )}
            </div>
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
            Lokaler Vektorkachel-Server, serviert Kacheln direkt aus PostGIS. URL via{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">VITE_MARTIN_URL</code>{' '}
            (Standard:{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">/api/v1/tiles</code>).
          </p>
          <p className="text-xs text-neutral-400 dark:text-neutral-500 font-mono break-all">{styleUrl}</p>
          {tileStatus === 'ok' && tileSources.length > 0 && (
            <div>
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1.5">
                Tile-Quellen ({tileSources.length})
              </p>
              <ul className="space-y-1">
                {tileSources.map(src => (
                  <li key={src} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 flex-shrink-0" />
                    <code className="font-mono">{src}</code>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>

        {/* Sub-block B: PMTiles offline basemap */}
        <div className="space-y-3 pt-4 border-t border-neutral-200 dark:border-neutral-700">
          <div>
            <h4 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              Offline-Basemap (PMTiles)
            </h4>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              PMTiles-Datei als Offline-Basemap für die Kartenansicht.
            </p>
          </div>
          <OfflineMapSettings />
        </div>
      </section>
      )}

      {osmImportConfigured && showImportModal && <OSMImportModal onClose={() => setShowImportModal(false)} />}
    </div>
  )
}
