export const OSM_CATEGORY_LABELS: Record<string, string> = {
  summit:        'Gipfel',
  pass:          'Pässe',
  settlement:    'Siedlungen',
  gastronomy:    'Gastronomie',
  accommodation: 'Unterkünfte',
  shelter:       'Schutzhütten',
  water_supply:  'Wasserversorgung',
  infrastructure:'Infrastruktur',
  parking:       'Parkplätze',
  barrier:       'Sperren',
  transport:     'Transport',
  aerodrome:     'Flugplätze',
  viewpoint:     'Aussichtspunkte',
  shop:          'Läden',
  sport:         'Sport',
  shooting_range:'Schießstände',
  wind_turbine:  'Windkraftanlagen',
  camping:       'Campingplätze',
  information:   'Informationsstellen',
  landmark:      'Sehenswürdigkeiten',
  religious:     'Religiöse Stätten',
}

export function osmCategoryLabel(key: string): string {
  return OSM_CATEGORY_LABELS[key] ?? key.replace(/_/g, ' ')
}
