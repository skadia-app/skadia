import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'
import type { SportType } from '@/api/types'

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
  if (meters >= 1000) return `${(meters / 1000).toFixed(1)} km`
  return `${Math.round(meters)} m`
}

// Gibt null zurück wenn kein sinnvoller Wert vorhanden (null, undefined, NaN, ≤ 0)
export function formatElevationGain(meters: number | null | undefined): string | null {
  if (meters == null || isNaN(meters) || meters <= 0) return null
  return `+${Math.round(meters)} m`
}

export function formatElevationLoss(meters: number | null | undefined): string | null {
  if (meters == null || isNaN(meters) || meters <= 0) return null
  return `−${Math.round(meters)} m`
}

export function sportColor(sport: SportType): string {
  const map: Record<string, string> = {
    cycling: '#F97316',
    running: '#3B82F6',
    hiking: '#22C55E',
    swimming: '#06B6D4',
    e_biking: '#A855F7',
    skiing: '#93C5FD',
    paddling: '#2DD4BF',
  }
  return map[sport] ?? '#6B7280'
}

export function sportLabel(sport: SportType, subType?: string | null): string {
  const labels: Record<string, string> = {
    cycling: 'Radfahren',
    running: 'Laufen',
    hiking: 'Wandern',
    swimming: 'Schwimmen',
    e_biking: 'E-Bike',
    skiing: 'Ski',
    paddling: 'Paddeln',
    walking: 'Gehen',
    strength: 'Kraft',
    rock_climbing: 'Klettern',
    stand_up_paddleboarding: 'SUP',
    generic: 'Sonstige',
    unknown: 'Unbekannt',
  }
  if (subType === 'mountain') return 'MTB'
  if (subType === 'road') return 'Rennrad'
  if (subType === 'e_bike_fitness') return 'E-Bike'
  if (subType === 'trail') return 'Trail Running'
  if (subType === 'bouldering') return 'Bouldern'
  if (subType === 'indoor_climbing') return 'Klettern (Halle)'
  return labels[sport] ?? sport
}

// Sub-Types die Indoor schon semantisch implizieren – kein separates Badge nötig
const implicitlyIndoorSubTypes = new Set(['bouldering', 'indoor_climbing'])

export function showIndoorBadge(isIndoor: boolean, subType?: string | null): boolean {
  if (!isIndoor) return false
  return !implicitlyIndoorSubTypes.has(subType ?? '')
}
