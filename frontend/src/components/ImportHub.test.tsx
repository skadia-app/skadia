import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import '@/i18n/i18n'
import ImportHub from './ImportHub'

const mocks = vi.hoisted(() => ({
  fetchImportQueueCount: vi.fn<() => Promise<number>>(),
  getShadowFSSettings: vi.fn<() => Promise<{ hotfolder_enabled: boolean }>>(),
}))

vi.mock('@/api/importQueue', async () => {
  const actual = await vi.importActual<typeof import('@/api/importQueue')>('@/api/importQueue')
  return {
    ...actual,
    fetchImportQueueCount: mocks.fetchImportQueueCount,
  }
})

vi.mock('@/api/settings', () => ({
  getShadowFSSettings: mocks.getShadowFSSettings,
}))

function LocationProbe() {
  const location = useLocation()
  return <div data-testid="location">{location.pathname + location.search}</div>
}

function renderHub(initialEntry = '/') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  return render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <ImportHub />
        <Routes>
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

describe('ImportHub', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.fetchImportQueueCount.mockResolvedValue(0)
    mocks.getShadowFSSettings.mockResolvedValue({ hotfolder_enabled: false })
  })

  it('navigates importer shortcuts to the central imports page', async () => {
    const user = userEvent.setup()
    renderHub('/dashboard')

    await user.click(screen.getByRole('button', { name: 'Import-Hub öffnen' }))
    await user.click(await screen.findByRole('button', { name: /Garmin Connect/i }))

    expect(screen.getByTestId('location')).toHaveTextContent('/imports?source=garmin')
  })

  it('navigates the queue badge to manual review', async () => {
    mocks.fetchImportQueueCount.mockResolvedValue(3)
    const user = userEvent.setup()
    renderHub('/dashboard')

    await user.click(screen.getByRole('button', { name: 'Import-Hub öffnen' }))
    await user.click(await screen.findByRole('button', { name: /3 zu prüfen/i }))

    expect(screen.getByTestId('location')).toHaveTextContent('/imports?view=review')
  })
})
