// Type aliases for gear category scopes and option shapes derived from the
// generated schema. These are consumed by GearForm, GearPage, and GearDetailPage.

import type { GearType, Attribute, OptionValue } from './generatedGearSchema'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'

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

const GEAR_CATEGORY_LABEL_MAPS: Record<GearCategoryScope, LocalizedString> = {
  bike: { de: 'Räder', en: 'Bikes', fr: 'Vélos', it: 'Bici', es: 'Bicicletas' },
  shoes: { de: 'Schuhe', en: 'Shoes', fr: 'Chaussures', it: 'Scarpe', es: 'Zapatillas' },
  winter: { de: 'Wintersport', en: 'Winter sports', fr: 'Sports d’hiver', it: 'Sport invernali', es: 'Deportes de invierno' },
  water: { de: 'Wassersport', en: 'Water sports', fr: 'Sports nautiques', it: 'Sport acquatici', es: 'Deportes acuáticos' },
  diving: { de: 'Tauchen', en: 'Diving', fr: 'Plongée', it: 'Immersioni', es: 'Buceo' },
  climb: { de: 'Klettern', en: 'Climbing', fr: 'Escalade', it: 'Arrampicata', es: 'Escalada' },
  apparel: { de: 'Bekleidung', en: 'Apparel', fr: 'Vêtements', it: 'Abbigliamento', es: 'Ropa' },
  other: { de: 'Sonstiges', en: 'Other', fr: 'Autre', it: 'Altro', es: 'Otros' },
}

export function gearCategoryLabel(scope: GearCategoryScope, language?: SupportedLanguage): string {
  return localizeString(GEAR_CATEGORY_LABEL_MAPS[scope], language) || GEAR_CATEGORY_LABELS[scope]
}

export interface GearTypeOption {
  value: string
  label: string
}

export function isGearCategoryScope(value: string | null | undefined): value is GearCategoryScope {
  return GEAR_CATEGORY_SCOPES.includes(value as GearCategoryScope)
}
