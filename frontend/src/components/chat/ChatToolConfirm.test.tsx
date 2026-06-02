import '@testing-library/jest-dom'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatToolConfirm, { type ToolConfirmRequest } from './ChatToolConfirm'
import { confirmChatToolCall } from '@/api/chat'

vi.mock('@/api/chat', () => ({
  confirmChatToolCall: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({ locale: 'de', units: 'metric' }),
}))

function request(overrides: Partial<ToolConfirmRequest> = {}): ToolConfirmRequest {
  return {
    toolCallID: 'call-1',
    tool: 'create_gear',
    summary: 'Neues Gear anlegen',
    expiresAtISO: new Date(Date.now() + 60_000).toISOString(),
    sessionID: 'session-1',
    status: 'pending',
    ...overrides,
  }
}

describe('ChatToolConfirm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('posts confirmation on click', async () => {
    const onConfirmed = vi.fn()
    render(<ChatToolConfirm request={request()} onConfirmed={onConfirmed} onDeclined={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /bestätigen/i }))

    await waitFor(() => {
      expect(confirmChatToolCall).toHaveBeenCalledWith('session-1', {
        tool_call_id: 'call-1',
        confirmed: true,
      })
      expect(onConfirmed).toHaveBeenCalled()
    })
  })

  it('does not post when expired', () => {
    render(<ChatToolConfirm request={request({ expiresAtISO: new Date(Date.now() - 1000).toISOString() })} onConfirmed={vi.fn()} onDeclined={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /bestätigen/i }))

    expect(confirmChatToolCall).not.toHaveBeenCalled()
  })

  it('renders structured preview details', () => {
    render(<ChatToolConfirm request={request({ preview: { tool: 'create_gear', arguments: { name: 'Kette' } } })} onConfirmed={vi.fn()} onDeclined={vi.fn()} />)

    expect(screen.getByText(/\"name\": \"Kette\"/)).toBeInTheDocument()
  })

  it('renders compact activity sample fields from backend previews', () => {
    render(<ChatToolConfirm request={request({ preview: { sample_activities: [{ title: 'Kurze Runde', date: '2026-05-13', sport: 'cycling', km: 9.4 }] } })} onConfirmed={vi.fn()} onDeclined={vi.fn()} />)

    expect(screen.getByText(/Kurze Runde · 2026-05-13 · cycling · 9,4 km/)).toBeInTheDocument()
  })

  it('does not render raw args when the backend does not provide a confirmation preview', () => {
    render(<ChatToolConfirm request={request({ args: { name: 'Kette' } })} onConfirmed={vi.fn()} onDeclined={vi.fn()} />)

    expect(screen.queryByText(/\"name\": \"Kette\"/)).not.toBeInTheDocument()
  })

  it('does not confirm from a global Enter keypress or autofocus the confirm action', () => {
    render(
      <>
        <button type="button">Vorheriger Fokus</button>
        <ChatToolConfirm request={request()} onConfirmed={vi.fn()} onDeclined={vi.fn()} />
      </>,
    )

    expect(screen.getByRole('button', { name: /bestätigen/i })).not.toHaveFocus()
    const previous = screen.getByRole('button', { name: 'Vorheriger Fokus' })
    previous.focus()

    fireEvent.keyDown(window, { key: 'Enter' })

    expect(confirmChatToolCall).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: /bestätigen/i })).not.toHaveFocus()
  })

  it('shows an inline error and keeps the request actionable when confirmation fails', async () => {
    vi.mocked(confirmChatToolCall).mockRejectedValueOnce(new Error('Backend nicht erreichbar'))
    const onConfirmed = vi.fn()
    render(<ChatToolConfirm request={request()} onConfirmed={onConfirmed} onDeclined={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /bestätigen/i }))

    expect(await screen.findByRole('alert')).toHaveTextContent('Backend nicht erreichbar')
    expect(screen.queryByText(/Bestätigt/i)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /bestätigen/i })).toBeEnabled()
    expect(onConfirmed).not.toHaveBeenCalled()
  })
})
