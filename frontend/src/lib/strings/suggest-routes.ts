export type SuggestRoutesTranslator = (key: string, options?: Record<string, unknown>) => string

export const suggestRoutesStrings = {
  variants: {
    shortest: { labelKey: 'routePlanner.comparison.variants.shortest' },
    medium: { labelKey: 'routePlanner.comparison.variants.medium' },
    longest: { labelKey: 'routePlanner.comparison.variants.longest' },
    only: { labelKey: 'routePlanner.comparison.variants.only' },
  },
  descriptors: {
    high_ascent: 'routePlanner.comparison.descriptors.high_ascent',
    low_ascent: 'routePlanner.comparison.descriptors.low_ascent',
    high_forest: 'routePlanner.comparison.descriptors.high_forest',
    high_water: 'routePlanner.comparison.descriptors.high_water',
    high_protected: 'routePlanner.comparison.descriptors.high_protected',
    low_traffic_noise: 'routePlanner.comparison.descriptors.low_traffic_noise',
    target_distance: 'routePlanner.comparison.descriptors.target_distance',
    mtb_technical: 'routePlanner.comparison.descriptors.mtb_technical',
    alpine_hiking: 'routePlanner.comparison.descriptors.alpine_hiking',
    exposed: 'routePlanner.comparison.descriptors.exposed',
    low_visibility: 'routePlanner.comparison.descriptors.low_visibility',
  } as Record<string, string>,
  loopQuality: {
    loop: { labelKey: 'routePlanner.comparison.loopQuality.loop', icon: '⟲' },
    partial_loop: { labelKey: 'routePlanner.comparison.loopQuality.partial_loop', icon: '⟳' },
    out_and_back: { labelKey: 'routePlanner.comparison.loopQuality.out_and_back', icon: '⇄' },
  },
  warnings: {
    ascentNotAchieved: {
      titleKey: 'routePlanner.comparison.warnings.ascentNotAchieved.title',
      messageKey: 'routePlanner.comparison.warnings.ascentNotAchieved.message',
    },
    distanceNotAchieved: {
      titleKey: 'routePlanner.comparison.warnings.distanceNotAchieved.title',
      messageKey: 'routePlanner.comparison.warnings.distanceNotAchieved.message',
    },
    partialSearch: {
      titleKey: 'routePlanner.comparison.warnings.partialSearch.title',
      messageKey: 'routePlanner.comparison.warnings.partialSearch.message',
    },
  },
  stats: {
    vfUnknownLabelKey: 'routePlanner.profileChart.vfUnknownLabel',
    vfUnknownTitleKey: 'routePlanner.profileChart.vfUnknownTitle',
    vfUnknownAriaLabelKey: 'routePlanner.profileChart.vfUnknownAriaLabel',
    summitsSummaryKey: 'routePlanner.summits.summary',
  },
  scale: {
    kindLabel: {
      sac: 'routePlanner.profileChart.scaleKind.sac',
      mtb: 'routePlanner.profileChart.scaleKind.mtb',
      vf: 'routePlanner.profileChart.scaleKind.vf',
    } as Record<'sac' | 'mtb' | 'vf', string>,
    maxKey: 'routePlanner.profileChart.scaleMax',
    ariaKey: 'routePlanner.profileChart.scaleAria',
  },
} as const

export type VariantID = keyof typeof suggestRoutesStrings.variants
export type DescriptorID = keyof typeof suggestRoutesStrings.descriptors
export type LoopQualityID = keyof typeof suggestRoutesStrings.loopQuality
export type ScaleKind = 'sac' | 'mtb' | 'vf'

export function variantLabel(id: string, t?: SuggestRoutesTranslator): string {
  // Strip numeric suffix for medium-2, medium-3 etc. to use the base key.
  const baseKey = id.replace(/-\d+$/, '')
  const v = suggestRoutesStrings.variants[baseKey as VariantID]
    ?? suggestRoutesStrings.variants[id as VariantID]
  return v && t ? t(v.labelKey) : id
}

export function descriptorLabel(key: string, displayValue?: string, t?: SuggestRoutesTranslator): string {
  // Strip numeric suffix for medium-2, medium-3 etc. to use the base key.
  const baseKey = key.replace(/-\d+$/, '')
  const keyOrFallback = suggestRoutesStrings.descriptors[baseKey] ?? suggestRoutesStrings.descriptors[key]
  const base = keyOrFallback && t ? t(keyOrFallback) : key
  return displayValue ? `${base} (${displayValue})` : base
}

export function loopQualityDisplay(id: string, t?: SuggestRoutesTranslator): { label: string; icon: string } {
  const q = suggestRoutesStrings.loopQuality[id as LoopQualityID]
  return q ? { label: t ? t(q.labelKey) : id, icon: q.icon } : { label: id, icon: '' }
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

export function statsSummitsLabel(n: number, t?: SuggestRoutesTranslator): string {
  return t ? t(suggestRoutesStrings.stats.summitsSummaryKey, { count: n }) : `⛰ ${n}`
}

// ── Scale helpers ─────────────────────────────────────────────────────────────

export function scaleMaxLabel(level: string, t?: SuggestRoutesTranslator): string {
  return t ? t(suggestRoutesStrings.scale.maxKey, { level }) : level
}

export function scaleAriaLabel(kind: ScaleKind, maxLevel: string, t?: SuggestRoutesTranslator): string {
  const kindKey = suggestRoutesStrings.scale.kindLabel[kind]
  const kindName = t ? t(kindKey) : kind
  return t ? t(suggestRoutesStrings.scale.ariaKey, { kind: kindName, level: maxLevel }) : `${kind}, ${maxLevel}`
}
