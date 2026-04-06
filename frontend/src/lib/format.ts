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

// Old DB data may have "UTC+HH:MM" format from before the b2f4b2f6 fix.
// Intl.DateTimeFormat rejects the "UTC" prefix with a RangeError — strip it.
export function normTz(tz: string | null | undefined): string | undefined {
  if (!tz) return undefined
  return tz.replace(/^UTC([+-])/, '$1')
}

/** Format an ISO date string as HH:MM (de-DE locale). Optional timezone (IANA or UTC offset). */
export function fmtTime(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
  return new Date(iso).toLocaleTimeString('de-DE', opts)
}

/** Format a Date object as HH:MM (de-DE locale). Optional timezone (IANA or UTC offset). */
export function fmtTimeFromDate(date: Date, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
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
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
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
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
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
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
  return new Date(iso).toLocaleString('de-DE', opts)
}

/**
 * Format an ISO date string as compact numeric date (de-DE locale).
 * Example: "28.03.2026"
 */
export function fmtDateCompact(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { day: '2-digit', month: '2-digit', year: 'numeric' }
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
  return new Date(iso).toLocaleDateString('de-DE', opts)
}

/**
 * Format an ISO date string as medium date with abbreviated month (de-DE locale).
 * Example: "28. Mär. 2026"
 */
export function fmtDateMedium(iso: string, timezone?: string | null): string {
  const opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short', year: 'numeric' }
  const tz = normTz(timezone)
  if (tz) opts.timeZone = tz
  return new Date(iso).toLocaleDateString('de-DE', opts)
}

/**
 * Format a Date as a `datetime-local` input value (YYYY-MM-DDTHH:MM).
 * Uses local time — suitable for `<input type="datetime-local">` default values.
 */
export function fmtLocalDateTimeInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// ── Byte / size formatters ──────────────────────────────────────────────────

/** Format bytes as human-readable string (B / KB / MB / GB / TB). Returns '—' for null/NaN/0. */
export function fmtBytes(bytes: number | null | undefined): string {
  if (bytes == null || isNaN(bytes) || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const val = bytes / Math.pow(1024, i)
  return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// ── User-Agent parsing ─────────────────────────────────────────────────────

/** Extract browser name + version from a User-Agent string. */
export function parseUserAgent(ua: string): string {
  if (!ua) return '—'
  if (/Firefox\//i.test(ua)) {
    const m = ua.match(/Firefox\/([\d.]+)/)
    return m ? `Firefox ${m[1]}` : 'Firefox'
  }
  if (/Edg\//i.test(ua)) {
    const m = ua.match(/Edg\/([\d.]+)/)
    return m ? `Edge ${m[1]}` : 'Edge'
  }
  if (/Chrome\//i.test(ua) && !/Chromium/i.test(ua)) {
    const m = ua.match(/Chrome\/([\d.]+)/)
    return m ? `Chrome ${m[1]}` : 'Chrome'
  }
  if (/Safari\//i.test(ua) && !/Chrome/i.test(ua)) {
    const m = ua.match(/Version\/([\d.]+)/)
    return m ? `Safari ${m[1]}` : 'Safari'
  }
  return ua.length > 60 ? ua.slice(0, 57) + '...' : ua
}

// ── Comparison / summary formatters ──────────────────────────────────────────

/** Format a heart-rate value in bpm. Returns '—' for null/NaN. */
export function fmtHr(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} bpm`
}

/** Format a power value in Watts. Returns '—' for null/NaN. */
export function fmtPower(v: number | null | undefined): string {
  if (v == null || isNaN(v)) return '\u2014'
  return `${Math.round(v)} W`
}

/** Format an elevation gain value in metres. Returns '—' for null/NaN/zero. */
export function fmtElevation(v: number | null | undefined): string {
  if (v == null || isNaN(v) || v <= 0) return '\u2014'
  return `${Math.round(v)} m`
}
