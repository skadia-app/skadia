import { Navigate, Outlet } from 'react-router-dom'
import { useUser } from '@/providers/UserProvider'

export function AdminGuard() {
  const { isAdmin } = useUser()
  if (!isAdmin) return <Navigate to="/" replace />
  return <Outlet />
}
