import { afterEach, describe, expect, it } from 'vitest'
import { i18n } from '@/i18n/i18n'
import { OSM_CATEGORY_LABELS, osmCategoryLabel } from './osmCategoryLabels'

describe('osmCategoryLabels', () => {
  afterEach(async () => {
    await i18n.changeLanguage('de')
  })

  it('localizes display labels and hides unknown OSM keys behind a generic label', async () => {
    await i18n.changeLanguage('en')
    expect(OSM_CATEGORY_LABELS.shooting_range).toBe('Shooting ranges')
    expect(osmCategoryLabel('custom_osm_key')).toBe('Unknown category')

    await i18n.changeLanguage('fr')
    expect(osmCategoryLabel('shooting_range')).toBe('Stands de tir')
    expect(osmCategoryLabel('custom_osm_key')).toBe('Catégorie inconnue')
  })
})
