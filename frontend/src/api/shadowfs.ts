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

export interface LibraryDir {
  library_dir_name: string
}

// ── API functions ──────────────────────────────────────────────────────────

export async function getFileSchemas(): Promise<FileSchema[]> {
  return apiClient<FileSchema[]>('/settings/file-schemas')
}

export async function updateFileSchema(
  type: FileType,
  schema: string,
): Promise<FileSchemaUpdateResponse> {
  return apiClient<FileSchemaUpdateResponse>(`/settings/file-schemas/${type}`, {
    method: 'PUT',
    body: { schema },
  })
}

export async function getLibraryDir(): Promise<LibraryDir> {
  return apiClient<LibraryDir>('/settings/library-dir')
}

export async function updateLibraryDir(libraryDirName: string): Promise<LibraryDir> {
  return apiClient<LibraryDir>('/settings/library-dir', {
    method: 'PUT',
    body: { library_dir_name: libraryDirName },
  })
}
