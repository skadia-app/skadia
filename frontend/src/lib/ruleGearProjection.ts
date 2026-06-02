export const GEAR_RULE_CONDITION_FIELDS = [
  // Options are populated dynamically in RuleEditor via the gear-list query resolver
  { field: 'gear_id', label: 'Gear', inputType: 'enum' as const, options: [], operators: ['eq', 'neq', 'is_null', 'is_not_null'] },
  { field: 'gear_total_distance_m', label: 'Total distance', inputType: 'number' as const, unit: 'distance' as const },
  { field: 'gear_total_moving_time_s', label: 'Total moving time', inputType: 'number' as const, unit: 'seconds' as const },
  { field: 'gear_total_time_s', label: 'Total elapsed time', inputType: 'number' as const, unit: 'seconds' as const },
  { field: 'gear_total_elevation_gain_m', label: 'Total elevation gain', inputType: 'number' as const, unit: 'length' as const },
  { field: 'gear_total_elevation_loss_m', label: 'Total elevation loss', inputType: 'number' as const, unit: 'length' as const },
  { field: 'gear_total_activities', label: 'Total activities', inputType: 'number' as const },
]

export const GEAR_RULE_CONDITION_CATEGORY = {
  key: 'gear',
  label: 'Gear',
  fields: GEAR_RULE_CONDITION_FIELDS,
}
