import {
  GENERATED_SPORT_MODIFIER_KEYS,
  GENERATED_SPORT_MODIFIERS,
  type SportModifierKey,
} from './generatedSportModifiers'

export type { SportModifierKey }

export type SportModifierCapability = keyof (typeof GENERATED_SPORT_MODIFIERS)[number]['capabilities']

export const SPORT_MODIFIER_KEYS = [...GENERATED_SPORT_MODIFIER_KEYS] as SportModifierKey[]

export function modifierLabel(key: SportModifierKey): string {
  return modifierByKey(key)?.label ?? key
}

export function modifierKeysForCapability(capability: SportModifierCapability): SportModifierKey[] {
  return GENERATED_SPORT_MODIFIERS
    .filter((modifier) => modifier.capabilities[capability])
    .map((modifier) => modifier.key)
}

export function modifierTagSync(key: SportModifierKey) {
  return modifierByKey(key)?.tagSync
}

export function modifierTagSyncOnlySportType(key: SportModifierKey): string | null {
  const sync = modifierTagSync(key)
  if (!sync || !('onlySportType' in sync)) return null
  return sync.onlySportType
}

export function modifierActionName(key: SportModifierKey): string {
  return modifierByKey(key)?.actionName ?? key
}

export function modifierActionParam(key: SportModifierKey): string {
  return modifierByKey(key)?.actionParam ?? key
}

export function modifierTSField(key: SportModifierKey): string {
  return modifierByKey(key)?.tsField ?? key
}

export function modifierFromTSField(field: string): SportModifierKey | null {
  return modifierByTSField.get(field) ?? null
}

export function sportModifierSuffix(state: { isElectric?: boolean; isIndoor?: boolean; isVirtual?: boolean; isCommute?: boolean }): string {
  return modifierKeysForCapability('activity_state')
    .filter((key) => Boolean(state[modifierTSField(key) as keyof typeof state]))
    .map((key) => modifierLabel(key).slice(0, 1).toUpperCase())
    .join('/')
}

export function modifierByKey(key: SportModifierKey) {
  return modifierByKeyMap.get(key)
}

const modifierByKeyMap = new Map(GENERATED_SPORT_MODIFIERS.map((modifier) => [modifier.key, modifier]))
const modifierByTSField = new Map<string, SportModifierKey>(GENERATED_SPORT_MODIFIERS.map((modifier) => [modifier.tsField, modifier.key]))
