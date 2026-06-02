import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import '@testing-library/jest-dom'
import HealthRangePresetBar from '../HealthRangePresetBar'
import { HealthRangeProvider } from '../HealthRangeContext'

const t = vi.fn((key: string) => key)

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t }),
}))

vi.mock('../RangePresetBar', () => ({
  default: ({ presets }: { presets: { value: string; label: string }[] }) => (
    <div>
      {presets.map(preset => (
        <button key={preset.value}>{preset.label}</button>
      ))}
    </div>
  ),
}))

describe('HealthRangePresetBar', () => {
  it('uses health range catalog keys without German default fallback labels', () => {
    render(
      <HealthRangeProvider>
        <HealthRangePresetBar />
      </HealthRangeProvider>,
    )

    expect(t).toHaveBeenCalledWith('analyticsHealthInsights.healthRange.7d')
    expect(t).toHaveBeenCalledWith('analyticsHealthInsights.healthRange.30d')
    expect(t).toHaveBeenCalledWith('analyticsHealthInsights.healthRange.90d')
    expect(t).toHaveBeenCalledWith('analyticsHealthInsights.healthRange.365d')
    expect(t).toHaveBeenCalledWith('analyticsHealthInsights.healthRange.custom')
  })
})
