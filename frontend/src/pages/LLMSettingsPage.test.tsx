import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@testing-library/jest-dom'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import LLMSettingsPage from './LLMSettingsPage'
import { runLLMEval, saveLLMRole, testLLMConnection, waiveLLMEval } from '@/api/llmSettings'

const mockState = vi.hoisted(() => ({
  configs: [
    { role: 'tool_agent', backend_type: 'llama_server', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'untested', eval_report: {} },
    { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'waived', eval_report: { admin_reason: 'manual' } },
  ] as any[],
}))

vi.mock('@/api/llmSettings', () => ({
  fetchLLMConfig: vi.fn(async () => mockState.configs),
  saveLLMRole: vi.fn(),
  fetchOllamaModels: vi.fn(),
  testLLMConnection: vi.fn(),
  runLLMEval: vi.fn(),
  waiveLLMEval: vi.fn(),
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

async function enableSeparateRoles(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole('switch', { name: 'Ein Backend für alle Rollen verwenden' }))
}

describe('LLMSettingsPage', () => {
  beforeEach(() => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'untested', eval_report: {} },
      { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'waived', eval_report: { admin_reason: 'manual' } },
    ]
    vi.clearAllMocks()
  })

  it('renders roles from API contract with readable status labels instead of a hard-coded default role', async () => {
    renderPage()

    expect(await screen.findByText('Gemeinsame Verbindung')).toBeInTheDocument()
    expect(screen.getByText(/Diese Backend-Daten/)).toBeInTheDocument()
    expect(screen.getByText('Tool-Agent')).toBeInTheDocument()
    expect(screen.getByText('Vision')).toBeInTheDocument()
    expect(screen.getByText('Status: nicht geprüft')).toBeInTheDocument()
    expect(screen.getByText('Status: freigegeben')).toBeInTheDocument()
    expect(screen.queryByText('Standard-LLM')).not.toBeInTheDocument()
    expect(screen.queryByText(/skadia-default/)).not.toBeInTheDocument()
  })

  it('does not render stale legacy roles as editable cards', async () => {
    mockState.configs = [
      { role: 'default', backend_type: 'ollama', api_base: 'http://ollama:11434/v1', model_name: 'qwen3:8b', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'untested', eval_report: {} },
      { role: 'vlm', backend_type: 'ollama', api_base: 'http://ollama:11434/v1', model_name: 'qwen3-vl:8b', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'untested', eval_report: {} },
      { role: 'tool_agent', backend_type: 'llama_server', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'untested', eval_report: {} },
    ]
    renderPage()

    expect(await screen.findByText('Gemeinsame Verbindung')).toBeInTheDocument()
    expect(screen.getByText('Status: nicht geprüft')).toBeInTheDocument()
    expect(screen.queryByText('default')).not.toBeInTheDocument()
    expect(screen.queryByText('vlm')).not.toBeInTheDocument()
  })

  it('submits an admin connection release only with the required reason', async () => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'failed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()
    await enableSeparateRoles(user)

    await screen.findByText('Tool-Agent')
    await user.type(screen.getByLabelText('Freigabe-Begründung'), 'Manuelle lokale Prüfung')
    await user.click(screen.getByRole('button', { name: 'Freigeben' }))

    expect(waiveLLMEval).toHaveBeenCalledWith('tool_agent', 'Manuelle lokale Prüfung')
  })

  it('chooses a valid backend before saving an enabled role that was disabled', async () => {
    mockState.configs = [
      { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'untested', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()
    await enableSeparateRoles(user)

    await user.click(await screen.findByLabelText('Vision aktivieren'))
    await user.type(await screen.findByPlaceholderText('gpt-4o'), 'vision-model')
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(saveLLMRole).toHaveBeenCalledWith('vision', expect.objectContaining({
      backend_type: 'llama_server',
      api_base: 'http://host.containers.internal:8000/v1',
      model_name: 'vision-model',
      enabled: true,
    }))
  })

  it('surfaces LiteLLM regeneration warnings returned by save', async () => {
    vi.mocked(saveLLMRole).mockResolvedValueOnce({
      status: 'saved',
      warning: 'optional LiteLLM config could not be regenerated',
    })
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'passed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Gemeinsame Verbindung')
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(await screen.findByRole('status')).toHaveTextContent('optional LiteLLM config could not be regenerated')
  })

  it('shows save errors returned by the role config API', async () => {
    vi.mocked(saveLLMRole).mockRejectedValueOnce(new Error('Backend nicht erreichbar'))
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'passed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Gemeinsame Verbindung')
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Backend nicht erreichbar')
  })

  it('does not start eval while role settings are dirty', async () => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'failed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()
    await enableSeparateRoles(user)

    const modelInput = await screen.findByDisplayValue('tool-model')
    await user.clear(modelInput)
    await user.type(modelInput, 'other-model')

    expect(screen.getByRole('button', { name: 'Qualitätsprüfung starten' })).toBeDisabled()
    expect(runLLMEval).not.toHaveBeenCalled()
  })

  it('does not allow connection release while shared role settings are dirty', async () => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'failed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    const modelInput = await screen.findByDisplayValue('tool-model')
    await user.clear(modelInput)
    await user.type(modelInput, 'other-model')
    await user.type(screen.getByLabelText('Tool-Agent Freigabe-Begründung'), 'Manuelle lokale Prüfung')

    expect(screen.getByRole('button', { name: 'Freigeben' })).toBeDisabled()
    expect(waiveLLMEval).not.toHaveBeenCalled()
  })

  it('surfaces connection test validation details from the API', async () => {
    vi.mocked(testLLMConnection).mockRejectedValueOnce(Object.assign(new Error('Bad Request'), {
      json: { message: 'api_base address host.containers.internal:8181 is blocked by LLM SSRF policy' },
    }))
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'http://host.containers.internal:8181/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'failed', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Gemeinsame Verbindung')
    await user.click(screen.getByRole('button', { name: 'Verbindung testen' }))

    expect(await screen.findByText(/host\.containers\.internal:8181 is blocked/)).toBeInTheDocument()
  })

  it('saves the shared backend to every managed role shown by the API', async () => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'shared-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'passed', eval_report: {} },
      { role: 'document_extract', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['document_extract'], eval_status: 'untested', eval_report: {} },
      { role: 'markdown_extract', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['markdown_extract'], eval_status: 'untested', eval_report: {} },
      { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['vision'], eval_status: 'untested', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Gemeinsame Verbindung')
    await user.click(screen.getByLabelText('Dokumente aktivieren'))
    await user.click(screen.getByLabelText('Markdown aktivieren'))
    await user.click(screen.getByLabelText('Vision aktivieren'))
    await user.type(screen.getByPlaceholderText('Standard aus Server-Umgebung'), '32768')
    await user.click(screen.getByRole('button', { name: 'Speichern' }))

    expect(saveLLMRole).toHaveBeenCalledTimes(4)
    expect(saveLLMRole).toHaveBeenCalledWith('tool_agent', expect.objectContaining({ model_name: 'shared-model', context_window: 32768, enabled: true }))
    expect(saveLLMRole).toHaveBeenCalledWith('document_extract', expect.objectContaining({ model_name: 'shared-model', context_window: 32768, enabled: true }))
    expect(saveLLMRole).toHaveBeenCalledWith('markdown_extract', expect.objectContaining({ model_name: 'shared-model', context_window: 32768, enabled: true }))
    expect(saveLLMRole).toHaveBeenCalledWith('vision', expect.objectContaining({ model_name: 'shared-model', context_window: 32768, enabled: true }))
  })

  it('allows releasing an active role from the shared backend view', async () => {
    mockState.configs = [
      { role: 'tool_agent', backend_type: 'llama_server', api_base: 'https://llm.example/v1', model_name: 'tool-model', enabled: true, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: ['tool_calls'], eval_status: 'untested', eval_report: {} },
      { role: 'vision', backend_type: 'disabled', enabled: false, has_api_key: false, updated_at: '2026-05-12T00:00:00Z', capability_flags: [], eval_status: 'untested', eval_report: {} },
    ]
    const user = userEvent.setup()
    renderPage()

    await screen.findByText('Gemeinsame Verbindung')
    await user.type(screen.getByLabelText('Tool-Agent Freigabe-Begründung'), 'Lokales Testmodell geprüft')
    await user.click(screen.getByRole('button', { name: 'Freigeben' }))

    expect(waiveLLMEval).toHaveBeenCalledWith('tool_agent', 'Lokales Testmodell geprüft')
  })
})
