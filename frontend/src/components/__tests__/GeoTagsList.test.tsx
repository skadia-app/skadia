import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { GeoTag } from '@/api/types'
import { i18n } from '@/i18n/i18n'

// ── Mock @tanstack/react-query ────────────────────────────────────────────────

let mockTags: GeoTag[] | undefined = undefined
const mockUserPrefs = vi.hoisted((): { locale: 'de' | 'en'; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: mockTags, isLoading: false }),
}))

// ── Mock @/api/enrichment (queryFn — not actually invoked via mock) ──────────

vi.mock('@/api/enrichment', () => ({
  fetchGeoTags: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

// ── Import after mocks ────────────────────────────────────────────────────────

import GeoTagsList from '../GeoTagsList'

const SAMPLE_TAGS: GeoTag[] = [
  {
    id: 'geo-1',
    activity_id: 'act-1',
    category: 'summit',
    name: 'Großglockner',
    ele_m: 3798,
    lat: 47.07,
    lng: 12.69,
    osm_id: 12345,
    created_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'geo-2',
    activity_id: 'act-1',
    category: 'still_water',
    name: 'Gosausee',
    ele_m: 933,
    lat: 47.54,
    lng: 13.5,
    dist_m: 500,
    osm_id: 67890,
    created_at: '2026-01-01T00:00:00Z',
  },
]

describe('GeoTagsList', () => {
  beforeEach(() => {
    mockTags = SAMPLE_TAGS
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    return i18n.changeLanguage('de')
  })

  it('renders in collapsed state by default — content not in DOM', () => {
    render(<GeoTagsList activityId="act-1" />)
    expect(screen.getByText(i18n.t('geoTags.title'))).toBeInTheDocument()
    expect(screen.queryByText('Großglockner')).not.toBeInTheDocument()
  })

  it('click on header expands and shows tag names', () => {
    render(<GeoTagsList activityId="act-1" />)
    fireEvent.click(screen.getByText(i18n.t('geoTags.title')))
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    expect(screen.getByText('Gosausee')).toBeInTheDocument()
  })

  it('formats geo-tag distance and elevation with user units', () => {
    mockUserPrefs.units = 'imperial'

    render(<GeoTagsList activityId="act-1" />)
    fireEvent.click(screen.getByText(i18n.t('geoTags.title')))

    expect(screen.getByText('(12.461 ft)')).toBeInTheDocument()
    expect(screen.getByText('3.061 ft')).toBeInTheDocument()
    expect(screen.getByText('1.640 ft')).toBeInTheDocument()
    expect(screen.queryByText('(3798 m)')).not.toBeInTheDocument()
    expect(screen.queryByText('500 m')).not.toBeInTheDocument()
  })

  it('click again collapses', () => {
    render(<GeoTagsList activityId="act-1" />)
    const header = screen.getByText(i18n.t('geoTags.title'))
    fireEvent.click(header)
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    fireEvent.click(header)
    expect(screen.queryByText('Großglockner')).not.toBeInTheDocument()
  })

  it('returns null when tags array is empty', () => {
    mockTags = []
    const { container } = render(<GeoTagsList activityId="act-1" />)
    expect(container.firstChild).toBeNull()
  })

  it('returns null when tags are undefined (loading)', () => {
    mockTags = undefined
    const { container } = render(<GeoTagsList activityId="act-1" />)
    expect(container.firstChild).toBeNull()
  })

  it('does not label every boundary settlement as Gemeinde', () => {
    mockTags = [{
      id: 'geo-settlement-1',
      activity_id: 'act-1',
      category: 'settlement',
      name: 'Affing',
      osm_id: 42,
      created_at: '2026-01-01T00:00:00Z',
      source: 'boundary',
      admin_level: 8,
    }]

    render(<GeoTagsList activityId="act-1" />)
    fireEvent.click(screen.getByText(i18n.t('geoTags.title')))

    expect(screen.getByText('Affing')).toBeInTheDocument()
    expect(screen.queryByText('Gemeinde')).not.toBeInTheDocument()
  })

  it('localizes geo-tag labels and hides raw unknown backend tokens', async () => {
    mockTags = [
      {
        id: 'geo-gastro-1',
        activity_id: 'act-1',
        category: 'gastronomy',
        name: 'Hütte am Weg',
        osm_id: 99,
        created_at: '2026-01-01T00:00:00Z',
        meta: { poi_type: 'alpine_hut' },
      },
      {
        id: 'geo-unknown-category',
        activity_id: 'act-1',
        category: 'new_backend_category',
        name: 'Unbekannter Ort',
        osm_id: 100,
        created_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'geo-unknown-poi',
        activity_id: 'act-1',
        category: 'gastronomy',
        name: 'Neuer POI',
        osm_id: 101,
        created_at: '2026-01-01T00:00:00Z',
        meta: { poi_type: 'new_backend_poi' },
      },
    ]

    for (const locale of ['de', 'en', 'fr', 'it', 'es']) {
      await i18n.changeLanguage(locale)
      const { unmount } = render(<GeoTagsList activityId="act-1" />)
      fireEvent.click(screen.getByRole('button'))

      expect(screen.queryByText('Geo-Tags')).not.toBeInTheDocument()
      expect(document.body).not.toHaveTextContent('alpine_hut')
      expect(document.body).not.toHaveTextContent('new_backend_category')
      expect(document.body).not.toHaveTextContent('new_backend_poi')
      expect(document.body).not.toHaveTextContent('geoTags.')

      unmount()
    }
  })
})
