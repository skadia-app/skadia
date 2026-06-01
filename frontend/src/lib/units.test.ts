import { describe, expect, it } from 'vitest'
import {
  distanceFromDisplayValue,
  distanceInputUnit,
  distanceToDisplayValue,
  elevationFromDisplayValue,
  elevationInputUnit,
  elevationToDisplayValue,
  fixedDisplayUnit,
  paceDisplayUnit,
  paceToDisplayValue,
  formatDistance,
  formatDistanceCompact,
  formatElevation,
  formatHeartRate,
  formatPace,
  formatPercent,
  formatPrecipitation,
  formatPower,
  formatShortLength,
  formatSpeed,
  formatTemperature,
  formatWeight,
  speedDisplayUnit,
  speedFromDisplayValue,
  speedToDisplayValue,
  temperatureDisplayUnit,
  temperatureFromDisplayValue,
  temperatureToDisplayValue,
  weightFromDisplayValue,
  weightInputUnit,
  weightToDisplayValue,
} from './units'

describe('localized unit formatting', () => {
  it('formats metric distance, elevation, speed, pace, weight, and temperature', () => {
    expect(formatDistance(10_000, { units: 'metric', locale: 'de-DE' })).toBe('10,0 km')
    expect(formatElevation(1234, { units: 'metric', locale: 'de-DE' })).toBe('1.234 m')
    expect(formatSpeed(10, { units: 'metric', locale: 'de-DE' })).toBe('36,0 km/h')
    expect(formatPace(300, { units: 'metric', locale: 'de-DE' })).toBe('5:00 min/km')
    expect(formatWeight(70, { units: 'metric', locale: 'de-DE' })).toBe('70,0 kg')
    expect(formatTemperature(20, { units: 'metric', locale: 'de-DE' })).toBe('20 °C')
    expect(formatPrecipitation(12.7, { units: 'metric', locale: 'de-DE' })).toBe('12,7 mm')
  })

  it('formats imperial distance, elevation, speed, pace, weight, and temperature', () => {
    expect(formatDistance(1609.344, { units: 'imperial', locale: 'en-US' })).toBe('1.0 mi')
    expect(formatElevation(100, { units: 'imperial', locale: 'en-US' })).toBe('328 ft')
    expect(formatSpeed(10, { units: 'imperial', locale: 'en-US' })).toBe('22.4 mph')
    expect(formatPace(300, { units: 'imperial', locale: 'en-US' })).toBe('8:03 min/mi')
    expect(formatWeight(70, { units: 'imperial', locale: 'en-US' })).toBe('154.3 lb')
    expect(formatTemperature(20, { units: 'imperial', locale: 'en-US' })).toBe('68 °F')
    expect(formatPrecipitation(12.7, { units: 'imperial', locale: 'en-US' })).toBe('0.50 in')
  })

  it('does not convert universal power units', () => {
    expect(formatPower(250, { locale: 'fr-FR' })).toBe('250 W')
  })

  it('keeps universal training and sensor units unchanged for both unit systems', () => {
    expect(formatPower(250, { locale: 'fr-FR' })).toBe('250 W')
    expect(formatHeartRate(151, { locale: 'fr-FR' })).toBe('151 bpm')
    expect(formatPercent(98.4, { locale: 'fr-FR' })).toBe('98,4 %')

    for (const unit of ['W', 'bpm', '%', 'TSS', 'FTP', 'PMC', 'HRV'] as const) {
      expect(fixedDisplayUnit(unit, 'metric')).toBe(unit)
      expect(fixedDisplayUnit(unit, 'imperial')).toBe(unit)
    }
  })

  it('formats compact route distances with local units', () => {
    expect(formatDistanceCompact(250, { units: 'metric', locale: 'de-DE' })).toBe('250 m')
    expect(formatDistanceCompact(1500, { units: 'metric', locale: 'de-DE' })).toBe('1,5 km')
    expect(formatDistanceCompact(250, { units: 'imperial', locale: 'en-US' })).toBe('820 ft')
    expect(formatDistanceCompact(3218.688, { units: 'imperial', locale: 'en-US' })).toBe('2.0 mi')
  })

  it('formats short movement lengths without rounding away precision', () => {
    expect(formatShortLength(1.25, { units: 'metric', locale: 'de-DE' })).toBe('1,25 m')
    expect(formatShortLength(1.25, { units: 'imperial', locale: 'en-US' })).toBe('4.1 ft')
  })

  it('converts distance input values without changing stored meters', () => {
    expect(distanceToDisplayValue(5000, 'metric')).toBe(5)
    expect(distanceFromDisplayValue(5, 'metric')).toBe(5000)
    expect(distanceInputUnit('metric')).toBe('km')

    expect(distanceToDisplayValue(1609.344, 'imperial')).toBe(1)
    expect(distanceFromDisplayValue(1, 'imperial')).toBe(1609.344)
    expect(distanceInputUnit('imperial')).toBe('mi')
  })

  it('converts elevation input values without changing stored meters', () => {
    expect(elevationToDisplayValue(100, 'metric')).toBe(100)
    expect(elevationFromDisplayValue(100, 'metric')).toBe(100)
    expect(elevationInputUnit('metric')).toBe('m')

    expect(elevationToDisplayValue(100, 'imperial')).toBeCloseTo(328.0839895)
    expect(elevationFromDisplayValue(328.0839895, 'imperial')).toBeCloseTo(100)
    expect(elevationInputUnit('imperial')).toBe('ft')
  })

  it('converts chart speed, pace, and temperature display values', () => {
    expect(speedToDisplayValue(10, 'metric')).toBe(36)
    expect(speedFromDisplayValue(36, 'metric')).toBe(10)
    expect(speedToDisplayValue(10, 'imperial')).toBeCloseTo(22.3693629)
    expect(speedFromDisplayValue(22.3693629, 'imperial')).toBeCloseTo(10)
    expect(speedDisplayUnit('metric')).toBe('km/h')
    expect(speedDisplayUnit('imperial')).toBe('mph')

    expect(paceToDisplayValue(300, 'metric')).toBe(5)
    expect(paceToDisplayValue(300, 'imperial')).toBeCloseTo(8.04672)
    expect(paceDisplayUnit('metric')).toBe('min/km')
    expect(paceDisplayUnit('imperial')).toBe('min/mi')

    expect(temperatureToDisplayValue(20, 'metric')).toBe(20)
    expect(temperatureFromDisplayValue(20, 'metric')).toBe(20)
    expect(temperatureToDisplayValue(20, 'imperial')).toBe(68)
    expect(temperatureFromDisplayValue(68, 'imperial')).toBe(20)
    expect(temperatureDisplayUnit('metric')).toBe('°C')
    expect(temperatureDisplayUnit('imperial')).toBe('°F')
  })

  it('converts weight input values without changing stored kilograms', () => {
    expect(weightToDisplayValue(70, 'metric')).toBe(70)
    expect(weightFromDisplayValue(70, 'metric')).toBe(70)
    expect(weightInputUnit('metric')).toBe('kg')

    expect(weightToDisplayValue(70, 'imperial')).toBeCloseTo(154.323583526)
    expect(weightFromDisplayValue(154.323583526, 'imperial')).toBeCloseTo(70)
    expect(weightInputUnit('imperial')).toBe('lb')
  })
})
