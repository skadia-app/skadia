import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity } from '@/api/types'
import { ToastProvider } from '@/lib/toast'
import { DataCorrectionsSection } from './DataCorrections'

const mocks = vi.hoisted(() => ({
  capabilities: {} as Record<string, unknown>,
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/api/activities', () => ({
  correctActivityElevation: vi.fn(),
  resetActivityElevation: vi.fn(),
  correctActivityDistance: vi.fn(),
  resetActivityDistance: vi.fn(),
}))

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return (
    <ToastProvider>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </ToastProvider>
  )
}

function activity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    has_gps: true,
    elevation_source: 'device',
    distance_source: 'device',
    distance_m: 12000,
    ...overrides,
  } as Activity
}

describe('DataCorrectionsSection capability gates', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.capabilities = {}
  })

  it('hides DEM correction action when dem_correction is configured but unavailable', async () => {
    mocks.capabilities = {
      features: {
        dem_correction: { configured: true, available: false, reason: 'missing_artifacts' },
      },
      dem_correction: false,
    }

    render(<DataCorrectionsSection activity={activity()} />, { wrapper })

    expect(await screen.findByText('Höhe')).toBeTruthy()
    expect(screen.queryByText('DEM-Korrektur anwenden')).toBeNull()
    expect(screen.getByText('Aus GPS berechnen')).toBeTruthy()
  })
})
