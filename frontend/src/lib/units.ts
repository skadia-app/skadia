export type UnitSystem = 'metric' | 'imperial'

export interface UnitFormatOptions {
  units: UnitSystem
  locale: string
}

export interface LocaleFormatOptions {
  locale: string
}

export type FixedDisplayUnit = 'W' | 'bpm' | '%' | 'TSS' | 'FTP' | 'PMC' | 'HRV'

const METERS_PER_MILE = 1609.344
const FEET_PER_METER = 3.280839895
const POUNDS_PER_KG = 2.2046226218
const MM_PER_INCH = 25.4

function formatNumber(value: number, locale: string, digits: number): string {
  return new Intl.NumberFormat(locale, {
    minimumFractionDigits: digits,
    maximumFractionDigits: digits,
  }).format(value)
}

function formatInteger(value: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    maximumFractionDigits: 0,
  }).format(Math.round(value))
}

function formatDurationMinutes(seconds: number): string {
  const totalSeconds = Math.round(seconds)
  const minutes = Math.floor(totalSeconds / 60)
  const secs = String(totalSeconds % 60).padStart(2, '0')
  return `${minutes}:${secs}`
}

export function formatDistance(meters: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatNumber(meters / METERS_PER_MILE, options.locale, 1)} mi`
  }
  return `${formatNumber(meters / 1000, options.locale, 1)} km`
}

export function formatDistanceCompact(meters: number, options: UnitFormatOptions): string {
  const absMeters = Math.abs(meters)
  if (options.units === 'imperial') {
    if (absMeters < METERS_PER_MILE) {
      return `${formatInteger(meters * FEET_PER_METER, options.locale)} ft`
    }
    return `${formatNumber(meters / METERS_PER_MILE, options.locale, 1)} mi`
  }
  if (absMeters < 1000) {
    return `${formatInteger(meters, options.locale)} m`
  }
  return `${formatNumber(meters / 1000, options.locale, 1)} km`
}

export function formatShortLength(meters: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatNumber(meters * FEET_PER_METER, options.locale, 1)} ft`
  }
  return `${formatNumber(meters, options.locale, 2)} m`
}

export function distanceToDisplayValue(meters: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return meters / METERS_PER_MILE
  }
  return meters / 1000
}

export function distanceFromDisplayValue(value: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return value * METERS_PER_MILE
  }
  return value * 1000
}

export function distanceInputUnit(units: UnitSystem): 'km' | 'mi' {
  return units === 'imperial' ? 'mi' : 'km'
}

export function speedToDisplayValue(metersPerSecond: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return (metersPerSecond * 3600) / METERS_PER_MILE
  }
  return metersPerSecond * 3.6
}

export function speedFromDisplayValue(value: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return (value * METERS_PER_MILE) / 3600
  }
  return value / 3.6
}

export function speedDisplayUnit(units: UnitSystem): 'km/h' | 'mph' {
  return units === 'imperial' ? 'mph' : 'km/h'
}

export function paceToDisplayValue(secondsPerKm: number, units: UnitSystem): number {
  const minutesPerKm = secondsPerKm / 60
  if (units === 'imperial') {
    return minutesPerKm * (METERS_PER_MILE / 1000)
  }
  return minutesPerKm
}

export function paceDisplayUnit(units: UnitSystem): 'min/km' | 'min/mi' {
  return units === 'imperial' ? 'min/mi' : 'min/km'
}

export function elevationToDisplayValue(meters: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return meters * FEET_PER_METER
  }
  return meters
}

export function elevationFromDisplayValue(value: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return value / FEET_PER_METER
  }
  return value
}

export function elevationInputUnit(units: UnitSystem): 'm' | 'ft' {
  return units === 'imperial' ? 'ft' : 'm'
}

export function weightToDisplayValue(kg: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return kg * POUNDS_PER_KG
  }
  return kg
}

export function weightFromDisplayValue(value: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return value / POUNDS_PER_KG
  }
  return value
}

export function weightInputUnit(units: UnitSystem): 'kg' | 'lb' {
  return units === 'imperial' ? 'lb' : 'kg'
}

export function formatElevation(meters: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatInteger(meters * FEET_PER_METER, options.locale)} ft`
  }
  return `${formatInteger(meters, options.locale)} m`
}

export function formatSpeed(metersPerSecond: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatNumber((metersPerSecond * 3600) / METERS_PER_MILE, options.locale, 1)} mph`
  }
  return `${formatNumber(metersPerSecond * 3.6, options.locale, 1)} km/h`
}

export function formatPace(secondsPerKm: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatDurationMinutes(secondsPerKm * (METERS_PER_MILE / 1000))} min/mi`
  }
  return `${formatDurationMinutes(secondsPerKm)} min/km`
}

export function formatWeight(kg: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatNumber(kg * POUNDS_PER_KG, options.locale, 1)} lb`
  }
  return `${formatNumber(kg, options.locale, 1)} kg`
}

export function formatTemperature(celsius: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatInteger((celsius * 9) / 5 + 32, options.locale)} °F`
  }
  return `${formatInteger(celsius, options.locale)} °C`
}

export function formatPrecipitation(millimeters: number, options: UnitFormatOptions): string {
  if (options.units === 'imperial') {
    return `${formatNumber(millimeters / MM_PER_INCH, options.locale, 2)} in`
  }
  return `${formatNumber(millimeters, options.locale, 1)} mm`
}

export function temperatureToDisplayValue(celsius: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return (celsius * 9) / 5 + 32
  }
  return celsius
}

export function temperatureFromDisplayValue(value: number, units: UnitSystem): number {
  if (units === 'imperial') {
    return ((value - 32) * 5) / 9
  }
  return value
}

export function temperatureDisplayUnit(units: UnitSystem): '°C' | '°F' {
  return units === 'imperial' ? '°F' : '°C'
}

export function fixedDisplayUnit(unit: FixedDisplayUnit, _units: UnitSystem): FixedDisplayUnit {
  return unit
}

export function formatPower(watts: number, options: LocaleFormatOptions): string {
  return `${formatInteger(watts, options.locale)} W`
}

export function formatHeartRate(bpm: number, options: LocaleFormatOptions): string {
  return `${formatInteger(bpm, options.locale)} bpm`
}

export function formatPercent(value: number, options: LocaleFormatOptions, digits = 1): string {
  return `${formatNumber(value, options.locale, digits)} %`
}
