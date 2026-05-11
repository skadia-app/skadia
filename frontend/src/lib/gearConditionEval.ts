// Pure condition-evaluation helpers for schema-driven visible_when /
// applies_to_gear_when logic. No React dependencies.

import type { Attribute, Condition } from './gearSchema'

export interface EvalContext {
  // Same-scope attributes (the attribute set the condition is attached to)
  self: Record<string, unknown>
  // Cross-item context — gear-level + (optionally) component-level attributes
  gear?: Record<string, unknown>
  component?: Record<string, unknown>
}

// Resolves a ref string to the appropriate value from ctx.
// Namespace rules:
//   "gear.<rest>"      → strip prefix, look in ctx.gear
//   "component.<rest>" → strip prefix, look in ctx.component
//   bare name          → look in ctx.self
function resolveRef(ref: string, ctx: EvalContext): unknown {
  if (ref.startsWith('gear.')) {
    return ctx.gear?.[ref.slice('gear.'.length)]
  }
  if (ref.startsWith('component.')) {
    return ctx.component?.[ref.slice('component.'.length)]
  }
  return ctx.self[ref]
}

// isSet returns true when v is non-null, non-undefined, non-empty-string,
// non-empty-array.
function isSet(v: unknown): boolean {
  if (v === null || v === undefined) return false
  if (typeof v === 'string') return v !== ''
  if (Array.isArray(v)) return v.length > 0
  return true
}

export function evaluateCondition(condition: Condition, ctx: EvalContext): boolean {
  const v = resolveRef(condition.ref, ctx)
  switch (condition.op) {
    case 'eq':
      // Use loose-ish coercion: compare as strings if types differ (schema
      // stores booleans/numbers in JSON but form state may be string).
      return String(v ?? '') === String(condition.value ?? '')
    case 'neq':
      return String(v ?? '') !== String(condition.value ?? '')
    case 'in': {
      const vals = condition.values ?? []
      const sv = String(v ?? '')
      return vals.some(cv => String(cv) === sv)
    }
    case 'not_in': {
      const vals = condition.values ?? []
      const sv = String(v ?? '')
      return !vals.some(cv => String(cv) === sv)
    }
    case 'is_set':
      return isSet(v)
    case 'is_unset':
      return !isSet(v)
    default:
      // Unknown op: treat as always-visible to avoid hiding fields on schema
      // extensions this client doesn't understand yet.
      return true
  }
}

// evaluateAllAnd returns true when conditions is undefined or empty (no
// constraint = always visible). All provided entries must pass (AND semantics).
export function evaluateAllAnd(
  conditions: Condition[] | undefined,
  ctx: EvalContext,
): boolean {
  if (!conditions || conditions.length === 0) return true
  return conditions.every(c => evaluateCondition(c, ctx))
}

// isAttributeVisible evaluates whether an attribute should be shown given the
// current eval context.
//
// mode = 'same_item' (default): only visibleWhen is checked.
// mode = 'cross_item' (component rendering): BOTH visibleWhen (against self)
//   AND appliesToGearWhen (against gear) are checked.
export function isAttributeVisible(
  attr: Attribute,
  ctx: EvalContext,
  opts?: { mode?: 'same_item' | 'cross_item' },
): boolean {
  const mode = opts?.mode ?? 'same_item'

  // visibleWhen is always evaluated against ctx.self (same-scope siblings)
  if (!evaluateAllAnd(attr.visibleWhen, ctx)) return false

  // appliesToGearWhen is evaluated in cross_item mode only
  if (mode === 'cross_item') {
    const gearCtx: EvalContext = { self: ctx.gear ?? {}, gear: ctx.gear, component: ctx.component }
    if (!evaluateAllAnd(attr.appliesToGearWhen, gearCtx)) return false
  }

  return true
}
