/**
 * Format a Date as YYYY-MM-DD using LOCAL timezone.
 * Never use `toISOString().slice(0,10)` for date-only strings — that converts
 * to UTC first, causing an off-by-one error in UTC+ timezones (e.g. Germany).
 */
export function toLocalISO(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
