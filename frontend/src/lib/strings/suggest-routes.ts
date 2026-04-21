// Centralised UI strings for the route suggestion feature.
// i18n rule: reused strings in central constants for future i18n sprint.

export const suggestRoutesStrings = {
  variants: {
    shortest:  { label: 'Die kürzeste' },
    medium:    { label: 'Die mittlere' },
    longest:   { label: 'Die längste' },
    only:      { label: 'Die einzige' },
  },
  descriptors: {
    high_ascent:      'Mehr Anstieg',
    low_ascent:       'Weniger Anstieg',
    high_forest:      'Mehr Wald',
    high_water:       'Mehr Gewässer',
    high_protected:   'Mehr Schutzgebiet',
    low_traffic_noise:'Weniger Verkehr',
    target_distance:  'Passt zur Wunschdistanz',
    mtb_technical:    'Schwere MTB-Passagen',
    alpine_hiking:    'Alpine Passagen',
    exposed:          'Klettersteig-Abschnitt',
    low_visibility:   'Pfad schwer findbar',
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
  stats: {
    vfUnknownLabel: '⚠️ Klettersteig K?',
    vfUnknownTitle: 'Klettersteig auf der Route, Schwierigkeit nicht in OSM bestätigt — Vorsicht beim Begehen',
    vfUnknownAriaLabel: 'Klettersteig ohne bestätigte Schwierigkeit auf der Route',
  },
  scale: {
    kindLabel: {
      sac: 'Schweizer Alpen-Club-Skala',
      mtb: 'Mountainbike-Skala',
      vf: 'Klettersteig-Skala',
    } as Record<'sac' | 'mtb' | 'vf', string>,
    breakdownOpenHint: 'Breakdown öffnen',
  },
} as const

export type VariantID = keyof typeof suggestRoutesStrings.variants
export type DescriptorID = keyof typeof suggestRoutesStrings.descriptors
export type LoopQualityID = keyof typeof suggestRoutesStrings.loopQuality
export type ScaleKind = 'sac' | 'mtb' | 'vf'

export function variantLabel(id: string): string {
  // Strip numeric suffix for medium-2, medium-3 etc. → use base key
  const baseKey = id.replace(/-\d+$/, '')
  const v = suggestRoutesStrings.variants[baseKey as VariantID]
    ?? suggestRoutesStrings.variants[id as VariantID]
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

// ── Stats helpers ─────────────────────────────────────────────────────────────

/** "🌲 42 %" */
export function statsForestLabel(pct: number): string {
  return `🌲 ${pct} %`
}

/** "🛡️ 42 %" */
export function statsProtectedLabel(pct: number): string {
  return `🛡️ ${pct} %`
}

/** "⛰ 3 Gipfel" — TODO i18n: Pluralisierung bei echtem i18n-Sprint */
export function statsSummitsLabel(n: number): string {
  return `⛰ ${n} Gipfel`
}

// ── Scale helpers ─────────────────────────────────────────────────────────────

/** "bis T3" */
export function scaleMaxLabel(level: string): string {
  return `bis ${level}`
}

/** Expanded aria-label: "Schweizer Alpen-Club-Skala, bis T3, Breakdown öffnen" */
export function scaleAriaLabel(kind: ScaleKind, maxLevel: string): string {
  const kindName = suggestRoutesStrings.scale.kindLabel[kind]
  return `${kindName}, bis ${maxLevel}, ${suggestRoutesStrings.scale.breakdownOpenHint}`
}
