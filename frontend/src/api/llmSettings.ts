import { apiClient } from '@/api/client'

export interface LLMSystemConfig {
  role: string
  backend_type: 'ollama' | 'llama_server' | 'openai' | 'disabled'
  api_base?: string
  model_name?: string
  context_window?: number | null
  has_api_key: boolean
  enabled: boolean
  capability_flags: string[]
  eval_status: 'untested' | 'passed' | 'failed' | 'waived'
  eval_report: Record<string, unknown>
  updated_at: string
}

export interface OllamaModel {
  name: string
  modified_at: string
  size: number
}

export interface LLMTestResult {
  success: boolean
  message: string
  warning?: string
  latency_ms?: number
}

export interface LLMEvalResult {
  status: 'untested' | 'passed' | 'failed' | 'waived'
  failed_cases?: string[]
}

export type LLMAPIKeyState = 'unchanged' | 'set' | 'clear'

export interface LLMSaveResult {
  status: 'saved'
  warning?: string
}

export async function fetchLLMConfig(): Promise<LLMSystemConfig[]> {
  return apiClient<LLMSystemConfig[]>('/admin/llm-config')
}

export async function saveLLMRole(role: string, update: {
  backend_type: string
  api_base?: string
  model_name?: string
  context_window?: number | null
  api_key?: string
  api_key_state: LLMAPIKeyState
  enabled: boolean
}): Promise<LLMSaveResult> {
  return apiClient<LLMSaveResult>(`/admin/llm-config/${role}`, {
    method: 'PUT',
    body: update,
  })
}

export async function fetchOllamaModels(apiBase: string): Promise<OllamaModel[]> {
  try {
    return await apiClient<OllamaModel[]>(`/admin/llm-config/models?backend=ollama&api_base=${encodeURIComponent(apiBase)}`)
  } catch {
    return []
  }
}

export async function testLLMConnection(params: {
  backend_type: string
  api_base: string
  model_name: string
  api_key?: string
}): Promise<LLMTestResult> {
  return apiClient<LLMTestResult>('/admin/llm-config/test', {
    method: 'POST',
    body: params,
  })
}

export async function runLLMEval(role: string): Promise<LLMEvalResult> {
  return apiClient<LLMEvalResult>(`/admin/llm-config/${role}/eval?flush=1`, {
    method: 'POST',
  })
}

export async function waiveLLMEval(role: string, adminReason: string): Promise<void> {
  return apiClient<void>(`/admin/llm-config/${role}/eval-waiver`, {
    method: 'PATCH',
    body: { admin_reason: adminReason },
  })
}
