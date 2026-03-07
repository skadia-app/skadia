import { useSearchParams } from 'react-router-dom'
import { useMemo, useCallback } from 'react'

export interface AnalyticsFilterState {
  from: string
  to: string
  sportTypes: string[]
  tags: string[]
  weather: string
  tempMin: string
  tempMax: string
  timeOfDay: string[]
  intensity: string[]
  distMin: string
  distMax: string
  elevMin: string
  elevMax: string
  gearId: string
  isCommute: string
  isRace: string
  paceMin: string
  paceMax: string
  countries: string[]
  seasons: string[]
  weekdays: number[]
  hrMin: string
  hrMax: string
  powerMin: string
  powerMax: string
  // Tri-state tag filters
  includeTags: string[]  // tag IDs to include (activity must have ALL)
  excludeTags: string[]  // tag IDs to exclude (activity must have NONE)
  // Tri-state gear filters
  includeGear: string[]  // gear IDs to include
  excludeGear: string[]  // gear IDs to exclude
}

const PRESET_KEY = 'skadia-analytics-presets'

export interface FilterPreset {
  name: string
  filters: Partial<AnalyticsFilterState>
}

export function getPresets(): FilterPreset[] {
  try {
    return JSON.parse(localStorage.getItem(PRESET_KEY) || '[]')
  } catch {
    return []
  }
}

export function savePreset(preset: FilterPreset) {
  const presets = getPresets()
  presets.push(preset)
  localStorage.setItem(PRESET_KEY, JSON.stringify(presets))
}

export function deletePreset(name: string) {
  const presets = getPresets().filter(p => p.name !== name)
  localStorage.setItem(PRESET_KEY, JSON.stringify(presets))
}

// Explizites Mapping camelCase → URL-Parameter (NICHT regex-basiert!)
const PARAM_MAP: Record<string, string> = {
  from: 'from', to: 'to', sportTypes: 'sport_type', tags: 'tags',
  weather: 'weather', tempMin: 'temp_min', tempMax: 'temp_max',
  timeOfDay: 'time_of_day', intensity: 'intensity',
  distMin: 'dist_min', distMax: 'dist_max', elevMin: 'elev_min', elevMax: 'elev_max',
  gearId: 'gear_id', isCommute: 'is_commute', isRace: 'is_race',
  paceMin: 'pace_min', paceMax: 'pace_max',
  countries: 'country', seasons: 'season', weekdays: 'weekday',
  hrMin: 'hr_min', hrMax: 'hr_max', powerMin: 'power_min', powerMax: 'power_max',
  includeTags: 'include_tags', excludeTags: 'exclude_tags',
  includeGear: 'include_gear', excludeGear: 'exclude_gear',
}

export function useAnalyticsFilters() {
  const [searchParams, setSearchParams] = useSearchParams()

  const filters = useMemo<AnalyticsFilterState>(() => ({
    from: searchParams.get('from') || '',
    to: searchParams.get('to') || '',
    sportTypes: searchParams.get('sport_type')?.split(',').filter(Boolean) || [],
    tags: searchParams.get('tags')?.split(',').filter(Boolean) || [],
    weather: searchParams.get('weather') || '',
    tempMin: searchParams.get('temp_min') || '',
    tempMax: searchParams.get('temp_max') || '',
    timeOfDay: searchParams.get('time_of_day')?.split(',').filter(Boolean) || [],
    intensity: searchParams.get('intensity')?.split(',').filter(Boolean) || [],
    distMin: searchParams.get('dist_min') || '',
    distMax: searchParams.get('dist_max') || '',
    elevMin: searchParams.get('elev_min') || '',
    elevMax: searchParams.get('elev_max') || '',
    gearId: searchParams.get('gear_id') || '',
    isCommute: searchParams.get('is_commute') || '',
    isRace: searchParams.get('is_race') || '',
    paceMin: searchParams.get('pace_min') || '',
    paceMax: searchParams.get('pace_max') || '',
    countries: searchParams.get('country')?.split(',').filter(Boolean) || [],
    seasons: searchParams.get('season')?.split(',').filter(Boolean) || [],
    weekdays: searchParams.get('weekday')?.split(',').map(Number).filter(n => !isNaN(n)) || [],
    hrMin: searchParams.get('hr_min') || '',
    hrMax: searchParams.get('hr_max') || '',
    powerMin: searchParams.get('power_min') || '',
    powerMax: searchParams.get('power_max') || '',
    includeTags: searchParams.get('include_tags')?.split(',').filter(Boolean) || [],
    excludeTags: searchParams.get('exclude_tags')?.split(',').filter(Boolean) || [],
    includeGear: searchParams.get('include_gear')?.split(',').filter(Boolean) || [],
    excludeGear: searchParams.get('exclude_gear')?.split(',').filter(Boolean) || [],
  }), [searchParams])

  const setFilters = useCallback((updates: Partial<AnalyticsFilterState>) => {
    setSearchParams(prev => {
      const next = new URLSearchParams(prev)
      for (const [key, value] of Object.entries(updates)) {
        const paramKey = PARAM_MAP[key] || key
        if (Array.isArray(value)) {
          if (value.length > 0) next.set(paramKey, value.join(','))
          else next.delete(paramKey)
        } else if (value) {
          next.set(paramKey, String(value))
        } else {
          next.delete(paramKey)
        }
      }
      return next
    }, { replace: true })
  }, [setSearchParams])

  const toQueryString = useCallback(() => {
    const params = new URLSearchParams()
    for (const [key, value] of Object.entries(filters)) {
      const paramKey = PARAM_MAP[key] || key
      if (Array.isArray(value) && value.length > 0) {
        params.set(paramKey, value.join(','))
      } else if (typeof value === 'string' && value) {
        params.set(paramKey, value)
      }
    }
    return params.toString()
  }, [filters])

  return { filters, setFilters, toQueryString }
}
