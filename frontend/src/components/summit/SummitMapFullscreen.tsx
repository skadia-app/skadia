import { useRef, useEffect, useState, useMemo } from 'react'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import type { SummitVisit } from '@/api/summits'
import SummitActivityList from './SummitActivityList'
import { ELEV_CLASSES, type ElevClass } from './summitConstants'
import { SUMMIT_MAP_STYLE } from './summitMapStyle'

interface SummitMapFullscreenProps {
  summits: SummitVisit[]
  initialSelected?: SummitVisit | null
  initialElevClass?: ElevClass
  initialCountry?: string
  initialYear?: string
  availableCountries: string[]
  availableYears: string[]
  onClose: () => void
}

export default function SummitMapFullscreen({
  summits,
  initialSelected,
  initialElevClass,
  initialCountry,
  initialYear,
  availableCountries,
  availableYears,
  onClose,
}: SummitMapFullscreenProps) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<maplibregl.Marker[]>([])

  const [elevClass, setElevClass] = useState<ElevClass>(initialElevClass ?? ELEV_CLASSES[0])
  const [countryFilter, setCountryFilter] = useState(initialCountry ?? '')
  const [yearFilter, setYearFilter] = useState(initialYear ?? '')
  const [selectedSummit, setSelectedSummit] = useState<SummitVisit | null>(initialSelected ?? null)

  const filteredSummits = useMemo(() => {
    return summits.filter(s => {
      if (elevClass.min != null && (s.elevation_m ?? 0) < elevClass.min) return false
      if (elevClass.max != null && (s.elevation_m ?? 0) > elevClass.max) return false
      if (countryFilter && !(s.country_codes ?? []).includes(countryFilter)) return false
      if (yearFilter) {
        const y = yearFilter
        const firstYear = s.first_visit ? new Date(s.first_visit).getFullYear().toString() : ''
        const lastYear = s.last_visit ? new Date(s.last_visit).getFullYear().toString() : ''
        if (firstYear !== y && lastYear !== y) return false
      }
      return true
    })
  }, [summits, elevClass, countryFilter, yearFilter])

  // MapLibre initialisieren
  useEffect(() => {
    if (!mapContainer.current || mapRef.current) return
    const map = new maplibregl.Map({
      container: mapContainer.current,
      style: SUMMIT_MAP_STYLE,
      center: [11.4, 47.3],
      zoom: 7,
    })
    map.addControl(new maplibregl.NavigationControl(), 'top-right')
    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
    }
  }, [])

  // Marker bei Filterwechsel aktualisieren
  useEffect(() => {
    const map = mapRef.current
    if (!map) return
    const update = () => {
      markersRef.current.forEach(m => m.remove())
      markersRef.current = filteredSummits
        .filter(s => s.lat != null && s.lng != null)
        .map(s => {
          const el = document.createElement('div')
          el.style.cssText = 'width:28px;height:28px;cursor:pointer;display:flex;align-items:center;justify-content:center;'
          const isSelected = selectedSummit?.name === s.name
          el.innerHTML = isSelected
            ? `<svg width="28" height="28" viewBox="0 0 24 24" fill="#f97316" stroke="white" stroke-width="2"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`
            : `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2.5"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`
          el.addEventListener('click', () => setSelectedSummit(s))
          return new maplibregl.Marker({ element: el })
            .setLngLat([s.lng!, s.lat!])
            .addTo(map)
        })
    }
    if (map.loaded()) {
      update()
    } else {
      map.on('load', update)
      return () => { map.off('load', update) }
    }
  }, [filteredSummits, selectedSummit])

  // Karte auf selektierten Gipfel zentrieren
  useEffect(() => {
    if (selectedSummit?.lat != null && selectedSummit?.lng != null && mapRef.current) {
      mapRef.current.flyTo({ center: [selectedSummit.lng, selectedSummit.lat], zoom: 12 })
    }
  }, [selectedSummit])

  return (
    <div className="fixed inset-0 z-50 flex bg-black overflow-hidden">
      {/* Karte (links, flex-1) */}
      <div ref={mapContainer} className="flex-1 h-full" />

      {/* Rechtes Panel (w-80) */}
      <div className="w-80 flex-none bg-white dark:bg-neutral-900 border-l border-neutral-200 dark:border-neutral-800
                      flex flex-col overflow-hidden">
        {/* Panel-Header */}
        <div className="flex items-center justify-between p-4 border-b border-neutral-200 dark:border-neutral-800">
          <span className="text-sm font-semibold text-neutral-700 dark:text-neutral-300">
            Gipfelsammlung
          </span>
          <button
            onClick={onClose}
            className="p-1 rounded-lg hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors"
            title="Schließen"
          >
            <svg className="w-5 h-5 text-neutral-500" viewBox="0 0 20 20" fill="currentColor">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        </div>

        {/* Filter-Bereich */}
        <div className="p-4 border-b border-neutral-200 dark:border-neutral-800 space-y-3">
          {/* Höhenklassen */}
          <div className="flex flex-wrap gap-1">
            {ELEV_CLASSES.map(cls => (
              <button
                key={cls.label}
                onClick={() => setElevClass(cls)}
                className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                  elevClass.label === cls.label
                    ? 'bg-accent-DEFAULT text-white'
                    : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
                }`}
              >
                {cls.label}
              </button>
            ))}
          </div>

          {/* Länder-Filter */}
          {availableCountries.length > 1 && (
            <select
              value={countryFilter}
              onChange={e => setCountryFilter(e.target.value)}
              className="w-full rounded border border-neutral-300 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-2 py-1.5 text-xs
                         text-neutral-700 dark:text-neutral-300 focus:outline-none"
            >
              <option value="">Alle Länder</option>
              {availableCountries.map(c => <option key={c} value={c}>{c}</option>)}
            </select>
          )}

          {/* Jahr-Filter */}
          {availableYears.length > 0 && (
            <select
              value={yearFilter}
              onChange={e => setYearFilter(e.target.value)}
              className="w-full rounded border border-neutral-300 dark:border-neutral-700
                         bg-white dark:bg-neutral-800 px-2 py-1.5 text-xs
                         text-neutral-700 dark:text-neutral-300 focus:outline-none"
            >
              <option value="">Alle Jahre</option>
              {availableYears.map(y => <option key={y} value={y}>{y}</option>)}
            </select>
          )}

          <div className="text-xs text-neutral-400">
            {filteredSummits.length} Gipfel
          </div>
        </div>

        {/* Gipfel-Detail-Bereich (scrollbar) */}
        <div className="flex-1 overflow-y-auto p-4">
          {selectedSummit ? (
            <div className="space-y-4">
              {/* Gipfel-Header */}
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
                    <span>{Math.round(selectedSummit.elevation_m)} m</span>
                  )}
                  <span>{selectedSummit.visit_count}× besucht</span>
                </div>
              </div>

              {/* Aktivitätenliste */}
              <div>
                <h4 className="text-xs font-medium text-neutral-500 dark:text-neutral-400 uppercase tracking-wide mb-2">
                  Aktivitäten
                </h4>
                <SummitActivityList summit={selectedSummit} />
              </div>
            </div>
          ) : (
            <div className="text-sm text-neutral-400 text-center mt-8">
              Gipfel anklicken für Details
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
