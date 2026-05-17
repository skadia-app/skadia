import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { RoutePlannerPanelProps } from '../RoutePlannerPanel'
import RoutePlannerPanel from '../RoutePlannerPanel'
import { ModalProvider } from '@/lib/modal'
import { ToastProvider } from '@/lib/toast'
import * as routesApi from '@/api/routes'

vi.mock('@/api/routes', () => ({
  getProfileDefaults: vi.fn().mockResolvedValue({ profile: 'hiking' }),
}))

vi.mock('@/api/routingProfiles', () => ({
  fetchAllProfiles: vi.fn().mockResolvedValue({ profiles: [] }),
  createProfile: vi.fn().mockResolvedValue({}),
  deleteProfile: vi.fn().mockResolvedValue({}),
  importProfiles: vi.fn().mockResolvedValue({ imported: 0 }),
  exportProfileUrl: vi.fn(() => '/api/v1/routing/profiles/profile-1/export'),
}))

vi.mock('@/api/client', () => ({
  apiDownload: vi.fn(),
}))

function renderPanel(overrides: Partial<RoutePlannerPanelProps> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })

  const props: RoutePlannerPanelProps = {
    waypoints: [],
    segments: [],
    profileConfig: { profile: 'hiking' },
    showHeatmap: false,
    routingEngine: 'badrouter',
    brouterProfile: 'trekking',
    badRouterAvailable: false,
    routeSuggestionAvailable: false,
    onRoutingEngineChange: vi.fn(),
    onBrouterProfileChange: vi.fn(),
    totalDistanceM: 0,
    totalElevationGain: 0,
    totalElevationLoss: 0,
    onProfileChange: vi.fn(),
    onRemoveWaypoint: vi.fn(),
    onReorderWaypoints: vi.fn(),
    onRecalculateAll: vi.fn(),
    onToggleSegmentMode: vi.fn(),
    onToggleHeatmap: vi.fn(),
    onClearRoute: vi.fn(),
    onSave: vi.fn(),
    onSuggest: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  }

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <ModalProvider>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </ModalProvider>
      </ToastProvider>
    )
  }

  return render(<RoutePlannerPanel {...props} />, { wrapper: Wrapper })
}

describe('RoutePlannerPanel capability gates', () => {
  it('hides BadRouter-only controls and falls back to BRouter surface in Core', async () => {
    renderPanel()

    expect(screen.queryByRole('button', { name: 'BadRouter' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'BRouter' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Route vorschlagen/ })).not.toBeInTheDocument()
    expect(screen.queryByTitle('Profilverwaltung')).not.toBeInTheDocument()

    await waitFor(() => {
      expect(routesApi.getProfileDefaults).not.toHaveBeenCalled()
    })
  })
})
