import { useRef, useState, useEffect } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useNavigate, Link } from 'react-router-dom'
import { Check, LogOut, Settings } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useUser } from '@/providers/UserProvider'
import { logout } from '@/api/auth'
import { clearToken } from '@/lib/auth'
import { cn } from '@/lib/utils'
import { LANGUAGE_OPTIONS, type SupportedLanguage } from '@/i18n/languages'

function getUserInitial(displayName?: string, username?: string): string {
  const name = displayName ?? username ?? '?'
  return name.charAt(0).toUpperCase()
}

function getLanguageAccessibleName(option: { label: string; nativeLabel: string }): string {
  return option.nativeLabel === option.label ? option.label : `${option.nativeLabel}, ${option.label}`
}

export default function UserMenu() {
  const { t } = useTranslation()
  const { user, locale, setLocale } = useUser()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [savingLocale, setSavingLocale] = useState<SupportedLanguage | null>(null)
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

  const handleLocaleChange = async (nextLocale: SupportedLanguage) => {
    if (nextLocale === locale || savingLocale) return
    setSavingLocale(nextLocale)
    try {
      await setLocale(nextLocale)
    } finally {
      setSavingLocale(null)
    }
  }

  const initial = getUserInitial(user?.display_name, user?.username)
  const displayName = user?.display_name ?? user?.username ?? ''
  const email = user?.email ?? ''

  return (
    <div ref={containerRef} className="relative">
      <button
        onClick={() => setOpen(prev => !prev)}
        aria-label={t('common.openUserMenu')}
        aria-expanded={open}
        className="relative flex items-center justify-center w-8 h-8 rounded-full bg-accent-DEFAULT text-white text-sm font-semibold select-none hover:opacity-90 cursor-pointer transition-opacity focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-DEFAULT focus-visible:ring-offset-2"
      >
        {initial}
        <span
          aria-hidden="true"
          className="absolute -right-1 -bottom-1 text-xs leading-none bg-white dark:bg-neutral-900 rounded-full ring-1 ring-white dark:ring-neutral-900"
        >
          {LANGUAGE_OPTIONS.find(option => option.code === locale)?.flag}
        </span>
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-2 w-64 rounded-lg border border-neutral-200 dark:border-neutral-700 bg-white dark:bg-neutral-900 shadow-lg z-50 overflow-hidden">
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

          <div className="px-3 py-3 border-b border-neutral-100 dark:border-neutral-800">
            <p className="text-xs font-medium uppercase text-neutral-500 dark:text-neutral-400 mb-2">
              {t('common.language')}
            </p>
            <div className="grid grid-cols-1 gap-1">
              {LANGUAGE_OPTIONS.map(option => {
                const selected = option.code === locale
                return (
                  <button
                    key={option.code}
                    type="button"
                    aria-label={getLanguageAccessibleName(option)}
                    aria-pressed={selected}
                    disabled={savingLocale !== null}
                    onClick={() => { void handleLocaleChange(option.code) }}
                    className={cn(
                      'flex items-center gap-2 w-full px-2 py-1.5 rounded-md text-sm text-left transition-colors',
                      'hover:bg-neutral-100 dark:hover:bg-neutral-800 disabled:opacity-60 disabled:cursor-wait',
                      selected ? 'text-neutral-900 dark:text-neutral-100 bg-neutral-100 dark:bg-neutral-800' : 'text-neutral-700 dark:text-neutral-300',
                    )}
                  >
                    <span aria-hidden="true" className="text-base leading-none">{option.flag}</span>
                    <span className="flex-1">{option.nativeLabel}</span>
                    {selected && <Check className="w-4 h-4 text-accent-DEFAULT" aria-hidden="true" />}
                  </button>
                )
              })}
            </div>
          </div>

          <div className="py-1">
            <Link
              to="/settings?tab=konto"
              onClick={() => setOpen(false)}
              className={cn(
                'flex items-center gap-2 w-full px-4 py-2 text-sm text-neutral-700 dark:text-neutral-300',
                'hover:bg-neutral-100 dark:hover:bg-neutral-800 transition-colors',
              )}
            >
              <Settings className="w-4 h-4 shrink-0" aria-hidden="true" />
              {t('common.settings')}
            </Link>

            <div className="my-1 border-t border-neutral-100 dark:border-neutral-800" />

            <button
              onClick={() => { void handleLogout() }}
              className={cn(
                'flex items-center gap-2 w-full px-4 py-2 text-sm text-red-600 dark:text-red-400',
                'hover:bg-red-50 dark:hover:bg-red-950/40 cursor-pointer transition-colors',
              )}
            >
              <LogOut className="w-4 h-4 shrink-0" aria-hidden="true" />
              {t('common.logout')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
