import { useEffect, useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router-dom'
import { isAuthenticated, tryRestoreSession } from '@/lib/auth'

export default function AuthGuard() {
  const [checking, setChecking] = useState(true)
  const [authed, setAuthed] = useState(false)

  useEffect(() => {
    if (isAuthenticated()) {
      setAuthed(true)
      setChecking(false)
      return
    }

    tryRestoreSession().then((ok) => {
      setAuthed(ok)
      setChecking(false)
    })
  }, [])

  if (checking) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
        <span className="text-sm text-neutral-400">Sitzung wird geprüft...</span>
      </div>
    )
  }

  const location = useLocation()

  if (!authed) {
    const returnTo = encodeURIComponent(location.pathname + location.search)
    return <Navigate to={`/login?returnTo=${returnTo}`} replace />
  }

  return <Outlet />
}
