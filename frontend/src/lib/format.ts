export type DurationStyle = 'clock' | 'human' | 'compact' | 'short' | 'lifetime' | 'abbr' | 'decimal'

/** Format seconds in various styles. Default 'clock' = h:mm:ss / m:ss */
export function fmtDuration(s: number, style: DurationStyle = 'clock'): string {
  if (s == null || isNaN(s)) return '—'
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = Math.floor(s % 60)
  switch (style) {
    case 'clock':
      if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${sec.toString().padStart(2, '0')}`
      return `${m}:${sec.toString().padStart(2, '0')}`
    case 'human':
      if (h > 0) return `${h}h ${m.toString().padStart(2, '0')}min`
      return `${m}min`
    case 'compact':
      if (h > 0) return `${h}h ${m}m`
      return `${m} min`
    case 'short':
      if (h > 0) return sec > 0 ? `${h}h ${m}m ${sec}s` : m > 0 ? `${h}h ${m}m` : `${h}h`
      if (m > 0) return sec > 0 ? `${m}m ${sec}s` : `${m} min`
      return `${sec}s`
    case 'abbr':
      if (h > 0) return `${h}h ${m}m`
      if (m > 0) return sec > 0 ? `${m}m ${sec}s` : `${m}m`
      return `${sec}s`
    case 'lifetime':
      if (h >= 240) return `${(h / 24).toFixed(1)} Tage`
      return `${h} h`
    case 'decimal': {
      const hours = s / 3600
      if (hours >= 1) return `${hours.toFixed(1)} h`
      return `${m} min`
    }
  }
}

/** Format minutes (not seconds) — wrapper around fmtDuration for router/planner use. */
export function fmtDurationMin(minutes: number, style: DurationStyle = 'human'): string {
  return fmtDuration(minutes * 60, style)
}

/** Format meters as km or m. Returns '--' for null/NaN. */
export function fmtDist(m?: number | null): string {
  if (m == null || isNaN(m)) return '—'
  if (m >= 1000) return `${(m / 1000).toFixed(2)} km`
  return `${Math.round(m)} m`
}

/** Format an ISO date string as HH:MM (de-DE locale). Optional timezone (IANA or UTC offset). */
export function fmtTime(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleTimeString('de-DE', opts)
}

/** Format a Date object as HH:MM (de-DE locale). Optional timezone (IANA or UTC offset). */
export function fmtTimeFromDate(date: Date, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }
  if (timezone) opts.timeZone = timezone
  return date.toLocaleTimeString('de-DE', opts)
}

/**
 * Format an ISO date string as a full weekday+date string (de-DE locale).
 * Optional timezone — avoids date-fns format() which ignores timezone.
 * Example: "Montag, 28. März 2026"
 */
export function fmtActivityDate(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = {
    weekday: 'long', day: '2-digit', month: 'long', year: 'numeric',
  }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleDateString('de-DE', opts)
}

/**
 * Format an ISO date string as short weekday+date (de-DE locale).
 * Example: "Mo., 28. Mär. 2026"
 */
export function fmtActivityDateShort(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = {
    weekday: 'short', day: '2-digit', month: 'short', year: 'numeric',
  }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleDateString('de-DE', opts)
}

/**
 * Format an ISO date string as date + time (de-DE locale).
 * Example: "28.03.2026, 10:15"
 */
export function fmtDateTime(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit',
  }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleString('de-DE', opts)
}

/**
 * Format an ISO date string as compact numeric date (de-DE locale).
 * Example: "28.03.2026"
 */
export function fmtDateCompact(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { day: '2-digit', month: '2-digit', year: 'numeric' }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleDateString('de-DE', opts)
}

/**
 * Format an ISO date string as medium date with abbreviated month (de-DE locale).
 * Example: "28. Mär. 2026"
 */
export function fmtDateMedium(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short', year: 'numeric' }
  if (timezone) opts.timeZone = timezone
  return new Date(iso).toLocaleDateString('de-DE', opts)
}
