import { apiClient, apiUpload, API_BASE } from './client'

// ─── Types ─────────────────────────────────────────────────────────────────

export interface RoutingProfileEntry {
  id: string | null
  name: string
  engine: 'badrouter' | 'brouter'
  base_profile: string
  sport_types: string[]
  source: 'system' | 'user' | 'filesystem'
  config: Record<string, unknown> | null
  editable: boolean
}

export interface RoutingProfilesResponse {
  profiles: RoutingProfileEntry[]
}

export interface ImportProfileResult {
  name: string
  success: boolean
  error?: string
}

export interface ImportProfilesResponse {
  results: ImportProfileResult[]
}

// ─── API Functions ──────────────────────────────────────────────────────────

export function fetchAllProfiles(): Promise<RoutingProfilesResponse> {
  return apiClient('/routing/profiles')
}

export function createProfile(body: {
  name: string
  base_profile: string
  sport_types: string[]
  config: Record<string, unknown>
}): Promise<RoutingProfileEntry> {
  return apiClient('/routing/profiles', { method: 'POST', body })
}

export function updateProfile(
  id: string,
  body: {
    name: string
    sport_types: string[]
    config: Record<string, unknown>
  },
): Promise<void> {
  return apiClient(`/routing/profiles/${id}`, { method: 'PUT', body })
}

export function deleteProfile(id: string): Promise<void> {
  return apiClient(`/routing/profiles/${id}`, { method: 'DELETE' })
}

export function importProfiles(files: File[]): Promise<ImportProfilesResponse> {
  const fd = new FormData()
  for (const f of files) fd.append('files', f)
  return apiUpload(`/routing/profiles/import`, fd)
}

export function exportProfileUrl(id: string): string {
  return `${API_BASE}/routing/profiles/${id}/export`
}
