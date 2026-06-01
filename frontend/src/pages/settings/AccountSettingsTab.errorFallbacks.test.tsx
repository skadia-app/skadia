import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { addToastMock, updateMeMock } = vi.hoisted(() => ({
  addToastMock: vi.fn(),
  updateMeMock: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({
    user: {
      username: 'test',
      display_name: 'Test User',
      email: 'test@example.com',
      has_password: true,
    },
    hasPassword: true,
    mustChangePassword: false,
  }),
}))

vi.mock('@/lib/toast', () => ({
  useToast: () => ({ addToast: addToastMock }),
}))

vi.mock('@/api/auth', async () => {
  const actual = await vi.importActual<typeof import('@/api/auth')>('@/api/auth')
  return {
    ...actual,
    updateMe: updateMeMock,
    getSessions: vi.fn().mockResolvedValue({ sessions: [] }),
    getLatestDataExport: vi.fn().mockResolvedValue(null),
  }
})

import AccountSettingsTab from './AccountSettingsTab'

function renderAccountSettings() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={queryClient}>
        <AccountSettingsTab />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('AccountSettingsTab error fallbacks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('maps an unmapped profile save error to the generic auth message', async () => {
    updateMeMock.mockRejectedValue(new Error('Update failed'))

    renderAccountSettings()

    const displayNameInput = screen.getByDisplayValue('Test User')
    await userEvent.clear(displayNameInput)
    await userEvent.type(displayNameInput, 'Neuer Name')
    await userEvent.click(screen.getByRole('button', { name: 'Speichern' }))

    await waitFor(() => {
      expect(addToastMock).toHaveBeenCalledWith({
        type: 'error',
        title: 'Ein interner Fehler ist aufgetreten.',
        duration: 0,
      })
    })
  })
})
