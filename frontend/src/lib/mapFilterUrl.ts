export interface MapModifierFilterState {
  isElectric: boolean | null
  isCommute: boolean | null
}

export function applyMapModifierParams(params: URLSearchParams, filters: MapModifierFilterState): void {
  if (filters.isElectric !== null) {
    params.set('is_electric', String(filters.isElectric))
  }
  if (filters.isCommute !== null) {
    params.set('is_commute', String(filters.isCommute))
  }
}
