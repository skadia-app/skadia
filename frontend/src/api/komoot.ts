import { apiClient } from './client'
import type { ImportOptions } from './importWizard'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface KomootScanResult {
  tour_count: number
  recorded_count: number
  planned_count: number
  new_count: number
  has_credentials: boolean
}

export interface KomootLoginResponse {
  status: 'ok' | 'invalid_credentials'
  error?: string
  komoot_user_id?: string
  credentials_saved?: boolean
  scan?: KomootScanResult
}

export interface KomootStartRequest {
  import_recorded: boolean
  import_planned: boolean
  import_photos: boolean
  tour_ids?: number[]
  options: ImportOptions
  delete_credentials_after?: boolean
}

export interface KomootJob {
  id: string
  job_type: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  phase: string | null
  progress_current: number | null
  progress_total: number | null
  entity_name: string | null
  started_at: string | null
  completed_at: string | null
  created_at: string
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

export async function komootLogin(email: string, password: string, saveCredentials: boolean): Promise<KomootLoginResponse> {
  return apiClient<KomootLoginResponse>('/import/komoot/login', {
    method: 'POST',
    body: { email, password, save_credentials: saveCredentials },
  })
}

export interface KomootCredentialsStatus {
  has_credentials: boolean
  email: string
}

export async function komootCredentialsStatus(): Promise<KomootCredentialsStatus> {
  return apiClient<KomootCredentialsStatus>('/import/komoot/credentials')
}

export async function komootDeleteCredentials(): Promise<void> {
  return apiClient<void>('/import/komoot/credentials', {
    method: 'DELETE',
  })
}

export async function komootScan(): Promise<KomootScanResult> {
  return apiClient<KomootScanResult>('/import/komoot/scan', {
    method: 'POST',
  })
}

export async function komootStartImport(req: KomootStartRequest): Promise<string> {
  const res = await apiClient<{ job_id: string }>('/import/komoot/start', {
    method: 'POST',
    body: req,
  })
  return res.job_id
}

export async function getKomootImportJob(jobId: string): Promise<KomootJob> {
  return apiClient<KomootJob>(`/import/komoot/jobs/${jobId}`)
}
