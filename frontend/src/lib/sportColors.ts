/** Sport-specific colors for charts/maps. Category-based sportColor() is NOT suitable here
 *  because it maps hiking+running to the same color (both "run" category). */
export const SPORT_COLORS: Record<string, string> = {
  cycling:               '#f97316',
  running:               '#22c55e',
  swimming:              '#3b82f6',
  open_water_swimming:   '#06b6d4',
  hiking:                '#84cc16',
  walking:               '#eab308',
  trail_running:         '#10b981',
  mountain_biking:       '#a3e635',
  transition:            '#a855f7',
  rock_climbing:         '#ef4444',
  skiing:                '#60a5fa',
  cross_country_skiing:  '#93c5fd',
  paddling:              '#0ea5e9',
  stand_up_paddleboarding: '#38bdf8',
  other:                 '#9ca3af',
}

export function sportChartColor(sport: string): string {
  return SPORT_COLORS[sport] ?? '#9ca3af'
}
