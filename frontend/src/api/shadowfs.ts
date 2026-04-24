import { apiClient } from './client'

// ── Types ──────────────────────────────────────────────────────────────────

export type FileType = 'activity' | 'media' | 'document' | 'health' | 'route'

/** Human-readable labels for all known file types (i18n-ready placeholder). */
export const FILE_TYPE_LABELS: Record<string, string> = {
  activity: 'Aktivitäten',
  media: 'Medien',
  document: 'Dokumente',
  health: 'Gesundheit',
  route: 'Routen',
  report: 'Berichte',
  edit: 'Bearbeitungen',
  chat: 'Chat',
  thumb: 'Thumbnails',
  preview: 'Vorschauen',
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

// LibraryDirResponse is used for GET /settings/library-dir and PUT responses.
// `library_dir_name` is nullable (null when admin has not set a name).
// `status` is set on conflict or async-job conditions.
export interface LibraryDirResponse {
  library_dir_name: string | null
  effective_name: string
  editable: boolean
  backend: 'local' | 's3'
  admin_managed_reason: 'admin_locked' | 's3_backend' | null
  status?: 'library_dir_name_taken' | 'library_dir_name_clashes_with_user_uuid'
}

// LibraryDir is kept as an alias for backward compatibility.
export type LibraryDir = LibraryDirResponse

export interface BulkRenameResponse {
  status: 'ok' | 'bulk_rename_in_progress'
  candidate_count?: number
  note?: string
}

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
}

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

export async function updateLibraryDir(libraryDirName: string): Promise<LibraryDirResponse> {
  return apiClient<LibraryDirResponse>('/settings/library-dir', {
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
