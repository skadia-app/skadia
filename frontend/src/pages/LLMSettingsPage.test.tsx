import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import LLMSettingsPage from './LLMSettingsPage'

vi.mock('@/api/llmSettings', () => ({
  fetchLLMConfig: vi.fn(async () => [
    { role: 'tool_agent', backend_type: 'llama_server', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'untested', eval_report: {} },
    { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'waived', eval_report: { admin_reason: 'manual' } },
  ]),
  saveLLMRole: vi.fn(),
  fetchOllamaModels: vi.fn(),
  testLLMConnection: vi.fn(),
}))

vi.mock('@/api/enrichment', () => ({
  fetchEnrichmentSettings: vi.fn(async () => ({})),
  updateEnrichmentSettings: vi.fn(),
}))

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <LLMSettingsPage />
    </QueryClientProvider>
  )
}

describe('LLMSettingsPage', () => {
  it('renders roles from API contract with eval status instead of a hard-coded default role', async () => {
    renderPage()

    expect(await screen.findByText('Tool-Agent')).toBeInTheDocument()
    expect(screen.getByText('Vision')).toBeInTheDocument()
    expect(screen.getByText('Eval: untested')).toBeInTheDocument()
    expect(screen.getByText('Eval: waived')).toBeInTheDocument()
    expect(screen.queryByText('Standard-LLM')).not.toBeInTheDocument()
    expect(screen.queryByText(/skadia-default/)).not.toBeInTheDocument()
  })
})
