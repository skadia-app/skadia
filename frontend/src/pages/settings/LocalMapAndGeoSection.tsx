import { useState, useEffect } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import OSMImportModal from '@/components/OSMImportModal'
import { DEMSettings } from '@/components/DEMSettings'
import { OfflineMapSettings } from '@/components/OfflineMapSettings'
import { OSMRegionManager } from '@/components/OSMRegionManager'
import { useToast } from '@/lib/toast'
import { getCapabilities, type Capabilities } from '@/api/routes'
import {
  type OSMStatus,
  getOSMSchedule,
  setOSMSchedule,
  type OSMSchedule,
  fetchOSMRegionStatus,
  fetchOSMStatus,
  importLifts,
} from '@/api/osm'

function CapBadge({ active, label }: { active: boolean; label: string }) {
  return (
    <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium ${
      active
        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
        : 'bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400'
    }`}>
      <span className={`h-1.5 w-1.5 rounded-full flex-shrink-0 ${active ? 'bg-emerald-500' : 'bg-neutral-400'}`} />
      {label}
    </span>
  )
}

export function LocalMapAndGeoSection() {
  const martinBase = (import.meta.env.VITE_MARTIN_URL as string | undefined) ?? '/tiles'
  const styleUrl = `${martinBase}/style.json`

  // Martin status
  const [martinStatus, setMartinStatus] = useState<'loading' | 'ok' | 'unavailable'>('loading')
  const [martinSources, setMartinSources] = useState<string[]>([])
  const [checkKey, setCheckKey] = useState(0)

  useEffect(() => {
    let cancelled = false
    setMartinStatus('loading')
    setMartinSources([])
    fetch(styleUrl, { signal: AbortSignal.timeout(5000) })
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then((json: { sources?: Record<string, unknown> }) => {
        if (cancelled) return
        setMartinStatus('ok')
        if (json.sources && typeof json.sources === 'object') {
          setMartinSources(Object.keys(json.sources))
        }
      })
      .catch(() => {
        if (!cancelled) setMartinStatus('unavailable')
      })
    return () => { cancelled = true }
  }, [styleUrl, checkKey])

  // Capabilities
  const { data: caps, isLoading: capsLoading } = useQuery<Capabilities>({
    queryKey: ['capabilities'],
    queryFn: getCapabilities,
    staleTime: 60_000,
  })

  // OSM status
  const [showImportModal, setShowImportModal] = useState(false)
  const { data: osmStatus } = useQuery<OSMStatus>({
    queryKey: ['osm-status'],
    queryFn: fetchOSMStatus,
  })
  const importLiftsMut = useMutation({
    mutationFn: importLifts,
  })
  const hasOsmData = osmStatus?.has_data ?? false

  // OSM region list (to know if any regions are imported)
  const { data: regionData } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
  })
  const importedRegions = regionData?.regions ?? []
  const hasImportedRegions = importedRegions.length > 0

  const { addToast } = useToast()

  // OSM scheduler
  const [schedInterval, setSchedInterval] = useState<string>('manual')
  const [schedWeekday, setSchedWeekday] = useState<number>(1)
  const [schedDay, setSchedDay] = useState<number>(1)
  const [schedHour, setSchedHour] = useState<number>(3)
  const [schedLoaded, setSchedLoaded] = useState(false)

  const { data: schedData } = useQuery<OSMSchedule>({
    queryKey: ['osm-schedule'],
    queryFn: getOSMSchedule,
    enabled: hasImportedRegions,
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
    const now = new Date();
    const next = new Date();
    next.setMinutes(0, 0, 0);
    next.setHours(hour);

    if (interval === 'weekly') {
      const isoWeekday = now.getDay() === 0 ? 7 : now.getDay();
      let daysUntil = weekday - isoWeekday;
      if (daysUntil < 0 || (daysUntil === 0 && now.getHours() >= hour)) {
        daysUntil += 7;
      }
      next.setDate(now.getDate() + daysUntil);
    } else if (interval === 'monthly') {
      next.setDate(day);
      if (next <= now) {
        next.setMonth(next.getMonth() + 1);
      }
    }

    return next.toLocaleDateString('de-DE', { weekday: 'long', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' });
  }

  function saveSchedule(patch: Partial<OSMSchedule>) {
    const schedule: OSMSchedule = {
      interval: schedInterval,
      weekday: schedWeekday,
      day: schedDay,
      hour: schedHour,
      ...patch,
    }
    setOSMSchedule(schedule).catch(() => {
      addToast({ type: 'error', title: 'Fehler beim Speichern des Update-Zeitplans', duration: 0 })
    })
  }

  const martinBadge =
    martinStatus === 'loading' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-neutral-100 text-neutral-500 dark:bg-neutral-800 dark:text-neutral-400">
        <span className="h-1.5 w-1.5 rounded-full bg-neutral-400 animate-pulse" />
        Prüfe…
      </span>
    ) : martinStatus === 'ok' ? (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400">
        <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
        Verbunden
      </span>
    ) : (
      <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-medium bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400">
        <span className="h-1.5 w-1.5 rounded-full bg-amber-500" />
        Nicht verfügbar
      </span>
    )

  return (
    <section className="border border-neutral-200 dark:border-neutral-800 rounded-xl p-6 space-y-6">
      {/* Header */}
      <div>
        <h2 className="text-lg font-medium text-neutral-900 dark:text-neutral-100">Lokale Karten &amp; Geodaten</h2>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Routing, Offline-Karte (Martin), OSM-Daten und Höhenmodell.
        </p>
      </div>

      {/* ── Routing-Dienste ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Routing-Dienste</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {capsLoading ? (
            <p className="text-xs text-neutral-400">Prüfe Verfügbarkeit…</p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                <CapBadge active={caps?.routing ?? false}           label="Routing (BRouter)" />
                <CapBadge active={caps?.isochrone ?? false}         label="Isochrone" />
                <CapBadge active={caps?.dem_correction ?? false}    label="DEM-Höhenkorrektur" />
                <CapBadge active={caps?.surface_enrichment ?? false} label="Oberflächen-Anreicherung" />
                <CapBadge active={caps?.route_suggestion ?? false}  label="Routen-Vorschläge" />
                <CapBadge active={caps?.offline_map ?? false}       label="Offline-Karte" />
              </div>
              {!(caps?.routing) && (
                <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
                  Routing ist nicht verfügbar. Stelle sicher, dass der BRouter-Dienst gestartet ist und OSM-Daten importiert wurden.
                </p>
              )}
            </>
          )}
        </div>
      </div>

      {/* ── DEM-Höhendaten ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">DEM-Höhendaten</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-2">
          <CapBadge active={caps?.dem_correction ?? false} label="DEM aktiv" />
          <DEMSettings />
        </div>
      </div>

      {/* ── Offline-Karte (Martin) ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Offline-Karte (Martin Tile Server)</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3">
          {/* Status line */}
          <div className="flex items-center gap-3 flex-wrap">
            {martinBadge}
            <span className="text-xs text-neutral-400 dark:text-neutral-500 font-mono break-all">{styleUrl}</span>
            {martinStatus !== 'loading' && (
              <button
                onClick={() => setCheckKey(k => k + 1)}
                className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 underline underline-offset-2 cursor-pointer"
              >
                Erneut prüfen
              </button>
            )}
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
            Der Martin Tile Server serviert Vektorkacheln aus PostGIS direkt und ermöglicht vollständigen
            Offline-Betrieb. URL via{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">VITE_MARTIN_URL</code>{' '}
            (Standard: <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">/tiles</code>).
          </p>
          {/* Available tile sources */}
          {martinStatus === 'ok' && martinSources.length > 0 && (
            <div>
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1.5">
                Tile-Quellen ({martinSources.length})
              </p>
              <ul className="space-y-1">
                {martinSources.map(src => (
                  <li key={src} className="flex items-center gap-2 text-xs text-neutral-500 dark:text-neutral-400">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 flex-shrink-0" />
                    <code className="font-mono">{src}</code>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {/* Hint */}
          <div className="flex items-start gap-2 rounded-lg bg-accent-100/40 dark:bg-accent-DEFAULT/10 border border-accent-200/60 dark:border-accent-800/30 px-3 py-2.5">
            <svg className="h-4 w-4 text-accent-DEFAULT mt-0.5 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M12 2a10 10 0 100 20A10 10 0 0012 2z" />
            </svg>
            <p className="text-xs text-neutral-600 dark:text-neutral-400 leading-relaxed">
              Den Lokal-Stil als Standardkarte einrichten: Wähle{' '}
              <strong className="text-neutral-800 dark:text-neutral-200">Lokal (Offline)</strong>{' '}
              im Dropdown <em>Standard-Kartenstil</em> im Abschnitt{' '}
              <strong className="text-neutral-800 dark:text-neutral-200">Karteneinstellungen</strong> oben.
              {martinStatus !== 'ok' && (
                <span className="text-amber-600 dark:text-amber-400">
                  {' '}Der Martin Server muss erreichbar sein.
                </span>
              )}
            </p>
          </div>
        </div>
      </div>

      {/* ── Offline-Karte (Protomaps PMTiles) ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">Offline-Karte (Protomaps)</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4">
          <OfflineMapSettings />
        </div>
      </div>

      {/* ── OSM-Geodaten ── */}
      <div className="space-y-3">
        <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">OSM-Geodaten</h3>
        <div className="rounded-lg border border-neutral-200 dark:border-neutral-700 p-4 space-y-3 text-sm">
          {/* Stats grid — POIs */}
          {osmStatus && (
            <div className="space-y-3">
              {/* POI-Kategorien */}
              {osmStatus.categories && Object.keys(osmStatus.categories).length > 0 && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">POIs</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {Object.entries(osmStatus.categories).map(([key, count]) => (
                      <div key={key} className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{count.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400 capitalize">{key}</div>
                      </div>
                    ))}
                    {/* Legacy fallback when categories is empty */}
                    {Object.keys(osmStatus.categories).length === 0 && (
                      <>
                        {osmStatus.summits !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.summits.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Gipfel</div></div>}
                        {osmStatus.passes !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.passes.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Pässe</div></div>}
                        {osmStatus.settlements !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.settlements.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Ortschaften</div></div>}
                        {osmStatus.amenities !== undefined && <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1"><div className="text-base font-bold">{osmStatus.amenities.toLocaleString('de')}</div><div className="text-xs text-neutral-500">Einkehr</div></div>}
                      </>
                    )}
                  </div>
                </div>
              )}
              {/* Wege */}
              {(osmStatus.lifts > 0 || osmStatus.pistes > 0 || osmStatus.ways > 0 || osmStatus.routes > 0 || osmStatus.railways > 0) && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Wege</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {osmStatus.ways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.ways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Wege</div>
                      </div>
                    )}
                    {osmStatus.routes > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.routes.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Routen</div>
                      </div>
                    )}
                    {osmStatus.lifts > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.lifts.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Lifte</div>
                      </div>
                    )}
                    {osmStatus.pistes > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.pistes.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Pisten</div>
                      </div>
                    )}
                    {osmStatus.railways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.railways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Eisenbahn</div>
                      </div>
                    )}
                  </div>
                </div>
              )}
              {/* Flächen */}
              {(osmStatus.waterways > 0 || osmStatus.forests > 0 || osmStatus.glaciers > 0 || osmStatus.wetlands > 0 || osmStatus.boundaries > 0 || osmStatus.landuse > 0) && (
                <div>
                  <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">Flächen</p>
                  <div className="grid grid-cols-3 gap-2 text-center">
                    {osmStatus.waterways > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.waterways.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Gewässer</div>
                      </div>
                    )}
                    {osmStatus.forests > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.forests.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Wälder</div>
                      </div>
                    )}
                    {osmStatus.glaciers > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.glaciers.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Gletscher</div>
                      </div>
                    )}
                    {osmStatus.wetlands > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.wetlands.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Feuchtgebiete</div>
                      </div>
                    )}
                    {osmStatus.boundaries > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.boundaries.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Grenzen</div>
                      </div>
                    )}
                    {osmStatus.landuse > 0 && (
                      <div className="bg-neutral-50 dark:bg-neutral-800/50 rounded-lg py-2 px-1">
                        <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{osmStatus.landuse.toLocaleString('de')}</div>
                        <div className="text-xs text-neutral-500 dark:text-neutral-400">Landnutzung</div>
                      </div>
                    )}
                  </div>
                </div>
              )}
              {/* Fallback: no data yet */}
              {!osmStatus.has_data && (
                <p className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-2">Keine OSM-Daten importiert.</p>
              )}
            </div>
          )}
          {/* Importierte Regionen */}
          <OSMRegionManager hasOsmData={hasOsmData} />

          {/* ── Automatische Karten-Updates (Scheduler) ── */}
          {hasImportedRegions && (
            <div className="space-y-3 pt-2 border-t border-neutral-200 dark:border-neutral-700">
              <h3 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
                Automatische Karten-Updates
              </h3>
              <div className="flex flex-wrap gap-3 items-center text-sm">
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">Intervall</label>
                  <select
                    value={schedInterval}
                    onChange={e => {
                      setSchedInterval(e.target.value)
                      saveSchedule({ interval: e.target.value })
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
                        saveSchedule({ weekday: v })
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
                        saveSchedule({ day: v })
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
                        saveSchedule({ hour: v })
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
                <p className="text-xs text-amber-600 dark:text-amber-400 mt-2">
                  Automatische Updates werden in einer zukünftigen Version aktiviert. Aktuell dient diese Einstellung nur zur Planung.
                </p>
              )}
            </div>
          )}

          {/* Buttons */}
          <div className="flex flex-col gap-2">
            <button
              onClick={() => setShowImportModal(true)}
              className="w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 transition-colors cursor-pointer"
            >
              Import-Assistent
            </button>
            <button
              onClick={() => importLiftsMut.mutate()}
              disabled={importLiftsMut.isPending || !hasOsmData}
              className="w-full py-2 text-sm font-medium rounded-lg bg-accent-DEFAULT text-white hover:bg-accent-600 disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer transition-colors"
            >
              {importLiftsMut.isPending ? 'Importiere…' : 'Lift-Segmente importieren'}
            </button>
          </div>
          {importLiftsMut.isSuccess && (
            <p className="text-xs text-emerald-600 dark:text-emerald-400">
              {importLiftsMut.data.imported} Lift-Segmente importiert.
            </p>
          )}
        </div>
      </div>

      {showImportModal && <OSMImportModal onClose={() => setShowImportModal(false)} />}
    </section>
  )
}
