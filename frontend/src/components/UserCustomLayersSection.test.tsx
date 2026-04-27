// Tests for UserCustomLayersSection
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
import { UserCustomLayersSection } from './UserCustomLayersSection'
import type { UserMapLayer } from '@/api/mapLayers'

// ── Mocks ─────────────────────────────────────────────────────────────────────

const mockFetchUserMapLayers = vi.fn()
const mockCreateUserMapLayer = vi.fn()
const mockPatchUserMapLayer = vi.fn()
const mockDeleteUserMapLayer = vi.fn()
const mockBulkSortUserMapLayers = vi.fn()
const mockGetAdminSettings = vi.fn()

vi.mock('@/api/mapLayers', async () => {
  const actual = await vi.importActual<typeof import('@/api/mapLayers')>('@/api/mapLayers')
  return {
    ...actual,
    fetchUserMapLayers: (...args: unknown[]) => mockFetchUserMapLayers(...args),
    createUserMapLayer: (...args: unknown[]) => mockCreateUserMapLayer(...args),
    patchUserMapLayer: (...args: unknown[]) => mockPatchUserMapLayer(...args),
    deleteUserMapLayer: (...args: unknown[]) => mockDeleteUserMapLayer(...args),
    bulkSortUserMapLayers: (...args: unknown[]) => mockBulkSortUserMapLayers(...args),
  }
})

vi.mock('@/api/adminSettings', () => ({
  getAdminSettings: (...args: unknown[]) => mockGetAdminSettings(...args),
}))

// ── Fixtures ─────────────────────────────────────────────────────────────────

function makeLayer(id: number, name: string): UserMapLayer {
  return {
    id,
    name,
    url_template: `https://tile-${id}.example.com/{z}/{x}/{y}.png`,
    default_opacity: 1,
    enabled: true,
    sort_order: id,
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
  }
}

const ADMIN_SETTINGS_50 = {
  library: {
    user_map_layer_limit: { value: '50', source: 'default' },
  },
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
          {/* ToastContainer renders toast text into the DOM for assertion */}
          <ToastContainer />
        </QueryClientProvider>
      </ToastProvider>
    )
  }
  return Wrapper
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('UserCustomLayersSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGetAdminSettings.mockResolvedValue(ADMIN_SETTINGS_50)
    mockFetchUserMapLayers.mockResolvedValue([makeLayer(1, 'Layer A'), makeLayer(2, 'Layer B')])
    mockCreateUserMapLayer.mockResolvedValue(makeLayer(3, 'Neu'))
    mockPatchUserMapLayer.mockResolvedValue(undefined)
    mockDeleteUserMapLayer.mockResolvedValue(undefined)
    mockBulkSortUserMapLayers.mockResolvedValue(undefined)
  })

  it('renders layer list', async () => {
    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Layer A')).toBeInTheDocument())
    expect(screen.getByText('Layer B')).toBeInTheDocument()
  })

  it('displays used / max count (49/50 OK — button enabled)', async () => {
    // 49 layers
    mockFetchUserMapLayers.mockResolvedValue(
      Array.from({ length: 49 }, (_, i) => makeLayer(i + 1, `L${i + 1}`)),
    )
    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText(/49 \/ 50 verwendet/)).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /Layer hinzufügen/ })).not.toBeDisabled()
  })

  it('at limit (50/50) → Add button is disabled', async () => {
    mockFetchUserMapLayers.mockResolvedValue(
      Array.from({ length: 50 }, (_, i) => makeLayer(i + 1, `L${i + 1}`)),
    )
    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText(/50 \/ 50 verwendet/)).toBeInTheDocument())
    expect(screen.getByRole('button', { name: /Layer hinzufügen/ })).toBeDisabled()
  })

  it('Add happy path calls createUserMapLayer', async () => {
    mockFetchUserMapLayers.mockResolvedValue([])
    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => screen.getByRole('button', { name: /Layer hinzufügen/ }))

    fireEvent.click(screen.getByRole('button', { name: /Layer hinzufügen/ }))
    await waitFor(() => expect(screen.getByText('Layer hinzufügen', { selector: 'h2' })).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Test Layer' } })
    fireEvent.change(screen.getByLabelText(/URL/), {
      target: { value: 'https://tiles.example.com/{z}/{x}/{y}.png' },
    })

    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)
    await waitFor(() => expect(mockCreateUserMapLayer).toHaveBeenCalledOnce())
    const body = mockCreateUserMapLayer.mock.calls[0][0] as Partial<UserMapLayer>
    expect(body.name).toBe('Test Layer')
  })

  it('Delete happy path calls deleteUserMapLayer', async () => {
    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Layer A')).toBeInTheDocument())

    const rows = screen.getAllByRole('listitem')
    const rowA = rows.find(r => r.textContent?.includes('Layer A'))!
    const deleteBtnInRow = Array.from(rowA.querySelectorAll('button')).find(
      b => b.textContent?.trim() === 'Löschen',
    )!
    fireEvent.click(deleteBtnInRow)

    await waitFor(() => screen.getByText(/dauerhaft gelöscht/))
    // Click the confirm button in the modal (also labeled "Löschen")
    const confirmBtns = screen.getAllByRole('button', { name: /Löschen/ })
    fireEvent.click(confirmBtns[confirmBtns.length - 1])
    await waitFor(() => {
      expect(mockDeleteUserMapLayer).toHaveBeenCalled()
      expect(mockDeleteUserMapLayer.mock.calls[0][0]).toBe(1)
    })
  })

  it('409 duplicate name shows toast', async () => {
    mockFetchUserMapLayers.mockResolvedValue([])
    mockCreateUserMapLayer.mockRejectedValue(new Error('409 duplicate_name already have a layer'))

    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => screen.getByRole('button', { name: /Layer hinzufügen/ }))

    fireEvent.click(screen.getByRole('button', { name: /Layer hinzufügen/ }))
    await waitFor(() => screen.getByText('Layer hinzufügen', { selector: 'h2' }))

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Dup' } })
    fireEvent.change(screen.getByLabelText(/URL/), {
      target: { value: 'https://dup.example.com/{z}/{x}/{y}.png' },
    })
    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(screen.getByText(/Name bereits vergeben/)).toBeInTheDocument()
    })
  })

  it('422 private_address_blocked shows toast', async () => {
    mockFetchUserMapLayers.mockResolvedValue([])
    mockCreateUserMapLayer.mockRejectedValue(new Error('422 private_address_blocked'))

    render(<UserCustomLayersSection />, { wrapper: makeWrapper() })
    await waitFor(() => screen.getByRole('button', { name: /Layer hinzufügen/ }))

    fireEvent.click(screen.getByRole('button', { name: /Layer hinzufügen/ }))
    await waitFor(() => screen.getByText('Layer hinzufügen', { selector: 'h2' }))

    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Internal' } })
    fireEvent.change(screen.getByLabelText(/URL/), {
      target: { value: 'http://192.168.1.1/{z}/{x}/{y}.png' },
    })
    const saveBtn = screen.getAllByRole('button').find(b => b.textContent === 'Speichern')!
    fireEvent.click(saveBtn)

    await waitFor(() => {
      expect(screen.getByText(/Private Adressen sind nicht erlaubt/)).toBeInTheDocument()
    })
  })
})
