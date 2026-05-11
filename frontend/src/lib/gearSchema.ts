// Thin barrel re-export: static generated schema + lookup helpers.
// A runtime per-user schema override was considered but deferred — see
// STATE.md follow-up. Backend endpoint GET /api/gear/schema exists; wiring
// it requires changes to AuthGuard / LoginPage / main.tsx outside gear scope.
import {
  gearSchema as generated,
  type GearSchema,
  type Attribute,
  type GearType,
  type Condition,
  type ConditionOp,
  type RecommendedAttribute,
  type RecommendedConstraints,
} from './generatedGearSchema'

export type {
  GearSchema,
  Attribute,
  GearType,
  Condition,
  ConditionOp,
  RecommendedAttribute,
  RecommendedConstraints,
}

export function lookupAttribute(key: string): Attribute | undefined {
  return generated.attributes.find(a => a.key === key)
}

export function lookupGearType(key: string): GearType | undefined {
  return generated.gearTypes.find(gt => gt.key === key)
}

export function getActiveSchema(): GearSchema {
  return generated
}
