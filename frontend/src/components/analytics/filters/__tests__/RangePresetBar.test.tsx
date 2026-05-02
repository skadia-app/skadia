import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import '@testing-library/jest-dom'
import RangePresetBar, { type Preset, type DateRange, type PresetValue } from '../RangePresetBar'

// Mock DateRangePicker so we can assert it mounts without running the real calendar.
vi.mock('@/components/DateRangePicker', () => ({
  default: ({ onChange }: { onChange: (from: string, to: string) => void }) => (
    <div data-testid="date-range-picker">
      <button
        data-testid="mock-picker-apply"
        onClick={() => onChange('2024-01-01', '2024-03-31')}
      >
        Anwenden
      </button>
    </div>
  ),
}))

const PRESETS: Preset[] = [
  { value: '7d',     label: '7 Tage' },
  { value: '30d',    label: '30 Tage' },
  { value: '90d',    label: '90 Tage' },
  { value: 'custom', label: 'Benutzerdefiniert' },
]

describe('RangePresetBar', () => {
  let onChange: ReturnType<typeof vi.fn<(range: DateRange, preset: PresetValue) => void>>

  beforeEach(() => {
    onChange = vi.fn<(range: DateRange, preset: PresetValue) => void>()
  })

  it('renders all non-custom presets as buttons', () => {
    render(<RangePresetBar presets={PRESETS} value="30d" onChange={onChange} />)
    expect(screen.getByText('7 Tage')).toBeInTheDocument()
    expect(screen.getByText('30 Tage')).toBeInTheDocument()
    expect(screen.getByText('90 Tage')).toBeInTheDocument()
    expect(screen.getByText('Benutzerdefiniert')).toBeInTheDocument()
  })

  it('calls onChange with correct date range when 7d preset is clicked', () => {
    render(<RangePresetBar presets={PRESETS} value="30d" onChange={onChange} />)
    fireEvent.click(screen.getByText('7 Tage'))

    expect(onChange).toHaveBeenCalledOnce()
    const [range, preset] = onChange.mock.calls[0] as [DateRange, PresetValue]
    expect(preset).toBe('7d')

    // Verify the range covers approximately 7 days (±1 day tolerance for test timing)
    const from = new Date(range.from)
    const to = new Date(range.to)
    const diffDays = Math.round((to.getTime() - from.getTime()) / (1000 * 60 * 60 * 24))
    expect(diffDays).toBe(7)
  })

  it('calls onChange with 30d range when 30d preset is clicked', () => {
    render(<RangePresetBar presets={PRESETS} value="7d" onChange={onChange} />)
    fireEvent.click(screen.getByText('30 Tage'))

    const [range, preset] = onChange.mock.calls[0] as [DateRange, PresetValue]
    expect(preset).toBe('30d')
    const diffDays = Math.round((new Date(range.to).getTime() - new Date(range.from).getTime()) / (1000 * 60 * 60 * 24))
    expect(diffDays).toBe(30)
  })

  it('clicking custom button opens DateRangePicker', () => {
    render(<RangePresetBar presets={PRESETS} value="30d" onChange={onChange} />)

    expect(screen.queryByTestId('date-range-picker')).not.toBeInTheDocument()
    fireEvent.click(screen.getByText('Benutzerdefiniert'))
    expect(screen.getByTestId('date-range-picker')).toBeInTheDocument()
  })

  it('confirming a custom range calls onChange with custom preset and given dates', () => {
    render(<RangePresetBar presets={PRESETS} value="30d" onChange={onChange} />)

    fireEvent.click(screen.getByText('Benutzerdefiniert'))
    fireEvent.click(screen.getByTestId('mock-picker-apply'))

    expect(onChange).toHaveBeenCalledOnce()
    const [range, preset] = onChange.mock.calls[0] as [DateRange, PresetValue]
    expect(preset).toBe('custom')
    expect(range.from).toBe('2024-01-01')
    expect(range.to).toBe('2024-03-31')
  })

  it('applies active styling to the currently selected preset', () => {
    render(<RangePresetBar presets={PRESETS} value="90d" onChange={onChange} />)
    const activeBtn = screen.getByText('90 Tage')
    expect(activeBtn.className).toContain('bg-neutral-900')
  })
})
