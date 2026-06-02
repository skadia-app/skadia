import '@testing-library/jest-dom/vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { I18nProvider } from '@/i18n/I18nProvider'

const userMock = vi.hoisted(() => ({
  setLocale: vi.fn(),
}))

vi.mock('@/providers/UserProvider', () => ({
  useUser: () => ({
    user: {
      user_id: 'user-1',
      username: 'demo',
      display_name: 'Demo User',
      email: 'demo@example.test',
      locale: 'de',
      units: 'metric',
    },
    locale: 'de',
    units: 'metric',
    setLocale: userMock.setLocale,
  }),
}))

vi.mock('@/api/auth', () => ({
  logout: vi.fn(),
}))

vi.mock('@/lib/auth', () => ({
  clearToken: vi.fn(),
}))

import UserMenu from './UserMenu'

describe('UserMenu language selector', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('persists a selected language from the account menu', async () => {
    userMock.setLocale.mockResolvedValue(undefined)

    render(
      <I18nProvider>
        <MemoryRouter>
          <UserMenu />
        </MemoryRouter>
      </I18nProvider>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Benutzermenü öffnen' }))
    await userEvent.click(screen.getByRole('button', { name: /English/ }))

    await waitFor(() => {
      expect(userMock.setLocale).toHaveBeenCalledWith('en')
    })
  })

  it('uses language metadata for accessible names and selected state', async () => {
    render(
      <I18nProvider>
        <MemoryRouter>
          <UserMenu />
        </MemoryRouter>
      </I18nProvider>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Benutzermenü öffnen' }))

    const germanButton = screen.getByRole('button', { name: 'Deutsch, German' })
    const englishButton = screen.getByRole('button', { name: 'English' })

    expect(germanButton.textContent).toContain('🇩🇪')
    expect(germanButton).toHaveAttribute('aria-pressed', 'true')
    expect(englishButton).toHaveAttribute('aria-pressed', 'false')
    expect(screen.queryByRole('button', { name: /🇩🇪/ })).not.toBeInTheDocument()
  })
})
