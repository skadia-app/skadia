export const labels = {
  trackSummary: {
    ascent: 'Aufstieg',
    descent: 'Abstieg',
    forestPctTpl: (pct: number) => `${Math.round(pct)} % Wald`,
    protectedPctTpl: (pct: number) => `${Math.round(pct)} % Schutzgebiet`,
    summitsCountTpl: (n: number) => (n === 1 ? '1 Gipfel' : `${n} Gipfel`),
    watersCountTpl: (n: number) => (n === 1 ? '1 Gewässer' : `${n} Gewässer`),
    settlementsCountTpl: (n: number) => (n === 1 ? '1 Ortschaft' : `${n} Ortschaften`),
    sacUpToTpl: (level: string) => `bis ${level}`,
    mtbUpToTpl: (level: string) => `bis ${level}`,
    vfUpToTpl: (level: string) => `bis ${level}`,
    vfUnknownPresent: 'Klettersteig ohne Skala',
    scalesDisabled: 'Schwierigkeitsskalen-Erkennung deaktiviert',
    scalesDisabledHint: 'Aktivierbar in Einstellungen → Analysen',
  },
} as const
