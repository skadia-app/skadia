import '@testing-library/jest-dom/vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Outlet, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import App from '@/App'

vi.mock('@/lib/theme', () => ({ ThemeProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('@/lib/accentColor', () => ({ AccentColorProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('@/lib/globalSportSettings', () => ({
  GlobalSportSettingsProvider: ({ children }: { children: React.ReactNode }) => children,
}))
vi.mock('@/lib/toast', () => ({ ToastProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('@/lib/modal', () => ({ ModalProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('@/components/Toast', () => ({ ToastContainer: () => null }))
vi.mock('@/components/Modal', () => ({ ModalContainer: () => null }))
vi.mock('@/components/ErrorBoundary', () => ({ default: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('@/components/AuthGuard', () => ({ default: () => <Outlet /> }))
vi.mock('@/components/MustChangePasswordGuard', () => ({ MustChangePasswordGuard: () => <Outlet /> }))
vi.mock('@/providers/UserProvider', () => ({
  UserProviderLayout: () => <Outlet />,
  useCurrentUserId: () => 'user-1',
}))
vi.mock('@/components/Layout', () => ({ default: () => <Outlet /> }))
vi.mock('@/components/AdminGuard', () => ({ AdminGuard: () => <Outlet /> }))
vi.mock('@/pages/ImportPage', () => ({ default: () => <div>Legacy Import Page</div> }))
vi.mock('@/pages/ImportQueuePage', () => ({ default: () => <div>Legacy Import Queue Page</div> }))
vi.mock('@/pages/ImportsPage', () => ({
  default: () => {
    const location = useLocation()
    return <div>Imports Page: {location.pathname + location.search}</div>
  },
}))

function renderAppAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <App />
    </MemoryRouter>,
  )
}

describe('legacy import routes', () => {
  it.each([
    ['/settings/import?tab=garmin', '/imports?source=garmin'],
    ['/settings/import?tab=strava', '/imports?source=strava'],
    ['/settings/import?tab=komoot', '/imports?source=komoot'],
    ['/settings/import?tab=queue', '/imports?view=review'],
    ['/queue', '/imports?view=review'],
    ['/workshop/import-queue', '/imports?view=review'],
    ['/settings/import', '/imports?view=import'],
  ])('redirects %s to %s', async (from, to) => {
    renderAppAt(from)

    expect(await screen.findByText(`Imports Page: ${to}`)).toBeInTheDocument()
  })
})
