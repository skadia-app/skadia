// Type aliases for gear category scopes and option shapes derived from the
// generated schema. These are consumed by GearForm, GearPage, and GearDetailPage.

import type { GearType, Attribute, OptionValue } from './generatedGearSchema'

export type { GearType, Attribute, OptionValue }

export type GearCategoryScope =
  | 'bike'
  | 'shoes'
  | 'winter'
  | 'water'
  | 'diving'
  | 'climb'
  | 'apparel'
  | 'other'

export const GEAR_CATEGORY_SCOPES: GearCategoryScope[] = [
  'bike',
  'shoes',
  'winter',
  'water',
  'diving',
  'climb',
  'apparel',
  'other',
]

export const GEAR_CATEGORY_LABELS: Record<GearCategoryScope, string> = {
  bike: 'Räder',
  shoes: 'Schuhe',
  winter: 'Wintersport',
  water: 'Wassersport',
  diving: 'Tauchen',
  climb: 'Klettern',
  apparel: 'Bekleidung',
  other: 'Sonstiges',
}

export interface GearTypeOption {
  value: string
  label: string
}

export function isGearCategoryScope(value: string | null | undefined): value is GearCategoryScope {
  return GEAR_CATEGORY_SCOPES.includes(value as GearCategoryScope)
}
