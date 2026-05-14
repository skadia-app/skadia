import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import '@testing-library/jest-dom'
import { fireEvent, render, screen, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatProfileSettings from './ChatProfileSettings'
import { apiFetch } from '@/lib/auth'

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(async (path: string) => ({
    ok: true,
    status: 200,
    json: async () => {
      if (path === '/api/v1/settings/ml/chat-profile-defaults') {
        return {
          default_profile: 'small',
          presets: {
            tiny: {
              max_tools: 4, use_router: true, max_tools_per_turn: 1, tool_result_limit: 5,
              schema_mode: 'filtered', enable_sql_agent: false, sql_complexity: 'simple', max_sql_retries: 0,
              few_shot_count: 3, system_prompt_mode: 'compact', max_context_tokens: 2000, history_messages: 5,
              reasoning_mode: 'guided', temperature: 0.2, top_p: 0.8, top_k: 20, presence_penalty: 0.3, max_response_tokens: 1000,
            },
            small: {
              max_tools: 6, use_router: true, max_tools_per_turn: 3, tool_result_limit: 12,
              schema_mode: 'filtered', enable_sql_agent: true, sql_complexity: 'simple', max_sql_retries: 1,
              few_shot_count: 3, system_prompt_mode: 'compact', max_context_tokens: 12000, history_messages: 10,
              reasoning_mode: 'guided', temperature: 0.3, top_p: 0.9, top_k: 40, presence_penalty: 0.3, max_response_tokens: 3000,
            },
            medium: {
              max_tools: 10, use_router: true, max_tools_per_turn: 2, tool_result_limit: 15,
              schema_mode: 'filtered', enable_sql_agent: true, sql_complexity: 'moderate', max_sql_retries: 2,
              few_shot_count: 2, system_prompt_mode: 'standard', max_context_tokens: 8000, history_messages: 20,
              reasoning_mode: 'free', temperature: 0.5, top_p: 0.9, top_k: 40, presence_penalty: 1.0, max_response_tokens: 6000,
            },
            large: {
              max_tools: 20, use_router: false, max_tools_per_turn: 5, tool_result_limit: 50,
              schema_mode: 'full', enable_sql_agent: true, sql_complexity: 'advanced', max_sql_retries: 2,
              few_shot_count: 0, system_prompt_mode: 'rich', max_context_tokens: 16000, history_messages: 30,
              reasoning_mode: 'free', temperature: 0.7, top_p: 0.95, top_k: 50, presence_penalty: 0.5, max_response_tokens: 12000,
            },
          },
        }
      }
      return {}
    },
  })),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}))

function renderSettings() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <ChatProfileSettings />
    </QueryClientProvider>
  )
}

describe('ChatProfileSettings', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('does not seed or offer a hard-coded skadia-default model', async () => {
    renderSettings()

    expect(await screen.findByText('AI-Agent')).toBeInTheDocument()
    expect(screen.queryByText(/skadia-default/)).not.toBeInTheDocument()
    expect(screen.queryByLabelText('LLM-Modell')).not.toBeInTheDocument()
  })

  it('loads preset defaults from the backend contract', async () => {
    renderSettings()

    expect(await screen.findByText('AI-Agent')).toBeInTheDocument()
    expect(apiFetch).toHaveBeenCalledWith('/api/v1/settings/ml/chat-profile-defaults')

    fireEvent.click(await screen.findByRole('button', { name: 'Tools & Routing' }))
    const multiTool = screen.getByText('Tool-Aufrufe pro Antwort').closest('div')
    expect(multiTool).not.toBeNull()
    expect(within(multiTool as HTMLElement).getByText('3')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Prompt & Kontext' }))
    const history = screen.getByText('Chat-History (Nachrichten)').closest('div')
    expect(history).not.toBeNull()
    expect(within(history as HTMLElement).getByText('10')).toBeInTheDocument()
    expect(screen.getByLabelText('Max. Kontext-Tokens')).toHaveValue(12000)
  })

  it('blocks editing instead of saving fallback defaults when user settings fail to load', async () => {
    vi.mocked(apiFetch).mockImplementation(async (path: string) => {
      if (path === '/api/v1/settings/ml/user') throw new Error('load failed')
      return {
        ok: true,
        status: 200,
        json: async () => ({
          default_profile: 'small',
          presets: {
            tiny: {
              max_tools: 4, use_router: true, max_tools_per_turn: 1, tool_result_limit: 5,
              schema_mode: 'filtered', enable_sql_agent: false, sql_complexity: 'simple', max_sql_retries: 0,
              few_shot_count: 3, system_prompt_mode: 'compact', max_context_tokens: 2000, history_messages: 5,
              reasoning_mode: 'guided', temperature: 0.2, top_p: 0.8, top_k: 20, presence_penalty: 0.3, max_response_tokens: 1000,
            },
            small: {
              max_tools: 6, use_router: true, max_tools_per_turn: 3, tool_result_limit: 12,
              schema_mode: 'filtered', enable_sql_agent: true, sql_complexity: 'simple', max_sql_retries: 1,
              few_shot_count: 3, system_prompt_mode: 'compact', max_context_tokens: 12000, history_messages: 10,
              reasoning_mode: 'guided', temperature: 0.3, top_p: 0.9, top_k: 40, presence_penalty: 0.3, max_response_tokens: 3000,
            },
            medium: {
              max_tools: 10, use_router: true, max_tools_per_turn: 2, tool_result_limit: 15,
              schema_mode: 'filtered', enable_sql_agent: true, sql_complexity: 'moderate', max_sql_retries: 2,
              few_shot_count: 2, system_prompt_mode: 'standard', max_context_tokens: 8000, history_messages: 20,
              reasoning_mode: 'free', temperature: 0.5, top_p: 0.9, top_k: 40, presence_penalty: 1.0, max_response_tokens: 6000,
            },
            large: {
              max_tools: 20, use_router: false, max_tools_per_turn: 5, tool_result_limit: 50,
              schema_mode: 'full', enable_sql_agent: true, sql_complexity: 'advanced', max_sql_retries: 2,
              few_shot_count: 0, system_prompt_mode: 'rich', max_context_tokens: 16000, history_messages: 30,
              reasoning_mode: 'free', temperature: 0.7, top_p: 0.95, top_k: 50, presence_penalty: 0.5, max_response_tokens: 12000,
            },
          },
        }),
      } as Response
    })

    renderSettings()

    expect(await screen.findByText('Agent-Einstellungen konnten nicht geladen werden.')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Speichern' })).not.toBeInTheDocument()
  })
})
