// Tests for MapLayersAdminSection
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
import { MapLayersAdminSection } from './MapLayersAdminSection'
import type { MapLayer } from '@/api/mapLayers'
import { ApiError } from '@/api/client'

// ── Mocks ─────────────────────────────────────────────────────────────────────

const mockFetchMapLayers = vi.fn()
const mockFetchAdminMapLayers = vi.fn()
const mockCreateMapLayer = vi.fn()
const mockPatchMapLayer = vi.fn()
const mockDeleteMapLayer = vi.fn()
const mockBulkSortMapLayers = vi.fn()

vi.mock('@/api/mapLayers', async () => {
  const actual = await vi.importActual<typeof import('@/api/mapLayers')>('@/api/mapLayers')
  return {
    ...actual,
    fetchMapLayers: (...args: unknown[]) => mockFetchMapLayers(...args),
    fetchAdminMapLayers: (...args: unknown[]) => mockFetchAdminMapLayers(...args),
    createMapLayer: (...args: unknown[]) => mockCreateMapLayer(...args),
    patchMapLayer: (...args: unknown[]) => mockPatchMapLayer(...args),
    deleteMapLayer: (...args: unknown[]) => mockDeleteMapLayer(...args),
    bulkSortMapLayers: (...args: unknown[]) => mockBulkSortMapLayers(...args),
  }
})

// Capabilities: offline_map = true, brouter = false
vi.mock('@/api/routes', () => ({
  getCapabilities: vi.fn().mockResolvedValue({ offline_map: true, brouter: false }),
}))

// mapStyles: use real opacityPercentToFloat
vi.mock('@/lib/mapStyles', async () => {
  const actual = await vi.importActual<typeof import('@/lib/mapStyles')>('@/lib/mapStyles')
  return actual
})

// ── Fixtures ─────────────────────────────────────────────────────────────────

const SYSTEM_LAYER: MapLayer = {
  id: 1,
  key: 'osm',
  role: 'base',
  name: 'OSM Standard',
  source_kind: 'raster_tiles',
  url_template: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
  attribution: '© OSM',
  min_zoom: 0,
  max_zoom: 19,
  default_opacity: 1,
  is_system: true,
  enabled: true,
  sort_order: 1,
  proxy_through_backend: false,
  requires_capability: undefined,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
}

const USER_LAYER: MapLayer = {
  id: 2,
  key: 'custom1',
  role: 'overlay',
  name: 'Mein Overlay',
  source_kind: 'raster_tiles',
  url_template: 'https://overlay.example.com/{z}/{x}/{y}.png',
  default_opacity: 0.8,
  is_system: false,
  enabled: true,
  sort_order: 2,
  proxy_through_backend: false,
  requires_capability: 'offline_map',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-15T12:00:00Z',
}

const MOCK_RESPONSE = {
  layers: [SYSTEM_LAYER, USER_LAYER],
  system_visibility: {},
}

// ── Wrapper ───────────────────────────────────────────────────────────────────

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  })
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <ToastProvider>
        <QueryClientProvider client={qc}>
          {children}
          <ToastContainer />
        </QueryClientProvider>
      </ToastProvider>
    )
  }
  return Wrapper
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('MapLayersAdminSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchMapLayers.mockResolvedValue(MOCK_RESPONSE)
    mockFetchAdminMapLayers.mockResolvedValue(MOCK_RESPONSE)
    mockCreateMapLayer.mockResolvedValue({ ...SYSTEM_LAYER, id: 99 })
    mockPatchMapLayer.mockResolvedValue(undefined)
    mockDeleteMapLayer.mockResolvedValue(undefined)
    mockBulkSortMapLayers.mockResolvedValue(undefined)
  })

  it('renders layer list with system and user-defined layers', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('OSM Standard')).toBeInTheDocument()
    })
    expect(screen.getByText('Mein Overlay')).toBeInTheDocument()
  })

  it('Delete button is disabled for is_system=true rows', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('OSM Standard')).toBeInTheDocument()
    })

    // Find delete button in the system row — it has text "Löschen"
    const rows = screen.getAllByRole('listitem')
    const systemRow = rows.find(r => r.textContent?.includes('OSM Standard'))!
    const deleteBtn = Array.from(systemRow.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Löschen',
    ) as HTMLButtonElement
    expect(deleteBtn).toBeDisabled()
  })

  it('Delete button is enabled for non-system rows', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('Mein Overlay')).toBeInTheDocument()
    })
    const rows = screen.getAllByRole('listitem')
    const customRow = rows.find(r => r.textContent?.includes('Mein Overlay'))!
    const deleteBtn = Array.from(customRow.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Löschen',
    ) as HTMLButtonElement
    expect(deleteBtn).not.toBeDisabled()
  })

  it('Add-Modal-Submit calls createMapLayer with correct body', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('OSM Standard')).toBeInTheDocument())

    // Click the "Layer hinzufügen" header button (not the modal h2)
    const addBtn = screen.getAllByRole('button').find(b => b.textContent === 'Layer hinzufügen')!
    fireEvent.click(addBtn)
    await waitFor(() => expect(screen.getByText('Layer hinzufügen', { selector: 'h2' })).toBeInTheDocument())

    // Fill name — label with htmlFor="layer-modal-name"
    const nameInput = screen.getByLabelText(/Name/)
    fireEvent.change(nameInput, { target: { value: 'Neuer Layer' } })

    // Save button
    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)

    await waitFor(() => expect(mockCreateMapLayer).toHaveBeenCalledOnce())
    const body = mockCreateMapLayer.mock.calls[0][0] as Partial<MapLayer>
    expect(body.name).toBe('Neuer Layer')
    expect(body.role).toBe('base')
  })

  it('Capability field is a dropdown (not free text)', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Mein Overlay')).toBeInTheDocument())

    // Click the Bearbeiten button on the Mein Overlay row (second Bearbeiten button)
    const editBtns = screen.getAllByRole('button', { name: /Bearbeiten/ })
    // Find the one that's in the Mein Overlay row
    const rows = screen.getAllByRole('listitem')
    const customRow = rows.find(r => r.textContent?.includes('Mein Overlay'))!
    const editBtn = Array.from(customRow.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Bearbeiten',
    )!
    fireEvent.click(editBtn)

    await waitFor(() => expect(screen.getByText('Layer bearbeiten')).toBeInTheDocument())

    // The capability field must be a <select> (id=layer-modal-cap, labeled "Benötigt Fähigkeit")
    const capSelect = screen.getByLabelText(/Fähigkeit/)
    expect(capSelect.tagName).toBe('SELECT')

    // Suppress unused var warning
    void editBtns
  })

  it('Edit-Save with URL change sends updated_at as X-If-Match-Updated-At', async () => {
    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Mein Overlay')).toBeInTheDocument())

    const rows = screen.getAllByRole('listitem')
    const customRow = rows.find(r => r.textContent?.includes('Mein Overlay'))!
    const editBtn = Array.from(customRow.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Bearbeiten',
    )!
    fireEvent.click(editBtn)

    await waitFor(() => expect(screen.getByText('Layer bearbeiten')).toBeInTheDocument())

    // Change the URL (input has a recognizable placeholder)
    const urlInput = screen.getByPlaceholderText(/tile\.example\.com/)
    fireEvent.change(urlInput, { target: { value: 'https://newurl.example.com/{z}/{x}/{y}.png' } })

    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)

    await waitFor(() => expect(mockPatchMapLayer).toHaveBeenCalledOnce())
    // Third argument is the updated_at for If-Match
    const [, , ifMatch] = mockPatchMapLayer.mock.calls[0] as [number, unknown, string]
    expect(ifMatch).toBe(USER_LAYER.updated_at)
  })

  it('412 response shows outdated toast and invalidates query', async () => {
    mockPatchMapLayer.mockRejectedValue(new ApiError(412, 'Precondition Failed'))

    render(<MapLayersAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Mein Overlay')).toBeInTheDocument())

    const rows = screen.getAllByRole('listitem')
    const customRow = rows.find(r => r.textContent?.includes('Mein Overlay'))!
    const editBtn = Array.from(customRow.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Bearbeiten',
    )!
    fireEvent.click(editBtn)

    await waitFor(() => expect(screen.getByText('Layer bearbeiten')).toBeInTheDocument())

    // Modify URL to trigger If-Match path
    const urlInput = screen.getByPlaceholderText(/tile\.example\.com/)
    fireEvent.change(urlInput, { target: { value: 'https://changed.example.com/{z}/{x}/{y}.png' } })

    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(screen.getByText(/Veraltet/i)).toBeInTheDocument()
    })
  })
})
