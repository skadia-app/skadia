import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { WeatherReading } from '@/api/types'
import { i18n } from '@/i18n/i18n'
import WeatherBadge from './WeatherBadge'

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'en', units: 'imperial' }),
}))

function reading(overrides: Partial<WeatherReading> = {}): WeatherReading {
  return {
    id: 'weather-1',
    activity_id: 'activity-1',
    label: 'Start',
    recorded_at: '2026-05-01T08:00:00Z',
    temp_c: 20,
    apparent_temp_c: 22,
    condition: 'Clear',
    humidity: 50,
    wind_speed_ms: 10,
    wind_gusts_ms: 15,
    precipitation_mm: 12.7,
    freezing_level_m: 100,
    uv_index: 0,
    model: 'test',
    ...overrides,
  } as WeatherReading
}

describe('WeatherBadge unit preferences', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en')
  })

  it('formats temperature, precipitation, wind speed, gusts, and freezing level with selected units', () => {
    render(<WeatherBadge readings={[reading()]} />)

    expect(screen.getByText('68 °F')).toBeInTheDocument()
    expect(screen.getByText('72 °F feels like')).toBeInTheDocument()
    expect(screen.getByText('🌧 0.50 in')).toBeInTheDocument()
    expect(screen.getByText(/22\.4 mph/)).toBeInTheDocument()
    expect(screen.getByText(/\(gusts 33\.6 mph\)/)).toBeInTheDocument()
    expect(screen.getByText('Freezing level: 328 ft')).toBeInTheDocument()
  })

  it('localizes known backend weather conditions without changing condition classification', async () => {
    await i18n.changeLanguage('fr')

    render(<WeatherBadge readings={[reading({ condition: 'Schneefall' })]} />)

    expect(screen.getByText('❄️')).toBeInTheDocument()
    expect(screen.getByText('Chute de neige')).toBeInTheDocument()
    expect(screen.queryByText('Schneefall')).not.toBeInTheDocument()
  })

  it('uses stable weather tags for unknown condition text labels and icons', async () => {
    await i18n.changeLanguage('en')

    render(<WeatherBadge readings={[reading({ condition: 'Clear', tags: ['sonnig'] })]} />)

    expect(screen.getByText('☀️')).toBeInTheDocument()
    expect(screen.getByText('sunny')).toBeInTheDocument()
    expect(screen.queryByText('Clear')).not.toBeInTheDocument()
  })

  it('uses weather tag priority consistently for fallback labels and icons', async () => {
    await i18n.changeLanguage('en')

    render(<WeatherBadge readings={[reading({ condition: 'Storm', tags: ['regen', 'gewitter'] })]} />)

    expect(screen.getByText('⛈️')).toBeInTheDocument()
    expect(screen.getByText('thunderstorm')).toBeInTheDocument()
    expect(screen.queryByText('Storm')).not.toBeInTheDocument()
  })

  it('keeps documented condition labels and icons ahead of tag fallback', async () => {
    await i18n.changeLanguage('en')

    render(<WeatherBadge readings={[reading({ condition: 'Schneefall', tags: ['regen'] })]} />)

    expect(screen.getByText('❄️')).toBeInTheDocument()
    expect(screen.getByText('snowfall')).toBeInTheDocument()
    expect(screen.queryByText('rain')).not.toBeInTheDocument()
  })

  it.each([
    ['klar', 'clear'],
    ['überwiegend klar', 'mostly clear'],
    ['teilweise bewölkt', 'partly cloudy'],
    ['bedeckt', 'overcast'],
    ['Nebel', 'fog'],
    ['Nieselregen', 'drizzle'],
    ['gefrierender Nieselregen', 'freezing drizzle'],
    ['Regen', 'rain'],
    ['gefrierender Regen', 'freezing rain'],
    ['Schneefall', 'snowfall'],
    ['Schneegriesel', 'snow grains'],
    ['Regenschauer', 'rain showers'],
    ['Schneeschauer', 'snow showers'],
    ['Gewitter', 'thunderstorm'],
    ['Gewitter mit leichtem Hagel', 'thunderstorm with light hail'],
    ['Gewitter mit starkem Hagel', 'thunderstorm with heavy hail'],
    ['unbekannt', 'unknown'],
  ])('localizes documented backend condition "%s"', (condition, expected) => {
    render(<WeatherBadge readings={[reading({ condition })]} />)

    expect(screen.getByText(expected)).toBeInTheDocument()
  })

  it('keeps unknown backend weather conditions unchanged', () => {
    render(<WeatherBadge readings={[reading({ condition: 'Local microclimate' })]} />)

    expect(screen.getByText('Local microclimate')).toBeInTheDocument()
  })
})
