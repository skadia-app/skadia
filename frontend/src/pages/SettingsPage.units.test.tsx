import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SettingsPage from './SettingsPage'
import { i18n } from '@/i18n/i18n'

const mocks = vi.hoisted(() => ({
  userPrefs: { locale: 'en', units: 'imperial' as 'metric' | 'imperial', isAdmin: false },
  enrichmentMutate: vi.fn(),
  updateHeatmapSettings: vi.fn(),
  updateImportSettings: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mocks.userPrefs,
}))

vi.mock('@/lib/accentColor', () => ({
  ACCENT_PALETTE: [{ shade500: '#f97316' }],
  DEFAULT_ACCENT_COLOR: '#f97316',
  deriveAccentEntry: (hex: string) => ({ shade500: hex }),
  useAccentColor: () => ({ accent: { shade500: '#f97316' }, setAccent: vi.fn() }),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: { features: {} } }),
}))

vi.mock('@/hooks/useMapLayers', () => ({
  useMapLayers: () => ({ base: [{ key: 'osm', name: 'OSM' }], disabled: [] }),
}))

vi.mock('@/api/settings', () => ({
  getShadowFSSettings: vi.fn(async () => ({ hotfolder_enabled: false, hotfolder_path: '' })),
  updateShadowFSSettings: vi.fn(),
  getHeatmapSettings: vi.fn(async () => ({
    sampling_interval_m: 20,
    teleport_threshold_m: 2000,
    teleport_filter_map: true,
    teleport_filter_activity: true,
  })),
  updateHeatmapSettings: mocks.updateHeatmapSettings,
}))

vi.mock('@/api/importWizard', () => ({
  getImportSettings: vi.fn(async () => ({
    conflict_strategy: 'enrich_missing',
    gps_duplicate_threshold: 75,
    fusion_time_window_min: 30,
  })),
  updateImportSettings: mocks.updateImportSettings,
}))

vi.mock('@/lib/useEnrichmentSettings', () => ({
  useEnrichmentSettings: () => ({
    settings: {
      geocode_fallback_provider: 'off',
      terra_incognita_radius_m: 1000,
      route_duplicate_hausdorff_m: 100,
      route_duplicate_length_diff_km: 0.5,
      phash_threshold_bits: 3,
      auto_title_display_policy: 'source_unless_generic',
      enable_auto_naming: true,
      summit_detection_radius_m: 50,
      climb_min_gradient_pct: 5,
      climb_min_elevation_m: 100,
      round_trip_threshold_m: 250,
    },
    isLoading: false,
    isError: false,
    mutation: { mutate: mocks.enrichmentMutate, isPending: false },
  }),
}))

vi.mock('@/api/routes', () => ({
  getCapabilities: vi.fn(async () => ({
    ml_service: false,
    features: { geo_poi_enrichment: { configured: true, available: true } },
  })),
}))

vi.mock('@/api/enrichment', () => ({
  backfillEnrichment: vi.fn(),
  fetchGeocodingGap: vi.fn(async () => ({ count: 0 })),
  startGeocodingBackfill: vi.fn(),
}))

vi.mock('@/api/activityPhotos', () => ({
  recomputePhotoPreviews: vi.fn(),
  recomputePhotoThumbnails: vi.fn(),
  recomputePhotoEmbeddings: vi.fn(),
  backfillPhash: vi.fn(),
}))

vi.mock('@/api/jobs', () => ({
  fetchActiveJobs: vi.fn(async () => ({ jobs: [], pending_enrichments: 0 })),
}))

vi.mock('@/api/stats', () => ({
  fetchSportTypeStats: vi.fn(async () => []),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/components/FileSchemaSettings', () => ({ default: () => null }))
vi.mock('@/components/WeatherSettings', () => ({ WeatherSettings: () => null }))
vi.mock('@/components/UserCustomLayersSection', () => ({ UserCustomLayersSection: () => null }))
vi.mock('@/pages/settings/AccountSettingsTab', () => ({ default: () => null }))
vi.mock('@/pages/settings/AdminSettingsTab', () => ({ default: () => null }))
vi.mock('@/pages/settings/MLServiceSection', () => ({ MLServiceSection: () => null }))
vi.mock('@/pages/settings/NotificationSettingsSection', () => ({ NotificationSettingsSection: () => null }))
vi.mock('./LLMSettingsPage', () => ({ default: () => null }))
vi.mock('@/components/ChatProfileSettings', () => ({ default: () => null }))
vi.mock('@/components/ChatWriteToolSettings', () => ({ default: () => null }))

function renderSettings(initialEntry: string) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <QueryClientProvider client={client}>
        <SettingsPage />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('SettingsPage unit preferences', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mocks.userPrefs.locale = 'en'
    mocks.userPrefs.units = 'imperial'
    mocks.userPrefs.isAdmin = false
    await i18n.changeLanguage('en')
  })

  it('shows map and import distance slider labels in imperial units', async () => {
    const { unmount } = renderSettings('/settings?tab=karte')

    expect(await screen.findByText('Map settings')).toBeInTheDocument()
    expect(screen.getByText('3,281 ft')).toBeInTheDocument()
    expect(screen.getByText('328 ft')).toBeInTheDocument()
    expect(screen.getByText('328 ft (fast)')).toBeInTheDocument()
    expect(screen.getByText('6.2 mi')).toBeInTheDocument()
    expect(screen.getByText('6.2 mi (generous)')).toBeInTheDocument()
    expect(screen.getByText('66 ft')).toBeInTheDocument()
    expect(screen.getByText('33 ft (detailed)')).toBeInTheDocument()
    expect(screen.getByText('1.2 mi')).toBeInTheDocument()

    unmount()
    renderSettings('/settings?tab=import')

    expect(await screen.findByRole('heading', { name: 'Import' })).toBeInTheDocument()
    expect(screen.getByText('328 ft')).toBeInTheDocument()
    expect(screen.getByText('1,640 ft')).toBeInTheDocument()
    expect(screen.getByText('1.2 mi (more generous)')).toBeInTheDocument()
  })

  it('edits analysis threshold inputs in imperial units while storing meters', async () => {
    renderSettings('/settings?tab=analysen')

    expect(await screen.findByText('164 ft')).toBeInTheDocument()
    const elevationInput = screen.getByDisplayValue('328.084')
    expect(screen.getByText('ft')).toBeInTheDocument()
    expect(screen.getByText('mi')).toBeInTheDocument()

    fireEvent.change(elevationInput, { target: { value: '656.168' } })
    fireEvent.blur(elevationInput)

    await waitFor(() => {
      expect(mocks.enrichmentMutate).toHaveBeenCalledWith({ climb_min_elevation_m: 200 })
    })
  })
})
