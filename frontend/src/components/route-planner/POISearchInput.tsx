import { useState, useRef, useCallback, useEffect } from 'react'
import { searchPOIs } from '@/api/routes'
import type { POIResult } from '@/api/routes'

// ── POI type helpers ────────────────────────────────────────────────────────

const POI_TYPE_LABELS: Record<string, string> = {
  summit: 'Gipfel',
  volcano: 'Vulkan',
  saddle: 'Sattel',
  alpine_hut: 'Alpenhütte',
  wilderness_hut: 'Berghütte',
  restaurant: 'Restaurant',
  cafe: 'Café',
  bar: 'Bar',
  fast_food: 'Schnellimbiss',
  hotel: 'Hotel',
  hostel: 'Hostel',
  camp_site: 'Campingplatz',
  drinking_water: 'Trinkwasser',
  toilets: 'WC',
  parking: 'Parkplatz',
  fuel: 'Tankstelle',
  bicycle_repair_station: 'Fahrradreparatur',
  viewpoint: 'Aussichtspunkt',
}

function poiTypeLabel(type: string): string {
  return POI_TYPE_LABELS[type] ?? type
}

function poiTypeIcon(type: string): string {
  if (type === 'summit' || type === 'volcano' || type === 'saddle') return '⛰'
  if (type === 'alpine_hut' || type === 'wilderness_hut') return '🏔'
  if (type === 'restaurant' || type === 'cafe' || type === 'bar' || type === 'fast_food') return '🍽'
  if (type === 'hotel' || type === 'hostel') return '🛏'
  if (type === 'camp_site') return '⛺'
  if (type === 'drinking_water') return '💧'
  if (type === 'viewpoint') return '👁'
  if (type === 'parking') return '🅿'
  if (type === 'fuel') return '⛽'
  if (type === 'bicycle_repair_station') return '🔧'
  return '📍'
}

// ── Filter tabs ─────────────────────────────────────────────────────────────

type TypeFilter = 'all' | 'peaks' | 'amenities'

const TYPE_FILTER_LABELS: Record<TypeFilter, string> = {
  all: 'Alle',
  peaks: 'Gipfel',
  amenities: 'Orte',
}

// Maps frontend filter tabs to API `types` param values
const TYPE_FILTER_API: Record<TypeFilter, string | undefined> = {
  all: undefined,
  peaks: 'peaks',
  amenities: 'amenities',
}

// ── Props ────────────────────────────────────────────────────────────────────

interface Props {
  /** Called when the user selects a POI to add as a waypoint */
  onSelect: (poi: POIResult) => void
  placeholder?: string
  /** Restrict results to a bounding box [minLat, minLng, maxLat, maxLng] */
  bbox?: [number, number, number, number]
  className?: string
}

// ── Component ────────────────────────────────────────────────────────────────

export default function POISearchInput({
  onSelect,
  placeholder = 'Gipfel, Hütte oder Ort suchen…',
  className = '',
}: Props) {
  const [query, setQuery] = useState('')
  const [typeFilter, setTypeFilter] = useState<TypeFilter>('all')
  const [results, setResults] = useState<POIResult[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)

  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // ── Debounced search ───────────────────────────────────────────────────────

  const runSearch = useCallback(
    async (q: string, filter: TypeFilter) => {
      const trimmed = q.trim()
      if (trimmed.length < 2) {
        setResults([])
        setIsOpen(false)
        return
      }
      setIsLoading(true)
      try {
        const data = await searchPOIs(trimmed, TYPE_FILTER_API[filter], 12)
        setResults(data)
        setIsOpen(true)
        setActiveIndex(-1)
      } catch {
        setResults([])
      } finally {
        setIsLoading(false)
      }
    },
    [],
  )

  const scheduleSearch = useCallback(
    (q: string, filter: TypeFilter) => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
      debounceRef.current = setTimeout(() => runSearch(q, filter), 300)
    },
    [runSearch],
  )

  // Re-run search when type filter changes (without additional debounce)
  useEffect(() => {
    if (query.trim().length >= 2) {
      runSearch(query, typeFilter)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [typeFilter])

  // Cleanup debounce on unmount
  useEffect(() => () => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }, [])

  // ── Click-outside to close ─────────────────────────────────────────────────

  useEffect(() => {
    function handlePointerDown(e: PointerEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false)
        setActiveIndex(-1)
      }
    }
    document.addEventListener('pointerdown', handlePointerDown)
    return () => document.removeEventListener('pointerdown', handlePointerDown)
  }, [])

  // ── Keyboard navigation ────────────────────────────────────────────────────

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (!isOpen || results.length === 0) return

    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) => Math.min(i + 1, results.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) => Math.max(i - 1, -1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (activeIndex >= 0 && activeIndex < results.length) {
        handleSelect(results[activeIndex])
      }
    } else if (e.key === 'Escape') {
      setIsOpen(false)
      setActiveIndex(-1)
      inputRef.current?.blur()
    }
  }

  // Scroll active item into view
  useEffect(() => {
    if (activeIndex >= 0 && listRef.current) {
      const item = listRef.current.children[activeIndex] as HTMLElement | undefined
      item?.scrollIntoView({ block: 'nearest' })
    }
  }, [activeIndex])

  // ── Select handler ─────────────────────────────────────────────────────────

  function handleSelect(poi: POIResult) {
    onSelect(poi)
    setQuery('')
    setResults([])
    setIsOpen(false)
    setActiveIndex(-1)
    inputRef.current?.focus()
  }

  // ── Clear ──────────────────────────────────────────────────────────────────

  function handleClear() {
    setQuery('')
    setResults([])
    setIsOpen(false)
    setActiveIndex(-1)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    inputRef.current?.focus()
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  const showDropdown = isOpen && (results.length > 0 || (isLoading && query.trim().length >= 2))
  const showEmpty = isOpen && !isLoading && results.length === 0 && query.trim().length >= 2

  return (
    <div ref={containerRef} className={`relative ${className}`}>
      {/* Search input row */}
      <div className="relative">
        {/* Magnifier icon */}
        <span
          className="absolute left-3 top-1/2 -translate-y-1/2 text-neutral-400 dark:text-neutral-500 pointer-events-none"
          aria-hidden="true"
        >
          {isLoading ? (
            <svg
              className="animate-spin w-4 h-4"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
            >
              <circle className="opacity-25" cx={12} cy={12} r={10} />
              <path className="opacity-75" d="M4 12a8 8 0 018-8" />
            </svg>
          ) : (
            <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <circle cx={11} cy={11} r={8} />
              <path d="m21 21-4.35-4.35" />
            </svg>
          )}
        </span>

        <input
          ref={inputRef}
          type="text"
          value={query}
          placeholder={placeholder}
          autoComplete="off"
          spellCheck={false}
          aria-label="POI suchen"
          aria-expanded={isOpen}
          aria-autocomplete="list"
          aria-activedescendant={activeIndex >= 0 ? `poi-result-${activeIndex}` : undefined}
          role="combobox"
          onChange={(e) => {
            const val = e.target.value
            setQuery(val)
            scheduleSearch(val, typeFilter)
            if (val.trim().length < 2) {
              setResults([])
              setIsOpen(false)
            }
          }}
          onKeyDown={handleKeyDown}
          className="w-full pl-9 pr-8 py-2 border border-neutral-300 dark:border-neutral-700 rounded-lg bg-white dark:bg-neutral-900 text-neutral-900 dark:text-neutral-100 text-sm focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent placeholder-neutral-400 dark:placeholder-neutral-500"
        />

        {/* Clear button */}
        {query && (
          <button
            type="button"
            onClick={handleClear}
            aria-label="Suche löschen"
            className="absolute right-2.5 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 transition-colors"
          >
            <svg className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
              <path d="M18 6 6 18M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>

      {/* Type filter tabs */}
      <div className="flex gap-1 mt-1.5">
        {(Object.keys(TYPE_FILTER_LABELS) as TypeFilter[]).map((f) => (
          <button
            key={f}
            type="button"
            onClick={() => setTypeFilter(f)}
            className={`px-2.5 py-0.5 rounded text-xs font-medium transition-colors cursor-pointer ${
              typeFilter === f
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-neutral-100 dark:bg-neutral-800 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-200 dark:hover:bg-neutral-700'
            }`}
          >
            {TYPE_FILTER_LABELS[f]}
          </button>
        ))}
      </div>

      {/* Results dropdown */}
      {showDropdown && (
        <div className="absolute z-50 left-0 right-0 mt-1 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg overflow-hidden">
          <ul
            ref={listRef}
            role="listbox"
            aria-label="POI-Suchergebnisse"
            className="max-h-64 overflow-y-auto divide-y divide-neutral-100 dark:divide-neutral-800"
          >
            {isLoading && results.length === 0 && (
              <li className="px-4 py-3 text-sm text-neutral-400 dark:text-neutral-500 italic">
                Suche läuft…
              </li>
            )}
            {results.map((poi, idx) => (
              <li
                key={`${poi.osm_id}-${idx}`}
                id={`poi-result-${idx}`}
                role="option"
                aria-selected={idx === activeIndex}
              >
                <button
                  type="button"
                  onClick={() => handleSelect(poi)}
                  onMouseEnter={() => setActiveIndex(idx)}
                  className={`w-full flex items-center gap-3 px-3 py-2.5 text-left transition-colors cursor-pointer ${
                    idx === activeIndex
                      ? 'bg-accent-DEFAULT/10 dark:bg-accent-DEFAULT/20'
                      : 'hover:bg-neutral-50 dark:hover:bg-neutral-800'
                  }`}
                >
                  {/* Type icon */}
                  <span
                    className="flex-shrink-0 w-8 h-8 flex items-center justify-center rounded-md bg-neutral-100 dark:bg-neutral-800 text-base"
                    aria-hidden="true"
                  >
                    {poiTypeIcon(poi.type)}
                  </span>

                  {/* Name + metadata */}
                  <div className="min-w-0 flex-1">
                    <div className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
                      {poi.name}
                    </div>
                    <div className="flex items-center gap-2 mt-0.5">
                      <span className="text-xs text-neutral-400 dark:text-neutral-500">
                        {poiTypeLabel(poi.type)}
                      </span>
                      {poi.ele !== undefined && poi.ele !== null && (
                        <>
                          <span className="text-neutral-300 dark:text-neutral-700 text-xs">·</span>
                          <span className="text-xs text-neutral-500 dark:text-neutral-400 font-medium">
                            {poi.ele.toLocaleString('de-DE')} m
                          </span>
                        </>
                      )}
                    </div>
                  </div>

                  {/* Add indicator */}
                  <span
                    className={`flex-shrink-0 text-xs font-medium transition-opacity ${
                      idx === activeIndex
                        ? 'text-accent-DEFAULT opacity-100'
                        : 'text-neutral-300 dark:text-neutral-600 opacity-0 group-hover:opacity-100'
                    }`}
                    aria-hidden="true"
                  >
                    <svg className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                      <path d="M12 5v14M5 12h14" />
                    </svg>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Empty state */}
      {showEmpty && (
        <div className="absolute z-50 left-0 right-0 mt-1 bg-white dark:bg-neutral-900 border border-neutral-200 dark:border-neutral-700 rounded-lg shadow-lg px-4 py-3">
          <p className="text-sm text-neutral-400 dark:text-neutral-500">
            Keine POIs für <span className="text-neutral-600 dark:text-neutral-300 font-medium">„{query.trim()}"</span> gefunden
          </p>
          <p className="text-xs text-neutral-400 dark:text-neutral-600 mt-0.5">
            Tipp: OSM-Daten müssen für die Region importiert sein.
          </p>
        </div>
      )}
    </div>
  )
}
