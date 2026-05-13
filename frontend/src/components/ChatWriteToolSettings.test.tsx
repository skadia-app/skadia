import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ChatWriteToolSettings from '@/components/ChatWriteToolSettings'
import { ToastProvider } from '@/lib/toast'

function renderSettings() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <ToastProvider>
        <ChatWriteToolSettings />
      </ToastProvider>
    </QueryClientProvider>,
  )
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function waitForLoadedAllowList() {
  await waitFor(() => {
    expect(screen.getByLabelText('Aktivität umbenennen')).toHaveProperty('checked', true)
  })
}

describe('ChatWriteToolSettings', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (!init || init.method === undefined || init.method === 'GET') {
        return jsonResponse({
          chat_expose_document_extracted_data: false,
          chat_write_tools_enabled: false,
          chat_allowed_write_tools: ['rename_activity'],
        })
      }
      return jsonResponse(JSON.parse(init.body as string))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('persists the OCR opt-in toggle', async () => {
    renderSettings()
    await waitForLoadedAllowList()
    const toggle = await screen.findByRole('switch', {
      name: 'Dokument-OCR-Text dem AI-Agenten zugänglich machen',
    })

    fireEvent.click(toggle)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    const body = JSON.parse(fetchMock.mock.calls[1][1].body as string)
    expect(body.chat_expose_document_extracted_data).toBe(true)
    expect(body.chat_allowed_write_tools).toEqual(['rename_activity'])
  })

  it('does not save fallback defaults after preferences fail to load', async () => {
    fetchMock.mockRejectedValue(new Error('load failed'))

    renderSettings()
    await screen.findByText('Agent-Berechtigungen konnten nicht geladen werden.')
    const toggle = await screen.findByRole('switch', {
      name: 'Dokument-OCR-Text dem AI-Agenten zugänglich machen',
    })

    expect(toggle).toHaveProperty('disabled', true)
    fireEvent.click(toggle)

    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('persists the global write-tool toggle without dropping the saved allow-list', async () => {
    renderSettings()
    await waitForLoadedAllowList()
    const toggle = await screen.findByRole('switch', {
      name: 'AI-Agent darf Änderungen vorschlagen',
    })

    fireEvent.click(toggle)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    const body = JSON.parse(fetchMock.mock.calls[1][1].body as string)
    expect(body.chat_write_tools_enabled).toBe(true)
    expect(body.chat_allowed_write_tools).toEqual(['rename_activity'])
  })

  it('disables per-tool controls while global write tools are off but preserves checked state', async () => {
    renderSettings()
    await waitForLoadedAllowList()
    const checkbox = screen.getByLabelText('Aktivität umbenennen') as HTMLInputElement

    expect(checkbox.checked).toBe(true)
    expect(checkbox.disabled).toBe(true)
  })

  it('persists per-tool checkbox changes when global write tools are enabled', async () => {
    fetchMock.mockImplementation(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (!init || init.method === undefined || init.method === 'GET') {
        return jsonResponse({
          chat_expose_document_extracted_data: false,
          chat_write_tools_enabled: true,
          chat_allowed_write_tools: ['rename_activity'],
        })
      }
      return jsonResponse(JSON.parse(init.body as string))
    })

    renderSettings()
    await waitForLoadedAllowList()
    const checkbox = screen.getByLabelText('Gear einer Aktivität zuweisen') as HTMLInputElement

    fireEvent.click(checkbox)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    const body = JSON.parse(fetchMock.mock.calls[1][1].body as string)
    expect(body.chat_allowed_write_tools).toEqual(['rename_activity', 'set_activity_gear'])
  })

  it('batches rapid checkbox changes into a single save request', async () => {
    fetchMock.mockImplementation(async (_input: RequestInfo | URL, init?: RequestInit) => {
      if (!init || init.method === undefined || init.method === 'GET') {
        return jsonResponse({
          chat_expose_document_extracted_data: false,
          chat_write_tools_enabled: true,
          chat_allowed_write_tools: ['rename_activity'],
        })
      }
      return jsonResponse(JSON.parse(init.body as string))
    })

    renderSettings()
    await waitForLoadedAllowList()

    fireEvent.click(screen.getByLabelText('Gear anlegen'))
    fireEvent.click(screen.getByLabelText('Gear-Metadaten ändern'))
    fireEvent.click(screen.getByLabelText('Aktivität taggen'))

    expect(fetchMock).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2)
    })
    const body = JSON.parse(fetchMock.mock.calls[1][1].body as string)
    expect(body.chat_allowed_write_tools).toEqual([
      'rename_activity',
      'create_gear',
      'update_gear_metadata',
      'tag_activity',
    ])
  })
})
