import { apiClient } from '@/api/client'

export interface LLMSystemConfig {
  role: string
  backend_type: 'ollama' | 'llama_server' | 'openai' | 'anthropic' | 'disabled'
  api_base?: string
  model_name?: string
  has_api_key: boolean
  enabled: boolean
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

export async function fetchLLMConfig(): Promise<LLMSystemConfig[]> {
  return apiClient<LLMSystemConfig[]>('/admin/llm-config')
}

export async function saveLLMRole(role: string, update: {
  backend_type: string
  api_base?: string
  model_name?: string
  api_key?: string
  enabled: boolean
}): Promise<void> {
  return apiClient<void>(`/admin/llm-config/${role}`, {
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
