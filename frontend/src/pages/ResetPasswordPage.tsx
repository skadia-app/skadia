import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { resetPassword } from '@/api/auth'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'
import { getAuthErrorMessage } from '@/pages/auth/errorMessages'

export default function ResetPasswordPage() {
  const { t } = useTranslation()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)

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
    if (!token) {
      setError(t('auth.errors.missing_token'))
      return
    }

    setLoading(true)
    try {
      await resetPassword(token, password)
      setSuccess(true)
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'reset_failed'
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
              {t('auth.reset.successTitle')}
            </p>
            <p className="text-sm text-green-600 dark:text-green-500 mt-1">
              {t('auth.reset.successDescription')}
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
          <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('auth.reset.title')}</p>
        </div>

        {!token && (
          <div className="p-3 rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-800">
            <p className="text-sm text-red-600 dark:text-red-400">
              {t('auth.reset.invalidLink')}
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
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder={t('auth.fields.newPasswordMin')}
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
            disabled={loading || !token}
            className="w-full cursor-pointer px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                       text-white dark:text-neutral-900 font-medium text-sm
                       hover:bg-neutral-800 dark:hover:bg-neutral-200
                       transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {loading ? t('auth.common.saving') : t('auth.reset.submit')}
          </button>
        </form>

        <div className="text-center">
          <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
            {t('auth.common.backToLogin')}
          </Link>
        </div>
      </div>
    </div>
  )
}
