export function normalizeCountryCode(code: string | null | undefined): string | null {
  const normalized = code?.trim().toUpperCase()
  return normalized && /^[A-Z]{2}$/.test(normalized) ? normalized : null
}

export function countryCodeToFlag(code: string | null | undefined): string | null {
  const normalized = normalizeCountryCode(code)
  if (!normalized) return null
  const base = 127397
  return [...normalized].map(char => String.fromCodePoint(char.charCodeAt(0) + base)).join('')
}

export function activityCountryCodes(activity: {
  visited_country_codes?: string[] | null
  start_country_code?: string | null
  end_country_code?: string | null
  location_country_code?: string | null
}): string[] {
  const raw = activity.visited_country_codes?.length
    ? activity.visited_country_codes
    : [activity.start_country_code, activity.end_country_code, activity.location_country_code]
  return [...new Set(raw.map(normalizeCountryCode).filter((v): v is string => Boolean(v)))]
}
