import React from 'react'
import { Check } from 'lucide-react'
import { Pill } from './Pill'
import { sportsInCategory, getSegmentGroup } from '@/lib/sportTaxonomy'
import { WINTER_SPORT_WHITELIST, WATER_SPORT_WHITELIST, type SegType } from './constants'

interface SportCategoryChipsProps {
  segType: SegType
  activeTypes: string[] | null
  onToggleCategory: (uiKey: string) => void
  onSetAll: () => void
}

const UI_SPORT_CATEGORIES = [
  { key: 'run',          label: 'Run & Hike',       taxonomyKeys: ['run'] },
  { key: 'bike',         label: 'Bike',             taxonomyKeys: ['bike'] },
  { key: 'winter',       label: 'Winter Sports',    taxonomyKeys: ['winter'] },
  { key: 'water_swim',   label: 'Swimming',         taxonomyKeys: ['water'], segmentGroups: ['swim'] },
  { key: 'water_paddle', label: 'Paddling',         taxonomyKeys: ['water'], segmentGroups: ['paddle'] },
  { key: 'water_surf',   label: 'Surfing & Sailing', taxonomyKeys: ['water'], segmentGroups: ['generic'] },
  { key: 'sonstige',     label: 'Other',            taxonomyKeys: ['fitness', 'game', 'climb', 'outdoor', 'multi', 'other'] },
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
    types = types.filter(t => (cat.segmentGroups as string[]).includes(getSegmentGroup(t)))
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

export function SportCategoryChips({ segType, activeTypes, onToggleCategory, onSetAll }: SportCategoryChipsProps) {
  const visible = getVisibleCategories(segType)
  const whitelist = segType === 'wintersport' ? WINTER_SPORT_WHITELIST : (segType === 'wassersport' ? WATER_SPORT_WHITELIST : undefined)

  return (
    <div className="flex flex-wrap gap-1.5" role="group" aria-label="Sportarten-Filter">
      <Pill
        role="checkbox"
        ariaChecked={activeTypes === null}
        active={activeTypes === null}
        onClick={onSetAll}
        leftSlot={activeTypes === null ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
      >
        All
      </Pill>

      {visible.map(cat => {
        const isActive = isCategoryFullyActive(cat.key, activeTypes, whitelist)
        const isPartial = isCategoryPartiallyActive(cat.key, activeTypes, whitelist)
        return (
          <Pill
            key={cat.key}
            role="checkbox"
            ariaChecked={isActive ? true : (isPartial ? 'mixed' : false)}
            active={isActive}
            onClick={() => onToggleCategory(cat.key)}
            leftSlot={isActive ? <Check className="h-3.5 w-3.5 shrink-0" /> : undefined}
            className={isPartial && !isActive ? 'border-accent-DEFAULT/40 bg-accent-DEFAULT/10' : undefined}
          >
            {cat.label}
          </Pill>
        )
      })}
    </div>
  )
}
