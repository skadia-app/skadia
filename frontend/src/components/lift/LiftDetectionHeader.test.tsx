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

vi.mock('@/api/jobs', () => ({
  fetchActiveJobs: vi.fn(),
}))

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(),
}))

import { getLiftDetectionConfig, putLiftDetectionConfig, deleteLiftDetectionConfig } from '@/api/liftDetectionConfig'
import { fetchActiveJobs } from '@/api/jobs'
import { apiFetch } from '@/lib/auth'

const mockGet = getLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockPut = putLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockDelete = deleteLiftDetectionConfig as ReturnType<typeof vi.fn>
const mockApiFetch = apiFetch as ReturnType<typeof vi.fn>
const mockFetchActiveJobs = fetchActiveJobs as ReturnType<typeof vi.fn>

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
  osmSnapAvailable?: boolean
}) {
  const { default: LiftDetectionHeader } = await import('./LiftDetectionHeader')
  const config = props.config ?? makeConfig()
  mockGet.mockResolvedValue(config)

  render(
    <LiftDetectionHeader
      activityId={props.activityId ?? 'act-1'}
      jobRunning={props.jobRunning ?? false}
      onJobStarted={props.onJobStarted ?? vi.fn()}
      osmSnapAvailable={props.osmSnapAvailable}
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
      expect(screen.getByTestId('override-badge').textContent).toMatch(/Override gesetzt/)
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

  it('disables toggles when jobRunning=true', async () => {
    await renderHeader({ jobRunning: true })
    await waitFor(() => {
      expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy()
    })
    const osmSwitch = screen.getByTestId('toggle-osm').querySelector('button')!
    expect(osmSwitch.hasAttribute('disabled')).toBe(true)
  })

  it('disables toggles when master_enabled=false', async () => {
    await renderHeader({ config: makeConfig({ master_enabled: false }) })
    await waitFor(() => {
      expect(screen.getByTestId('master-off-notice')).toBeTruthy()
    })
    const osmSwitch = screen.getByTestId('toggle-osm').querySelector('button')!
    expect(osmSwitch.hasAttribute('disabled')).toBe(true)
  })

  it('hides OSM-Snap while keeping Heuristik available when OSM snap capability is unavailable', async () => {
    await renderHeader({ osmSnapAvailable: false })
    await waitFor(() => {
      expect(screen.getByTestId('toggle-heuristic')).toBeTruthy()
    })
    expect(screen.queryByTestId('toggle-osm')).toBeNull()
  })

  it('shows master-off notice when master_enabled=false', async () => {
    await renderHeader({ config: makeConfig({ master_enabled: false }) })
    await waitFor(() => {
      expect(screen.getByTestId('master-off-notice').textContent).toMatch(/Globale Lift-Erkennung deaktiviert/)
    })
  })

  it('hides Reset control when no override exists', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: null, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('lift-detection-header')).toBeTruthy()
    })
    expect(screen.queryByTestId('override-badge')).toBeNull()
  })

  it('enables Reset control when override exists', async () => {
    await renderHeader({ config: makeConfig({ override: { osm: true, heuristic: null } }) })
    await waitFor(() => {
      expect(screen.getByTestId('override-badge')).toBeTruthy()
    })
    expect(screen.getByTestId('override-badge').hasAttribute('disabled')).toBe(false)
  })
})

describe('LiftDetectionHeader — automatic recompute on toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchActiveJobs.mockResolvedValue({ jobs: [] })
  })

  it('toggle calls PUT then recalculate-all (202) and invokes onJobStarted', async () => {
    const onJobStarted = vi.fn()
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: true, status: 202 })

    await renderHeader({ onJobStarted })

    await waitFor(() => {
      expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy()
    })

    fireEvent.click(screen.getByTestId('toggle-osm').querySelector('button')!)

    await waitFor(() => {
      expect(mockPut).toHaveBeenCalledWith('act-1', { osm: false, heuristic: true })
      expect(mockApiFetch).toHaveBeenCalledWith(
        '/api/v1/activities/act-1/lifts/recalculate-all',
        { method: 'POST' },
      )
      expect(onJobStarted).toHaveBeenCalled()
    })
  })

  it('turning off the last mode resets candidates without entering background-job polling', async () => {
    const onJobStarted = vi.fn()
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: true, status: 204 })

    await renderHeader({
      onJobStarted,
      config: makeConfig({
        effective: { osm: true, heuristic: false },
        override: { osm: true, heuristic: false },
      }),
    })

    await waitFor(() => expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy())
    fireEvent.click(screen.getByTestId('toggle-osm').querySelector('button')!)

    await waitFor(() => {
      expect(mockPut).toHaveBeenCalledWith('act-1', { osm: false, heuristic: false })
      expect(mockApiFetch).toHaveBeenCalledWith(
        '/api/v1/activities/act-1/lifts/recalculate-all',
        { method: 'POST' },
      )
      expect(onJobStarted).not.toHaveBeenCalled()
    })
  })

  it('409 from recalculate-all shows "Job läuft bereits" toast and still calls onJobStarted', async () => {
    const onJobStarted = vi.fn()
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue(new Response('{}', {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    }))

    await renderHeader({ onJobStarted })

    await waitFor(() => expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy())
    fireEvent.click(screen.getByTestId('toggle-osm').querySelector('button')!)

    await waitFor(() => {
      expect(screen.getByText(/Job läuft bereits/)).toBeTruthy()
      expect(onJobStarted).toHaveBeenCalled()
    })
  })

  it('checks for a running lift job before persisting override settings', async () => {
    const onJobStarted = vi.fn()
    mockFetchActiveJobs.mockResolvedValue({
      jobs: [
        {
          id: 'job-1',
          job_type: 'lift_recompute',
          status: 'running',
          entity_id: 'act-1',
          progress_current: 0,
          progress_total: 1,
          started_at: '2026-05-11T00:00:00Z',
        },
      ],
    })
    mockPut.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: true, status: 202 })

    await renderHeader({ onJobStarted })

    await waitFor(() => expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy())
    fireEvent.click(screen.getByTestId('toggle-osm').querySelector('button')!)

    await waitFor(() => {
      expect(screen.getByText(/Job läuft bereits/)).toBeTruthy()
      expect(onJobStarted).toHaveBeenCalled()
    })
    expect(mockPut).not.toHaveBeenCalled()
    expect(mockApiFetch).not.toHaveBeenCalled()
  })

  it('does not invalidate lift success queries when recalculate-all fails', async () => {
    const invalidateSpy = vi.spyOn(QueryClient.prototype, 'invalidateQueries')
    try {
      mockPut.mockResolvedValue(undefined)
      mockApiFetch.mockResolvedValue(new Response(JSON.stringify({
        detail: 'sport cycling hat keine Lift-Erkennung',
        code: 'NOT_LIFT_ELIGIBLE',
      }), {
        status: 400,
        headers: { 'Content-Type': 'application/problem+json' },
      }))

      await renderHeader({})

      await waitFor(() => expect(screen.getByTestId('toggle-osm').querySelector('button')).toBeTruthy())
      fireEvent.click(screen.getByTestId('toggle-osm').querySelector('button')!)

      await waitFor(() => {
        expect(screen.getByText(/sport cycling hat keine Lift-Erkennung/)).toBeTruthy()
      })

      const successInvalidations = invalidateSpy.mock.calls.filter(([arg]) => {
        const key = (arg as { queryKey?: unknown[] } | undefined)?.queryKey
        return Array.isArray(key) &&
          key[1] === 'act-1' &&
          (key[0] === 'lift-candidates' || key[0] === 'lift-candidates-inspector' || key[0] === 'activity')
      })
      expect(successInvalidations).toEqual([])
    } finally {
      invalidateSpy.mockRestore()
    }
  })
})

describe('LiftDetectionHeader — Reset control', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockFetchActiveJobs.mockResolvedValue({ jobs: [] })
  })

  it('Reset calls DELETE and starts recalculation', async () => {
    mockDelete.mockResolvedValue(undefined)
    mockApiFetch.mockResolvedValue({ ok: true, status: 202 })
    // Re-fetch after reset returns no override
    mockGet
      .mockResolvedValueOnce(makeConfig({ override: { osm: true, heuristic: null } }))
      .mockResolvedValue(makeConfig({ override: { osm: null, heuristic: null } }))

    await renderHeader({ config: makeConfig({ override: { osm: true, heuristic: null } }) })

    await waitFor(() => {
      expect(screen.getByTestId('override-badge').hasAttribute('disabled')).toBe(false)
    })

    fireEvent.click(screen.getByTestId('override-badge'))

    await waitFor(() => {
      expect(mockDelete).toHaveBeenCalledWith('act-1')
      expect(mockApiFetch).toHaveBeenCalledWith(
        '/api/v1/activities/act-1/lifts/recalculate-all',
        { method: 'POST' },
      )
    })
  })
})
