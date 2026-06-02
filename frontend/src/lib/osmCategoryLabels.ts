import { i18n } from '@/i18n/i18n'

const OSM_CATEGORY_KEYS = [
  'summit',
  'pass',
  'settlement',
  'gastronomy',
  'accommodation',
  'shelter',
  'water_supply',
  'infrastructure',
  'parking',
  'barrier',
  'transport',
  'aerodrome',
  'viewpoint',
  'shop',
  'sport',
  'shooting_range',
  'wind_turbine',
  'camping',
  'information',
  'landmark',
  'religious',
] as const

const OSM_CATEGORY_KEY_SET = new Set<string>(OSM_CATEGORY_KEYS)

export const OSM_CATEGORY_LABELS = Object.defineProperties(
  {} as Record<string, string>,
  Object.fromEntries(
    OSM_CATEGORY_KEYS.map(key => [
      key,
      {
        enumerable: true,
        get: () => i18n.t(`osmCategories.${key}`),
      },
    ]),
  ),
)

export function osmCategoryLabel(key: string): string {
  if (OSM_CATEGORY_KEY_SET.has(key)) {
    return i18n.t(`osmCategories.${key}`)
  }
  return i18n.t('osmCategories.unknown')
}
