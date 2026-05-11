import { USAGE_METRIC_LABELS } from './gearUsageMetrics'

export const GEAR_RULE_CONDITION_FIELDS = [
  // Options are populated dynamically in RuleEditor via the gear-list query resolver
  { field: 'gear_id', label: 'Ausrüstung', inputType: 'enum' as const, options: [], operators: ['eq', 'neq', 'is_null', 'is_not_null'] },
  { field: 'gear_total_distance_m', label: `${USAGE_METRIC_LABELS.distance_m} gesamt`, inputType: 'number' as const, unit: 'm' },
  { field: 'gear_total_moving_time_s', label: `${USAGE_METRIC_LABELS.moving_time_s} gesamt`, inputType: 'number' as const, unit: 's' },
  { field: 'gear_total_time_s', label: `${USAGE_METRIC_LABELS.elapsed_time_s} gesamt`, inputType: 'number' as const, unit: 's' },
  { field: 'gear_total_elevation_gain_m', label: `${USAGE_METRIC_LABELS.elevation_gain_m} gesamt`, inputType: 'number' as const, unit: 'm' },
  { field: 'gear_total_elevation_loss_m', label: `${USAGE_METRIC_LABELS.elevation_loss_m} gesamt`, inputType: 'number' as const, unit: 'm' },
  { field: 'gear_total_activities', label: `${USAGE_METRIC_LABELS.activity_count} gesamt`, inputType: 'number' as const },
]

export const GEAR_RULE_CONDITION_CATEGORY = {
  key: 'gear',
  label: 'Ausrüstung',
  fields: GEAR_RULE_CONDITION_FIELDS,
}
