import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AutomatischeAnalysenSection, PhotoPreviewSection } from './SettingsPage'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      photo_embeddings: { configured: false, available: false, reason: 'core_profile' },
    },
  } as Record<string, unknown>,
  mutation: {
    mutate: vi.fn(),
    isPending: false,
  },
  getCapabilities: vi.fn(),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/api/routes', () => ({
  getCapabilities: () => mocks.getCapabilities(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ isAdmin: true }),
}))

vi.mock('@/lib/useEnrichmentSettings', () => ({
  useEnrichmentSettings: () => ({
    settings: {
      photo_preview_size: 'wqhd',
      video_clip_frames: 4,
      auto_title_display_policy: 'source_unless_generic',
      enable_auto_naming: true,
      summit_detection_radius_m: 50,
      climb_min_gradient_pct: 5,
      climb_min_elevation_m: 100,
      round_trip_threshold_m: 250,
    },
    isLoading: false,
    mutation: mocks.mutation,
  }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

vi.mock('@/components/ui/SettingsSection', () => ({
  SettingsSection: ({ title, description, children }: { title: string; description?: string; children: ReactNode }) => (
    <section aria-label={title}>
      <p>{description}</p>
      {children}
    </section>
  ),
}))

vi.mock('@/api/activityPhotos', () => ({
  recomputePhotoPreviews: vi.fn(),
  recomputePhotoThumbnails: vi.fn(),
  recomputePhotoEmbeddings: vi.fn(),
  backfillPhash: vi.fn(),
}))

vi.mock('@/pages/settings/AccountSettingsTab', () => ({ default: () => null }))
vi.mock('@/pages/settings/AdminSettingsTab', () => ({ default: () => null }))
vi.mock('@/pages/settings/MLServiceSection', () => ({ MLServiceSection: () => null }))
vi.mock('@/pages/settings/NotificationSettingsSection', () => ({ NotificationSettingsSection: () => null }))
vi.mock('./LLMSettingsPage', () => ({ default: () => null }))

function renderWithQueryClient(children: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <MemoryRouter>
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('PhotoPreviewSection deployment capabilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.capabilities = {
      features: {
        photo_embeddings: { configured: false, available: false, reason: 'core_profile' },
      },
    }
  })

  it('keeps preview recompute visible but hides photo embedding controls in Core', () => {
    render(<PhotoPreviewSection />)

    expect(screen.getByRole('button', { name: /Vorschauen neu erstellen/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Bildsuche neu indexieren/i })).not.toBeInTheDocument()
    expect(screen.queryByText(/Video-Analyse: Frames/i)).not.toBeInTheDocument()
  })

  it('shows photo embedding controls when photo embeddings are available', () => {
    mocks.capabilities = {
      features: {
        photo_embeddings: { configured: true, available: true },
      },
    }

    render(<PhotoPreviewSection />)

    expect(screen.getByRole('button', { name: /Bildsuche neu indexieren/i })).toBeInTheDocument()
    expect(screen.getByText(/Video-Analyse: Frames/i)).toBeInTheDocument()
  })
})

describe('AutomatischeAnalysenSection deployment capabilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getCapabilities.mockResolvedValue({
      features: {
        geo_poi_enrichment: { configured: false, available: false, reason: 'core_profile' },
      },
    })
  })

  it('hides summit radius settings when geodata enrichment is unavailable', async () => {
    renderWithQueryClient(<AutomatischeAnalysenSection />)

    await screen.findByRole('region', { name: 'Automatische Analysen' })

    expect(screen.queryByLabelText(/Gipfel-Erkennungs-Radius/i)).not.toBeInTheDocument()
  })
})
