import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'

const authMock = vi.hoisted(() => ({
  getMe: vi.fn(),
  updateMe: vi.fn(),
}))

const i18nMock = vi.hoisted(() => ({
  changeLanguage: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  getMe: authMock.getMe,
  updateMe: authMock.updateMe,
}))

vi.mock('@/i18n/i18n', () => ({
  i18n: i18nMock,
}))

import { UserProvider, useUser } from './UserProvider'

const baseUser = {
  user_id: 'user-1',
  username: 'demo',
  display_name: 'Demo User',
  email: 'demo@example.test',
  is_admin: false,
  email_verified: true,
  has_password: true,
  must_change_password: false,
  registration_source: 'local',
  locale: 'de',
  units: 'metric',
}

function renderProvider() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <UserProvider>
        <PreferenceConsumer />
      </UserProvider>
    </QueryClientProvider>,
  )
}

function PreferenceConsumer() {
  const { locale, units, setLocale, setUnits } = useUser()
  return (
    <div>
      <p data-testid="locale">{locale}</p>
      <p data-testid="units">{units}</p>
      <button onClick={() => { void setLocale('en') }}>English</button>
      <button onClick={() => { void setUnits('imperial') }}>Imperial</button>
    </div>
  )
}

describe('UserProvider preferences', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    authMock.getMe.mockResolvedValue(baseUser)
  })

  it('updates the cached locale and switches the active i18n language', async () => {
    authMock.updateMe.mockResolvedValue({ ...baseUser, locale: 'en' })

    renderProvider()
    expect((await screen.findByTestId('locale')).textContent).toBe('de')

    await userEvent.click(screen.getByRole('button', { name: 'English' }))

    await waitFor(() => {
      expect(authMock.updateMe.mock.calls[0]?.[0]).toEqual({ locale: 'en' })
      expect(i18nMock.changeLanguage).toHaveBeenCalledWith('en')
      expect(screen.getByTestId('locale').textContent).toBe('en')
    })
  })

  it('updates the cached unit preference', async () => {
    authMock.updateMe.mockResolvedValue({ ...baseUser, units: 'imperial' })

    renderProvider()
    expect((await screen.findByTestId('units')).textContent).toBe('metric')

    await userEvent.click(screen.getByRole('button', { name: 'Imperial' }))

    await waitFor(() => {
      expect(authMock.updateMe.mock.calls[0]?.[0]).toEqual({ units: 'imperial' })
      expect(screen.getByTestId('units').textContent).toBe('imperial')
    })
  })
})
