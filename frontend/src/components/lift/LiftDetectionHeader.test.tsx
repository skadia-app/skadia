import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { ToastProvider } from '@/lib/toast'
import { ToastContainer } from '@/components/Toast'
import type { LiftDetectionConfigResponse } from '@/api/liftDetectionConfig'

// ── Mock modules ────────────────────────────────────────────────────────────

vi.mock('@/api/liftDetectionConfig', () => ({
  getLiftDetectionConfig: vi.fn(),
  putLiftDetectionConfig: vi.fn(),
  deleteLiftDetectionConfig: vi.fn(),
}))

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

import { getLiftDetectionConfig, putLiftDetectionConfig, deleteLiftDetectionConfig } from '@/api/liftDetectionConfig'
import { apiFetch } from '@/lib/auth'

const mockGet = getLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockPut = putLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockDelete = deleteLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>

// ── Helpers ─────────────────────────────────────────────────────────────────

function makeConfig(overrides: Partial<LiftDetectionConfigResponse> = {}): LiftDetectionConfigResponse {
  return {
    effective: { osm: true, heuristic: true },
    user_setting: { osm: true, heuristic: true },
    override: { osm: null, heuristic: null },
    master_enabled: true,
    ...overrides,
  }
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      <ToastContainer />
    </ToastProvider>
  )
}

async function renderHeader(props: {
  activityId?: string
  jobRunning?: boolean
  onJobStarted?: () => void
  config?: LiftDetectionConfigResponse
}) {
  const { default: LiftDetectionHeader } = await import('./LiftDetectionHeader')
  const config = props.config ?? makeConfig()
  mockGet.mockResolvedValue(config)

  render(
    <LiftDetectionHeader
      activityId={props.activityId ?? 'act-1'}
      jobRunning={props.jobRunning ?? false}
      onJobStarted={props.onJobStarted ?? vi.fn()}
    />,
    { wrapper },
  )
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('LiftDetectionHeader — Override-Badge', () => {
  beforeEach(() => vi.clearAllMocks())

  it('shows Override-Badge when override.osm is non-null', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: false, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('override-badge')).toBeTruthy()
      expect(screen.getByTestId('override-badge').textContent).toMatch(/eigene Einstellungen/)
    })
  })

  it('shows Override-Badge when override.heuristic is non-null', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: null, heuristic: true } }) })
    await waitFor(() => {
      expect(screen.getByTestId('override-badge')).toBeTruthy()
    })
  })

  it('hides Override-Badge when both overrides are null', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: null, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('lift-detection-header')).toBeTruthy()
    })
    expect(screen.queryByTestId('override-badge')).toBeNull()
  })
})

describe('LiftDetectionHeader — disabled state', () => {
  beforeEach(() => vi.clearAllMocks())

  it('disables toggles and Apply when jobRunning=true', async () => {
    await renderHeader({ jobRunning: true })
    await waitFor(() => {
      expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy()
    })
    const osmSwitch = screen.getByTestId('toggle-osm').querySelector('button')!
    expect(osmSwitch.hasAttribute('disabled')).toBe(true)
    expect(screen.getByTestId('apply-button').hasAttribute('disabled')).toBe(true)
  })

  it('disables toggles and Apply when master_enabled=false', async () => {
    await renderHeader({ config: makeConfig({ master_enabled: false }) })
    await waitFor(() => {
      expect(screen.getByTestId('master-off-notice')).toBeTruthy()
    })
    const osmSwitch = screen.getByTestId('toggle-osm').querySelector('button')!
    expect(osmSwitch.hasAttribute('disabled')).toBe(true)
    expect(screen.getByTestId('apply-button').hasAttribute('disabled')).toBe(true)
  })

  it('shows master-off notice when master_enabled=false', async () => {
    await renderHeader({ config: makeConfig({ master_enabled: false }) })
    await waitFor(() => {
      expect(screen.getByTestId('master-off-notice').textContent).toMatch(/Globale Lift-Erkennung deaktiviert/)
    })
  })

  it('disables Reset button when no override exists', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: null, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('reset-button')).toBeTruthy()
    })
    expect(screen.getByTestId('reset-button').hasAttribute('disabled')).toBe(true)
  })

  it('enables Reset button when override exists', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: true, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('reset-button')).toBeTruthy()
    })
    expect(screen.getByTestId('reset-button').hasAttribute('disabled')).toBe(false)
  })
})

describe('LiftDetectionHeader — Apply button', () => {
  beforeEach(() => vi.clearAllMocks())

  it('Apply calls PUT then recalculate-all (202) and invokes onJobStarted', async () => {
    const onJobStarted = vi.fn()
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: true, status: 202 })

    await renderHeader({ onJobStarted })

    await waitFor(() => {
      expect(screen.getByTestId('apply-button')).toBeTruthy()
    })

    fireEvent.click(screen.getByTestId('apply-button'))

    await waitFor(() => {
      expect(mockPut).toHaveBeenCalledWith('act-1', { osm: true, heuristic: true })
      expect(mockApiFetch).toHaveBeenCalledWith(
        '/api/v1/activities/act-1/lifts/recalculate-all',
        { method: 'POST' },
      )
      expect(onJobStarted).toHaveBeenCalled()
    })
  })

  it('Apply: 409 from recalculate-all shows "Job läuft bereits" toast and still calls onJobStarted', async () => {
    const onJobStarted = vi.fn()
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: false, status: 409, json: async () => ({}) })

    await renderHeader({ onJobStarted })

    await waitFor(() => expect(screen.getByTestId('apply-button')).toBeTruthy())
    fireEvent.click(screen.getByTestId('apply-button'))

    await waitFor(() => {
      expect(screen.getByText(/Job läuft bereits/)).toBeTruthy()
      expect(onJobStarted).toHaveBeenCalled()
    })
  })
})

describe('LiftDetectionHeader — Reset button', () => {
  beforeEach(() => vi.clearAllMocks())

  it('Reset calls DELETE and shows success toast', async () => {
    mockDelete.mockResolvedValue(undefined)
    // Re-fetch after reset returns no override
    mockGet
      .mockResolvedValueOnce(makeConfig({ override: { osm: true, heuristic: null } }))
      .mockResolvedValue(makeConfig({ override: { osm: null, heuristic: null } }))

    await renderHeader({ config: makeConfig({ override: { osm: true, heuristic: null } }) })

    await waitFor(() => {
      expect(screen.getByTestId('reset-button').hasAttribute('disabled')).toBe(false)
    })

    fireEvent.click(screen.getByTestId('reset-button'))

    await waitFor(() => {
      expect(mockDelete).toHaveBeenCalledWith('act-1')
      expect(screen.getByText(/zurückgesetzt/)).toBeTruthy()
    })
  })
})
