import { apiClient } from './client'

export interface ShadowFSSettings {
  schema: string
  hotfolder_path: string
  hotfolder_enabled: boolean
}

export interface HotfolderImport {
  id: string
  filename: string
  status: string
  created_at: string
}

export interface RenamePreviewItem {
  old_path: string
  new_path: string
  activity_id: string
}

export interface RenameResult {
  succeeded: number
  failed: number
}

export async function getShadowFSSettings(): Promise<ShadowFSSettings> {
  return apiClient<ShadowFSSettings>('/settings/shadow-fs')
}

export async function updateShadowFSSettings(settings: Partial<ShadowFSSettings>): Promise<ShadowFSSettings> {
  return apiClient<ShadowFSSettings>('/settings/shadow-fs', { method: 'PUT', body: settings })
}

export async function previewRename(): Promise<RenamePreviewItem[]> {
  return apiClient<RenamePreviewItem[]>('/shadow-fs/preview-rename', { method: 'POST' })
}

export async function executeRename(): Promise<RenameResult> {
  return apiClient<RenameResult>('/shadow-fs/execute-rename', { method: 'POST' })
}

export async function getHotfolderImports(): Promise<HotfolderImport[]> {
  return apiClient<HotfolderImport[]>('/hotfolder/imports?limit=5')
}
