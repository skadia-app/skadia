import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import SportOnlyFilterBar, { expandSportSelection } from '../SportOnlyFilterBar'
import type { SportOnlyFilterValue } from '../SportOnlyFilterBar'

// Mock child components that have complex dependencies.
vi.mock('@/components/sport/SportFilter', () => ({
  SportFilter: ({
    value,
    onChange,
  }: {
    value: { sportTypes: string[] | null }
    onChange: (v: { sportTypes: string[] | null }) => void
  }) => (
    <div data-testid="sport-filter">
      <button
        data-testid="sport-filter-toggle-cycling"
        onClick={() => onChange({ sportTypes: ['cycling:road', 'cycling:mountain'] })}
      >
        Radfahren auswählen
      </button>
      <span data-testid="sport-filter-value">
        {value.sportTypes ? value.sportTypes.join(',') : 'all'}
      </span>
    </div>
  ),
}))

vi.mock('../RangePresetBar', () => ({
  default: ({
    value,
    onChange,
  }: {
    value: string
    onChange: (range: { from: string; to: string }, preset: string) => void
  }) => (
    <div data-testid="range-preset-bar">
      <button
        data-testid="preset-1y"
        onClick={() => onChange({ from: '2024-01-01', to: '2024-12-31' }, '1y')}
      >
        1 Jahr
      </button>
      <span data-testid="active-preset">{value}</span>
    </div>
  ),
  presetToRange: vi.fn(),
}))

vi.mock('@/components/DateRangePicker', () => ({
  default: () => <div data-testid="date-range-picker" />,
}))

const DEFAULT_VALUE: SportOnlyFilterValue = {
  selectedSports: null,
  from: '',
  to: '',
}

describe('SportOnlyFilterBar', () => {
  let onChange: ReturnType<typeof vi.fn<(value: SportOnlyFilterValue) => void>>

  beforeEach(() => {
    onChange = vi.fn<(value: SportOnlyFilterValue) => void>()
  })

  it('renders sport filter and range preset bar', () => {
    render(<SportOnlyFilterBar value={DEFAULT_VALUE} onChange={onChange} />)
    expect(screen.getByTestId('sport-filter')).toBeInTheDocument()
    expect(screen.getByTestId('range-preset-bar')).toBeInTheDocument()
  })

  it('has default empty sport selection', () => {
    render(<SportOnlyFilterBar value={DEFAULT_VALUE} onChange={onChange} />)
    expect(screen.getByTestId('sport-filter-value')).toHaveTextContent('all')
  })

  it('calls onChange when sport filter changes', () => {
    render(<SportOnlyFilterBar value={DEFAULT_VALUE} onChange={onChange} />)
    fireEvent.click(screen.getByTestId('sport-filter-toggle-cycling'))

    expect(onChange).toHaveBeenCalledOnce()
    const [updatedValue] = onChange.mock.calls[0] as [SportOnlyFilterValue]
    expect(updatedValue.selectedSports).toEqual(['cycling:road', 'cycling:mountain'])
  })

  it('calls onChange when range preset changes', () => {
    render(<SportOnlyFilterBar value={DEFAULT_VALUE} onChange={onChange} />)
    fireEvent.click(screen.getByTestId('preset-1y'))

    expect(onChange).toHaveBeenCalledOnce()
    const [updatedValue] = onChange.mock.calls[0] as [SportOnlyFilterValue]
    expect(updatedValue.from).toBe('2024-01-01')
    expect(updatedValue.to).toBe('2024-12-31')
  })

  it('preserves existing selectedSports when range changes', () => {
    const valueWithSports: SportOnlyFilterValue = { selectedSports: ['running:trail'], from: '', to: '' }
    render(<SportOnlyFilterBar value={valueWithSports} onChange={onChange} />)
    fireEvent.click(screen.getByTestId('preset-1y'))

    const [updatedValue] = onChange.mock.calls[0] as [SportOnlyFilterValue]
    expect(updatedValue.selectedSports).toEqual(['running:trail'])
  })
})

describe('expandSportSelection', () => {
  it('passes through sport:subtype tokens unchanged', () => {
    const result = expandSportSelection(['cycling:road', 'running:trail'])
    expect(result).toContain('cycling:road')
    expect(result).toContain('running:trail')
  })

  it('expands a category key to all subtype tokens in that category', () => {
    const result = expandSportSelection(['bike'])
    // cycling is in the bike category and has many subtypes
    expect(result.some(t => t.startsWith('cycling:'))).toBe(true)
    // should not contain bare 'bike' token
    expect(result).not.toContain('bike')
  })

  it('expands cycling category to include known subtypes', () => {
    const result = expandSportSelection(['bike'])
    expect(result).toContain('cycling:road')
    expect(result).toContain('cycling:mountain')
    expect(result).toContain('cycling:gravel_cycling')
  })

  it('handles mixed input of categories and sport:subtype tokens', () => {
    const result = expandSportSelection(['running:trail', 'bike'])
    expect(result).toContain('running:trail')
    expect(result).toContain('cycling:road')
    expect(result).toContain('cycling:mountain')
  })

  it('handles empty selection', () => {
    expect(expandSportSelection([])).toEqual([])
  })

  it('expands run category to foot sports', () => {
    const result = expandSportSelection(['run'])
    // running is in the run category and has subtypes
    expect(result.some(t => t.startsWith('running:'))).toBe(true)
    expect(result).not.toContain('run')
  })

  it('drops unknown bare tokens and emits a console warning', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = expandSportSelection(['Rennrad'])
    expect(result).toEqual([])
    expect(warnSpy).toHaveBeenCalledWith(
      '[expandSportSelection] dropping unrecognised sport token:',
      'Rennrad',
    )
    warnSpy.mockRestore()
  })

  it('passes through sport:subtype tokens unchanged and drops bare unknown tokens', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = expandSportSelection(['cycling:road', 'Rennrad'])
    expect(result).toContain('cycling:road')
    expect(result).not.toContain('Rennrad')
    expect(warnSpy).toHaveBeenCalledTimes(1)
    warnSpy.mockRestore()
  })

  it('handles mixed category expansion and sport:subtype without bare-token drift', () => {
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const result = expandSportSelection(['cycling', 'cycling:road'])
    // 'cycling' is not a category key — it should be dropped
    expect(result).not.toContain('cycling')
    // 'cycling:road' passes through unchanged
    expect(result).toContain('cycling:road')
    warnSpy.mockRestore()
  })
})
