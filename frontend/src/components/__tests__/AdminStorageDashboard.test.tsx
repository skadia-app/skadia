import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/api/client'

const { addToastMock, apiClientMock } = vi.hoisted(() => ({
  addToastMock: vi.fn(),
  apiClientMock: vi.fn(),
}))

vi.mock('@/api/client', async () => {
  const actual = await vi.importActual<typeof import('@/api/client')>('@/api/client')
  return {
    ...actual,
    apiClient: apiClientMock,
  }
})

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: addToastMock }),
}))

vi.mock('@/pages/settings/AdminStorageLimitsSection', () => ({
  AdminStorageLimitsSection: () => <div data-testid="storage-limits" />,
}))

import AdminStorageDashboard from '@/components/AdminStorageDashboard'

const dashboardData = {
  backend: 'local',
  base_path: '/data/skadia',
  keep_versions: 1,
  sync_enabled: true,
  users: [],
  type_counts: {},
}

function renderDashboard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AdminStorageDashboard />
    </QueryClientProvider>,
  )
}

describe('AdminStorageDashboard error fallbacks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiClientMock.mockImplementation((path: string) => {
      if (path === '/admin/storage') return Promise.resolve(dashboardData)
      if (path === '/admin/storage/reconcile') return Promise.reject(new Error('API error 500: {"detail":"stack trace"}'))
      return Promise.resolve({})
    })
  })

  it('does not expose raw technical action errors as toast copy', async () => {
    renderDashboard()

    await screen.findByText('Speicher-Übersicht')
    await userEvent.click(screen.getByRole('button', { name: 'Abgleich starten' }))

    await waitFor(() => {
      expect(addToastMock).toHaveBeenCalledWith({
        type: 'error',
        title: 'Abgleich fehlgeschlagen',
        duration: 0,
      })
    })
  })

  it('maps structured already-running action errors to the existing running label', async () => {
    apiClientMock.mockImplementation((path: string) => {
      if (path === '/admin/storage') return Promise.resolve(dashboardData)
      if (path === '/admin/storage/reconcile') {
        return Promise.reject(new ApiError(409, '{"error":"already_running"}', { error: 'already_running' }))
      }
      return Promise.resolve({})
    })

    renderDashboard()

    await screen.findByText('Speicher-Übersicht')
    await userEvent.click(screen.getByRole('button', { name: 'Abgleich starten' }))

    await waitFor(() => {
      expect(addToastMock).toHaveBeenCalledWith({
        type: 'warning',
        title: 'Abgleich läuft bereits',
      })
    })
  })
})
