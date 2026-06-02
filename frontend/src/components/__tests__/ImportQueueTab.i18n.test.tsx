import '@testing-library/jest-dom/vitest'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ImportQueueTab from '../ImportQueueTab'
import type { ImportQueueItem } from '@/api/importQueue'
import { i18n } from '@/i18n/i18n'

const mocks = vi.hoisted(() => ({
  fetchImportQueue: vi.fn(),
  assignQueueItem: vi.fn(),
  patchQueueItem: vi.fn(),
  deleteQueueItem: vi.fn(),
  resolvePdfQueueItem: vi.fn(),
}))

vi.mock('@/api/importQueue', () => ({
  fetchImportQueue: mocks.fetchImportQueue,
  assignQueueItem: mocks.assignQueueItem,
  patchQueueItem: mocks.patchQueueItem,
  deleteQueueItem: mocks.deleteQueueItem,
  resolvePdfQueueItem: mocks.resolvePdfQueueItem,
}))

vi.mock('@/api/gear', () => ({
  fetchGear: vi.fn(async () => []),
}))

vi.mock('@/api/sportConflict', () => ({
  resolveSportConflict: vi.fn(),
}))

vi.mock('@/api/activities', () => ({
  fetchActivities: vi.fn(async () => ({ activities: [] })),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
}

describe('ImportQueueTab i18n labels', () => {
  it('uses a localized generic item type label for unknown queue types', async () => {
    await i18n.changeLanguage('de')
    const unknownType = 'private_backend_type'
    const item: ImportQueueItem = {
      id: 'queue-1',
      item_type: unknownType as ImportQueueItem['item_type'],
      status: 'pending',
      filename: 'upload.fit',
      created_at: '2026-05-31T10:00:00Z',
    }
    mocks.fetchImportQueue.mockResolvedValueOnce([item])

    render(<ImportQueueTab />, { wrapper })

    expect(await screen.findByText('Unbekannter Eintragstyp')).toBeInTheDocument()
    expect(screen.getByText('Dieser Eintrag kann nicht automatisch verarbeitet werden.')).toBeInTheDocument()
    expect(screen.queryByText(unknownType, { exact: false })).not.toBeInTheDocument()
  })

  it('uses a localized generic source label for unknown conflict sources', async () => {
    await i18n.changeLanguage('de')
    const unknownSource = 'private_backend_source'
    const item: ImportQueueItem = {
      id: 'queue-2',
      item_type: 'sport_conflict',
      status: 'pending',
      filename: 'upload.fit',
      created_at: '2026-05-31T10:00:00Z',
      metadata: {
        platform_sport: 'ride',
        file_sport: 'run',
        platform_source: unknownSource,
        file_source: 'fit',
      },
    }
    mocks.fetchImportQueue.mockResolvedValueOnce([item])

    render(<ImportQueueTab />, { wrapper })

    await waitFor(() => {
      expect(document.body).toHaveTextContent('Unbekannte Quelle')
    })
    expect(screen.queryByText(unknownSource, { exact: false })).not.toBeInTheDocument()
  })
})
