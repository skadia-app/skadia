import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import PhotoGalleryPage from './PhotoGalleryPage'

const mocks = vi.hoisted(() => ({
  capabilities: {
    features: {
      photo_embeddings: { configured: false, available: false, reason: 'core_profile' },
    },
  } as Record<string, unknown>,
  listPhotoFeed: vi.fn(),
  searchPhotos: vi.fn(),
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/api/photos', async () => {
  const actual = await vi.importActual<typeof import('@/api/photos')>('@/api/photos')
  return {
    ...actual,
    listPhotoFeed: mocks.listPhotoFeed,
    searchPhotos: mocks.searchPhotos,
  }
})

vi.mock('@/components/ActivityMiniCard', () => ({
  default: () => <div data-testid="activity-mini-card" />,
}))

vi.mock('@/components/AuthenticatedImage', () => ({
  default: ({ src, alt, className }: { src?: string; alt?: string; className?: string }) => (
    <img src={src} alt={alt} className={className} />
  ),
}))

vi.mock('@/components/AuthenticatedMedia', () => ({
  default: () => <div data-testid="authenticated-media" />,
}))

function renderGallery() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <PhotoGalleryPage />
    </QueryClientProvider>,
  )
}

describe('PhotoGalleryPage deployment capabilities', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.capabilities = {
      features: {
        photo_embeddings: { configured: false, available: false, reason: 'core_profile' },
      },
    }
    mocks.listPhotoFeed.mockResolvedValue({ groups: [], next_page: null })
    mocks.searchPhotos.mockResolvedValue([])
  })

  it('keeps the normal gallery feed visible while hiding semantic photo search in Core', async () => {
    renderGallery()

    expect(screen.queryByPlaceholderText(/Fotos durchsuchen/i)).not.toBeInTheDocument()
    expect(await screen.findByText('Noch keine Fotos vorhanden')).toBeInTheDocument()
    expect(mocks.listPhotoFeed).toHaveBeenCalled()
    expect(mocks.searchPhotos).not.toHaveBeenCalled()
  })

  it('shows semantic search when photo embeddings are available', async () => {
    mocks.capabilities = {
      features: {
        photo_embeddings: { configured: true, available: true },
      },
    }

    renderGallery()

    expect(await screen.findByPlaceholderText(/Fotos durchsuchen/i)).toBeInTheDocument()
  })
})
