// frontend/src/lib/libraryLabels.ts

import type { FileType } from '@/api/shadowfs'

// Pattern follows bestEffortLabels.ts, jobLabels.ts etc. — all labels files
// live in frontend/src/lib/. Static Record-of-strings (no functions) —
// compatible with future i18n framework that uses {placeholder} interpolation.

// ── Variable definitions per FileType ─────────────────────────────────────
// IMPORTANT: Keep in sync with backend variable list defined in
// services/api/internal/shadowfs/schema.go (SchemaRenderer.Render) and
// services/api/internal/shadowfs/legacy_template.go.
// Any new variable added on the backend MUST also be added here.

export interface VarDef {
  key: string
  label: string
}

export const TYPE_VARIABLES: Record<FileType, VarDef[]> = {
  activity: [
    { key: '{year}',        label: 'Jahr'          },
    { key: '{month}',       label: 'Monat'         },
    { key: '{day}',         label: 'Tag'           },
    { key: '{time}',        label: 'Uhrzeit'       },
    { key: '{format}',      label: 'Dateiformat'   },
    { key: '{sport}',       label: 'Sportart'      },
    { key: '{subCategory}', label: 'Unterkategorie'},
    { key: '{city}',        label: 'Stadt'         },
    { key: '{country}',     label: 'Land'          },
    { key: '{title}',       label: 'Titel'         },
    { key: '{distance_km}', label: 'Distanz (km)'  },
    { key: '{duration_h}',  label: 'Dauer (h)'     },
    { key: '{elevation_m}', label: 'Höhenmeter'    },
  ],
  media: [
    { key: '{year}',        label: 'Jahr'          },
    { key: '{month}',       label: 'Monat'         },
    { key: '{day}',         label: 'Tag'           },
    { key: '{time}',        label: 'Uhrzeit'       },
    { key: '{title}',       label: 'Titel'         },
    { key: '{parent_type}', label: 'Eltern-Typ'    },
    { key: '{parent_name}', label: 'Eltern-Name'   },
    { key: '{camera}',      label: 'Kamera'        },
    { key: '{width}',       label: 'Breite (px)'   },
    { key: '{height}',      label: 'Höhe (px)'     },
  ],
  document: [
    { key: '{year}',        label: 'Jahr'        },
    { key: '{month}',       label: 'Monat'       },
    { key: '{day}',         label: 'Tag'         },
    { key: '{category}',    label: 'Kategorie'   },
    { key: '{title}',       label: 'Titel'       },
    { key: '{parent_type}', label: 'Eltern-Typ'  },
    { key: '{parent_name}', label: 'Eltern-Name' },
  ],
  health: [
    { key: '{year}',  label: 'Jahr'  },
    { key: '{month}', label: 'Monat' },
    { key: '{day}',   label: 'Tag'   },
    { key: '{type}',  label: 'Typ'   },
  ],
  route: [
    { key: '{sport}',       label: 'Sportart'      },
    { key: '{title}',       label: 'Titel'         },
    { key: '{distance_km}', label: 'Distanz (km)'  },
    { key: '{elevation_m}', label: 'Höhenmeter'    },
    { key: '{difficulty}',  label: 'Schwierigkeit' },
  ],
}

export const EXAMPLE_VALUES: Record<string, string> = {
  '{year}':        '2024',
  '{month}':       '03',
  '{day}':         '15',
  '{time}':        '1430',
  '{format}':      'fit',
  '{sport}':       'cycling',
  '{subCategory}': 'mountain',
  '{city}':        'Berlin',
  '{country}':     'Deutschland',
  '{title}':       'Feierabendrunde',
  '{distance_km}': '42',
  '{duration_h}':  '2-0',
  '{elevation_m}': '850',
  '{parent_type}': 'activity',
  '{parent_name}': 'Feierabendrunde',
  '{camera}':      'Nikon-Z6',
  '{width}':       '5504',
  '{height}':      '3096',
  '{category}':    'rechnung',
  '{type}':        'hrv',
  '{difficulty}':  'T3',
}

export const FILE_TYPE_ORDER: FileType[] = ['activity', 'media', 'document', 'health', 'route']

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
