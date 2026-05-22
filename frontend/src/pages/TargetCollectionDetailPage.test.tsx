import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import TargetCollectionDetailPage from './TargetCollectionDetailPage'

vi.mock('@/api/auth', () => ({
  getMe: vi.fn(async () => ({ user_id: 'viewer-1', is_admin: false })),
}))

vi.mock('@/api/targetObjects', () => ({
  getTargetCollection: vi.fn(async () => ({
    collection: {
      id: 'collection-1',
      owner_user_id: 'owner-1',
      name: 'Gemeinsame Sammlung',
      collection_kind: 'route_set',
      visibility: 'shared',
      owner_member_count: 1,
      members: [{
        id: 'member-1',
        collection_id: 'collection-1',
        member_kind: 'place',
        member_id: 'place-1',
        binding_mode: 'live_current',
        role: 'poi',
        sort_order: 0,
      }],
    },
  })),
  addTargetCollectionMember: vi.fn(),
  removeTargetCollectionMember: vi.fn(),
  createGovernanceScope: vi.fn(),
  addGovernanceRole: vi.fn(),
}))

vi.mock('@/api/segments', () => ({
  fetchSegments: vi.fn(async () => ({ segments: [], has_more: false })),
}))
vi.mock('@/api/routes', () => ({
  listRoutePublications: vi.fn(async () => ({ publications: [] })),
}))
vi.mock('@/api/privacy', () => ({
  listPrivacyZones: vi.fn(async () => []),
}))
vi.mock('@/api/tags', () => ({
  fetchTags: vi.fn(async () => []),
}))
vi.mock('@/api/targetOverlays', () => ({
  getTargetOverlaySummary: vi.fn(async () => ({ summary: null })),
  listTargetObservations: vi.fn(async () => ({ observations: [] })),
  listTargetReports: vi.fn(async () => ({ reports: [] })),
  listTargetUserContent: vi.fn(async () => ({ content: [] })),
  addTargetReportStatus: vi.fn(),
}))

function renderPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <MemoryRouter initialEntries={['/collections/collection-1']}>
      <QueryClientProvider client={queryClient}>
        <Routes>
          <Route path="/collections/:id" element={<TargetCollectionDetailPage />} />
        </Routes>
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('TargetCollectionDetailPage permissions', () => {
  it('hides membership and governance controls from non-managing viewers', async () => {
    renderPage()

    expect(await screen.findByText('Gemeinsame Sammlung')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Governance vorbereiten' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Hinzufügen' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Entfernen' })).not.toBeInTheDocument()
  })
})
