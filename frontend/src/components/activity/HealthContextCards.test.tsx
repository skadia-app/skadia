import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import '@/i18n/i18n'
import { i18n } from '@/i18n/i18n'
import type { HealthContext } from '@/api/types'
import { HealthContextCards } from './HealthContextCards'
import { fetchHealthContext } from '@/api/activities'

vi.mock('@/api/activities', () => ({
  fetchHealthContext: vi.fn(),
}))

const fetchHealthContextMock = vi.mocked(fetchHealthContext)

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

describe('HealthContextCards', () => {
  beforeEach(async () => {
    fetchHealthContextMock.mockReset()
    await i18n.changeLanguage('de')
  })

  it('formats health values and measured time with the active locale', async () => {
    const healthContext: HealthContext = {
      sleep_duration_min: 125,
      sleep_score: 81,
      deep_sleep_min: 35,
      rem_sleep_min: 40,
      hrv_rmssd: 1234.4,
      hrv_measured_at: '2026-01-01T14:05:00Z',
      resting_hr: 1234,
    }
    fetchHealthContextMock.mockResolvedValue(healthContext)

    const { container } = render(<HealthContextCards activityId="activity-1" />, { wrapper })

    expect(await screen.findByText('2 h 5 min')).toBeInTheDocument()
    expect(screen.getAllByText('1.234')).toHaveLength(2)
    expect(screen.getByText(new Date('2026-01-01T14:05:00Z').toLocaleTimeString('de-DE', {
      hour: '2-digit',
      minute: '2-digit',
    }))).toBeInTheDocument()
    expect(screen.getByText('Ruhepuls')).toBeInTheDocument()
    expect(container.querySelector('[title="Tiefschlaf: 35 min"]')).toBeInTheDocument()
  })
})
