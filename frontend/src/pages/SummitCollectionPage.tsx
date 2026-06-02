import { useState, useMemo, useCallback } from 'react'
import { useLocation, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { type SummitVisit } from '@/api/summits'
import SummitBadges from '@/components/summit/SummitBadges'
import SummitMap from '@/components/summit/SummitMap'
import SummitMapFullscreen from '@/components/summit/SummitMapFullscreen'
import { ELEV_CLASSES, findElevationClass, type ElevClass } from '@/components/summit/summitConstants'
import { useSummitCollection } from '@/hooks/useSummitCollection'
import { useCapabilities } from '@/hooks/useCapabilities'
import { featureAvailable } from '@/lib/deploymentCapabilities'
import { useUnitFormatters } from '@/lib/useUnitFormatters'

type SortField = 'name' | 'elevation_m' | 'first_visit' | 'last_visit' | 'visit_count'
type SortDir = 'asc' | 'desc'
type ViewMode = 'list' | 'map'

type SummitCollectionPageProps = { defaultView?: 'list' | 'map' }

const SUMMIT_COUNTRY_PARAM = 'summit_country'
const SUMMIT_YEAR_PARAM = 'summit_year'
const SUMMIT_ELEV_PARAM = 'summit_elev'

function formatDate(iso: string, locale: string): string {
  if (!iso) return '-'
  return new Date(iso).toLocaleDateString(locale, { day: '2-digit', month: '2-digit', year: 'numeric' })
}

export default function SummitCollectionPage({ defaultView = 'list' }: SummitCollectionPageProps) {
  const { t } = useTranslation()
  const { data: capabilities, isLoading: capabilitiesLoading } = useCapabilities()

  if (capabilitiesLoading && !capabilities) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-accent-DEFAULT" />
      </div>
    )
  }

  if (!featureAvailable(capabilities, 'geo_poi_enrichment')) {
    return (
      <div className="rounded-lg border border-neutral-200 bg-white p-6 text-sm text-neutral-600 dark:border-neutral-800 dark:bg-neutral-900 dark:text-neutral-300">
        {t('summitCollection.unavailable')}
      </div>
    )
  }

  return <SummitCollectionContent defaultView={defaultView} />
}

function SummitCollectionContent({ defaultView = 'list' }: SummitCollectionPageProps) {
  const { i18n, t } = useTranslation()
  const { formatElevation } = useUnitFormatters()
  const location = useLocation()
  const [sortField, setSortField] = useState<SortField>('elevation_m')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [viewMode, setViewMode] = useState<ViewMode>(defaultView)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [selectedSummit, setSelectedSummit] = useState<SummitVisit | null>(null)
  const [nameFilter, setNameFilter] = useState('')

  // Summit filters use namespaced URL params so Analytics filters can remain in
  // the URL without changing the summit collection tab.
  const [searchParams, setSearchParams] = useSearchParams()

  const allowLegacySummitParams = location.pathname.startsWith('/map/summits')
  const countryFilter = searchParams.get(SUMMIT_COUNTRY_PARAM) ?? (allowLegacySummitParams ? searchParams.get('country') ?? '' : '')
  const yearFilter = searchParams.get(SUMMIT_YEAR_PARAM) ?? (allowLegacySummitParams ? searchParams.get('year') ?? '' : '')
  const elevClassValue = searchParams.get(SUMMIT_ELEV_PARAM) ?? (allowLegacySummitParams ? searchParams.get('elev') : null)
  const elevClass = findElevationClass(elevClassValue)

  const setCountryFilter = (v: string) => setSearchParams(prev => {
    const next = new URLSearchParams(prev)
    v ? next.set(SUMMIT_COUNTRY_PARAM, v) : next.delete(SUMMIT_COUNTRY_PARAM)
    if (allowLegacySummitParams) next.delete('country')
    return next
  }, { replace: true })

  const setYearFilter = (v: string) => setSearchParams(prev => {
    const next = new URLSearchParams(prev)
    v ? next.set(SUMMIT_YEAR_PARAM, v) : next.delete(SUMMIT_YEAR_PARAM)
    if (allowLegacySummitParams) next.delete('year')
    return next
  }, { replace: true })

  const setElevClass = (cls: ElevClass) => setSearchParams(prev => {
    const next = new URLSearchParams(prev)
    cls.id !== 'all' ? next.set(SUMMIT_ELEV_PARAM, cls.id) : next.delete(SUMMIT_ELEV_PARAM)
    if (allowLegacySummitParams) next.delete('elev')
    return next
  }, { replace: true })

  const { allSummits, displaySummits: summits, isLoading, isFiltered, availableCountries, availableYears } = useSummitCollection({ countryFilter, elevClass, yearFilter })

  const sortedSummits = useMemo(() => {
    const sorted = [...summits]
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
  }, [summits, sortField, sortDir])

  // Client-side name filter on top of sorted results.
  const displaySummits = useMemo(() =>
    sortedSummits.filter(s =>
      !nameFilter || s.name.toLowerCase().includes(nameFilter.toLowerCase())
    )
  , [sortedSummits, nameFilter])

  const highestSummit = useMemo(() => {
    if (!summits.length) return null
    return summits.reduce((best, s) =>
      (s.elevation_m ?? 0) > (best.elevation_m ?? 0) ? s : best
    )
  }, [summits])

  // Stable reference prevents SummitMap from recreating markers on every render.
  const handleSelectSummit = useCallback((s: SummitVisit) => {
    setSelectedSummit(s)
    setIsFullscreen(true)
  }, [])

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

  if (isLoading && !summits.length) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="h-8 w-8 animate-spin rounded-full border-4 border-neutral-300 border-t-accent-DEFAULT" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold text-neutral-900 dark:text-neutral-100">{t('summitCollection.title')}</h1>
        <p className="text-sm text-neutral-500 dark:text-neutral-400 mt-1">
          {t('summitCollection.description')}
        </p>
      </div>

      {/* Stats Cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <StatCard
          label={t('summitCollection.stats.visitedSummits')}
          value={summits.length.toString()}
          sub={isFiltered ? t('summitCollection.stats.ofTotal', { total: allSummits.length }) : undefined}
        />
        <StatCard
          label={t('summitCollection.stats.highestSummit')}
          value={highestSummit ? formatElevation(highestSummit.elevation_m) : '-'}
          sub={highestSummit?.name}
        />
        <StatCard
          label={t('summitCollection.stats.totalAscents')}
          value={summits.reduce((s, p) => s + p.visit_count, 0).toString()}
        />
        <StatCard
          label={t('summitCollection.stats.over3000', { threshold: formatElevation(3000) })}
          value={summits.filter(s => (s.elevation_m ?? 0) >= 3000).length.toString()}
        />
      </div>

      {/* Badges — always based on all summits (unfiltered) */}
      {allSummits.length > 0 && (
        <div className="rounded-xl border border-neutral-200 dark:border-neutral-800 bg-white dark:bg-neutral-900 p-4">
          <h2 className="text-sm font-semibold text-neutral-700 dark:text-neutral-300 mb-3">{t('summitCollection.achievements')}</h2>
          <SummitBadges summits={allSummits} />
        </div>
      )}

      {/* Filters + View Toggle */}
      <div className="flex flex-wrap items-center gap-3">
        {/* Elevation class filter */}
        <div className="flex rounded-lg overflow-hidden border border-neutral-300 dark:border-neutral-700">
          {ELEV_CLASSES.map(cls => (
            <button
              key={cls.id}
              onClick={() => setElevClass(cls)}
              className={`px-3 py-1.5 text-xs font-medium cursor-pointer transition-colors ${
                elevClass.id === cls.id
                  ? 'bg-accent-DEFAULT text-white'
                  : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
              }`}
            >
              {t(cls.labelKey)}
            </button>
          ))}
        </div>

        {/* Country filter */}
        {availableCountries.length > 1 && (
          <select
            value={countryFilter}
            onChange={e => setCountryFilter(e.target.value)}
            className="rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900
                       px-2.5 py-1.5 text-xs text-neutral-700 dark:text-neutral-300
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40"
          >
            <option value="">{t('summitCollection.filters.allCountries')}</option>
            {availableCountries.map(c => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
        )}

        {/* Year filter */}
        {availableYears.length > 0 && (
          <select
            value={yearFilter}
            onChange={e => setYearFilter(e.target.value)}
            className="rounded-lg border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-900
                       px-2.5 py-1.5 text-xs text-neutral-700 dark:text-neutral-300
                       focus:outline-none focus:ring-2 focus:ring-accent-DEFAULT/40"
          >
            <option value="">{t('summitCollection.filters.allYears')}</option>
            {availableYears.map(y => <option key={y} value={y}>{y}</option>)}
          </select>
        )}

        {/* Summit name search */}
        <input
          type="text"
          value={nameFilter}
          onChange={e => setNameFilter(e.target.value)}
          placeholder={t('summitCollection.filters.searchPlaceholder')}
          className="rounded border border-neutral-300 dark:border-neutral-700 bg-white dark:bg-neutral-800
                     px-2 py-1.5 text-sm text-neutral-700 dark:text-neutral-300
                     focus:outline-none focus:ring-1 focus:ring-neutral-400 dark:focus:ring-neutral-600"
        />

        {(isFiltered || !!nameFilter.trim()) && (
          <button
            onClick={() => {
              setSearchParams(prev => {
                const next = new URLSearchParams(prev)
                next.delete(SUMMIT_COUNTRY_PARAM)
                next.delete(SUMMIT_YEAR_PARAM)
                next.delete(SUMMIT_ELEV_PARAM)
                if (allowLegacySummitParams) {
                  next.delete('country')
                  next.delete('year')
                  next.delete('elev')
                }
                return next
              }, { replace: true })
              setNameFilter('')
            }}
            className="text-xs px-2.5 py-1.5 rounded-lg border border-neutral-300 dark:border-neutral-700 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800 cursor-pointer transition-colors"
            aria-label={t('summitCollection.filters.resetAria')}
          >
            {t('summitCollection.filters.reset')}
          </button>
        )}

        {/* View toggle */}
        <div className="ml-auto flex rounded-lg border border-neutral-300 dark:border-neutral-700 overflow-hidden">
          <button
            onClick={() => setViewMode('list')}
            className={`px-3 py-1.5 text-sm font-medium cursor-pointer transition-colors ${
              viewMode === 'list'
                ? 'bg-accent-DEFAULT text-white'
                : 'bg-white dark:bg-neutral-900 text-neutral-600 dark:text-neutral-400 hover:bg-neutral-50 dark:hover:bg-neutral-800'
            }`}
          >
            <ListIcon />
          </button>
          <button
            onClick={() => setViewMode('map')}
            className={`px-3 py-1.5 text-sm font-medium cursor-pointer transition-colors ${
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
      {isFullscreen && (
        <SummitMapFullscreen
          initialSelected={selectedSummit}
          initialElevClass={elevClass}
          initialCountry={countryFilter}
          initialYear={yearFilter}
          onClose={() => { setIsFullscreen(false); setSelectedSummit(null) }}
        />
      )}

      {viewMode === 'list' ? (
        <div className="overflow-x-auto rounded-xl border border-neutral-200 dark:border-neutral-800">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-neutral-200 dark:border-neutral-800 bg-neutral-50 dark:bg-neutral-900/50">
                <SortableHeader field="name" label={t('summitCollection.table.name')} sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="elevation_m" label={t('summitCollection.table.elevation')} sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="first_visit" label={t('summitCollection.table.firstVisit')} sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="last_visit" label={t('summitCollection.table.lastVisit')} sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
                <SortableHeader field="visit_count" label={t('summitCollection.table.visits')} sortField={sortField} sortDir={sortDir} onToggle={toggleSort} indicator={sortIndicator} />
              </tr>
            </thead>
            <tbody>
              {displaySummits.map((summit, i) => (
                <tr
                  key={`${summit.osm_id ?? summit.name}-${i}`}
                  onClick={() => { setSelectedSummit(summit); setIsFullscreen(true) }}
                  className="border-b border-neutral-100 dark:border-neutral-800/50 hover:bg-neutral-50 dark:hover:bg-neutral-900/30 transition-colors cursor-pointer"
                >
                  <td className="px-4 py-3 font-medium text-neutral-900 dark:text-neutral-100">
                    <div className="flex items-center gap-2">
                      <MountainIconSmall />
                      {summit.name}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {summit.elevation_m != null ? formatElevation(summit.elevation_m) : '-'}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {formatDate(summit.first_visit, i18n.language)}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {formatDate(summit.last_visit, i18n.language)}
                  </td>
                  <td className="px-4 py-3 text-neutral-600 dark:text-neutral-400 tabular-nums">
                    {summit.visit_count}
                  </td>
                </tr>
              ))}
              {displaySummits.length === 0 && (
                <tr>
                  <td colSpan={5} className="px-4 py-8 text-center text-neutral-400">
                    {t('summitCollection.empty')}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      ) : (
        <SummitMap
          summits={displaySummits}
          selectedSummit={selectedSummit}
          onSelectSummit={handleSelectSummit}
          onMaximize={() => setIsFullscreen(true)}
        />
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
