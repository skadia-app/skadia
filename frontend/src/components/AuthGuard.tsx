import { useEffect, useState } from 'react'
import { Navigate, Outlet } from 'react-router-dom'
import { isAuthenticated, tryRestoreSession } from '@/lib/auth'
import { migrateTagConditions } from '@/api/rules'

const TAG_MIGRATION_KEY = 'skadia-tag-migration-done'

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

  // One-time migration of tag conditions to automation rules
  useEffect(() => {
    if (!authed) return
    if (localStorage.getItem(TAG_MIGRATION_KEY)) return
    migrateTagConditions()
      .then((res) => {
        if (res.migrated > 0) {
          console.info(`[Skadia] Migrated ${res.migrated} tag condition(s) to automation rules`)
        }
        localStorage.setItem(TAG_MIGRATION_KEY, '1')
      })
      .catch(() => {
        // Silently ignore — will retry next login
      })
  }, [authed])

  if (checking) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
        <span className="text-sm text-neutral-400">Sitzung wird geprüft...</span>
      </div>
    )
  }

  if (!authed) {
    return <Navigate to="/login" replace />
  }

  return <Outlet />
}
