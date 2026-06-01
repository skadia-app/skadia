import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { Activity } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import { RunningFormCard } from './RunningFormCard'

const fetchRunningFormBaseline = vi.hoisted(() => vi.fn())

vi.mock('@/api/analytics', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/analytics')>()
  return {
    ...actual,
    fetchRunningFormBaseline,
  }
})

function renderCard(activity: Activity) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={client}>
      <RunningFormCard activity={activity} />
    </QueryClientProvider>,
  )
}

describe('RunningFormCard i18n', () => {
  beforeEach(async () => {
    fetchRunningFormBaseline.mockReset()
    fetchRunningFormBaseline.mockResolvedValue({
      activity_count: 2,
      duty_factor_median: null,
      vertical_ratio_median: null,
      gct_asymmetry_median: null,
    })
    await i18n.changeLanguage('de')
  })

  it('renders the tier badge and baseline fallback tooltip from the locale catalog', async () => {
    renderCard({
      running_form_tier: 2,
      running_form_duty_factor: 0.32,
    } as Activity)

    expect(screen.getByText('Stufe 2')).toBeInTheDocument()
    expect(screen.getByText('Laufform')).toBeInTheDocument()
    expect(await screen.findByTitle('Absoluter Wert (noch keine Basislinie)')).toBeInTheDocument()
    expect(screen.queryByText('Tier 2')).not.toBeInTheDocument()
  })
})
