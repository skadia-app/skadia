// Shared sport-option types and constants used by both MapPage and MapFilters.
// Extracted into a separate file to avoid the circular import that would otherwise
// exist between MapPage (which imports MapFilters) and MapFilters (which needs
// SPORT_OPTIONS / getAllSportKeys from MapPage).
//
// In Vite dev mode, circular ESM imports cause a TDZ ReferenceError because
// `const` bindings in the re-imported module are not yet initialised when the
// dependent module is first evaluated.  Moving shared data here breaks the cycle.

export interface SportOption {
  value: string
  label: string
  /** sub_type filter value (for sub-categories like cycling:mountain) */
  subType?: string
  children?: SportOption[]
  /** stable key for the group used in expand/collapse state */
  groupKey?: string
}

export const SPORT_OPTIONS: SportOption[] = [
  {
    value: 'cycling', label: 'Radfahren', groupKey: 'cycling',
    children: [
      { value: 'cycling', label: 'Straße' },
      { value: 'cycling', label: 'MTB', subType: 'mountain' },
      { value: 'cycling', label: 'Gravel', subType: 'gravel' },
      { value: 'e_biking', label: 'E-Bike' },
    ],
  },
  {
    value: 'foot', label: 'Laufen & Wandern', groupKey: 'foot',
    children: [
      { value: 'running', label: 'Laufen' },
      { value: 'running', label: 'Trail Running', subType: 'trail' },
      { value: 'hiking', label: 'Wandern' },
      { value: 'walking', label: 'Gehen' },
    ],
  },
  {
    value: 'water', label: 'Wassersport', groupKey: 'water',
    children: [
      { value: 'swimming', label: 'Schwimmen' },
      { value: 'paddling', label: 'Paddeln' },
      { value: 'stand_up_paddleboarding', label: 'SUP' },
    ],
  },
  {
    value: 'rock_climbing', label: 'Klettern', groupKey: 'climbing',
    children: [
      { value: 'rock_climbing', label: 'Klettern' },
      { value: 'rock_climbing', label: 'Bouldern', subType: 'bouldering' },
      { value: 'rock_climbing', label: 'Indoor', subType: 'indoor_climbing' },
    ],
  },
  { value: 'skiing', label: 'Ski' },
  { value: 'generic', label: 'Sonstige' },
]

/**
 * Returns the composite filter key for a leaf option.
 * Options without a subType use the plain sport value (e.g. "cycling").
 * Options with a subType use "sport:subType" (e.g. "cycling:mountain").
 */
export function getOptionKey(opt: SportOption): string {
  return opt.subType ? `${opt.value}:${opt.subType}` : opt.value
}

/**
 * Flat list of all selectable leaf keys across the option tree.
 * Used for "all selected" checks and default filter state.
 */
export function getAllSportKeys(): string[] {
  const keys: string[] = []
  for (const opt of SPORT_OPTIONS) {
    if (opt.children) {
      for (const child of opt.children) {
        keys.push(getOptionKey(child))
      }
    } else {
      keys.push(opt.value)
    }
  }
  return keys
}
