import { modifierKeysForCapability, modifierLabel, modifierTSField, type SportModifierKey } from './sportModifiers'

export interface ActivityModifierLabelState {
  isElectric?: boolean
  isIndoor?: boolean
  isVirtual?: boolean
}

export function activityModifierLabels(state: ActivityModifierLabelState): string[] {
  return modifierKeysForCapability('activity_state')
    .filter((key) => key !== 'is_commute')
    .filter((key) => Boolean(state[modifierTSField(key) as keyof ActivityModifierLabelState]))
    .map((key) => modifierLabel(key as SportModifierKey))
}
