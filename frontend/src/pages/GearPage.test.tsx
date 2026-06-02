import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { beforeEach, describe, it, expect, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import GearPage from './GearPage'
import { downloadGearDocumentBlob, fetchGear, fetchGearDocuments } from '@/api/gear'
import type { Gear, GearDocument } from '@/api/types'

const mockUserPrefs = vi.hoisted((): { locale: string; units: 'metric' | 'imperial' } => ({
  locale: 'de',
  units: 'metric',
}))

// Stub out API calls so tests don't reach the network.
vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(() => Promise.resolve([])),
  createGear: vi.fn(),
  fetchGearDocuments: vi.fn(() => Promise.resolve([])),
  downloadGearDocumentBlob: vi.fn(() => Promise.resolve('blob:gear-photo')),
}))
vi.mock('@/api/hardware', () => ({
  fetchDevices: vi.fn(() => Promise.resolve([])),
}))
vi.mock('@/components/MaintenanceAlerts', () => ({
  default: () => null,
}))
vi.mock('@/components/GearMergePanel', () => ({
  default: () => null,
}))
vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mockUserPrefs,
}))

function renderGearPage(initialEntry = '/gear') {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <GearPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('GearPage', () => {
  beforeEach(() => {
    mockUserPrefs.locale = 'de'
    mockUserPrefs.units = 'metric'
    vi.mocked(fetchGear).mockResolvedValue([])
    vi.mocked(fetchGearDocuments).mockResolvedValue([])
    vi.mocked(downloadGearDocumentBlob).mockResolvedValue('blob:gear-photo')
  })

  it('TestGearPageRendersAllEightScopes', async () => {
    renderGearPage()

    // The page renders 8 gear category tiles plus hardware tiles.
    // The 8 gear scopes are: bike, shoes, winter, water, diving, climb, apparel, other.
    const expectedTiles = [
      'Räder',
      'Schuhe',
      'Wintersport',
      'Wassersport',
      'Tauchen',
      'Klettern',
      'Bekleidung',
      'Sonstiges',
    ]

    for (const title of expectedTiles) {
      expect(screen.getByText(title)).toBeInTheDocument()
    }
  })

  it('TestGearPageIncludesBikeLeafTypesInBikeScope', async () => {
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({
        id: 'bike-1',
        name: 'Enduro',
        type: 'bike.mtb.enduro',
        scope: 'bike',
      }),
    ])

    renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Enduro')).toBeInTheDocument()
    expect(screen.getByText('Fahrrad / Mountainbike / Enduro')).toBeInTheDocument()
  })

  it('TestGearCardImageLoadsProtectedPhotoWithAuthenticatedBlobFetch', async () => {
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({ id: 'bike-1', name: 'Bulls 600', type: 'bike', scope: 'bike' }),
    ])
    vi.mocked(fetchGearDocuments).mockResolvedValue([
      gearDocumentFixture({ id: 'doc-photo', gear_id: 'bike-1' }),
    ])
    vi.mocked(downloadGearDocumentBlob).mockResolvedValue('blob:doc-photo')

    const { container } = renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Bulls 600')).toBeInTheDocument()
    await waitFor(() => {
      expect(downloadGearDocumentBlob).toHaveBeenCalledWith('doc-photo')
    })
    expect(container.querySelector('img')).toHaveAttribute('src', 'blob:doc-photo')
  })

  it('TestGearCardUsesCompactLifetimeStatsAndHidesGenericSportBadge', async () => {
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({
        id: 'bike-1',
        name: 'Propain Tyee 2020',
        type: 'bike',
        scope: 'bike',
        total_distance_m: 3_262_140,
        total_time_s: 1_590_180,
        applicable_sport_predicates: [
          { sport_type: 'generic' },
          { sport_type: 'cycling' },
        ],
      }),
    ])

    renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Propain Tyee 2020')).toBeInTheDocument()
    expect(screen.getByText('3.262,1 km')).toBeInTheDocument()
    expect(screen.getByText('442 h')).toBeInTheDocument()
    expect(screen.getByText('Radfahren')).toBeInTheDocument()
    expect(screen.queryByText('Aktivität')).not.toBeInTheDocument()
    expect(screen.queryByText('3262.14 km')).not.toBeInTheDocument()
    expect(screen.queryByText('441h 43min')).not.toBeInTheDocument()
  })

  it('TestGearCardUsesUserUnitPreferenceForDistanceAndElevation', async () => {
    mockUserPrefs.locale = 'en'
    mockUserPrefs.units = 'imperial'
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({
        id: 'bike-1',
        name: 'Unit Bike',
        type: 'bike',
        scope: 'bike',
        total_distance_m: 3218.688,
        total_elevation_gain_m: 100,
      }),
    ])

    renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Unit Bike')).toBeInTheDocument()
    expect(screen.getByText('2.0 mi')).toBeInTheDocument()
    expect(screen.getByText('+328 ft')).toBeInTheDocument()
    expect(screen.queryByText('3 km')).not.toBeInTheDocument()
    expect(screen.queryByText('+100 m')).not.toBeInTheDocument()
  })

  it('does not expose unknown backend gear status values as labels', async () => {
    vi.mocked(fetchGear).mockResolvedValue([
      gearFixture({
        id: 'gear-status-1',
        name: 'Status Test',
        status: 'private_backend_status' as Gear['status'],
      }),
    ])

    renderGearPage('/gear?type=bike')

    expect(await screen.findByText('Status Test')).toBeInTheDocument()
    expect(screen.getByText('Unbekannter Status')).toBeInTheDocument()
    expect(screen.queryByText('private_backend_status')).not.toBeInTheDocument()
  })
})

function gearFixture(overrides: Partial<Gear> = {}): Gear {
  return {
    id: 'gear-1',
    user_id: 'user-1',
    name: 'Rad',
    type: 'bike',
    total_distance_m: 0,
    total_elevation_gain_m: 0,
    total_elevation_loss_m: 0,
    total_time_s: 0,
    total_moving_time_s: 0,
    total_activities: 0,
    is_retired: false,
    status: 'active',
    applicable_sport_predicates: [],
    created_at: '2026-05-11T00:00:00Z',
    ...overrides,
  }
}

function gearDocumentFixture(overrides: Partial<GearDocument> = {}): GearDocument {
  return {
    id: 'doc-1',
    gear_id: 'gear-1',
    component_id: null,
    service_event_id: null,
    doc_type: 'photo',
    title: null,
    original_filename: 'photo.jpg',
    content_type: 'image/jpeg',
    file_size_bytes: 1024,
    file_hash: 'hash',
    processing_status: 'processed',
    created_at: '2026-05-11T00:00:00Z',
    ...overrides,
  }
}
