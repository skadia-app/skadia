import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { useUser } from '@/providers/UserProvider'

export function MustChangePasswordGuard() {
  const { mustChangePassword } = useUser()
  const location = useLocation()

  // Allow access to settings page (password change tab lives there)
  const isSettingsRoute = location.pathname.startsWith('/settings')

  if (mustChangePassword && !isSettingsRoute) {
    return <Navigate to="/settings?tab=konto" replace />
  }
  return <Outlet />
}
