import '@testing-library/jest-dom/vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import type { BestEffortRanked } from '@/api/bestEfforts'
import { DetailTable } from './DetailTable'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

function record(overrides: Partial<BestEffortRanked>): BestEffortRanked {
  return {
    id: 'record-1',
    activity_id: 'activity-1',
    category: 'power_20min',
    sport_group: 'cycling',
    achieved_at: '2026-05-01T12:00:00Z',
    rank: 1,
    value_numeric: 50_000,
    ...overrides,
  }
}

describe('DetailTable unit formatting', () => {
  it('formats expanded power deltas with locale-aware fixed units', () => {
    render(
      <MemoryRouter>
        <DetailTable
          records={[
            record({ id: 'record-1', rank: 1, value_numeric: 50_000 }),
            record({ id: 'record-2', activity_id: 'activity-2', rank: 2, value_numeric: 37_654.4 }),
          ]}
          sportGroup="cycling"
          language="en"
        />
      </MemoryRouter>,
    )

    fireEvent.click(screen.getByText('20 min'))

    expect(screen.getByText('−12,346 W')).toBeInTheDocument()
    expect(screen.queryByText('−12346 W')).not.toBeInTheDocument()
  })
})
