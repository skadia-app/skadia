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

/** Show blue "I" badge when activity is indoor. Always shown, no suppression. */
export function showIndoorBadge(isIndoor: boolean): boolean {
  return isIndoor
}

// Delay revoking the object URL so the browser has time to initiate the download.
// Revoking synchronously after click() can race the browser's download initiation
// and silently abort the download in Firefox and some Chromium versions.
export function triggerBlobDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  setTimeout(() => URL.revokeObjectURL(url), 10_000)
}

export function triggerUrlDownload(url: string, filename: string): void {
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

export function findClosestIdx(arr: number[], target: number): number {
  if (arr.length === 0) return 0
  let bestIdx = 0
  let bestDiff = Math.abs(arr[0] - target)
  for (let i = 1; i < arr.length; i++) {
    const d = Math.abs(arr[i] - target)
    if (d < bestDiff) { bestDiff = d; bestIdx = i }
  }
  return bestIdx
}
