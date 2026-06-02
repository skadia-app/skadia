import { useRef, useEffect, useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { useEscapeKey } from '@/hooks/useEscapeKey'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { type SummitVisit } from '@/api/summits'
import SummitActivityList from './SummitActivityList'
import { ELEV_CLASSES, type ElevClass } from './summitConstants'
import { buildLayerStyle } from '@/lib/mapStyles'
import { useMapLayers } from '@/hooks/useMapLayers'
import { useCapabilities } from '@/hooks/useCapabilities'
import { createSummitMarker, type SummitMarker } from './markerUtils'
import { useSummitCollection } from '@/hooks/useSummitCollection'
import { useUnitFormatters } from '@/lib/useUnitFormatters'
import { elevationInputUnit, elevationToDisplayValue, type UnitFormatOptions } from '@/lib/units'

interface SummitMapFullscreenProps {
  initialSelected?: SummitVisit | null
  initialElevClass?: ElevClass
  initialCountry?: string
  initialYear?: string
  onClose: () => void
}

function formatElevationClassLabel(
  cls: ElevClass,
  defaultLabel: string,
  options: UnitFormatOptions,
): string {
  if (cls.min == null) return defaultLabel
  const formatter = new Intl.NumberFormat(options.locale, { maximumFractionDigits: 0 })
  const unit = elevationInputUnit(options.units)
  const minDisplay = formatter.format(Math.round(elevationToDisplayValue(cls.min, options.units)))
  if (cls.max == null) return `${minDisplay}+ ${unit}`
  const maxDisplay = formatter.format(Math.round(elevationToDisplayValue(cls.max, options.units)))
  return `${minDisplay}-${maxDisplay} ${unit}`
}

export default function SummitMapFullscreen({
  initialSelected,
  initialElevClass,
  initialCountry,
  initialYear,
  onClose,
}: SummitMapFullscreenProps) {
  const { t } = useTranslation()
  const { options, formatElevation } = useUnitFormatters()
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<SummitMarker[]>([])

  const [elevClass, setElevClass] = useState<ElevClass>(initialElevClass ?? ELEV_CLASSES[0])
  const [countryFilter, setCountryFilter] = useState(initialCountry ?? '')
  const [yearFilter, setYearFilter] = useState(initialYear ?? '')
  const [selectedSummit, setSelectedSummit] = useState<SummitVisit | null>(initialSelected ?? null)

  useEscapeKey(onClose)

  const { displaySummits, availableCountries, availableYears } = useSummitCollection({ countryFilter, elevClass, yearFilter })

  const { base } = useMapLayers()
  const { data: caps } = useCapabilities()
  const defaultLayer = base[0]
  const mapStyle = useMemo(
    () => (defaultLayer ? buildLayerStyle(defaultLayer, caps ?? {}) : null),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [defaultLayer?.key, caps],
  )

  // Initialize MapLibre
  useEffect(() => {
    if (!mapContainer.current || mapRef.current || !mapStyle) return
    const map = new maplibregl.Map({
      container: mapContainer.current,
      style: mapStyle as import('maplibre-gl').StyleSpecification | string,
      center: [11.4, 47.3],
      zoom: 7,
    })
    map.addControl(new maplibregl.NavigationControl(), 'top-right')
    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
    }
    // mapStyle triggers init once layers arrive; mapRef.current guard prevents re-init.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mapStyle])

  // Update markers when filters change
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    const update = () => {
      markersRef.current.forEach(m => { m.abort(); m.marker.remove() })
      markersRef.current = displaySummits
        .filter(s => s.lat != null && s.lng != null)
        .map(s => {
          const isSelected = selectedSummit?.name === s.name
          const { marker, abort } = createSummitMarker(isSelected, () => setSelectedSummit(s))
          marker.setLngLat([s.lng!, s.lat!]).addTo(map)
          return { marker, abort }
        })
    }
    if (map.loaded()) {
      update()
    } else {
      map.on('load', update)
      return () => { map.off('load', update) }
    }
  }, [displaySummits, selectedSummit])

  // Center the map on the selected summit
  useEffect(() => {
    if (selectedSummit?.lat != null && selectedSummit?.lng != null && mapRef.current) {
      mapRef.current.flyTo({ center: [selectedSummit.lng, selectedSummit.lat], zoom: 12 })
    }
  }, [selectedSummit])

  return (
    <div className="fixed inset-0 z-50 flex flex-col md:flex-row bg-black overflow-hidden">
      {/* Map */}
      <div ref={mapContainer} className="flex-1 h-[60vh] md:h-full" />

      {/* Panel: horizontal bottom strip on mobile, right side on desktop */}
      <div className="w-full md:w-80 md:flex-none h-[40vh] md:h-full bg-white dark:bg-neutral-900
                      border-t md:border-t-0 md:border-l border-neutral-200 dark:border-neutral-800
                      flex flex-col overflow-hidden">
        {/* Panel header */}
        <div className="flex items-center justify-between p-4 border-b border-neutral-200 dark:border-neutral-800">
          <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
            {t('summitCollection.title')}
          </span>
          <button
            onClick={onClose}
            className="cursor-pointer p-1 rounded-lg hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            title={t('common.close')}
          >
            <svg className="w-5 h-5 text-neutral-500" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        {/* Filters */}
        <div className="p-4 border-b border-neutral-200 dark:border-neutral-800 space-y-3">
          {/* Elevation classes */}
          <div className="flex flex-wrap gap-1">
            {ELEV_CLASSES.map(cls => (
              <button
                key={cls.id}
                onClick={() => setElevClass(cls)}
                className={`px-2.5 py-1 rounded text-xs font-medium cursor-pointer transition-colors ${
                  elevClass.id === cls.id
                    ? 'bg-accent-DEFAULT text-white'
                    : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
                }`}
              >
                {formatElevationClassLabel(cls, t(cls.labelKey), options)}
              </button>
            ))}
          </div>

          {/* Country filter */}
          {availableCountries.length > 1 && (
            <select
              value={countryFilter}
              onChange={e => setCountryFilter(e.target.value)}
              className="w-full rounded border border-neutral-300 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-2 py-1.5 text-xs
                         text-neutral-700 dark:text-neutral-300 focus:outline-none"
            >
              <option value="">{t('summitCollection.filters.allCountries')}</option>
              {availableCountries.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          )}

          {/* Year filter */}
          {availableYears.length > 0 && (
            <select
              value={yearFilter}
              onChange={e => setYearFilter(e.target.value)}
              className="w-full rounded border border-neutral-300 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-2 py-1.5 text-xs
                         text-neutral-700 dark:text-neutral-300 focus:outline-none"
            >
              <option value="">{t('summitCollection.filters.allYears')}</option>
              {availableYears.map(y => <option key={y} value={y}>{y}</option>)}
            </select>
          )}

          <div className="text-xs text-neutral-400">
            {t('summitCollection.map.summitCount', { count: displaySummits.length })}
          </div>
        </div>

        {/* Summit detail area (scrollable) */}
        <div className="flex-1 overflow-y-auto p-4">
          {selectedSummit ? (
            <div className="space-y-4">
              {/* Summit header */}
              <div>
                <div className="flex items-center gap-2">
                  <svg className="w-5 h-5 text-accent-DEFAULT shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                    <path strokeLinecap="round" strokeLinejoin="round" d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
                  </svg>
                  <h3 className="font-semibold text-neutral-900 dark:text-neutral-100 truncate">
                    {selectedSummit.name}
                  </h3>
                </div>
                <div className="text-sm text-neutral-500 dark:text-neutral-400 mt-1 pl-7 flex gap-3">
                  {selectedSummit.elevation_m != null && (
                    <span>{formatElevation(selectedSummit.elevation_m)}</span>
                  )}
                  <span>{t('summitCollection.map.visitedTimes', { count: selectedSummit.visit_count })}</span>
                </div>
              </div>

              {/* Activity list */}
              <div>
                <h4 className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
                  {t('summitCollection.activities.title')}
                </h4>
                <SummitActivityList summit={selectedSummit} />
              </div>
            </div>
          ) : (
            <div className="text-sm text-neutral-400 text-center mt-8">
              {t('summitCollection.map.clickForDetails')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
