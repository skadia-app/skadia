import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { ToastProvider } from '@/lib/toast'
import { DataCorrectionsSection } from './DataCorrections'

const mocks = vi.hoisted(() => ({
  capabilities: {} as Record<string, unknown>,
  userPrefs: { locale: 'de', units: 'metric' as 'metric' | 'imperial' },
}))

vi.mock('@/hooks/useCapabilities', () => ({
  useCapabilities: () => ({ data: mocks.capabilities }),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mocks.userPrefs,
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
    <I18nextProvider i18n={i18n}>
      <ToastProvider>
        <QueryClientProvider client={qc}>{children}</QueryClientProvider>
      </ToastProvider>
    </I18nextProvider>
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
  beforeAll(async () => {
    await i18n.changeLanguage('de')
  })

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.capabilities = {}
    mocks.userPrefs = { locale: 'de', units: 'metric' }
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

  it('formats the distance summary with selected user units', async () => {
    await i18n.changeLanguage('en')
    mocks.userPrefs = { locale: 'en', units: 'imperial' }

    render(<DataCorrectionsSection activity={activity({ distance_m: 1609.344 })} />, { wrapper })

    expect(await screen.findByText(/1\.0 mi/)).toBeInTheDocument()
    expect(screen.queryByText(/1\.61 km/)).not.toBeInTheDocument()
  })
})
