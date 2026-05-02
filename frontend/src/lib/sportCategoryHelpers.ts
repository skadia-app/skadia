import { WATER_SPORT_UI_GROUPS, getSegmentGroup, sportsInCategory, type SportDef } from '@/lib/sportTaxonomy'
import { type SegType } from '@/components/segment-editor/constants'
import { segmentEligibleSelectionKeys } from '@/lib/segmentProjection'

export interface UISportCategory {
  key: string
  label: string
  taxonomyKeys: string[]
  segmentGroups?: SportDef['segmentGroup'][]
}

export const UI_SPORT_CATEGORIES: UISportCategory[] = [
  { key: 'run',          label: 'Laufen & Wandern', taxonomyKeys: ['run'] },
  { key: 'bike',         label: 'Rad',              taxonomyKeys: ['bike'] },
  { key: 'winter',       label: 'Wintersport',      taxonomyKeys: ['winter'] },
  ...WATER_SPORT_UI_GROUPS.map(group => ({
    key: group.key,
    label: group.label,
    taxonomyKeys: ['water'],
    segmentGroups: group.segmentGroups,
  })),
  { key: 'sonstige',     label: 'Sonstige',         taxonomyKeys: ['fitness', 'game', 'climb', 'outdoor', 'multi', 'other'] },
]

export function getVisibleCategories(segType: SegType) {
  if (segType === 'wintersport') return UI_SPORT_CATEGORIES.filter(c => c.key === 'winter')
  if (segType === 'wassersport') return UI_SPORT_CATEGORIES.filter(c => c.key.startsWith('water_'))
  return UI_SPORT_CATEGORIES.filter(c => !c.key.startsWith('water_') && c.key !== 'winter')
}

export function expandCategoryToSportTypes(uiKey: string, whitelist?: readonly string[]): string[] {
  const cat = UI_SPORT_CATEGORIES.find(c => c.key === uiKey)
  if (!cat) return []
  let types = cat.taxonomyKeys.flatMap(tk => sportsInCategory(tk).map(s => s.sportType))
  if (cat.segmentGroups) {
    types = types.filter(t => cat.segmentGroups?.includes(getSegmentGroup(t)))
  }
  if (whitelist) {
    types = types.filter(t => whitelist.includes(t))
  }
  return types
}

export function isCategoryFullyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.length > 0 && types.every(t => active.includes(t))
}

export function isCategoryPartiallyActive(uiKey: string, active: string[] | null, whitelist?: readonly string[]): boolean {
  if (active === null) return false
  const types = expandCategoryToSportTypes(uiKey, whitelist)
  return types.some(t => active.includes(t)) && !types.every(t => active.includes(t))
}

export function whitelistForSegmentType(segType: SegType): readonly string[] {
  return segmentEligibleSelectionKeys(segType)
}
