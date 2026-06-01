import { i18n } from '@/i18n/i18n'

function tt(key: string, options?: Record<string, unknown>): string {
  return i18n.t(`trackSummary.${key}`, options)
}

export const labels = {
  trackSummary: {
    get ascent() { return tt('ascent') },
    get descent() { return tt('descent') },
    forestPctTpl: (pct: number) => tt('forestPct', { pct: Math.round(pct) }),
    protectedPctTpl: (pct: number) => tt('protectedPct', { pct: Math.round(pct) }),
    summitsCountTpl: (n: number) => tt('summitsCount', { count: n }),
    watersCountTpl: (n: number) => tt('watersCount', { count: n }),
    settlementsCountTpl: (n: number) => tt('settlementsCount', { count: n }),
    sacUpToTpl: (level: string) => tt('sacUpTo', { level }),
    mtbUpToTpl: (level: string) => tt('mtbUpTo', { level }),
    vfUpToTpl: (level: string) => tt('vfUpTo', { level }),
    get vfUnknownPresent() { return tt('vfUnknownPresent') },
  },
} as const
