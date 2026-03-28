import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { fmtDuration, fmtDist } from '@/lib/format'
export { sportColor, sportLabel } from '@/lib/sportTaxonomy'

/** Backwards-compatible re-export — defaults to 'human' style (was the old utils.ts format). */
export const formatDuration = (s: number) => fmtDuration(s, 'human')
export const formatDistance = fmtDist

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// Gibt null zurueck wenn kein sinnvoller Wert vorhanden (null, undefined, NaN, <= 0)
export function formatElevationGain(meters: number | null | undefined): string | null {
  if (meters == null || isNaN(meters) || meters <= 0) return null
  return `+${Math.round(meters)} m`
}

export function formatElevationLoss(meters: number | null | undefined): string | null {
  if (meters == null || isNaN(meters) || meters <= 0) return null
  return `−${Math.round(meters)} m`
}

// Sub-Types die Indoor schon semantisch implizieren – kein separates Badge noetig
const implicitlyIndoorSubTypes = new Set(['bouldering', 'indoor_climbing'])

export function showIndoorBadge(isIndoor: boolean, subType?: string | null): boolean {
  if (!isIndoor) return false
  return !implicitlyIndoorSubTypes.has(subType ?? '')
}
