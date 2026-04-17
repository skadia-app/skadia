// Centralised UI strings for the route suggestion feature.
// i18n rule: reused strings in central constants for future i18n sprint.

export const suggestRoutesStrings = {
  variants: {
    // v2 keys
    shortest:  { label: 'Die kürzeste' },
    medium:    { label: 'Die mittlere' },
    longest:   { label: 'Die längste' },
    only:      { label: 'Die einzige' },
    // legacy keys — kept for client-cache resilience
    calmest:   { label: 'Die gemütlichste' },
    greenest:  { label: 'Die grünste' },
    hilly:     { label: 'Die anspruchsvolle' },
    balanced:  { label: 'Die ausgewogene' },
    geometric: { label: 'Alternative Richtung' },
  },
  descriptors: {
    high_ascent:      'Mehr Anstieg',
    low_ascent:       'Weniger Anstieg',
    high_forest:      'Mehr Wald',
    high_water:       'Mehr Gewässer',
    high_protected:   'Mehr Schutzgebiet',
    low_traffic_noise:'Weniger Verkehr',
    target_distance:  'Passt zur Wunschdistanz',
  } as Record<string, string>,
  loopQuality: {
    loop:         { label: 'Rundtour', icon: '⟲' },
    partial_loop: { label: 'Rundtour mit Umweg', icon: '⟳' },
    out_and_back: { label: 'Hin & zurück', icon: '⇄' },
  },
  warnings: {
    ascentNotAchieved: {
      title: 'Höhenmeter-Ziel nicht erreichbar',
      message: 'In diesem Gebiet ist der gewünschte Anstieg nicht möglich. Angezeigt wird der nächstmögliche Vorschlag.',
    },
    distanceNotAchieved: {
      title: 'Distanz-Ziel nicht erreichbar',
      message: 'Der gewünschte Distanz-Bereich konnte nicht gefunden werden.',
    },
    partialSearch: {
      title: 'Suchraum begrenzt',
      message: 'Möglicherweise gibt es bessere Routen außerhalb des aktuellen Suchbereichs.',
    },
  },
} as const

export type VariantID = keyof typeof suggestRoutesStrings.variants
export type DescriptorID = keyof typeof suggestRoutesStrings.descriptors
export type LoopQualityID = keyof typeof suggestRoutesStrings.loopQuality

export function variantLabel(id: string): string {
  const v = suggestRoutesStrings.variants[id as VariantID]
  return v ? v.label : id
}

export function descriptorLabel(key: string, displayValue?: string): string {
  // Strip numeric suffix for medium-2, medium-3 etc. → use base key
  const baseKey = key.replace(/-\d+$/, '')
  const base = suggestRoutesStrings.descriptors[baseKey] ?? suggestRoutesStrings.descriptors[key] ?? key
  return displayValue ? `${base} (${displayValue})` : base
}

export function loopQualityDisplay(id: string): { label: string; icon: string } {
  const q = suggestRoutesStrings.loopQuality[id as LoopQualityID]
  return q ?? { label: id, icon: '' }
}
