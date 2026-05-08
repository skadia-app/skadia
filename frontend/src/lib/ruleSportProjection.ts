import { GENERATED_RULE_MODIFIER_PROJECTIONS, GENERATED_RULE_TRAIT_PROJECTIONS, GENERATED_RULE_TUPLE_OPTIONS } from './generatedRuleProjections'

export interface RuleSportTupleValue {
  sportType: string
  subType?: string
  isElectric?: boolean
  isIndoor?: boolean
  isVirtual?: boolean
  isCommute?: boolean
}

export interface RuleModifierControl {
  key: string
  label: string
  conditionApplicable: boolean
  actionApplicable: boolean
  renderExisting: boolean
}

export interface RuleTraitConditionField {
  key: string
  field: string
  label: string
  operators: string[]
  options: Array<{ value: string; label: string }>
}

export function ruleTupleOptions(): Array<{ value: string; sportType: string; subType: string; label: string }> {
  return GENERATED_RULE_TUPLE_OPTIONS.map((option) => {
    const subType = 'subType' in option ? option.subType : ''
    return {
      value: tupleKey(option.sportType, subType),
      sportType: option.sportType,
      subType,
      label: option.label,
    }
  })
}

export function ruleModifierControlsForTuple(sportType: string, subType = ''): RuleModifierControl[] {
  return [...(GENERATED_RULE_MODIFIER_PROJECTIONS[tupleKey(sportType, subType) as keyof typeof GENERATED_RULE_MODIFIER_PROJECTIONS] ?? [])]
    .map((modifier) => ({ ...modifier }))
}

export function ruleTraitConditionFields(): RuleTraitConditionField[] {
  return GENERATED_RULE_TRAIT_PROJECTIONS.map((field) => ({
    key: field.key,
    field: field.field,
    label: field.label,
    operators: [...field.operators],
    options: field.options.map((option) => ({ ...option })),
  }))
}

export function tupleKey(sportType: string, subType = ''): string {
  return subType ? `${sportType}:${subType}` : sportType
}
