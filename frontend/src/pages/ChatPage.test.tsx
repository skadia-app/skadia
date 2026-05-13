import '@testing-library/jest-dom'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChatPage from './ChatPage'
import { ToastContainer } from '@/components/Toast'
import { ToastProvider } from '@/lib/toast'
import { createSession, getMessages, listSessions, type ChatMessage } from '@/api/chat'
import { clearToken, redirectToLogin, refreshToken } from '@/lib/auth'

vi.mock('@/api/chat', () => ({
  listSessions: vi.fn(),
  createSession: vi.fn(),
  getMessages: vi.fn(),
  deleteSession: vi.fn(),
}))

vi.mock('@/lib/auth', () => ({
  getToken: vi.fn(() => null),
  refreshToken: vi.fn(async () => false),
  clearToken: vi.fn(),
  redirectToLogin: vi.fn(),
}))

const sessions = [
  {
    id: 'session-1',
    title: 'Chat 1',
    created_at: '2026-05-13T08:00:00Z',
    updated_at: '2026-05-13T08:00:00Z',
  },
  {
    id: 'session-2',
    title: 'Chat 2',
    created_at: '2026-05-13T09:00:00Z',
    updated_at: '2026-05-13T09:00:00Z',
  },
]

function renderPage() {
  return render(
    <ToastProvider>
      <ChatPage />
      <ToastContainer />
    </ToastProvider>,
  )
}

function sseResponse(events: Array<Record<string, unknown>>): Response {
  const encoder = new TextEncoder()
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const event of events) {
          controller.enqueue(encoder.encode(`data: ${JSON.stringify(event)}\n\n`))
        }
        controller.close()
      },
    }),
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  )
}

function pendingSseResponse(): Response {
  return new Response(
    new ReadableStream({
      start() {
        // Keep the stream open so the page remains in the streaming state.
      },
    }),
    { status: 200, headers: { 'Content-Type': 'text/event-stream' } },
  )
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(res => {
    resolve = res
  })
  return { promise, resolve }
}

async function sendMessage(text = 'Bitte vorbereiten') {
  const input = await screen.findByPlaceholderText('Frag mich etwas…')
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter' })
}

describe('ChatPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(listSessions).mockResolvedValue(sessions)
    vi.mocked(getMessages).mockResolvedValue([])
  })

  it('renders chat.user_question and finishes the streaming state', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        sseResponse([
          { type: 'chat.thinking', step: 'Prüfe Änderung' },
          {
            type: 'chat.user_question',
            tool_call_id: 'call-1',
            tool: 'create_tag',
            question: "Der Tag 'Intervalle' existiert noch nicht. Soll ich ihn anlegen?",
          },
        ]),
      ),
    )

    renderPage()
    await sendMessage()

    expect(await screen.findByText("Der Tag 'Intervalle' existiert noch nicht. Soll ich ihn anlegen?")).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.getByPlaceholderText('Frag mich etwas…')).toBeEnabled()
    })
  })

  it('keeps streamed messages when the first session is auto-created', async () => {
    vi.mocked(listSessions).mockResolvedValue([])
    vi.mocked(createSession).mockResolvedValue({
      id: 'session-new',
      title: 'Neu',
      created_at: '2026-05-13T10:00:00Z',
      updated_at: '2026-05-13T10:00:00Z',
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        sseResponse([
          {
            type: 'chat.user_question',
            tool_call_id: 'call-1',
            tool: 'create_tag',
            question: 'Soll ich den Tag anlegen?',
          },
        ]),
      ),
    )

    renderPage()
    await sendMessage()

    expect(await screen.findByText('Soll ich den Tag anlegen?')).toBeInTheDocument()
  })

  it('preserves an active newly-created session when the initial list resolves later', async () => {
    const initialList = deferred<typeof sessions>()
    vi.mocked(listSessions).mockReturnValue(initialList.promise)
    vi.mocked(createSession).mockResolvedValue({
      id: 'session-new',
      title: 'Neu',
      created_at: '2026-05-13T10:00:00Z',
      updated_at: '2026-05-13T10:00:00Z',
    })

    renderPage()
    fireEvent.click(screen.getByRole('button', { name: 'Neu' }))

    await waitFor(() => {
      expect(createSession).toHaveBeenCalled()
    })

    initialList.resolve(sessions)

    await waitFor(() => {
      expect(getMessages).toHaveBeenCalledWith('session-new')
    })
    expect(getMessages).not.toHaveBeenCalledWith('session-1')
  })

  it('allows session switching while a stream is active', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => pendingSseResponse()))

    renderPage()
    await screen.findByRole('button', { name: /Chat 1/i })
    await sendMessage()

    await waitFor(() => {
      expect(screen.getByTitle('Antwort abbrechen')).toBeInTheDocument()
    })
    fireEvent.click(screen.getByRole('button', { name: /Chat 1/i }))
    fireEvent.click(await screen.findByText('Chat 2'))

    await waitFor(() => {
      expect(getMessages).toHaveBeenCalledWith('session-2')
    })
  })

  it('removes the optimistic user message when sending fails before persistence', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('rate limited', { status: 429 })))

    renderPage()
    await sendMessage('nicht speichern')

    await waitFor(() => {
      expect(screen.queryByText('nicht speichern')).not.toBeInTheDocument()
    })
    expect(await screen.findByText('Zu viele Anfragen')).toBeInTheDocument()
  })

  it('removes the optimistic user message when token refresh fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('unauthorized', { status: 401 })))
    vi.mocked(refreshToken).mockResolvedValueOnce(false)

    renderPage()
    await sendMessage('nicht speichern')

    await waitFor(() => {
      expect(screen.queryByText('nicht speichern')).not.toBeInTheDocument()
    })
    expect(clearToken).toHaveBeenCalled()
    expect(redirectToLogin).toHaveBeenCalled()
  })

  it('clears a pending confirmation immediately when switching sessions after streaming', async () => {
    vi.mocked(getMessages).mockImplementation((sessionId: string) => {
      if (sessionId === 'session-2') return new Promise<ChatMessage[]>(() => undefined)
      return Promise.resolve([])
    })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        sseResponse([
          {
            type: 'chat.tool_confirm_request',
            tool_call_id: 'call-1',
            tool: 'create_gear',
            summary: 'Neues Gear anlegen?',
            expires_at_iso: new Date(Date.now() + 60_000).toISOString(),
          },
          { type: 'chat.done' },
        ]),
      ),
    )

    renderPage()
    await sendMessage()
    expect(await screen.findByText('Neues Gear anlegen?')).toBeInTheDocument()

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /Chat 1/i })).toBeEnabled()
    })
    fireEvent.click(screen.getByRole('button', { name: /Chat 1/i }))
    fireEvent.click(screen.getByText('Chat 2'))

    await waitFor(() => {
      expect(screen.queryByText('Neues Gear anlegen?')).not.toBeInTheDocument()
    })
  })
})
