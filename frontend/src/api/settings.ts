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

export async function previewRename(schema: string): Promise<RenamePreviewItem[]> {
  return apiClient<RenamePreviewItem[]>('/shadow-fs/preview-rename', {
    method: 'POST',
    body: { new_schema: schema },
  })
}

export async function executeRename(items: RenamePreviewItem[]): Promise<RenameResult> {
  return apiClient<RenameResult>('/shadow-fs/execute-rename', {
    method: 'POST',
    body: items,
  })
}

export async function getHotfolderImports(): Promise<HotfolderImport[]> {
  return apiClient<HotfolderImport[]>('/hotfolder/imports?limit=5')
}

export interface HeatmapSettings {
  sampling_interval_m: number
}

export async function getHeatmapSettings(): Promise<HeatmapSettings> {
  return apiClient<HeatmapSettings>('/settings/heatmap')
}

export async function updateHeatmapSettings(settings: HeatmapSettings): Promise<HeatmapSettings> {
  return apiClient<HeatmapSettings>('/settings/heatmap', { method: 'PUT', body: settings })
}
