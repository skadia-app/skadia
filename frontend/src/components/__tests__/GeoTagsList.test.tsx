import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { GeoTag } from '@/api/types'

// ── Mock @tanstack/react-query ────────────────────────────────────────────────

let mockTags: GeoTag[] | undefined = undefined

vi.mock('@tanstack/react-query', () => ({
  useQuery: () => ({ data: mockTags, isLoading: false }),
}))

// ── Mock @/api/enrichment (queryFn — not actually invoked via mock) ──────────

vi.mock('@/api/enrichment', () => ({
  fetchGeoTags: vi.fn(),
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
  })

  it('renders in collapsed state by default — content not in DOM', () => {
    render(<GeoTagsList activityId="act-1" />)
    expect(screen.getByText('Geo-Tags')).toBeInTheDocument()
    expect(screen.queryByText('Großglockner')).not.toBeInTheDocument()
  })

  it('click on header expands and shows tag names', () => {
    render(<GeoTagsList activityId="act-1" />)
    fireEvent.click(screen.getByText('Geo-Tags'))
    expect(screen.getByText('Großglockner')).toBeInTheDocument()
    expect(screen.getByText('Gosausee')).toBeInTheDocument()
  })

  it('click again collapses', () => {
    render(<GeoTagsList activityId="act-1" />)
    const header = screen.getByText('Geo-Tags')
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
    fireEvent.click(screen.getByText('Geo-Tags'))

    expect(screen.getByText('Affing')).toBeInTheDocument()
    expect(screen.queryByText('Gemeinde')).not.toBeInTheDocument()
  })
})
