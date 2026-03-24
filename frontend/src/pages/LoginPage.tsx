import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { isAuthenticated, isDevAuth, setToken } from '@/lib/auth'
import { getAuthConfig, loginWithEmail, type AuthConfig } from '@/api/auth'
import { AUTH_AUTH_INPUT_CLASS } from '@/pages/auth/styles'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? '/api/v1'

const ERROR_MESSAGES: Record<string, string> = {
  email_exists:
    'Diese E-Mail ist bereits registriert. Bitte melde dich an und verknüpfe dein Konto in den Einstellungen.',
  account_delayed: 'Zu viele Fehlversuche. Bitte versuche es später erneut.',
  email_not_verified: 'Bitte verifiziere zuerst deine E-Mail-Adresse.',
  account_inactive: 'Dein Konto ist noch nicht freigeschaltet.',
  invalid_credentials: 'E-Mail oder Passwort ungültig.',
  password_compromised:
    'Dieses Passwort wurde in einem Datenleck gefunden. Bitte wähle ein anderes.',
  provider_error: 'Fehler beim Social Login. Bitte versuche es erneut.',
  exchange_failed: 'Anmeldung fehlgeschlagen. Bitte versuche es erneut.',
}

export default function LoginPage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)

  useEffect(() => {
    if (isAuthenticated()) {
      navigate('/activities', { replace: true })
      return
    }

    const errorParam = searchParams.get('error')
    if (errorParam) {
      setError(ERROR_MESSAGES[errorParam] ?? 'Ein Fehler ist aufgetreten.')
    }

    getAuthConfig()
      .then(setConfig)
      .catch(() => {})
  }, [navigate, searchParams])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)

    try {
      const data = await loginWithEmail(email, password)
      setToken(data.token)
      navigate('/activities', { replace: true })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Anmeldung fehlgeschlagen'
      setError(ERROR_MESSAGES[msg] ?? msg)
    } finally {
      setLoading(false)
    }
  }

  function handleDevLogin() {
    setToken('dev-token')
    navigate('/activities', { replace: true })
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            Melde dich an, um fortzufahren
          </p>
        </div>

        {error && (
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            placeholder="E-Mail"
            required
            autoComplete="email"
            className={AUTH_INPUT_CLASS}
          />
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            placeholder="Passwort"
            required
            autoComplete="current-password"
            className={AUTH_INPUT_CLASS}
          />
          <button
            type="submit"
            disabled={loading}
            className="w-full px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200
                       transition-colors disabled:opacity-50"
          >
            {loading ? 'Wird angemeldet…' : 'Anmelden'}
          </button>
        </form>

        <div className="text-center">
          <Link
            to="/forgot-password"
            className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors"
          >
            Passwort vergessen?
          </Link>
        </div>

        {(config?.social_providers?.length ?? 0) > 0 && (
          <>
            <div className="relative flex items-center">
              <div className="flex-grow border-t border-neutral-200 dark:border-neutral-700" />
              <span className="flex-shrink mx-3 text-xs text-neutral-400">oder</span>
              <div className="flex-grow border-t border-neutral-200 dark:border-neutral-700" />
            </div>

            <div className="space-y-3">
              {config!.social_providers.includes('strava') && (
                <button
                  onClick={() => { window.location.href = `${API_BASE}/auth/social/strava` }}
                  className="w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-700 dark:text-neutral-300 text-sm font-medium
                             hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  Mit Strava anmelden
                </button>
              )}
              {config!.social_providers.includes('google') && (
                <button
                  onClick={() => { window.location.href = `${API_BASE}/auth/social/google` }}
                  className="w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-700 dark:text-neutral-300 text-sm font-medium
                             hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  Mit Google anmelden
                </button>
              )}
            </div>
          </>
        )}

        {config?.registration_mode !== 'invite' && (
          <div className="text-center text-sm text-neutral-500 dark:text-neutral-400">
            Noch kein Konto?{' '}
            <Link
              to="/register"
              className="font-medium text-accent-DEFAULT hover:underline"
            >
              Registrieren
            </Link>
          </div>
        )}

        {isDevAuth() && (
          <button
            onClick={handleDevLogin}
            className="w-full px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-300 text-sm
                       hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            Dev-Anmeldung (Test)
          </button>
        )}
      </div>
    </div>
  )
}
