import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatToolConfirm, { type ToolConfirmRequest } from './ChatToolConfirm'
import { confirmChatToolCall } from '@/api/chat'

vi.mock('@/api/chat', () => ({
  confirmChatToolCall: vi.fn().mockResolvedValue(undefined),
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
})
