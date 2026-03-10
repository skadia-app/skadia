import { useState, useEffect, useRef, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import maplibregl from 'maplibre-gl'
import 'maplibre-gl/dist/maplibre-gl.css'
import { fetchPeakCollection, type PeakVisit } from '@/api/peaks'
import SummitBadges from '@/components/summit/SummitBadges'

type SortField = 'name' | 'elevation_m' | 'first_visit' | 'last_visit' | 'visit_count'
type SortDir = 'asc' | 'desc'
type ViewMode = 'list' | 'map'

interface ElevClass {
  label: string
  min: number | null
  max: number | null
}

const ELEV_CLASSES: ElevClass[] = [
  { label: 'Alle', min: null, max: null },
  { label: '1000er', min: 1000, max: 1999 },
  { label: '2000er', min: 2000, max: 2999 },
  { label: '3000er', min: 3000, max: 3999 },
  { label: '4000er+', min: 4000, max: null },
]

const MAP_STYLE: maplibregl.StyleSpecification = {
  version: 8,
  sources: {
    basemap: {
      type: 'raster',
      tiles: ['https://tile.openstreetmap.org/{z}/{x}/{y}.png'],
      tileSize: 256,
      attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>',
      maxzoom: 19,
    },
  },
  layers: [{ id: 'basemap-tiles', type: 'raster', source: 'basemap' }],
}

function formatDate(iso: string): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleDateString('de-DE', { day: '2-digit', month: '2-digit', year: 'numeric' })
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

export default function PeakCollectionPage() {
  const [sortField, setSortField] = useState<SortField>('elevation_m')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [viewMode, setViewMode] = useState<ViewMode>('list')
  const [elevClass, setElevClass] = useState<ElevClass>(ELEV_CLASSES[0])
  const [countryFilter, setCountryFilter] = useState('')
  const [yearFilter, setYearFilter] = useState<string>('')

  // All peaks (unfiltered) — for badges and for deriving available filter options
  const { data: allPeaks } = useQuery({
    queryKey: ['peak-collection-all'],
    queryFn: () => fetchPeakCollection(),
    staleTime: 5 * 60 * 1000,
  })

  // Filtered peaks via server-side query
  const filterParams = {
    country: countryFilter || undefined,
    min_elevation: elevClass.min ?? undefined,
    max_elevation: elevClass.max ?? undefined,
    year: yearFilter ? Number(yearFilter) : undefined,
  }
  const isFiltered = !!(countryFilter || elevClass.min != null || yearFilter)

  const { data: filteredPeaks, isLoading, error } = useQuery({
    queryKey: ['peak-collection', countryFilter, elevClass.label, yearFilter],
    queryFn: () => fetchPeakCollection(filterParams),
    enabled: isFiltered,
    staleTime: 2 * 60 * 1000,
  })

  // If no filters active, use allPeaks to avoid a duplicate request
  const peaks = isFiltered ? (filteredPeaks ?? []) : (allPeaks ?? [])

  const sortedPeaks = useMemo(() => {
    const sorted = [...peaks]
    sorted.sort((a, b) => {
      let cmp = 0
      switch (sortField) {
        case 'name':
          cmp = a.name.localeCompare(b.name)
          break
        case 'elevation_m':
          cmp = (a.elevation_m ?? 0) - (b.elevation_m ?? 0)
          break
        case 'first_visit':
          cmp = a.first_visit.localeCompare(b.first_visit)
          break
        case 'last_visit':
          cmp = a.last_visit.localeCompare(b.last_visit)
          break
        case 'visit_count':
          cmp = a.visit_count - b.visit_count
          break
      }
      return sortDir === 'asc' ? cmp : -cmp
    })
    return sorted
  }, [peaks, sortField, sortDir])

  // Derive available years and countries from all peaks
  const availableYears = useMemo(() => {
    if (!allPeaks) return []
    const years = new Set<string>()
    for (const p of allPeaks) {
      if (p.first_visit) years.add(new Date(p.first_visit).getFullYear().toString())
    }
    return [...years].sort().reverse()
  }, [allPeaks])

  const availableCountries = useMemo(() => {
    if (!allPeaks) return []
    const codes = new Set<string>()
    for (const p of allPeaks) {
      for (const c of (p.country_codes ?? [])) {
        if (c) codes.add(c)
      }
    }
    return [...codes].sort()
  }, [allPeaks])

  const highestPeak = useMemo(() => {
    if (!peaks.length) return null
    return peaks.reduce((best, p) =>
      (p.elevation_m ?? 0) > (best.elevation_m ?? 0) ? p : best
    )
  }, [peaks])

  function toggleSort(field: SortField) {
    if (sortField === field) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc')
    } else {
      setSortField(field)
      setSortDir(field === 'name' ? 'asc' : 'desc')
    }
  }

  function sortIndicator(field: SortField) {
    if (sortField !== field) return null
    return <span className="ml-1">{sortDir === 'asc' ? '\u25B2' : '\u25BC'}</span>
  }

  if (isLoading && isFiltered && !peaks.length) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-accent-DEFAULT" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="p-6 text-red-600 dark:text-red-400">
        Fehler beim Laden der Gipfelsammlung.
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">Gipfelsammlung</h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          Alle besuchten Gipfel mit Statistiken und Kartenansicht.
        </p>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <StatCard label="Besuchte Gipfel" value={peaks.length.toString()} />
        <StatCard
          label="Höchster Gipfel"
          value={highestPeak ? `${Math.round(highestPeak.elevation_m ?? 0)} m` : '-'}
          sub={highestPeak?.name}
        />
        <StatCard
          label="Gesamte Besteigungen"
          value={peaks.reduce((s, p) => s + p.visit_count, 0).toString()}
        />
        <StatCard
          label="Ueber 3000 m"
          value={peaks.filter(p => (p.elevation_m ?? 0) >= 3000).length.toString()}
        />
      </div>

      {/* Badges — always based on all peaks (unfiltered) */}
      {allPeaks && allPeaks.length > 0 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">Errungenschaften</h2>
          <SummitBadges peaks={allPeaks} />
        </div>
      )}

      {/* Filters + View Toggle */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Höhenklassen-Filter */}
        <div className="flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-700">
          {ELEV_CLASSES.map(cls => (
            <button
              key={cls.label}
              onClick={() => setElevClass(cls)}
              className={`px-3 py-1.5 text-xs font-medium transition-colors ${
                elevClass.label === cls.label
                  ? 'bg-accent-DEFAULT text-white'
                  : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
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
            className="rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900
                       px-2.5 py-1.5 text-xs text-neutral-700 dark:text-neutral-300
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40"
          >
            <option value="">Alle Länder</option>
            {availableCountries.map(c => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
        )}

        {/* Jahr-Filter */}
        {availableYears.length > 0 && (
          <select
            value={yearFilter}
            onChange={e => setYearFilter(e.target.value)}
            className="rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900
                       px-2.5 py-1.5 text-xs text-neutral-700 dark:text-neutral-300
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40"
          >
            <option value="">Alle Jahre</option>
            {availableYears.map(y => <option key={y} value={y}>{y}</option>)}
          </select>
        )}

        {/* View toggle */}
        <div className="ml-auto flex rounded-lg border border-neutral-300 dark:border-neutral-700 overflow-hidden">
          <button
            onClick={() => setViewMode('list')}
            className={`px-3 py-1.5 text-sm font-medium transition-colors ${
              viewMode === 'list'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            <ListIcon />
          </button>
          <button
            onClick={() => setViewMode('map')}
            className={`px-3 py-1.5 text-sm font-medium transition-colors ${
              viewMode === 'map'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            <MapIcon />
          </button>
        </div>
      </div>

      {/* Content */}
      {viewMode === 'list' ? (
        <div className="overflow-x-auto rounded-xl border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
                <SortableHeader field="name" label="Name" sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="elevation_m" label="Höhe" sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="first_visit" label="Erstbesuch" sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="last_visit" label="Letzter Besuch" sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="visit_count" label="Besuche" sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
              </tr>
            </thead>
            <tbody>
              {sortedPeaks.map((peak, i) => (
                <tr
                  key={`${peak.osm_id ?? peak.name}-${i}`}
                  className="border-b border-neutral-100 dark:border-neutral-800/50 hover:bg-neutral-50 dark:hover:bg-neutral-900/30 transition-colors"
                >
                  <td className="px-4 py-3 font-medium text-neutral-900 dark:text-neutral-100">
                    <div className="flex items-center gap-2">
                      <MountainIconSmall />
                      {peak.name}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {peak.elevation_m != null ? `${Math.round(peak.elevation_m)} m` : '-'}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {formatDate(peak.first_visit)}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {formatDate(peak.last_visit)}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {peak.visit_count}
                  </td>
                </tr>
              ))}
              {sortedPeaks.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-neutral-400">
                    Keine Gipfel gefunden.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      ) : (
        <PeakMap peaks={sortedPeaks} />
      )}
    </div>
  )
}

// --------------- Sub-components ---------------

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 p-4">
      <div className="text-sm text-neutral-500 dark:text-neutral-400">{label}</div>
      <div className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100 mt-1">{value}</div>
      {sub && <div className="text-xs text-neutral-400 mt-0.5 truncate">{sub}</div>}
    </div>
  )
}

function SortableHeader({
  field, label, sortField: _sortField, sortDir: _sortDir, onToggle, indicator,
}: {
  field: SortField
  label: string
  sortField: SortField
  sortDir: SortDir
  onToggle: (f: SortField) => void
  indicator: (f: SortField) => React.ReactNode
}) {
  return (
    <th
      className="px-4 py-3 text-left font-medium text-neutral-500 dark:text-neutral-400 cursor-pointer select-none hover:text-neutral-700 dark:hover:text-neutral-200 transition-colors"
      onClick={() => onToggle(field)}
    >
      {label}{indicator(field)}
    </th>
  )
}

function PeakMap({ peaks }: { peaks: PeakVisit[] }) {
  const mapContainer = useRef<HTMLDivElement>(null)
  const mapRef = useRef<maplibregl.Map | null>(null)
  const markersRef = useRef<maplibregl.Marker[]>([])

  useEffect(() => {
    if (!mapContainer.current) return

    const peaksWithCoords = peaks.filter(p => p.lat != null && p.lng != null)

    if (mapRef.current) {
      // Update markers on existing map
      markersRef.current.forEach(m => m.remove())
      markersRef.current = addMarkers(mapRef.current, peaksWithCoords)
      return
    }

    // Initial center: center of all peaks, or Europe
    let center: [number, number] = [11.4, 47.3]
    let zoom = 7
    if (peaksWithCoords.length > 0) {
      const lats = peaksWithCoords.map(p => p.lat!)
      const lngs = peaksWithCoords.map(p => p.lng!)
      center = [
        (Math.min(...lngs) + Math.max(...lngs)) / 2,
        (Math.min(...lats) + Math.max(...lats)) / 2,
      ]
    }

    const map = new maplibregl.Map({
      container: mapContainer.current,
      style: MAP_STYLE,
      center,
      zoom,
    })

    map.addControl(new maplibregl.NavigationControl(), 'top-right')

    map.on('load', () => {
      markersRef.current = addMarkers(map, peaksWithCoords)

      // Fit bounds if we have peaks
      if (peaksWithCoords.length > 1) {
        const bounds = new maplibregl.LngLatBounds()
        peaksWithCoords.forEach(p => bounds.extend([p.lng!, p.lat!]))
        map.fitBounds(bounds, { padding: 60, maxZoom: 13 })
      } else if (peaksWithCoords.length === 1) {
        map.setCenter([peaksWithCoords[0].lng!, peaksWithCoords[0].lat!])
        map.setZoom(12)
      }
    })

    mapRef.current = map

    return () => {
      map.remove()
      mapRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Update markers when peaks change (filters)
  useEffect(() => {
    if (!mapRef.current) return
    const peaksWithCoords = peaks.filter(p => p.lat != null && p.lng != null)
    markersRef.current.forEach(m => m.remove())
    markersRef.current = addMarkers(mapRef.current, peaksWithCoords)
  }, [peaks])

  return (
    <div
      ref={mapContainer}
      className="w-full h-[500px] rounded-xl border border-neutral-200 dark:border-neutral-800 overflow-hidden"
    />
  )
}

function addMarkers(map: maplibregl.Map, peaks: PeakVisit[]): maplibregl.Marker[] {
  return peaks.map(peak => {
    const el = document.createElement('div')
    el.className = 'peak-marker'
    el.style.cssText = `
      width: 28px; height: 28px; cursor: pointer;
      display: flex; align-items: center; justify-content: center;
    `
    el.innerHTML = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#c2410c" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M3 20l6.5-11L13 15l4-6 4 11H3z"/></svg>`

    const popup = new maplibregl.Popup({ offset: 16, closeButton: false })
      .setHTML(`
        <div style="font-family: system-ui; font-size: 13px;">
          <strong>${escapeHtml(peak.name)}</strong><br/>
          ${peak.elevation_m != null ? `${escapeHtml(String(Math.round(peak.elevation_m)))} m` : ''}
          ${peak.visit_count > 1 ? ` &middot; ${escapeHtml(String(peak.visit_count))}x besucht` : ''}
        </div>
      `)

    return new maplibregl.Marker({ element: el })
      .setLngLat([peak.lng!, peak.lat!])
      .setPopup(popup)
      .addTo(map)
  })
}

// --------------- Icons ---------------

function MountainIconSmall() {
  return (
    <svg className="w-4 h-4 text-accent-DEFAULT shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M3 20l6.5-11L13 15l4-6 4 11H3z" />
    </svg>
  )
}

function ListIcon() {
  return (
    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <line x1="8" y1="6" x2="21" y2="6" />
      <line x1="8" y1="12" x2="21" y2="12" />
      <line x1="8" y1="18" x2="21" y2="18" />
      <line x1="3" y1="6" x2="3.01" y2="6" />
      <line x1="3" y1="12" x2="3.01" y2="12" />
      <line x1="3" y1="18" x2="3.01" y2="18" />
    </svg>
  )
}

function MapIcon() {
  return (
    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <polygon points="1 6 1 22 8 18 16 22 23 18 23 2 16 6 8 2 1 6" />
      <line x1="8" y1="2" x2="8" y2="18" />
      <line x1="16" y1="6" x2="16" y2="22" />
    </svg>
  )
}
