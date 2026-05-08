import { describe, expect, it } from 'vitest'

import { ruleModifierControlsForTuple, ruleTraitConditionFields, ruleTupleOptions, tupleKey } from '../ruleSportProjection'

describe('rule sport projection', () => {
  it('exposes selectable parent and subtype tuple options', () => {
    const options = ruleTupleOptions()
    const keys = options.map((option) => option.value)

    expect(keys).toContain('cycling')
    expect(keys).toContain('cycling:mountain')
    expect(keys).toContain('hiking:rucking')
    expect(keys).toContain('diving:dynamic_apnea')
    expect(keys).toContain('mobility')
    expect(keys).toContain('archery')
	expect(keys).toContain('shooting')
	expect(keys).toContain('geocaching')
	expect(keys).toContain('running:trail')
	expect(keys).toContain('unknown')
	expect(keys).not.toContain('training')
  })

  it('uses selectable tuple labels from the generated taxonomy projection', () => {
    const options = ruleTupleOptions()

    expect(options.find((option) => option.value === 'cycling')?.label).toBe('Radfahren')
    expect(options.find((option) => option.value === 'cycling:mountain')?.label).toBe('MTB')
    expect(options.find((option) => option.value === 'hiking:rucking')?.label).toBe('Rucking')
    expect(options.find((option) => option.value === 'diving:dynamic_apnea')?.label).toBe('Dynamische Apnoe')
    expect(options.find((option) => option.value === 'mobility')?.label).toBe('Mobilität')
    expect(options.find((option) => option.value === 'archery')?.label).toBe('Bogenschießen')
    expect(options.find((option) => option.value === 'shooting')?.label).toBe('Schießen')
    expect(options.find((option) => option.value === 'geocaching')?.label).toBe('Geocaching')
    expect(options.find((option) => option.value === 'running:trail')?.label).toBe('Trail')
  })

  it('exposes modifier applicability for new taxonomy tuples', () => {
    expect(ruleModifierControlsForTuple('mobility').find((modifier) => modifier.key === 'is_indoor')?.actionApplicable).toBe(true)
    expect(ruleModifierControlsForTuple('diving', 'dynamic_apnea').find((modifier) => modifier.key === 'is_indoor')?.actionApplicable).toBe(true)
    expect(ruleModifierControlsForTuple('archery').find((modifier) => modifier.key === 'is_indoor')?.actionApplicable).toBe(false)
    expect(ruleModifierControlsForTuple('shooting').find((modifier) => modifier.key === 'is_electric')?.conditionApplicable).toBe(false)
  })

  it('exposes cycling electric applicability and running non-electric applicability', () => {
    const cyclingElectric = ruleModifierControlsForTuple('cycling', 'mountain').find((modifier) => modifier.key === 'is_electric')
    const runningElectric = ruleModifierControlsForTuple('running', 'trail').find((modifier) => modifier.key === 'is_electric')

    expect(cyclingElectric?.label).toBe('E-Antrieb')
    expect(cyclingElectric?.conditionApplicable).toBe(true)
    expect(cyclingElectric?.actionApplicable).toBe(true)
    expect(runningElectric?.conditionApplicable).toBe(false)
    expect(runningElectric?.actionApplicable).toBe(false)
  })

  it('returns copies of generated projection arrays and objects', () => {
    const firstOptions = ruleTupleOptions()
    firstOptions[0].label = 'Mutated'
    expect(ruleTupleOptions()[0].label).not.toBe('Mutated')

    const firstModifiers = ruleModifierControlsForTuple('cycling', 'mountain')
    firstModifiers[0].label = 'Mutated'
    expect(ruleModifierControlsForTuple('cycling', 'mountain')[0].label).not.toBe('Mutated')
  })

  it('uses stable tuple keys', () => {
    expect(tupleKey('cycling')).toBe('cycling')
    expect(tupleKey('cycling', 'mountain')).toBe('cycling:mountain')
  })

  it('exposes generated trait condition fields and vocabularies', () => {
    const fields = ruleTraitConditionFields()

    expect(fields.find((field) => field.field === 'sport_environment')?.options.map((option) => option.value)).toContain('water')
    expect(fields.find((field) => field.field === 'sport_equipment_family')?.options.map((option) => option.value)).toContain('board')
    expect(fields.find((field) => field.field === 'sport_equipment_family')?.options.map((option) => option.value)).toContain('weapon')
    expect(fields.find((field) => field.field === 'sport_equipment_family')?.options.map((option) => option.value)).toContain('weights')
    expect(fields.find((field) => field.field === 'sport_movement_family')?.options.map((option) => option.value)).toContain('paddle')

    fields[0].options[0].label = 'Mutated'
    expect(ruleTraitConditionFields()[0].options[0].label).not.toBe('Mutated')
  })
})
