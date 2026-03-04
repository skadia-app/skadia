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
  latency_ms?: number
}

export async function fetchLLMConfig(): Promise<LLMSystemConfig[]> {
  const res = await fetch('/api/v1/admin/llm-config')
  if (!res.ok) throw new Error('Failed to fetch LLM config')
  return res.json()
}

export async function saveLLMRole(role: string, update: {
  backend_type: string
  api_base?: string
  model_name?: string
  api_key?: string
  enabled: boolean
}): Promise<void> {
  const res = await fetch(`/api/v1/admin/llm-config/${role}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(update),
  })
  if (!res.ok) throw new Error('Failed to save LLM config')
}

export async function fetchOllamaModels(apiBase: string): Promise<OllamaModel[]> {
  const res = await fetch(`/api/v1/admin/llm-config/models?backend=ollama&api_base=${encodeURIComponent(apiBase)}`)
  if (!res.ok) return []
  return res.json()
}

export async function testLLMConnection(params: {
  backend_type: string
  api_base: string
  model_name: string
  api_key?: string
}): Promise<LLMTestResult> {
  const res = await fetch('/api/v1/admin/llm-config/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(params),
  })
  if (!res.ok) throw new Error('Test failed')
  return res.json()
}
