import { GENERATED_RULE_MODIFIER_PROJECTIONS, GENERATED_RULE_TRAIT_PROJECTIONS, GENERATED_RULE_TUPLE_OPTIONS } from './generatedRuleProjections'
import { localizeString, type LocalizedString } from '@/i18n/localizedString'
import type { SupportedLanguage } from '@/i18n/languages'

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
  labels: LocalizedString
  conditionApplicable: boolean
  actionApplicable: boolean
  renderExisting: boolean
}

export interface RuleTraitConditionField {
  key: string
  field: string
  label: string
  labels: LocalizedString
  operators: string[]
  options: Array<{ value: string; label: string; labels: LocalizedString }>
}

export function ruleTupleOptions(language?: SupportedLanguage): Array<{ value: string; sportType: string; subType: string; label: string; labels: LocalizedString }> {
  return GENERATED_RULE_TUPLE_OPTIONS.map((option) => {
    const subType = 'subType' in option ? option.subType : ''
    const labels = { ...option.labels }
    return {
      value: tupleKey(option.sportType, subType),
      sportType: option.sportType,
      subType,
      label: localizeString(labels, language),
      labels,
    }
  })
}

export function ruleModifierControlsForTuple(sportType: string, subType = '', language?: SupportedLanguage): RuleModifierControl[] {
  return [...(GENERATED_RULE_MODIFIER_PROJECTIONS[tupleKey(sportType, subType) as keyof typeof GENERATED_RULE_MODIFIER_PROJECTIONS] ?? [])]
    .map((modifier) => {
      const labels = { ...modifier.labels }
      return { ...modifier, labels, label: localizeString(labels, language) }
    })
}

export function ruleTraitConditionFields(language?: SupportedLanguage): RuleTraitConditionField[] {
  return GENERATED_RULE_TRAIT_PROJECTIONS.map((field) => ({
    key: field.key,
    field: field.field,
    label: localizeString(field.labels, language),
    labels: { ...field.labels },
    operators: [...field.operators],
    options: field.options.map((option) => {
      const labels = { ...option.labels }
      return { ...option, labels, label: localizeString(labels, language) }
    }),
  }))
}

export function tupleKey(sportType: string, subType = ''): string {
  return subType ? `${sportType}:${subType}` : sportType
}
