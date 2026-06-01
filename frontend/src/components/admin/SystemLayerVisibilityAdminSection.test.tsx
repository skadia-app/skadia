// Tests for SystemLayerVisibilityAdminSection
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import '@testing-library/jest-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { SystemLayerVisibilityAdminSection } from './SystemLayerVisibilityAdminSection'
import type { SystemLayerDescriptor } from '@/api/mapLayers'

// ── Mocks ─────────────────────────────────────────────────────────────────────

const mockFetchMapLayers = vi.fn()
const mockFetchSystemLayerKeys = vi.fn()
const mockPatchSystemLayerVisibility = vi.fn()

vi.mock('react-i18next', async importOriginal => {
  const actual = await importOriginal<typeof import('react-i18next')>()
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, values?: Record<string, unknown>) => {
        const labels: Record<string, string> = {
          'mapLayersAdmin.systemVisibility.groupVector': 'Vector tiles',
          'mapLayersAdmin.systemVisibility.groupCoverage': 'Coverage layers',
          'mapLayersAdmin.systemVisibility.colEnabled': 'Enabled',
          'mapLayersAdmin.systemVisibility.colDefaultNew': 'Default for new users',
          'mapLayersAdmin.systemVisibility.colOpacity': 'Default opacity (%)',
          'mapLayersAdmin.systemVisibility.colLabel': 'Label override',
          'mapLayersAdmin.systemVisibility.colOpacityAndLabel': 'Default opacity (%) / Label override',
          'mapLayersAdmin.systemVisibility.placeholderLabel': '(Default label)',
          'mapLayersAdmin.systemVisibility.unavailable': 'Unavailable',
          'mapLayersAdmin.systemVisibility.unavailableWithReason': `Unavailable: ${values?.reason}`,
          'mapLayersAdmin.systemVisibility.enabledAria': `${values?.label} — Enabled`,
          'mapLayersAdmin.systemVisibility.defaultNewAria': `${values?.label} — Default for new users`,
          'mapLayersAdmin.systemVisibility.opacityAria': `${values?.label} — Default opacity (%)`,
          'mapLayersAdmin.systemVisibility.labelAria': `${values?.label} — Label override`,
        }
        return labels[key] ?? key
      },
    }),
  }
})

vi.mock('@/api/mapLayers', async () => {
  const actual = await vi.importActual<typeof import('@/api/mapLayers')>('@/api/mapLayers')
  return {
    ...actual,
    fetchMapLayers: (...args: unknown[]) => mockFetchMapLayers(...args),
    fetchSystemLayerKeys: (...args: unknown[]) => mockFetchSystemLayerKeys(...args),
    patchSystemLayerVisibility: (...args: unknown[]) => mockPatchSystemLayerVisibility(...args),
  }
})

vi.mock('@/api/routes', () => ({
  getCapabilities: vi.fn().mockResolvedValue({}),
}))

// ── Fixtures ─────────────────────────────────────────────────────────────────

const VECTOR_KEY: SystemLayerDescriptor = {
  key: 'hiking_trails',
  category: 'vector',
  default_label: 'Wanderwege',
}

const COVERAGE_KEY: SystemLayerDescriptor = {
  key: 'coverage_heatmap',
  category: 'coverage',
  default_label: 'Abdeckungs-Heatmap',
}

const MOCK_LAYERS_RESPONSE = {
  layers: [],
  system_visibility: {
    hiking_trails: {
      layer_key: 'hiking_trails',
      enabled: true,
      default_enabled_for_new_users: false,
      default_opacity: undefined,
      label_override: undefined,
      updated_at: '2025-01-01T00:00:00Z',
    },
    coverage_heatmap: {
      layer_key: 'coverage_heatmap',
      enabled: false,
      default_enabled_for_new_users: true,
      default_opacity: 0.6,
      label_override: 'Heatmap',
      updated_at: '2025-01-01T00:00:00Z',
    },
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
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    )
  }
  return Wrapper
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe('SystemLayerVisibilityAdminSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchMapLayers.mockResolvedValue(MOCK_LAYERS_RESPONSE)
    mockFetchSystemLayerKeys.mockResolvedValue([VECTOR_KEY, COVERAGE_KEY])
    mockPatchSystemLayerVisibility.mockResolvedValue(undefined)
  })

  it('renders vector and coverage groups', async () => {
    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => {
      expect(screen.getByText('Vector tiles')).toBeInTheDocument()
    })
    expect(screen.getByText('Coverage layers')).toBeInTheDocument()
    expect(screen.getByText('Wanderwege')).toBeInTheDocument()
    expect(screen.getByText('Abdeckungs-Heatmap')).toBeInTheDocument()
  })

  it('toggle triggers PATCH with correct body', async () => {
    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Wanderwege')).toBeInTheDocument())

    const enabledCheckbox = screen.getByRole('checkbox', {
      name: /Wanderwege.*Enabled/i,
    })
    fireEvent.click(enabledCheckbox)

    // Debounced at 400ms — fast-forward vitest fake timers or just await
    await waitFor(
      () => expect(mockPatchSystemLayerVisibility).toHaveBeenCalledWith('hiking_trails', { enabled: false }),
      { timeout: 800 },
    )
  })

  it('coverage row shows opacity slider', async () => {
    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Abdeckungs-Heatmap')).toBeInTheDocument())

    const opacitySlider = screen.getByRole('slider', {
      name: /Abdeckungs-Heatmap.*opacity/i,
    })
    expect(opacitySlider).toBeInTheDocument()
  })

  it('vector row hides opacity slider', async () => {
    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Wanderwege')).toBeInTheDocument())

    const sliders = screen.queryAllByRole('slider', { name: /Wanderwege.*opacity/i })
    expect(sliders).toHaveLength(0)
  })

  it('disables unavailable capability-backed rows', async () => {
    mockFetchSystemLayerKeys.mockResolvedValue([
      { ...VECTOR_KEY, required_capability: 'system_layers_vector', available: false, reason: 'core_profile' },
      COVERAGE_KEY,
    ])

    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Wanderwege')).toBeInTheDocument())

    const enabledCheckbox = screen.getByRole('checkbox', {
      name: /Wanderwege.*Enabled/i,
    })
    expect(enabledCheckbox).toBeDisabled()
    fireEvent.click(enabledCheckbox)
    expect(mockPatchSystemLayerVisibility).not.toHaveBeenCalled()
  })

  it('label_override input is length-limited to 100 characters', async () => {
    render(<SystemLayerVisibilityAdminSection />, { wrapper: makeWrapper() })
    await waitFor(() => expect(screen.getByText('Wanderwege')).toBeInTheDocument())

    const labelInput = screen.getByRole('textbox', { name: /Wanderwege.*Label override/i })
    expect(labelInput).toHaveAttribute('maxlength', '100')
  })
})
