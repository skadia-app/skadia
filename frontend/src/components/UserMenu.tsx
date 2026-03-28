import { useRef, useState, useEffect } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useNavigate, Link } from 'react-router-dom'
import { useUser } from '@/providers/UserProvider'
import { logout } from '@/api/auth'
import { clearToken } from '@/lib/auth'
import { cn } from '@/lib/utils'

const LABELS = {
  settings: 'Einstellungen',
  logout: 'Abmelden',
}

function getUserInitial(displayName?: string, username?: string): string {
  const name = displayName ?? username ?? '?'
  return name.charAt(0).toUpperCase()
}

export default function UserMenu() {
  const { user } = useUser()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  // Close on outside click
  useClickOutside(containerRef, () => setOpen(false), open)

  // Close on Escape
  useEffect(() => {
    if (!open) return
    function handleKey(e: KeyboardEvent) {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [open])

  const handleLogout = async () => {
    setOpen(false)
    // CRITICAL: clear token immediately to prevent 401 race condition
    clearToken()
    try {
      await logout()
    } catch {
      // best-effort server-side revoke — ignore errors
    }
    navigate('/login')
  }

  const initial = getUserInitial(user?.display_name, user?.username)
  const displayName = user?.display_name ?? user?.username ?? ''
  const email = user?.email ?? ''

  return (
    <div ref={containerRef} className="relative">
      <button
        onClick={() => setOpen(prev => !prev)}
        aria-label="Benutzermenü öffnen"
        aria-expanded={open}
        className="flex items-center justify-center w-8 h-8 rounded-full bg-accent-DEFAULT text-white text-sm font-semibold select-none hover:opacity-90 cursor-pointer transition-opacity focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-2"
      >
        {initial}
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-56 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 overflow-hidden">
          {/* User info header */}
          <div className="px-4 py-3 border-b border-neutral-100 dark:border-neutral-800">
            <p className="text-sm font-medium text-neutral-900 dark:text-neutral-100 truncate">
              {displayName}
            </p>
            {email && (
              <p className="text-xs text-neutral-500 dark:text-neutral-400 truncate mt-0.5">
                {email}
              </p>
            )}
          </div>

          {/* Menu items */}
          <div className="py-1">
            <Link
              to="/settings?tab=konto"
              onClick={() => setOpen(false)}
              className={cn(
                'flex items-center gap-2 w-full px-4 py-2 text-sm text-neutral-700 dark:text-neutral-300',
                'hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors',
              )}
            >
              <SettingsIcon />
              {LABELS.settings}
            </Link>

            <div className="my-1 border-t border-neutral-100 dark:border-neutral-800" />

            <button
              onClick={() => { void handleLogout() }}
              className={cn(
                'flex items-center gap-2 w-full px-4 py-2 text-sm text-red-600 dark:text-red-400',
                'hover:bg-red-50 dark:hover:bg-red-950/40 cursor-pointer transition-colors',
              )}
            >
              <LogoutIcon />
              {LABELS.logout}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function SettingsIcon() {
  return (
    <svg className="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </svg>
  )
}

function LogoutIcon() {
  return (
    <svg className="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" y1="12" x2="9" y2="12" />
    </svg>
  )
}
