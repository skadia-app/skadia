import { useMemo } from 'react'
import { useUser } from '@/providers/UserProvider'
import { localeForLanguage } from '@/i18n/languages'
import {
  formatDistance as formatUnitDistance,
  formatDistanceCompact as formatUnitDistanceCompact,
  formatElevation as formatUnitElevation,
  formatPace as formatUnitPace,
  formatPrecipitation as formatUnitPrecipitation,
  formatSpeed as formatUnitSpeed,
  formatTemperature as formatUnitTemperature,
  type UnitFormatOptions,
} from '@/lib/units'

export interface UnitFormatters {
  options: UnitFormatOptions
  formatDistance: (meters: number | null | undefined) => string
  formatDistanceCompact: (meters: number | null | undefined) => string
  formatElevation: (meters: number | null | undefined) => string
  formatElevationGain: (meters: number | null | undefined) => string | null
  formatElevationLoss: (meters: number | null | undefined) => string | null
  formatPace: (secondsPerKm: number | null | undefined) => string
  formatPrecipitation: (millimeters: number | null | undefined) => string
  formatSpeed: (metersPerSecond: number) => string
  formatTemperature: (celsius: number | null | undefined) => string
}

export function useUnitFormatters(): UnitFormatters {
  const { locale, units } = useUser()

  return useMemo(() => {
    const options: UnitFormatOptions = { locale: localeForLanguage(locale), units }

    return {
      options,
      formatDistance: (meters: number | null | undefined) => {
        if (meters == null || Number.isNaN(meters)) return '\u2014'
        return formatUnitDistance(meters, options)
      },
      formatDistanceCompact: (meters: number | null | undefined) => {
        if (meters == null || Number.isNaN(meters)) return '\u2014'
        return formatUnitDistanceCompact(meters, options)
      },
      formatElevation: (meters: number | null | undefined) => {
        if (meters == null || Number.isNaN(meters)) return '\u2014'
        return formatUnitElevation(meters, options)
      },
      formatElevationGain: (meters: number | null | undefined) => {
        if (meters == null || Number.isNaN(meters) || meters <= 0) return null
        return `+${formatUnitElevation(meters, options)}`
      },
      formatElevationLoss: (meters: number | null | undefined) => {
        if (meters == null || Number.isNaN(meters) || meters <= 0) return null
        return `\u2212${formatUnitElevation(meters, options)}`
      },
      formatPace: (secondsPerKm: number | null | undefined) => {
        if (secondsPerKm == null || Number.isNaN(secondsPerKm) || secondsPerKm <= 0) return '\u2014'
        return formatUnitPace(secondsPerKm, options)
      },
      formatPrecipitation: (millimeters: number | null | undefined) => {
        if (millimeters == null || Number.isNaN(millimeters)) return '\u2014'
        return formatUnitPrecipitation(millimeters, options)
      },
      formatSpeed: (metersPerSecond: number) => formatUnitSpeed(metersPerSecond, options),
      formatTemperature: (celsius: number | null | undefined) => {
        if (celsius == null || Number.isNaN(celsius)) return '\u2014'
        return formatUnitTemperature(celsius, options)
      },
    }
  }, [locale, units])
}
