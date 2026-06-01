import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { WeatherSettings } from './WeatherSettings'

const mocks = vi.hoisted(() => ({
  userPrefs: { locale: 'en', units: 'imperial' as 'metric' | 'imperial' },
  getWeatherSettings: vi.fn(),
  updateWeatherSettings: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => mocks.userPrefs,
}))

vi.mock('@/api/settings', () => ({
  getWeatherSettings: mocks.getWeatherSettings,
  updateWeatherSettings: mocks.updateWeatherSettings,
}))

vi.mock('@/api/client', () => ({
  apiClient: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function renderWeatherSettings() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <WeatherSettings />
    </QueryClientProvider>,
  )
}

describe('WeatherSettings unit preferences', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    mocks.userPrefs.locale = 'en'
    mocks.userPrefs.units = 'imperial'
    mocks.getWeatherSettings.mockResolvedValue({
      reading_mode: 'auto',
      mountain_threshold_m: 100,
      long_duration_s: 7200,
      display_fields: ['temp_c', 'freezing_level_m'],
      auto_tags: true,
    })
    mocks.updateWeatherSettings.mockImplementation(async prefs => prefs)
    await i18n.changeLanguage('en')
  })

  it('edits elevation threshold in imperial units while storing meters', async () => {
    renderWeatherSettings()

    const input = await screen.findByLabelText('Mountain/valley weather from elevation difference')
    expect(input).toHaveValue(328.084)
    expect(screen.getByText('ft elevation difference')).toBeInTheDocument()

    fireEvent.change(input, { target: { value: '656.168' } })

    await waitFor(() => {
      expect(mocks.updateWeatherSettings).toHaveBeenCalled()
    }, { timeout: 1200 })
    expect(mocks.updateWeatherSettings.mock.calls[0][0]).toEqual(expect.objectContaining({
      mountain_threshold_m: 200,
    }))
  })
})
