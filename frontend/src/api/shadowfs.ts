import { apiClient } from './client'

// ── Types ──────────────────────────────────────────────────────────────────

export type FileType = 'activity' | 'media' | 'document' | 'health' | 'route'

export interface FileSchema {
  file_type: FileType
  schema: string
}

export interface FileSchemaUpdateResponse {
  file_type: FileType
  schema: string
  warnings: string[]
}

// Discriminated union for library-dir update responses.
// Success: backend returns { library_dir_name } with no status field.
// Conflict: backend returns { status: 'library_dir_name_taken' } with no library_dir_name.
export type LibraryDirResponse =
  | { status?: undefined; library_dir_name: string }
  | { status: 'library_dir_name_taken'; library_dir_name?: undefined }

// LibraryDir is used for GET /settings/library-dir responses.
// For PUT responses use LibraryDirResponse (discriminated union above).
export interface LibraryDir {
  library_dir_name: string
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

export async function getLibraryDir(): Promise<LibraryDir> {
  return apiClient<LibraryDir>('/settings/library-dir')
}

export async function updateLibraryDir(libraryDirName: string): Promise<LibraryDirResponse> {
  return apiClient<LibraryDirResponse>('/settings/library-dir', {
    method: 'PUT',
    body: { library_dir_name: libraryDirName },
  })
}
