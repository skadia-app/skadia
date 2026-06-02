// Admin sub-tab bundling all map/tiles admin sections.
// Sections in order (grouped by data flow: raw geodata first, derived layers next):
//   1. MapLayersAdminSection             — system layer CRUD
//   2. SystemLayerVisibilityAdminSection — overlay-key toggles
//   3. Routing services                  — capability badges
//   4. OSM geodata                       — stats, regions, scheduler, primary-action row
//   5. Elevation data (DEM)              — coverage + bottom-anchored download action
//   6. Offline map                       — combined tile server + PMTiles card
import { useEffect, useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { MapLayersAdminSection } from '@/components/admin/MapLayersAdminSection'
import { SystemLayerVisibilityAdminSection } from '@/components/admin/SystemLayerVisibilityAdminSection'
import { DEMSettings } from '@/components/DEMSettings'
import { OfflineMapSettings } from '@/components/OfflineMapSettings'
import { OSMRegionManager, type OSMRegionManagerBatchUpdateProps } from '@/components/OSMRegionManager'
import OSMImportModal from '@/components/OSMImportModal'
import { osmCategoryLabel } from '@/lib/osmCategoryLabels'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable, featureConfigured } from '@/lib/deploymentCapabilities'
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
      <div className="text-base font-bold text-neutral-900 dark:text-neutral-100">{value.toLocaleString()}</div>
      <div className={`text-xs text-neutral-500 dark:text-neutral-400${labelClassName ? ` ${labelClassName}` : ''}`}>{label}</div>
    </div>
  )
}

// ── Main component ───────────────────────────────────────────────────────────

export default function AdminMapTilesSubTab() {
  const { t, i18n } = useTranslation()
  const { addToast } = useToast()

  // Capabilities (powers routing-service badges + DEM badge)
  const { data: caps, isLoading: capsLoading } = useCapabilities()
  const osmImportConfigured = featureConfigured(caps, 'osm_import')
  const osmImportAvailable = featureAvailable(caps, 'osm_import')
  const osmImportEnabled = osmImportConfigured
  const demConfigured = featureConfigured(caps, 'dem_correction')
  const demAvailable = featureAvailable(caps, 'dem_correction')
  const offlineMapConfigured = featureConfigured(caps, 'offline_map')
  const offlineMapAvailable = featureAvailable(caps, 'offline_map')
  const brouterAvailable = featureAvailable(caps, 'brouter')
  const badrouterAvailable = featureAvailable(caps, 'badrouter')
  const isochroneAvailable = featureAvailable(caps, 'isochrone')
  const surfaceEnrichmentAvailable = featureAvailable(caps, 'surface_enrichment')
  const routeSuggestionAvailable = featureAvailable(caps, 'route_suggestion')
  const segmentLiftImportAvailable = featureAvailable(caps, 'segment_osm_lift_import')
  const tileProbeConfigured = featureConfigured(caps, 'routing_tiles') || featureConfigured(caps, 'system_layers_vector') || offlineMapConfigured
  const tileProbeAvailable = featureAvailable(caps, 'routing_tiles') || featureAvailable(caps, 'system_layers_vector') || offlineMapAvailable

  // OSM data presence drives whether lift-import button is available
  const { data: osmStatus } = useQuery({
    queryKey: ['osm-status'],
    queryFn: fetchOSMStatus,
    enabled: osmImportEnabled,
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
    if (!tileProbeAvailable) {
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
  }, [styleUrl, tileCheckKey, tileProbeAvailable])

  // OSM region list + schedule (only meaningful when at least one region is imported)
  const { data: regionData } = useQuery({
    queryKey: ['osm-regions-status'],
    queryFn: fetchOSMRegionStatus,
    enabled: osmImportEnabled,
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
    enabled: osmImportEnabled && hasImportedRegions,
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
    return next.toLocaleDateString(i18n.language, {
      weekday: 'long', day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit',
    })
  }

  function saveSchedule(schedule: OSMSchedule) {
    setOSMSchedule(schedule).catch(() => {
      addToast({ type: 'error', title: t('settings.adminMapTiles.scheduleSaveError'), duration: 0 })
    })
  }

  const tileBadgeStatus: CapBadgeStatus =
    tileStatus === 'loading' ? 'loading' : tileStatus === 'ok' ? 'active' : 'warning'
  const tileBadgeLabel =
    tileStatus === 'loading'
      ? t('settings.adminMapTiles.tileStatus.checking')
      : tileStatus === 'ok'
        ? t('settings.adminMapTiles.tileStatus.connected')
        : t('settings.adminMapTiles.tileStatus.unavailable')

  return (
    <div className="space-y-6">
      {/* 1. System map layer CRUD */}
      <MapLayersAdminSection />

      {/* 2. System overlay visibility */}
      <SystemLayerVisibilityAdminSection />

      {/* 3. Routing services */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-3">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {t('settings.adminMapTiles.routing.title')}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {t('settings.adminMapTiles.routing.description')}
          </p>
        </div>
        {capsLoading ? (
          <p className="text-xs text-neutral-400">{t('settings.adminMapTiles.checkingAvailability')}</p>
        ) : (
          <>
            <div className="flex flex-wrap gap-2">
              <CapBadge status={brouterAvailable ? 'active' : 'inactive'}           label="BRouter" />
              <CapBadge status={badrouterAvailable ? 'active' : 'inactive'}         label="BadRouter" />
              <CapBadge status={isochroneAvailable ? 'active' : 'inactive'}         label={t('settings.adminMapTiles.capabilities.isochrone')} />
              <CapBadge status={demAvailable ? 'active' : 'inactive'}               label={t('settings.adminMapTiles.capabilities.demCorrection')} />
              <CapBadge status={surfaceEnrichmentAvailable ? 'active' : 'inactive'} label={t('settings.adminMapTiles.capabilities.surfaceEnrichment')} />
              <CapBadge status={routeSuggestionAvailable ? 'active' : 'inactive'}   label={t('settings.adminMapTiles.capabilities.routeSuggestion')} />
              <CapBadge status={offlineMapAvailable ? 'active' : 'inactive'}        label={t('settings.adminMapTiles.capabilities.offlineMap')} />
            </div>
            {!brouterAvailable && !badrouterAvailable && (
              <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
                {t('settings.adminMapTiles.routing.unavailableHint')}
              </p>
            )}
          </>
        )}
      </section>

      {/* 4. Geodata region manager + BRouter segment download */}
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-4">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {osmImportConfigured ? t('settings.adminMapTiles.osm.title') : t('settings.adminMapTiles.osm.brouterTitle')}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {osmImportConfigured
              ? t('settings.adminMapTiles.osm.description')
              : t('settings.adminMapTiles.osm.brouterDescription')}
          </p>
        </div>

        {/* OSM stats grid */}
        {osmImportConfigured && osmStatus && (
          <div className="space-y-3">
            {osmStatus.categories && Object.keys(osmStatus.categories).length > 0 && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">{t('settings.adminMapTiles.osm.stats.pois')}</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {Object.entries(osmStatus.categories).map(([key, count]) => (
                    <OsmStatCard key={key} value={count} label={osmCategoryLabel(key)} />
                  ))}
                </div>
              </div>
            )}
            {(osmStatus.lifts > 0 || osmStatus.pistes > 0 || osmStatus.ways > 0 || osmStatus.routes > 0 || osmStatus.railways > 0) && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">{t('settings.adminMapTiles.osm.stats.paths')}</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {(
                    [
                      osmStatus.ways > 0     ? { value: osmStatus.ways,     label: t('settings.adminMapTiles.osm.stats.ways') }      : null,
                      osmStatus.routes > 0   ? { value: osmStatus.routes,   label: t('settings.adminMapTiles.osm.stats.routes') }    : null,
                      osmStatus.lifts > 0    ? { value: osmStatus.lifts,    label: t('settings.adminMapTiles.osm.stats.lifts') }     : null,
                      osmStatus.pistes > 0   ? { value: osmStatus.pistes,   label: t('settings.adminMapTiles.osm.stats.pistes') }    : null,
                      osmStatus.railways > 0 ? { value: osmStatus.railways, label: t('settings.adminMapTiles.osm.stats.railways') } : null,
                    ] as const
                  ).filter((item): item is NonNullable<typeof item> => item !== null).map(item => (
                    <OsmStatCard key={item.label} value={item.value} label={item.label} />
                  ))}
                </div>
              </div>
            )}
            {(osmStatus.waterways > 0 || osmStatus.forests > 0 || osmStatus.glaciers > 0 || osmStatus.wetlands > 0 || osmStatus.boundaries > 0 || osmStatus.landuse > 0) && (
              <div>
                <p className="text-xs font-semibold text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">{t('settings.adminMapTiles.osm.stats.areas')}</p>
                <div className="grid grid-cols-3 gap-2 text-center">
                  {(
                    [
                      osmStatus.waterways > 0  ? { value: osmStatus.waterways,  label: t('settings.adminMapTiles.osm.stats.waterways') }      : null,
                      osmStatus.forests > 0    ? { value: osmStatus.forests,    label: t('settings.adminMapTiles.osm.stats.forests') }        : null,
                      osmStatus.glaciers > 0   ? { value: osmStatus.glaciers,   label: t('settings.adminMapTiles.osm.stats.glaciers') }     : null,
                      osmStatus.wetlands > 0   ? { value: osmStatus.wetlands,   label: t('settings.adminMapTiles.osm.stats.wetlands') } : null,
                      osmStatus.boundaries > 0 ? { value: osmStatus.boundaries, label: t('settings.adminMapTiles.osm.stats.boundaries') }       : null,
                      osmStatus.landuse > 0    ? { value: osmStatus.landuse,    label: t('settings.adminMapTiles.osm.stats.landuse') }   : null,
                    ] as const
                  ).filter((item): item is NonNullable<typeof item> => item !== null).map(item => (
                    <OsmStatCard key={item.label} value={item.value} label={item.label} />
                  ))}
                </div>
              </div>
            )}
            {!osmStatus.has_data && (
              <p className="text-xs text-neutral-400 dark:text-neutral-500 text-center py-2">{t('settings.adminMapTiles.osm.empty')}</p>
            )}
          </div>
        )}

        <OSMRegionManager
          hasOsmData={hasOsmData}
          fullGeodataAvailable={osmImportEnabled}
          onBatchUpdateState={osmImportEnabled ? setBatchUpdateProps : undefined}
        />
        {osmImportConfigured && !osmImportAvailable && (
          <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
            {t('settings.adminMapTiles.osm.missingArtifacts')}
          </p>
        )}

        {/* OSM update scheduler — only meaningful with at least one imported region */}
        {osmImportEnabled && hasImportedRegions && (
          <div className="space-y-3 pt-3 border-t border-neutral-200 dark:border-neutral-700">
            <h4 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              {t('settings.adminMapTiles.schedule.title')}
            </h4>
            <div className="flex flex-wrap gap-3 items-center text-sm">
              <div className="flex flex-col gap-1">
                <label className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminMapTiles.schedule.interval')}</label>
                <select
                  value={schedInterval}
                  onChange={e => {
                    const v = e.target.value
                    setSchedInterval(v)
                    saveSchedule({ interval: v, weekday: schedWeekday, day: schedDay, hour: schedHour })
                  }}
                  className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                >
                  <option value="manual">{t('settings.adminMapTiles.schedule.options.manual')}</option>
                  <option value="weekly">{t('settings.adminMapTiles.schedule.options.weekly')}</option>
                  <option value="monthly">{t('settings.adminMapTiles.schedule.options.monthly')}</option>
                </select>
              </div>

              {schedInterval === 'weekly' && (
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminMapTiles.schedule.weekday')}</label>
                  <select
                    value={schedWeekday}
                    onChange={e => {
                      const v = Number(e.target.value)
                      setSchedWeekday(v)
                      saveSchedule({ interval: schedInterval, weekday: v, day: schedDay, hour: schedHour })
                    }}
                    className="rounded-md border border-neutral-300 dark:border-neutral-600 bg-white dark:bg-neutral-800 text-neutral-900 dark:text-neutral-100 px-2 py-1.5 text-sm focus:outline-none focus:ring-1 focus:ring-accent-DEFAULT"
                  >
                    <option value={1}>{t('settings.adminMapTiles.schedule.weekdays.monday')}</option>
                    <option value={2}>{t('settings.adminMapTiles.schedule.weekdays.tuesday')}</option>
                    <option value={3}>{t('settings.adminMapTiles.schedule.weekdays.wednesday')}</option>
                    <option value={4}>{t('settings.adminMapTiles.schedule.weekdays.thursday')}</option>
                    <option value={5}>{t('settings.adminMapTiles.schedule.weekdays.friday')}</option>
                    <option value={6}>{t('settings.adminMapTiles.schedule.weekdays.saturday')}</option>
                    <option value={7}>{t('settings.adminMapTiles.schedule.weekdays.sunday')}</option>
                  </select>
                </div>
              )}

              {schedInterval === 'monthly' && (
                <div className="flex flex-col gap-1">
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminMapTiles.schedule.dayOfMonth')}</label>
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
                  <label className="text-xs text-neutral-500 dark:text-neutral-400">{t('settings.adminMapTiles.schedule.time')}</label>
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
                {t('settings.adminMapTiles.schedule.nextUpdate', { date: computeNextUpdate(schedInterval, schedWeekday, schedDay, schedHour) })}
              </p>
            )}
            {schedInterval !== 'manual' && (
              <p className="text-xs text-amber-600 dark:text-amber-400">
                {t('settings.adminMapTiles.schedule.futureHint')}
              </p>
            )}
          </div>
        )}

        {/* Bottom-anchored primary-action row — all OSM data-modifying actions live here */}
        {osmImportEnabled && (
          <>
            <div className="space-y-2 pt-3 border-t border-neutral-200 dark:border-neutral-700">
              <button
                type="button"
                onClick={() => setShowImportModal(true)}
                className="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 cursor-pointer transition-colors"
              >
                {t('settings.adminMapTiles.osm.importAssistant')}
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
              {segmentLiftImportAvailable && (
                <button
                  type="button"
                  onClick={() => importLiftsMut.mutate()}
                  disabled={importLiftsMut.isPending || !hasOsmData}
                  className="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-accent-DEFAULT hover:bg-accent-600 disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer transition-colors"
                >
                  {importLiftsMut.isPending ? t('settings.adminMapTiles.osm.importing') : t('settings.adminMapTiles.osm.importLiftSegments')}
                </button>
              )}
            </div>
            {importLiftsMut.isSuccess && (
              <p className="text-xs text-emerald-600 dark:text-emerald-400">
                {t('settings.adminMapTiles.osm.importedLiftSegments', { count: importLiftsMut.data.imported })}
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
              {t('settings.adminMapTiles.dem.title')}
            </h3>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {t('settings.adminMapTiles.dem.description')}
            </p>
          </div>
          <CapBadge
            status={demAvailable ? 'active' : 'warning'}
            label={demAvailable ? t('settings.adminMapTiles.dem.active') : t('settings.adminMapTiles.tileStatus.unavailable')}
          />
        </div>
        <DEMSettings />
        {!demAvailable && (
          <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
            {t('settings.adminMapTiles.dem.missingArtifacts')}
          </p>
        )}
      </section>
      )}

      {/* 6. Offline map: combined card with tile-server status and PMTiles basemap */}
      {(offlineMapConfigured || tileProbeConfigured) && (
      <section className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-5 space-y-5">
        <div>
          <h3 className="text-sm font-medium text-neutral-900 dark:text-neutral-100">
            {t('settings.adminMapTiles.offline.title')}
          </h3>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
            {t('settings.adminMapTiles.offline.description')}
          </p>
        </div>

        {/* Sub-block A: local vector-tile server (status fetched from style.json) */}
        <div className="space-y-3">
          <div className="flex items-center justify-between gap-3 flex-wrap">
            <h4 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
              {t('settings.adminMapTiles.offline.tileServer')}
            </h4>
            <div className="flex items-center gap-3 flex-wrap">
              <CapBadge status={tileBadgeStatus} label={tileBadgeLabel} />
              {tileProbeAvailable && tileStatus !== 'loading' && (
                <button
                  onClick={() => setTileCheckKey(k => k + 1)}
                  className="text-xs text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-300 underline underline-offset-2 cursor-pointer"
                >
                  {t('common.retry')}
                </button>
              )}
            </div>
          </div>
          <p className="text-xs text-neutral-500 dark:text-neutral-400 leading-relaxed">
            {t('settings.adminMapTiles.offline.tileServerDescriptionPrefix')}{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">VITE_MARTIN_URL</code>{' '}
            {t('settings.adminMapTiles.offline.tileServerDescriptionSuffix')}{' '}
            <code className="px-1 py-0.5 rounded bg-neutral-100 dark:bg-neutral-800 font-mono text-xs">/api/v1/tiles</code>).
          </p>
          <p className="text-xs text-neutral-400 dark:text-neutral-500 font-mono break-all">{styleUrl}</p>
          {tileStatus === 'ok' && tileSources.length > 0 && (
            <div>
              <p className="text-xs font-medium text-neutral-600 dark:text-neutral-400 mb-1.5">
                {t('settings.adminMapTiles.offline.tileSources', { count: tileSources.length })}
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
              {t('settings.adminMapTiles.offline.basemap')}
            </h4>
            <p className="text-xs text-neutral-500 dark:text-neutral-400 mt-0.5">
              {t('settings.adminMapTiles.offline.basemapDescription')}
            </p>
          </div>
          {offlineMapAvailable ? (
            <OfflineMapSettings />
          ) : (
            <p className="text-xs text-amber-600 dark:text-amber-400 leading-relaxed">
              {t('settings.adminMapTiles.offline.missingArtifacts')}
            </p>
          )}
        </div>
      </section>
      )}

      {osmImportEnabled && showImportModal && <OSMImportModal onClose={() => setShowImportModal(false)} />}
    </div>
  )
}
