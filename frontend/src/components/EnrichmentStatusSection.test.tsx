import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { i18n } from '@/i18n/i18n'
import EnrichmentStatusSection from './EnrichmentStatusSection'

const mocks = vi.hoisted(() => ({
  fetchEnrichmentStatus: vi.fn(),
  fetchEnrichmentSettings: vi.fn(),
  addToast: vi.fn(),
}))

vi.mock('@/api/enrichment', async () => {
  const actual = await vi.importActual<typeof import('@/api/enrichment')>('@/api/enrichment')
  return {
    ...actual,
    fetchEnrichmentStatus: mocks.fetchEnrichmentStatus,
    fetchEnrichmentSettings: mocks.fetchEnrichmentSettings,
  }
})

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: mocks.addToast }),
}))

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <EnrichmentStatusSection
          activityId="activity-1"
          enrichmentStatus="done"
          hiddenModules={new Set()}
          onToggleModule={vi.fn()}
        />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('EnrichmentStatusSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    void i18n.changeLanguage('de')
    mocks.fetchEnrichmentSettings.mockResolvedValue({})
    mocks.fetchEnrichmentStatus.mockResolvedValue({
      modules: [{
        name: 'backend_added_module',
        status: 'completed',
        count: 1,
      }],
    })
  })

  it('does not expose unknown enrichment module names', async () => {
    renderSection()

    fireEvent.click(await screen.findByText(i18n.t('enrichmentStatus.title')))

    await waitFor(() => {
      expect(screen.getByText(i18n.t('enrichmentStatus.modules.unknown.label'))).toBeInTheDocument()
    })
    expect(document.body).not.toHaveTextContent('backend_added_module')
  })
})
