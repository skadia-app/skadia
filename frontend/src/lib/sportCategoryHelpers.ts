import { sportsInCategory } from '@/lib/sportTaxonomy'
import { type SegType } from '@/components/segment-editor/constants'
import { segmentEligibleSelectionKeys } from '@/lib/segmentProjection'
import { GENERATED_ACTIVITY_FILTER_PRESETS } from '@/lib/generatedSportFilterProjections'

export interface UISportCategory {
  key: string
  label: string
  taxonomyKeys: string[]
  selectionKeys?: string[]
}

const activityAllPreset = GENERATED_ACTIVITY_FILTER_PRESETS.find(preset => preset.key === 'activity_all')

const GENERATED_WATER_UI_CATEGORIES: UISportCategory[] =
  activityAllPreset?.groups
    .filter(group => group.key.startsWith('water_'))
    .map(group => ({
      key: group.key,
      label: group.label,
      taxonomyKeys: ['water'],
      selectionKeys: Array.from(new Set(group.leaves.map(leaf => {
        const subType = 'subType' in leaf ? leaf.subType : undefined
        return subType ? `${leaf.sportType}:${subType}` : leaf.sportType
      }))),
    })) ?? []

export const UI_SPORT_CATEGORIES: UISportCategory[] = [
  { key: 'run',          label: 'Laufen & Wandern', taxonomyKeys: ['run'] },
  { key: 'bike',         label: 'Rad',              taxonomyKeys: ['bike'] },
  { key: 'winter',       label: 'Wintersport',      taxonomyKeys: ['winter'] },
  ...GENERATED_WATER_UI_CATEGORIES,
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
  let types = cat.selectionKeys ?? cat.taxonomyKeys.flatMap(tk => sportsInCategory(tk).map(s => s.sportType))
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

export function whitelistForSegmentType(segType: SegType): readonly string[] | undefined {
  if (segType === 'wintersport' || segType === 'wassersport') {
    return segmentEligibleSelectionKeys(segType)
  }
  return undefined
}
