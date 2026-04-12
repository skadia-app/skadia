// Centralised UI strings for the Account Settings page.
//
// Frontend-CLAUDE.md i18n-Regel 1: Wiederverwendete oder im Code verstreute
// Strings müssen zentralisiert sein, damit der spätere i18n-Sprint sie an
// genau einer Stelle ersetzen kann.
//
// Conventions:
// - keys = camelCase, organised by feature area
// - DE values today, will be replaced by t() lookups in the i18n sprint
// - keep titles short (≤ 18 chars) so they fit on mobile toast width
// - separator is a plain ASCII slash (no middle-dot Unicode quirks)

export const accountSettingsStrings = {
  dataExport: {
    started: { title: 'Datenexport gestartet' },
    timeout: {
      title: 'Export-Timeout',
      message: 'Der Datenexport hat zu lange gedauert. Bitte versuche es später erneut.',
    },
    invalidUrl: { title: 'Ungültige Download-URL' },
    failed: {
      title: 'Datenexport fehlgeschlagen',
      messageGeneric: 'Bitte versuche es später erneut. Falls das Problem bestehen bleibt, kontaktiere den Support.',
    },
    completed: { title: 'Datenexport heruntergeladen' },
    detailSeparator: ' / ',
  },
} as const
