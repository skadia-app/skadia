// frontend/src/lib/libraryLabels.ts

// Pattern follows bestEffortLabels.ts, jobLabels.ts etc. — all labels files
// live in frontend/src/lib/. Static Record-of-strings (no functions) —
// compatible with future i18n framework that uses {placeholder} interpolation.
export const LIB_LABELS = {
  // Library-Dir
  libraryDirPrefix: 'Pfad-Präfix:',
  libraryDirManagedByAdmin: 'Wird vom Administrator verwaltet.',
  libraryDirNotApplicableS3: 'Nicht anwendbar bei S3-Storage.',

  // File-Schemas
  schemaAdminManaged: 'Diese Einstellung wird vom Administrator verwaltet.',
  schemaS3Inactive: 'Pfad-Einstellungen sind bei S3-Storage inaktiv.',

  // Admin-Library-Tab
  adminTabTitle: 'Dateipfade & Schemas',
  toggleUserCanEditDirName: 'User dürfen Verzeichnisname bearbeiten',
  toggleUserCanEditSchemas: 'User dürfen Schema-Templates bearbeiten',
  hintDirEditDisabledS3: 'Nicht anwendbar bei S3-Storage',
  buttonBulkRename: 'Rename auf alle User anwenden',
  bulkRenameConfirmTitle: 'Bulk-Rename bestätigen',
  bulkRenameConfirmLabel: 'Bulk-Rename starten',
  // Template with {ft} placeholder — replaced at call-site via .replace('{ft}', ft).
  // Future i18n framework (e.g. i18next) natively supports this pattern.
  bulkRenameConfirmMessage:
    'Rename für alle User mit file_type={ft} ausführen? Dies läuft im Hintergrund und kann nicht abgebrochen werden.',

  // Toasts
  toastSaved: 'Einstellungen gespeichert',
  toastRenameStarted: 'Rename gestartet',
  toastBulkAlreadyRunning: 'Bulk-Rename läuft bereits',
  toastLibraryDirTaken: 'Verzeichnisname bereits vergeben',
  toastLibraryDirClashesUUID: 'Name entspricht einer fremden User-UUID',
  toastError: 'Fehler',

  // Schema effective-value hint
  schemaEffectiveLabel: 'Effektiv:',
  schemaAdminDefaultHint: 'Standard: ',

  // Common action buttons
  saveButton: 'Speichern',
  savingButton: 'Speichert...',
  discardButton: 'Verwerfen',
  closeButton: 'Schließen',

  // UserLibraryDirDialog
  editLibraryDirMenuItem: 'Library-Dir bearbeiten',
  effectiveLabel: 'Effektiv:',
  resetToUUIDHint: 'Leer lassen = auf UUID zurücksetzen',
  executeRenameButton: 'Rename ausführen',
  renameJobBackgroundHint: 'Startet einen Rename-Job im Hintergrund.',
  loadLibraryDirErrorTitle: 'Fehler beim Laden der Library-Dir.',

  // FileSchemaSettings — section
  fileSchemaSectionTitle: 'Dateibenennung',
  fileSchemaSectionDescription:
    'Benennungsschema für jeden Dateityp. Variablen werden durch Werte des jeweiligen Eintrags ersetzt.',
  fileSchemaAvailableVars: 'Verfügbare Variablen',
  fileSchemaTemplateLabel: 'Schema-Template',
  fileSchemaInsertVariable: 'Variable einfügen',
  fileSchemaPreview: 'Vorschau',
  fileSchemaToastSaved: 'Schema gespeichert',
  fileSchemaErrorLoad: 'Schemata konnten nicht geladen werden',
  fileSchemaErrorSave: 'Fehler beim Speichern',

  // FileSchemaSettings — library dir subsection
  libraryDirSectionTitle: 'Bibliotheks-Verzeichnis',
  libraryDirSectionDesc: 'Name des Wurzel-Verzeichnisses in der Bibliothek.',
  libraryDirInputPlaceholder: 'skadia-library',
  libraryDirToastSaved: 'Bibliotheks-Verzeichnis gespeichert',

} as const
