import { apiClient } from './client'
import { i18n } from '@/i18n/i18n'

// ── Types ──────────────────────────────────────────────────────────────────

export type FileType = 'activity' | 'media' | 'document' | 'health' | 'route'

/** Human-readable labels for all known file types (i18n-ready placeholder). */
export const FILE_TYPE_LABELS: Record<string, string> = {
  get activity() { return i18n.t('fileTypes.activity') },
  get media() { return i18n.t('fileTypes.media') },
  get document() { return i18n.t('fileTypes.document') },
  get health() { return i18n.t('fileTypes.health') },
  get route() { return i18n.t('fileTypes.route') },
  get report() { return i18n.t('fileTypes.report') },
  get edit() { return i18n.t('fileTypes.edit') },
  get chat() { return i18n.t('fileTypes.chat') },
  get thumb() { return i18n.t('fileTypes.thumb') },
  get preview() { return i18n.t('fileTypes.preview') },
}

export interface FileSchema {
  file_type: FileType
  schema: string
  effective_schema: string
  is_user_override: boolean
}

export interface FileSchemaUpdateResponse {
  file_type: FileType
  schema: string
  warnings: string[]
}

// LibraryDirResponse is the shape returned by GET /settings/library-dir.
// `library_dir_name` is nullable (null when admin has not set a name).
export interface LibraryDirResponse {
  library_dir_name: string | null
  effective_name: string
  editable: boolean
  backend: 'local' | 's3'
  admin_managed_reason: 'admin_locked' | 's3_backend' | null
}

// UpdateLibraryDirResponse is the shape returned by PUT /settings/library-dir.
// Backend always returns status-only; frontend re-fetches via invalidateQueries
// to get the full LibraryDirResponse shape (R2 MED #7).
export interface UpdateLibraryDirResponse {
  status: 'ok' | 'library_dir_name_taken' | 'library_dir_name_clashes_with_user_uuid'
}

// LibraryDir is kept as an alias for backward compatibility.
export type LibraryDir = LibraryDirResponse

// BulkRenameResponse uses a discriminated union so candidate_count is required
// when status='ok' and absent when status='bulk_rename_in_progress' (R2 NIT #18).
export type BulkRenameResponse =
  | { status: 'ok'; candidate_count: number; note?: string }
  | { status: 'bulk_rename_in_progress' }

export interface RenameResponse {
  status: 'ok' | 'rename_in_progress'
  job_id?: string
}

export interface FileSchemasContext {
  editable: boolean
  backend: 'local' | 's3'
  admin_managed_reason: 'admin_locked' | 's3_backend' | null
}

export interface AdminLibrarySettings {
  user_can_edit_dir_name: boolean
  user_can_edit_schemas: boolean
  default_schemas: Record<string, string>
  backend: 'local' | 's3'
  local_actions_available: boolean
  admin_managed_reason: 's3_backend' | null
}

// API response status constants — mirror backend status strings from
// handler/shadowfs_settings.go + admin_user_library_dir.go.
export const API_STATUS = {
  libraryDirNameTaken: 'library_dir_name_taken',
  libraryDirNameClashesUUID: 'library_dir_name_clashes_with_user_uuid',
  bulkRenameInProgress: 'bulk_rename_in_progress',
  renameInProgress: 'rename_in_progress',
} as const

// ── API functions ──────────────────────────────────────────────────────────

export async function getFileSchemas(): Promise<FileSchema[]> {
  return apiClient<FileSchema[]>('/settings/file-schemas')
}

export async function updateFileSchema(
  fileType: FileType,
  schema: string,
): Promise<FileSchemaUpdateResponse> {
  return apiClient<FileSchemaUpdateResponse>(`/settings/file-schemas/${fileType}`, {
    method: 'PUT',
    body: { schema },
  })
}

export async function getLibraryDir(): Promise<LibraryDirResponse> {
  return apiClient<LibraryDirResponse>('/settings/library-dir')
}

export async function updateLibraryDir(libraryDirName: string): Promise<UpdateLibraryDirResponse> {
  return apiClient<UpdateLibraryDirResponse>('/settings/library-dir', {
    method: 'PUT',
    body: { library_dir_name: libraryDirName },
  })
}

export async function getFileSchemasContext(): Promise<FileSchemasContext> {
  return apiClient<FileSchemasContext>('/settings/file-schemas-context')
}

export async function getAdminLibrarySettings(): Promise<AdminLibrarySettings> {
  return apiClient<AdminLibrarySettings>('/admin/library-settings')
}

export async function updateAdminLibrarySettings(
  body: AdminLibrarySettings,
): Promise<{ status: string }> {
  return apiClient<{ status: string }>('/admin/library-settings', {
    method: 'PUT',
    body,
  })
}

export async function adminRenameBulk(fileType: string): Promise<BulkRenameResponse> {
  return apiClient<BulkRenameResponse>('/admin/rename-execute-bulk', {
    method: 'POST',
    body: { file_type: fileType },
  })
}

export async function adminRenameUser(userId: string, fileType: string): Promise<RenameResponse> {
  return apiClient<RenameResponse>(`/admin/users/${userId}/files/${fileType}/rename-execute`, {
    method: 'POST',
  })
}

export interface AdminUserLibraryDir {
  user_id: string
  library_dir_name: string | null
  effective_name: string
}

export async function getAdminUserLibraryDir(userId: string): Promise<AdminUserLibraryDir> {
  return apiClient<AdminUserLibraryDir>(`/admin/users/${userId}/library-dir`)
}

export async function adminSetUserLibraryDir(
  userId: string,
  name: string | null,
): Promise<{ status: string }> {
  return apiClient<{ status: string }>(`/admin/users/${userId}/library-dir`, {
    method: 'PUT',
    body: { library_dir_name: name },
  })
}
