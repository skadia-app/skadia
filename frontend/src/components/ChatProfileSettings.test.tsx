import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import ChatProfileSettings from './ChatProfileSettings'

vi.mock('@/lib/auth', () => ({
  apiFetch: vi.fn(async () => ({ json: async () => ({}) })),
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
  it('does not seed or offer a hard-coded skadia-default model', async () => {
    renderSettings()

    expect(await screen.findByText('KI-Assistent')).toBeInTheDocument()
    expect(screen.queryByText(/skadia-default/)).not.toBeInTheDocument()
    expect(screen.queryByLabelText('LLM-Modell')).not.toBeInTheDocument()
  })
})
