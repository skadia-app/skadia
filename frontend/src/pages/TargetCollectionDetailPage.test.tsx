import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import TargetCollectionDetailPage from './TargetCollectionDetailPage'
import type { TargetCollectionDetail } from '@/api/targetObjects'

const mocks = vi.hoisted(() => ({
  me: { user_id: 'viewer-1', is_admin: false },
  addToast: vi.fn(),
  deleteTargetCollection: vi.fn(),
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
  } as TargetCollectionDetail,
}))

vi.mock('@/api/auth', () => ({
  getMe: vi.fn(async () => mocks.me),
}))

vi.mock('@/api/targetObjects', () => ({
  getTargetCollection: vi.fn(async () => ({
    collection: mocks.collection,
  })),
  addTargetCollectionMember: vi.fn(),
  deleteTargetCollection: mocks.deleteTargetCollection,
  removeTargetCollectionMember: vi.fn(),
  updateTargetCollection: vi.fn(),
  updateTargetCollectionMember: vi.fn(),
  createGovernanceScope: vi.fn(),
  addGovernanceRole: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: mocks.addToast }),
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
  beforeEach(() => {
    mocks.me = { user_id: 'viewer-1', is_admin: false }
    mocks.addToast.mockReset()
    mocks.deleteTargetCollection.mockReset()
    mocks.deleteTargetCollection.mockResolvedValue({ ok: true })
    mocks.collection = {
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
    } as TargetCollectionDetail
  })

  it('hides membership and governance controls from non-managing viewers', async () => {
    renderPage()

    expect(await screen.findByText('Gemeinsame Sammlung')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Governance vorbereiten' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Hinzufügen' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Entfernen' })).not.toBeInTheDocument()
  })

  it('shows member controls for delegated managers without offering governance bootstrap', async () => {
    mocks.collection = {
      ...mocks.collection,
      effective_permissions: ['manage_members'],
    }

    renderPage()

    expect(await screen.findByText('Gemeinsame Sammlung')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Hinzufügen' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Entfernen' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Governance vorbereiten' })).not.toBeInTheDocument()
  })

  it('allows delegated archive permission to archive a collection with feedback', async () => {
    mocks.collection = {
      ...mocks.collection,
      effective_permissions: ['hide_or_archive'],
    }

    renderPage()

    fireEvent.click(await screen.findByRole('button', { name: 'Archivieren' }))

    await waitFor(() => expect(mocks.deleteTargetCollection).toHaveBeenCalledWith('collection-1'))
    await waitFor(() => expect(mocks.addToast).toHaveBeenCalledWith(expect.objectContaining({
      type: 'success',
      title: 'Sammlung archiviert',
    })))
  })

  it('shows a read-only permission audit for delegated collection access', async () => {
    mocks.collection = {
      ...mocks.collection,
      effective_permissions: ['manage_members', 'manage_reports'],
    }

    renderPage()

    expect(await screen.findByText('Berechtigungsstatus')).toBeInTheDocument()
    expect(screen.getByText('manage_members, manage_reports')).toBeInTheDocument()
    expect(screen.getByText(/Governance-Verwaltung ist hier noch nicht vollständig verfügbar/)).toBeInTheDocument()
  })
})
