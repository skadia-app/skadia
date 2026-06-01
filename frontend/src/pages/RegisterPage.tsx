import { useState, useEffect } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { register, getAuthConfig, type AuthConfig } from '@/api/auth'
import { isAuthenticated } from '@/lib/auth'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'

export default function RegisterPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const [config, setConfig] = useState<AuthConfig | null>(null)

  useEffect(() => {
    if (isAuthenticated()) {
      navigate('/activities', { replace: true })
      return
    }
    getAuthConfig()
      .then((cfg) => {
        if (cfg.registration_mode === 'invite' && !cfg.bootstrap_mode) {
          navigate('/login', { replace: true })
          return
        }
        setConfig(cfg)
      })
      .catch(() => {})
  }, [navigate])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (password.length < 10) {
      setError(t('auth.errors.password_too_short'))
      return
    }
    if (password !== confirmPassword) {
      setError(t('auth.errors.password_mismatch'))
      return
    }

    setLoading(true)
    try {
      await register(email, username, password)
      setSuccess(true)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'registration_failed'
      setError(getAuthErrorMessage(msg))
    } finally {
      setLoading(false)
    }
  }

  if (success) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
        <div className="w-full max-w-sm space-y-6 px-4">
          <div className="text-center space-y-2">
            <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          </div>
          <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
            <p className="text-sm text-green-700 dark:text-green-400 font-medium">
              {t('auth.register.successTitle')}
            </p>
            <p className="text-sm text-green-600 dark:text-green-500 mt-1">
              {t('auth.register.successDescription')}
            </p>
          </div>
          <div className="text-center">
            <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
              {t('auth.common.toLogin')}
            </Link>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('auth.register.title')}</p>
        </div>

        {config?.bootstrap_mode === true && (
          <div className="rounded-md border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950 dark:text-amber-100">
            <p className="font-medium">{t('auth.register.bootstrapTitle')}</p>
            <p className="mt-1">
              {t('auth.register.bootstrapDescription')}
            </p>
          </div>
        )}

        {error && (
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
            <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder={t('auth.fields.email')}
            required
            autoComplete="email"
            className={AUTH_INPUT_CLASS}
          />
          <input
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder={t('auth.fields.username')}
            required
            autoComplete="username"
            className={AUTH_INPUT_CLASS}
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={t('auth.fields.passwordMin')}
            required
            minLength={10}
            autoComplete="new-password"
            className={AUTH_INPUT_CLASS}
          />
          <input
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            placeholder={t('auth.fields.confirmPassword')}
            required
            autoComplete="new-password"
            className={AUTH_INPUT_CLASS}
          />
          <button
            type="submit"
            disabled={loading || config === null}
            className="w-full cursor-pointer px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200
                       transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? t('auth.register.submitting') : t('auth.register.submit')}
          </button>
        </form>

        <div className="text-center text-sm text-neutral-500 dark:text-neutral-400">
          {t('auth.register.hasAccount')}{' '}
          <Link to="/login" className="font-medium text-accent-DEFAULT hover:underline">
            {t('auth.login.submit')}
          </Link>
        </div>
      </div>
    </div>
  )
}
