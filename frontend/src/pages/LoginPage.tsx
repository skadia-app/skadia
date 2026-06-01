import { useEffect, useState } from 'react'
import { useNavigate, useSearchParams, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { isAuthenticated, isDevAuth, setToken } from '@/lib/auth'
import { clearQueryClientCache } from '@/lib/queryClient'
import { getAuthConfig, loginWithEmail, resendVerificationEmail, type AuthConfig } from '@/api/auth'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'
import { useToast } from '@/lib/toast'
import { API_BASE } from '@/api/client'

const LOGIN_MESSAGE_KEYS: Record<string, string> = {
  password_changed: 'passwordChanged',
  deletion_scheduled: 'deletionScheduled',
}

export default function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { addToast } = useToast()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [resendingVerification, setResendingVerification] = useState(false)
  const [canResendVerification, setCanResendVerification] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)

  const returnTo = searchParams.get('returnTo') || '/activities'

  useEffect(() => {
    if (isAuthenticated()) {
      navigate(returnTo, { replace: true })
      return
    }

    const errorParam = searchParams.get('error')
    if (errorParam) {
      setError(getAuthErrorMessage(errorParam))
    }

    const messageParam = searchParams.get('message')
    const messageKey = messageParam ? LOGIN_MESSAGE_KEYS[messageParam] : null
    if (messageKey) {
      addToast({
        type: 'info',
        title: t(`auth.loginMessages.${messageKey}.title`),
        message: t(`auth.loginMessages.${messageKey}.description`),
      })
    }

    getAuthConfig()
      .then(setConfig)
      .catch(() => {})
  }, [navigate, searchParams, addToast, t])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setCanResendVerification(false)
    setLoading(true)

    try {
      const data = await loginWithEmail(email, password)
      // Multi-user safety: drop any anonymous-keyed query results (e.g. cached
      // 401 errors under userId='anon') before the new user's queries fire.
      // clearToken handles the logout side; this handles the login transition.
      clearQueryClientCache()
      setToken(data.token)
      navigate(returnTo, { replace: true })
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'login_failed'
      setCanResendVerification(msg === 'email_not_verified')
      setError(getAuthErrorMessage(msg))
    } finally {
      setLoading(false)
    }
  }

  async function handleResendVerification() {
    if (!email || resendingVerification) return
    setResendingVerification(true)
    try {
      await resendVerificationEmail(email)
      addToast({ type: 'success', title: t('auth.login.verificationRequested') })
    } catch (err) {
      addToast({ type: 'error', title: t('auth.login.requestFailed'), message: err instanceof Error ? getAuthErrorMessage(err.message) : undefined })
    } finally {
      setResendingVerification(false)
    }
  }

  function handleDevLogin() {
    clearQueryClientCache()
    setToken('dev-token')
    navigate('/activities', { replace: true })
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            {t('auth.login.subtitle')}
          </p>
        </div>

        {error && (
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
            {canResendVerification && (
              <button
                type="button"
                onClick={handleResendVerification}
                disabled={resendingVerification || !email}
                className="mt-2 text-sm font-medium text-red-700 dark:text-red-300 hover:underline disabled:opacity-60 disabled:no-underline"
              >
                {resendingVerification ? t('auth.common.sending') : t('auth.login.resendVerification')}
              </button>
            )}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            placeholder={t('auth.fields.email')}
            required
            autoComplete="email"
            className={AUTH_INPUT_CLASS}
          />
          <input
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            placeholder={t('auth.fields.password')}
            required
            autoComplete="current-password"
            className={AUTH_INPUT_CLASS}
          />
          <button
            type="submit"
            disabled={loading}
            className="w-full cursor-pointer px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200
                       transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? t('auth.login.signingIn') : t('auth.login.submit')}
          </button>
        </form>

        <div className="text-center">
          <Link
            to="/forgot-password"
            className="text-sm text-neutral-500 hover:text-neutral-700 dark:hover:text-neutral-300 transition-colors"
          >
            {t('auth.login.forgotPassword')}
          </Link>
        </div>

        {(config?.social_providers?.length ?? 0) > 0 && (
          <>
            <div className="relative flex items-center">
              <div className="flex-grow border-t border-neutral-200 dark:border-neutral-700" />
              <span className="flex-shrink mx-3 text-xs text-neutral-400">{t('auth.common.or')}</span>
              <div className="flex-grow border-t border-neutral-200 dark:border-neutral-700" />
            </div>

            <div className="space-y-3">
              {config!.social_providers.includes('strava') && (
                <button
                  onClick={() => { window.location.href = `${API_BASE}/auth/social/strava` }}
                  className="w-full cursor-pointer px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-700 dark:text-neutral-300 text-sm font-medium
                             hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  {t('auth.login.socialLogin', { provider: 'Strava' })}
                </button>
              )}
              {config!.social_providers.includes('google') && (
                <button
                  onClick={() => { window.location.href = `${API_BASE}/auth/social/google` }}
                  className="w-full cursor-pointer px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                             text-neutral-700 dark:text-neutral-300 text-sm font-medium
                             hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
                >
                  {t('auth.login.socialLogin', { provider: 'Google' })}
                </button>
              )}
            </div>
          </>
        )}

        {(config?.registration_mode !== 'invite' || config?.bootstrap_mode === true) && (
          <div className="text-center text-sm text-neutral-500 dark:text-neutral-400">
            {t('auth.login.noAccount')}{' '}
            <Link
              to="/register"
              className="font-medium text-accent-DEFAULT hover:underline"
            >
              {t('auth.register.submit')}
            </Link>
          </div>
        )}

        {isDevAuth() && (
          <button
            onClick={handleDevLogin}
            className="w-full cursor-pointer px-4 py-3 rounded-lg border border-neutral-200 dark:border-neutral-700
                       text-neutral-600 dark:text-neutral-300 text-sm
                       hover:bg-neutral-50 dark:hover:bg-neutral-800 transition-colors"
          >
            {t('auth.login.devLogin')}
          </button>
        )}
      </div>
    </div>
  )
}
