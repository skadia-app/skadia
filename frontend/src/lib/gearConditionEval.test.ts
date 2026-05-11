import { describe, it, expect } from 'vitest'
import {
  evaluateCondition,
  evaluateAllAnd,
  isAttributeVisible,
  type EvalContext,
} from './gearConditionEval'
import type { Attribute, Condition } from './gearSchema'

// ── evaluateCondition ─────────────────────────────────────────────────────────

describe('evaluateCondition — op × namespace', () => {
  // ctx used for self-scope tests
  const ctx: EvalContext = {
    self: { color: 'red', count: '3', active: 'true', empty: '' },
    gear: { frame_design: 'full_suspension' },
    component: { material: 'carbon' },
  }

  // -- eq -----------------------------------------------------------------------
  it('eq matches bare ref in self scope', () => {
    const c: Condition = { ref: 'color', op: 'eq', value: 'red' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('eq fails when value differs (self scope)', () => {
    const c: Condition = { ref: 'color', op: 'eq', value: 'blue' }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  it('eq resolves gear. namespace', () => {
    const c: Condition = { ref: 'gear.frame_design', op: 'eq', value: 'full_suspension' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('eq resolves component. namespace', () => {
    const c: Condition = { ref: 'component.material', op: 'eq', value: 'carbon' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  // -- neq ----------------------------------------------------------------------
  it('neq passes when value differs (self scope)', () => {
    const c: Condition = { ref: 'color', op: 'neq', value: 'blue' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('neq fails when value matches (gear scope)', () => {
    const c: Condition = { ref: 'gear.frame_design', op: 'neq', value: 'full_suspension' }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  // -- in -----------------------------------------------------------------------
  it('in passes when value is in list (self scope)', () => {
    const c: Condition = { ref: 'color', op: 'in', values: ['red', 'green'] }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('in fails when value not in list (gear scope)', () => {
    const c: Condition = { ref: 'gear.frame_design', op: 'in', values: ['rigid', 'hardtail'] }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  it('in passes for component. namespace', () => {
    const c: Condition = { ref: 'component.material', op: 'in', values: ['carbon', 'aluminum'] }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  // -- not_in -------------------------------------------------------------------
  it('not_in passes when value is not in list (self scope)', () => {
    const c: Condition = { ref: 'color', op: 'not_in', values: ['blue', 'green'] }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('not_in fails when value is in list (gear scope)', () => {
    const c: Condition = { ref: 'gear.frame_design', op: 'not_in', values: ['full_suspension', 'hardtail'] }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  // -- is_set -------------------------------------------------------------------
  it('is_set true for non-empty string (self scope)', () => {
    const c: Condition = { ref: 'color', op: 'is_set' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('is_set false for empty string (self scope)', () => {
    const c: Condition = { ref: 'empty', op: 'is_set' }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  it('is_set false for missing key in gear scope', () => {
    const c: Condition = { ref: 'gear.nonexistent', op: 'is_set' }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })

  it('is_set true for populated array in component scope', () => {
    const arrCtx: EvalContext = { self: {}, component: { tags: ['a', 'b'] } }
    const c: Condition = { ref: 'component.tags', op: 'is_set' }
    expect(evaluateCondition(c, arrCtx)).toBe(true)
  })

  // -- is_unset -----------------------------------------------------------------
  it('is_unset true for missing key (self scope)', () => {
    const c: Condition = { ref: 'nonexistent', op: 'is_unset' }
    expect(evaluateCondition(c, ctx)).toBe(true)
  })

  it('is_unset false for non-empty string (gear scope)', () => {
    const c: Condition = { ref: 'gear.frame_design', op: 'is_unset' }
    expect(evaluateCondition(c, ctx)).toBe(false)
  })
})

// ── evaluateAllAnd ────────────────────────────────────────────────────────────

describe('evaluateAllAnd', () => {
  const ctx: EvalContext = {
    self: { x: 'a', y: 'b' },
  }

  it('returns true for undefined (no constraint)', () => {
    expect(evaluateAllAnd(undefined, ctx)).toBe(true)
  })

  it('returns true for empty array (no constraint)', () => {
    expect(evaluateAllAnd([], ctx)).toBe(true)
  })

  it('returns true when all conditions pass', () => {
    const conditions: Condition[] = [
      { ref: 'x', op: 'eq', value: 'a' },
      { ref: 'y', op: 'eq', value: 'b' },
    ]
    expect(evaluateAllAnd(conditions, ctx)).toBe(true)
  })

  it('returns false when one condition fails', () => {
    const conditions: Condition[] = [
      { ref: 'x', op: 'eq', value: 'a' },
      { ref: 'y', op: 'eq', value: 'WRONG' },
    ]
    expect(evaluateAllAnd(conditions, ctx)).toBe(false)
  })

  it('returns false when all conditions fail', () => {
    const conditions: Condition[] = [
      { ref: 'x', op: 'eq', value: 'WRONG' },
      { ref: 'y', op: 'eq', value: 'WRONG' },
    ]
    expect(evaluateAllAnd(conditions, ctx)).toBe(false)
  })
})

// ── isAttributeVisible ────────────────────────────────────────────────────────

describe('isAttributeVisible', () => {
  const attrNoConditions: Attribute = {
    key: 'test.attr',
    labelDe: 'Test',
    valueType: 'string',
  }

  const attrWithVisible: Attribute = {
    key: 'test.dependent',
    labelDe: 'Dependent',
    valueType: 'string',
    visibleWhen: [{ ref: 'trigger', op: 'eq', value: 'yes' }],
  }

  const attrWithAppliesToGear: Attribute = {
    key: 'test.gear_dep',
    labelDe: 'GearDep',
    valueType: 'string',
    appliesToGearWhen: [{ ref: 'gear.frame_design', op: 'eq', value: 'full_suspension' }],
  }

  const attrBothConditions: Attribute = {
    key: 'test.both',
    labelDe: 'Both',
    valueType: 'string',
    visibleWhen: [{ ref: 'trigger', op: 'eq', value: 'yes' }],
    appliesToGearWhen: [{ ref: 'gear.frame_design', op: 'eq', value: 'full_suspension' }],
  }

  it('visible when no conditions set', () => {
    const ctx: EvalContext = { self: {} }
    expect(isAttributeVisible(attrNoConditions, ctx)).toBe(true)
  })

  it('same_item mode: visibleWhen evaluated against self', () => {
    const ctx: EvalContext = { self: { trigger: 'yes' } }
    expect(isAttributeVisible(attrWithVisible, ctx, { mode: 'same_item' })).toBe(true)
  })

  it('same_item mode: hides when visibleWhen fails', () => {
    const ctx: EvalContext = { self: { trigger: 'no' } }
    expect(isAttributeVisible(attrWithVisible, ctx, { mode: 'same_item' })).toBe(false)
  })

  it('same_item mode: appliesToGearWhen is NOT evaluated', () => {
    // Even with no gear context, same_item mode ignores appliesToGearWhen
    const ctx: EvalContext = { self: {} }
    expect(isAttributeVisible(attrWithAppliesToGear, ctx, { mode: 'same_item' })).toBe(true)
  })

  it('cross_item mode: both conditions must pass', () => {
    const ctx: EvalContext = {
      self: { trigger: 'yes' },
      gear: { frame_design: 'full_suspension' },
    }
    expect(isAttributeVisible(attrBothConditions, ctx, { mode: 'cross_item' })).toBe(true)
  })

  it('cross_item mode: hidden when appliesToGearWhen fails', () => {
    const ctx: EvalContext = {
      self: { trigger: 'yes' },
      gear: { frame_design: 'rigid' },
    }
    expect(isAttributeVisible(attrBothConditions, ctx, { mode: 'cross_item' })).toBe(false)
  })

  it('cross_item mode: hidden when visibleWhen fails even if appliesToGearWhen passes', () => {
    const ctx: EvalContext = {
      self: { trigger: 'no' },
      gear: { frame_design: 'full_suspension' },
    }
    expect(isAttributeVisible(attrBothConditions, ctx, { mode: 'cross_item' })).toBe(false)
  })

  it('default mode is same_item', () => {
    // No mode specified — default is same_item, appliesToGearWhen ignored
    const ctx: EvalContext = { self: {} }
    expect(isAttributeVisible(attrWithAppliesToGear, ctx)).toBe(true)
  })
})
