import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import { sportLabel as taxonomySportLabel, sportColor as taxonomySportColor } from '@/lib/sportTaxonomy'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function formatDuration(seconds: number): string {
  if (!seconds || isNaN(seconds)) return '—'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m.toString().padStart(2, '0')}min`
  return `${m}min`
}

export function formatDistance(meters: number | null): string {
  if (meters === null || isNaN(meters) || meters < 0) return '—'
  if (meters >= 1000) return `${(meters / 1000).toFixed(2)} km`
  return `${Math.round(meters)} m`
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

export function sportColor(sport: string): string {
  return taxonomySportColor(sport)
}

export function sportLabel(sport: string, subType?: string | null): string {
  return taxonomySportLabel(sport, subType)
}

// Sub-Types die Indoor schon semantisch implizieren – kein separates Badge noetig
const implicitlyIndoorSubTypes = new Set(['bouldering', 'indoor_climbing'])

export function showIndoorBadge(isIndoor: boolean, subType?: string | null): boolean {
  if (!isIndoor) return false
  return !implicitlyIndoorSubTypes.has(subType ?? '')
}
