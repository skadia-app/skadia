import type { ExpressionSpecification } from 'maplibre-gl'

import { SPORT_DEFS, categoryForSport, sportColor } from '@/lib/sportTaxonomy'

export interface MapSportColorConfig {
  mode: 'uniform' | 'per-type'
  uniformColor: string
  colors: Record<string, string>
}

type GlobalSportColors = Record<string, string>

function tupleColor(
  config: MapSportColorConfig,
  globalColors: GlobalSportColors,
  sportType: string,
): string {
  const catKey = categoryForSport(sportType).key
  return (
    globalColors[sportType] ??
    config.colors[sportType] ??
    config.colors[catKey] ??
    sportColor(sportType)
  )
}

export function buildMapLineColor(
  config: MapSportColorConfig,
  globalColors: GlobalSportColors,
): string | ExpressionSpecification {
  if (config.mode === 'uniform') {
    return config.uniformColor
  }

  const entries: string[] = []
  for (const sport of SPORT_DEFS) {
    entries.push(sport.sportType, tupleColor(config, globalColors, sport.sportType))
    for (const subType of sport.subTypes) {
      entries.push(
        `${sport.sportType}:${subType.subType}`,
        tupleColor(config, globalColors, sport.sportType),
      )
    }
  }

  const tupleKeyExpression: ExpressionSpecification = [
    'case',
    ['all', ['has', 'sub_type'], ['!=', ['get', 'sub_type'], null], ['!=', ['get', 'sub_type'], '']],
    ['concat', ['get', 'sport_type'], ':', ['get', 'sub_type']],
    ['get', 'sport_type'],
  ] as ExpressionSpecification

  return [
    'match',
    tupleKeyExpression,
    ...entries,
    config.uniformColor,
  ] as ExpressionSpecification
}
