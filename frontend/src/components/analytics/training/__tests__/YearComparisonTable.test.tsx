import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import '@testing-library/jest-dom'
import YearComparisonTable, { lockedMonthIndex } from '../YearComparisonTable'
import type { YearComparisonTableProps } from '../YearComparisonTable'
import type { YearProgressionYearData } from '@/api/analytics'

// Helper to build minimal YearProgressionYearData.
function makeYear(year: number, monthly: number[], cumulative?: number[]): YearProgressionYearData {
  const cum = cumulative ?? monthly.map((_, i) => monthly.slice(0, i + 1).reduce((a, b) => a + b, 0))
  return { year, monthly, cumulative: cum, total: monthly.reduce((a, b) => a + b, 0) }
}

function defaultProps(overrides: Partial<YearComparisonTableProps> = {}): YearComparisonTableProps {
  return {
    years: [],
    yearColorIndex: new Map(),
    metric: 'distance_m',
    lockedDate: null,
    onLockedDateChange: vi.fn(),
    currentYear: 2024,
    ...overrides,
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// lockedMonthIndex helper
// ─────────────────────────────────────────────────────────────────────────────

describe('lockedMonthIndex', () => {
  it('returns previous month for day 2 (May 2nd → April = index 3)', () => {
    expect(lockedMonthIndex(new Date(2024, 4, 2))).toBe(3)
  })

  it('returns previous month for day 1 (May 1st → April = index 3)', () => {
    expect(lockedMonthIndex(new Date(2024, 4, 1))).toBe(3)
  })

  it('returns null for January (Jan 15th — prior month is Dec of previous year, outside current year)', () => {
    expect(lockedMonthIndex(new Date(2024, 0, 15))).toBeNull()
  })

  it('returns null for January 1st (no completed month in the current year)', () => {
    expect(lockedMonthIndex(new Date(2024, 0, 1))).toBeNull()
  })

  it('returns 0 (January) for February 2nd', () => {
    expect(lockedMonthIndex(new Date(2024, 1, 2))).toBe(0)
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// Δ calculations
// ─────────────────────────────────────────────────────────────────────────────

describe('YearComparisonTable — Δ calculations', () => {
  const yearData = [
    makeYear(2024, [100000, 200000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),  // total 300000 m
    makeYear(2023, [50000, 100000, 50000, 0, 0, 0, 0, 0, 0, 0, 0, 0]), // total 200000 m
    makeYear(2022, [80000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),          // total 80000 m
  ]
  const colorIndex = new Map([[2024, 0], [2023, 1], [2022, 2]])

  it('renders rows for each year', () => {
    render(<YearComparisonTable {...defaultProps({ years: yearData, yearColorIndex: colorIndex })} />)
    expect(screen.getByText('2024')).toBeInTheDocument()
    expect(screen.getByText('2023')).toBeInTheDocument()
    expect(screen.getByText('2022')).toBeInTheDocument()
  })

  it('shows Ø for current year in Δ-with-current-year column', () => {
    render(<YearComparisonTable {...defaultProps({ years: yearData, yearColorIndex: colorIndex, currentYear: 2024 })} />)
    expect(screen.getAllByText('Ø').length).toBeGreaterThanOrEqual(1)
  })

  it('shows — for first row in Δ-prev-year column (no previous year)', () => {
    // 2022 is the last row, no earlier year → Δ prev = —
    render(<YearComparisonTable {...defaultProps({ years: yearData, yearColorIndex: colorIndex })} />)
    // The table row for 2022 should have a dash in the Δ-prev column.
    // We look for — in the rendered table; 2022 is the last row.
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(1)
  })

  it('Δ prev year: 2023 row shows difference against 2022', () => {
    render(<YearComparisonTable {...defaultProps({ years: yearData, yearColorIndex: colorIndex, currentYear: 2024 })} />)
    // 2023 total = 200000 m = 200 km; 2022 total = 80000 m = 80 km; Δ = +120 km
    expect(screen.getByText('+120.0 km')).toBeInTheDocument()
  })

  it('Δ with 2024: 2023 row shows difference against 2024', () => {
    render(<YearComparisonTable {...defaultProps({ years: yearData, yearColorIndex: colorIndex, currentYear: 2024 })} />)
    // 2023 = 200 km, 2024 = 300 km → 2023 Δ against 2024 = 200-300 = -100 km
    expect(screen.getByText('−100.0 km')).toBeInTheDocument()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// Empty-year handling
// ─────────────────────────────────────────────────────────────────────────────

describe('YearComparisonTable — empty-year handling', () => {
  it('shows 0.0 km for empty year, not a dash', () => {
    const years = [
      makeYear(2024, [0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),
    ]
    const colorIndex = new Map([[2024, 0]])
    render(<YearComparisonTable {...defaultProps({ years, yearColorIndex: colorIndex, currentYear: 2024 })} />)
    expect(screen.getByText('0.0 km')).toBeInTheDocument()
  })
})

// ─────────────────────────────────────────────────────────────────────────────
// Locked-date behaviour
// ─────────────────────────────────────────────────────────────────────────────

describe('YearComparisonTable — locked-date behaviour', () => {
  it('shows header with month range when lockedDate is set', () => {
    // Lock to a date in May → last completed month = April
    const lockedDate = new Date(2024, 4, 2) // May 2nd

    render(
      <YearComparisonTable
        {...defaultProps({ lockedDate })}
      />
    )
    // Header should say "1. Januar → 30. April"
    expect(screen.getByText(/1\. Januar.*30\. April/)).toBeInTheDocument()
  })

  it('shows "Kein abgeschlossener Monat" header when locked to January', () => {
    // January lock: no completed month in the current year's data.
    const lockedDate = new Date(2024, 0, 15) // January 15th
    render(<YearComparisonTable {...defaultProps({ lockedDate })} />)
    expect(screen.getByText('Kein abgeschlossener Monat')).toBeInTheDocument()
  })

  it('shows 0 m for all years when locked to January (no completed month)', () => {
    const years = [
      makeYear(2024, [100000, 200000, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]),
    ]
    const colorIndex = new Map([[2024, 0]])
    render(
      <YearComparisonTable
        {...defaultProps({
          years,
          yearColorIndex: colorIndex,
          lockedDate: new Date(2024, 0, 15), // January
          currentYear: 2024,
          metric: 'elevation_gain_m',
        })}
      />
    )
    // 0 m (no completed month, value should be 0)
    expect(screen.getByText('0 m')).toBeInTheDocument()
  })

  it('shows full-year header when no lock', () => {
    render(<YearComparisonTable {...defaultProps({ lockedDate: null })} />)
    expect(screen.getByText(/1\. Januar.*31\. Dezember/)).toBeInTheDocument()
  })

  it('shows reset button only when locked', () => {
    const { rerender } = render(<YearComparisonTable {...defaultProps({ lockedDate: null })} />)
    expect(screen.queryByLabelText('Datumslock zurücksetzen')).toBeNull()

    rerender(
      <YearComparisonTable {...defaultProps({ lockedDate: new Date(2024, 4, 2) })} />
    )
    expect(screen.getByLabelText('Datumslock zurücksetzen')).toBeInTheDocument()
  })

  it('calls onLockedDateChange(null) when reset button is clicked', async () => {
    const onLockedDateChange = vi.fn()
    render(
      <YearComparisonTable
        {...defaultProps({
          lockedDate: new Date(2024, 4, 2),
          onLockedDateChange,
        })}
      />
    )
    await userEvent.click(screen.getByLabelText('Datumslock zurücksetzen'))
    expect(onLockedDateChange).toHaveBeenCalledWith(null)
  })

  it('uses cumulative value up to locked month', () => {
    // Lock to May 2nd → upToMonth = April = index 3
    // cumulative through April = 400000 m
    const years = [
      makeYear(2024,
        [100000, 100000, 100000, 100000, 100000, 0, 0, 0, 0, 0, 0, 0],
        [100000, 200000, 300000, 400000, 500000, 500000, 500000, 500000, 500000, 500000, 500000, 500000],
      ),
    ]
    const colorIndex = new Map([[2024, 0]])

    render(
      <YearComparisonTable
        {...defaultProps({
          years,
          yearColorIndex: colorIndex,
          lockedDate: new Date(2024, 4, 2), // May 2nd → April end
          currentYear: 2024,
        })}
      />
    )
    // cumulative[3] = 400000 m = 400 km
    expect(screen.getByText('400.0 km')).toBeInTheDocument()
  })
})
