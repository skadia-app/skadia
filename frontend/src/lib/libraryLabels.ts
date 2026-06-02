// frontend/src/lib/libraryLabels.ts

import type { FileType } from '@/api/shadowfs'
import { i18n } from '@/i18n/i18n'

// Pattern follows bestEffortLabels.ts, jobLabels.ts etc. — all labels files
// live in frontend/src/lib/. Static Record-of-strings (no functions) —
// compatible with future i18n framework that uses {placeholder} interpolation.

// ── Variable definitions per FileType ─────────────────────────────────────
// IMPORTANT: Keep in sync with backend activity tags in
// services/api/internal/shadowfs/activity_schema_tags.go and path rendering in
// services/api/internal/shadowfs/schema.go.
// Any new variable added on the backend MUST also be added here.

export interface VarDef {
  key: string
  label: string
}

const l = (key: string) => i18n.t(key)

export const TYPE_VARIABLES: Record<FileType, VarDef[]> = {
  activity: [
    { key: '{year}',        label: l('libraryLabels.variables.year') },
    { key: '{month}',       label: l('libraryLabels.variables.month') },
    { key: '{day}',         label: l('libraryLabels.variables.day') },
    { key: '{time}',        label: l('libraryLabels.variables.time') },
    { key: '{format}',      label: l('libraryLabels.variables.format') },
    { key: '{sport}',       label: l('libraryLabels.variables.sport') },
    { key: '{subCategory}', label: l('libraryLabels.variables.subCategory') },
    { key: '{modifiers}',   label: l('libraryLabels.variables.modifiers') },
    { key: '{city}',        label: l('libraryLabels.variables.city') },
    { key: '{country}',     label: l('libraryLabels.variables.country') },
    { key: '{title}',       label: l('libraryLabels.variables.title') },
    { key: '{distance_km}', label: l('libraryLabels.variables.distanceKm') },
    { key: '{duration_h}',  label: l('libraryLabels.variables.durationH') },
    { key: '{elevation_m}', label: l('libraryLabels.variables.elevationM') },
  ],
  media: [
    { key: '{year}',        label: l('libraryLabels.variables.year') },
    { key: '{month}',       label: l('libraryLabels.variables.month') },
    { key: '{day}',         label: l('libraryLabels.variables.day') },
    { key: '{time}',        label: l('libraryLabels.variables.time') },
    { key: '{title}',       label: l('libraryLabels.variables.title') },
    { key: '{parent_type}', label: l('libraryLabels.variables.parentType') },
    { key: '{parent_name}', label: l('libraryLabels.variables.parentName') },
    { key: '{camera}',      label: l('libraryLabels.variables.camera') },
    { key: '{width}',       label: l('libraryLabels.variables.width') },
    { key: '{height}',      label: l('libraryLabels.variables.height') },
  ],
  document: [
    { key: '{year}',        label: l('libraryLabels.variables.year') },
    { key: '{month}',       label: l('libraryLabels.variables.month') },
    { key: '{day}',         label: l('libraryLabels.variables.day') },
    { key: '{category}',    label: l('libraryLabels.variables.category') },
    { key: '{title}',       label: l('libraryLabels.variables.title') },
    { key: '{parent_type}', label: l('libraryLabels.variables.parentType') },
    { key: '{parent_name}', label: l('libraryLabels.variables.parentName') },
  ],
  health: [
    { key: '{year}',  label: l('libraryLabels.variables.year') },
    { key: '{month}', label: l('libraryLabels.variables.month') },
    { key: '{day}',   label: l('libraryLabels.variables.day') },
    { key: '{type}',  label: l('libraryLabels.variables.type') },
  ],
  route: [
    { key: '{sport}',       label: l('libraryLabels.variables.sport') },
    { key: '{title}',       label: l('libraryLabels.variables.title') },
    { key: '{distance_km}', label: l('libraryLabels.variables.distanceKm') },
    { key: '{elevation_m}', label: l('libraryLabels.variables.elevationM') },
    { key: '{difficulty}',  label: l('libraryLabels.variables.difficulty') },
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
  '{modifiers}':   'electric_virtual',
  '{city}':        'Berlin',
  '{country}':     'DE',
  '{title}':       'Feierabendrunde',
  '{distance_km}': '42',
  '{duration_h}':  '0002-00',
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
  libraryDirPrefix: l('libraryLabels.labels.libraryDirPrefix'),
  libraryDirManagedByAdmin: l('libraryLabels.labels.libraryDirManagedByAdmin'),
  libraryDirNotApplicableS3: l('libraryLabels.labels.libraryDirNotApplicableS3'),

  // File-Schemas
  schemaAdminManaged: l('libraryLabels.labels.schemaAdminManaged'),
  schemaS3Inactive: l('libraryLabels.labels.schemaS3Inactive'),

  // Admin-Library-Tab
  adminTabTitle: l('libraryLabels.labels.adminTabTitle'),
  toggleUserCanEditDirName: l('libraryLabels.labels.toggleUserCanEditDirName'),
  toggleUserCanEditSchemas: l('libraryLabels.labels.toggleUserCanEditSchemas'),
  hintDirEditDisabledS3: l('libraryLabels.labels.hintDirEditDisabledS3'),
  buttonBulkRename: l('libraryLabels.labels.buttonBulkRename'),
  bulkRenameConfirmTitle: l('libraryLabels.labels.bulkRenameConfirmTitle'),
  bulkRenameConfirmLabel: l('libraryLabels.labels.bulkRenameConfirmLabel'),
  // Template with {ft} placeholder — replaced at call-site via .replace('{ft}', ft).
  // Future i18n framework (e.g. i18next) natively supports this pattern.
  bulkRenameConfirmMessage:
    l('libraryLabels.labels.bulkRenameConfirmMessage'),

  // Toasts
  toastSaved: l('libraryLabels.labels.toastSaved'),
  toastRenameStarted: l('libraryLabels.labels.toastRenameStarted'),
  toastBulkAlreadyRunning: l('libraryLabels.labels.toastBulkAlreadyRunning'),
  toastLibraryDirTaken: l('libraryLabels.labels.toastLibraryDirTaken'),
  toastLibraryDirClashesUUID: l('libraryLabels.labels.toastLibraryDirClashesUUID'),
  toastError: l('common.error'),
  unknownErrorFallback: l('common.unknownError'),

  // Schema effective-value hint
  schemaEffectiveLabel: l('libraryLabels.labels.schemaEffectiveLabel'),
  schemaAdminDefaultHint: l('libraryLabels.labels.schemaAdminDefaultHint'),

  // Common action buttons
  saveButton: l('common.save'),
  savingButton: l('common.saving'),
  discardButton: l('libraryLabels.labels.discardButton'),
  closeButton: l('common.close'),

  // UserLibraryDirDialog
  editLibraryDirMenuItem: l('libraryLabels.labels.editLibraryDirMenuItem'),
  effectiveLabel: l('libraryLabels.labels.effectiveLabel'),
  resetToUUIDHint: l('libraryLabels.labels.resetToUUIDHint'),
  executeRenameButton: l('libraryLabels.labels.executeRenameButton'),
  renameJobBackgroundHint: l('libraryLabels.labels.renameJobBackgroundHint'),
  loadLibraryDirErrorTitle: l('libraryLabels.labels.loadLibraryDirErrorTitle'),

  // FileSchemaSettings — section
  fileSchemaSectionTitle: l('libraryLabels.labels.fileSchemaSectionTitle'),
  fileSchemaSectionDescription:
    l('libraryLabels.labels.fileSchemaSectionDescription'),
  fileSchemaAvailableVars: l('libraryLabels.labels.fileSchemaAvailableVars'),
  fileSchemaTemplateLabel: l('libraryLabels.labels.fileSchemaTemplateLabel'),
  fileSchemaInsertVariable: l('libraryLabels.labels.fileSchemaInsertVariable'),
  fileSchemaPreview: l('libraryLabels.labels.fileSchemaPreview'),
  fileSchemaToastSaved: l('libraryLabels.labels.fileSchemaToastSaved'),
  fileSchemaErrorLoad: l('libraryLabels.labels.fileSchemaErrorLoad'),
  fileSchemaErrorSave: l('libraryLabels.labels.fileSchemaErrorSave'),

  // FileSchemaSettings — library dir subsection
  libraryDirSectionTitle: l('libraryLabels.labels.libraryDirSectionTitle'),
  libraryDirSectionDesc: l('libraryLabels.labels.libraryDirSectionDesc'),
  libraryDirInputPlaceholder: 'skadia-library',
  libraryDirToastSaved: l('libraryLabels.labels.libraryDirToastSaved'),

} as const
