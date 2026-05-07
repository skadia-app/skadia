import {
  GENERATED_SPORT_MODIFIER_KEYS,
  GENERATED_SPORT_MODIFIERS,
  type SportModifierKey,
  type SportModifierTSField,
} from './generatedSportModifiers'

export type { SportModifierKey, SportModifierTSField }

export type SportModifierCapability = keyof (typeof GENERATED_SPORT_MODIFIERS)[number]['capabilities']
export type SportModifierState = Partial<Record<SportModifierTSField, boolean>>

export const SPORT_MODIFIER_KEYS = [...GENERATED_SPORT_MODIFIER_KEYS] as SportModifierKey[]

export function modifierLabel(key: SportModifierKey): string {
  return requireModifier(key).label
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
  return requireModifier(key).actionName
}

export function modifierActionParam(key: SportModifierKey): string {
  return requireModifier(key).actionParam
}

export function modifierTSField(key: SportModifierKey): SportModifierTSField {
  return requireModifier(key).tsField
}

export function modifierJSONKey(key: SportModifierKey): string {
  return requireModifier(key).jsonKey
}

export function modifierFromTSField(field: string): SportModifierKey | null {
  return modifierByTSField.get(field) ?? null
}

export function sportModifierSuffix(state: SportModifierState): string {
  return modifierKeysForCapability('activity_state')
    .filter((key) => Boolean(state[modifierTSField(key)]))
    .map((key) => modifierLabel(key).slice(0, 1).toUpperCase())
    .join('/')
}

export function modifierByKey(key: SportModifierKey) {
  return modifierByKeyMap.get(key)
}

const modifierByKeyMap = new Map(GENERATED_SPORT_MODIFIERS.map((modifier) => [modifier.key, modifier]))
const modifierByTSField = new Map<string, SportModifierKey>(GENERATED_SPORT_MODIFIERS.map((modifier) => [modifier.tsField, modifier.key]))

function requireModifier(key: SportModifierKey) {
  const modifier = modifierByKey(key)
  if (!modifier) throw new Error(`Unknown sport modifier ${key}`)
  return modifier
}
