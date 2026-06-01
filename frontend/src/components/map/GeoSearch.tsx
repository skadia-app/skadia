import { useState, useCallback, useRef, useEffect, type ChangeEvent, type KeyboardEvent } from 'react'
import type maplibregl from 'maplibre-gl'
import { apiFetch } from '@/lib/auth'
import { useTranslation } from 'react-i18next'

interface GeoSearchResult {
  name: string
  type: string
  subtype?: string
  lat: number
  lng: number
  source?: string
}

interface GeoSearchResponse {
  results: GeoSearchResult[]
}

export default function GeoSearch({ mapRef, embedded }: { mapRef: React.RefObject<maplibregl.Map | null>, embedded?: boolean }) {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<GeoSearchResult[]>([])
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(-1)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const abortRef = useRef<AbortController | null>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  const searchPlaces = useCallback(async (q: string) => {
    if (q.length < 3) {
      setResults([])
      setIsOpen(false)
      return
    }
    abortRef.current?.abort()
    abortRef.current = new AbortController()
    setIsLoading(true)
    try {
      const params = new URLSearchParams({
        q,
        type: 'all',
        limit: '5',
        sources: 'local',
      })
      const resp = await apiFetch(`/api/v1/geo/search?${params.toString()}`, {
        signal: abortRef.current.signal,
      })
      if (!resp.ok) throw new Error(`Geosearch: ${resp.status}`)
      const data = await resp.json() as GeoSearchResponse
      const next = data.results ?? []
      setResults(next)
      setIsOpen(next.length > 0)
      setActiveIndex(-1)
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return
      console.error('Geocoding error:', err)
      setResults([])
      setIsOpen(false)
    } finally {
      setIsLoading(false)
    }
  }, [])

  const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
    const value = e.target.value
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (value.length < 3) {
      setResults([])
      setIsOpen(false)
      return
    }
    debounceRef.current = setTimeout(() => searchPlaces(value), 500)
  }

  const handleSelect = (result: GeoSearchResult) => {
    const map = mapRef.current
    if (!map) return
    map.flyTo({ center: [result.lng, result.lat], zoom: 14, duration: 1500 })
    setQuery(result.name)
    setIsOpen(false)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Escape') {
      setIsOpen(false)
      return
    }
    if (!isOpen || results.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((prev) => (prev < results.length - 1 ? prev + 1 : 0))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((prev) => (prev > 0 ? prev - 1 : results.length - 1))
    } else if (e.key === 'Enter' && activeIndex >= 0) {
      e.preventDefault()
      handleSelect(results[activeIndex])
    }
  }

  const handleClear = () => {
    setQuery('')
    setResults([])
    setIsOpen(false)
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }

  return (
    <div ref={containerRef} className={embedded ? 'relative w-80' : 'absolute top-3 left-1/2 -translate-x-1/2 z-10 w-80'}>
      <div className="relative">
        <input
          type="text"
          value={query}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onFocus={() => { if (results.length > 0) setIsOpen(true) }}
          placeholder={t('mapEditor.geoSearch.placeholder')}
          className="w-full px-3 py-2 pr-8 text-sm rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/90 dark:bg-neutral-800/90 backdrop-blur-sm shadow-md text-neutral-900 dark:text-neutral-100 placeholder-neutral-400 dark:placeholder-neutral-500 focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT focus:border-transparent"
        />
        {isLoading && (
          <div className="absolute right-8 top-1/2 -translate-y-1/2">
            <div className="w-4 h-4 border-2 border-neutral-300 border-t-orange-500 rounded-full animate-spin" />
          </div>
        )}
        {query && (
          <button
            onClick={handleClear}
            className="absolute right-2 top-1/2 -translate-y-1/2 text-neutral-400 hover:text-neutral-600 dark:hover:text-neutral-200 cursor-pointer"
            aria-label={t('mapEditor.geoSearch.clear')}
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
              <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
            </svg>
          </button>
        )}
      </div>
      {isOpen && results.length > 0 && (
        <ul className="mt-1 max-h-60 overflow-y-auto rounded-lg border border-neutral-300 dark:border-neutral-600 bg-white/95 dark:bg-neutral-800/95 backdrop-blur-sm shadow-lg">
          {results.map((r, i) => (
            <li key={`${r.source ?? 'local'}-${r.type}-${r.lat}-${r.lng}-${i}`}>
              <button
                onClick={() => handleSelect(r)}
                className={`w-full text-left px-3 py-2 text-sm cursor-pointer transition-colors ${
                  i === activeIndex
                    ? 'bg-accent-DEFAULT text-white'
                    : 'text-neutral-700 dark:text-neutral-200 hover:bg-neutral-100 dark:hover:bg-neutral-700'
                }`}
              >
                <span className="line-clamp-2">{r.name}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
