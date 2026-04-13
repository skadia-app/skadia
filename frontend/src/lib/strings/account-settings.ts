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
  dataImport: {
    uploadLabel: 'Daten importieren',
    uploadDescription: 'Skadia-Export-ZIP importieren (DSGVO Art. 20).',
    uploadHint: 'Max. 4 GB · nur .zip-Dateien',
    uploadButton: 'Import starten',
    uploadButtonPending: 'Wird hochgeladen...',
    selectFile: 'ZIP-Datei auswählen',
    noFileSelected: 'Keine Datei ausgewählt',
    started: { title: 'Import gestartet' },
    completed: { title: 'Import abgeschlossen' },
    failed: {
      title: 'Import fehlgeschlagen',
      messageGeneric: 'Bitte versuche es später erneut.',
    },
    timeout: {
      title: 'Import-Timeout',
      message: 'Der Import hat zu lange gedauert. Bitte wende dich an den Support.',
    },
    progressPhase: 'Phase',
    resultImported: 'importiert',
    resultSkipped: 'übersprungen',
    resultErrors: 'Fehler',
    resultEnrichmentHint: 'Abgeleitete Daten (Best Efforts, PMC, Segmente) werden im Hintergrund berechnet — das kann einige Minuten dauern.',
    resultOAuthHint: 'Verbindungen zu externen Diensten (Garmin, Wahoo etc.) müssen nach dem Import erneut verknüpft werden.',
    errorsHeading: 'Fehlerdetails',
    newImportButton: 'Neuer Import',
  },
} as const
