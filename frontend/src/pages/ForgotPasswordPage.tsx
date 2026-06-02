import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { forgotPassword } from '@/api/auth'
import { AUTH_INPUT_CLASS } from '@/pages/auth/styles'

export default function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [submitted, setSubmitted] = useState(false)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    try {
      await forgotPassword(email)
    } catch {
      // Intentionally ignored — always show success for timing-safety
    } finally {
      setLoading(false)
      setSubmitted(true)
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-white dark:bg-neutral-950">
      <div className="w-full max-w-sm space-y-6 px-4">
        <div className="text-center space-y-2">
          <h1 className="text-3xl font-bold text-accent-DEFAULT tracking-tight">skadia</h1>
          <p className="text-sm text-neutral-500 dark:text-neutral-400">{t('auth.forgot.title')}</p>
        </div>

        {submitted ? (
          <>
            <div className="p-4 rounded-lg bg-green-50 dark:bg-green-950/30 border border-green-200 dark:border-green-800">
              <p className="text-sm text-green-700 dark:text-green-400">
                {t('auth.forgot.submitted')}
              </p>
            </div>
            <div className="text-center">
              <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
                {t('auth.common.backToLogin')}
              </Link>
            </div>
          </>
        ) : (
          <>
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
              <button
                type="submit"
                disabled={loading}
                className="w-full cursor-pointer px-4 py-3 rounded-lg bg-neutral-900 dark:bg-neutral-100
                           text-white dark:text-neutral-900 font-medium text-sm
                           hover:bg-neutral-800 dark:hover:bg-neutral-200
                           transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {loading ? t('auth.common.sending') : t('auth.forgot.submit')}
              </button>
            </form>

            <div className="text-center">
              <Link to="/login" className="text-sm text-accent-DEFAULT hover:underline">
                {t('auth.common.backToLogin')}
              </Link>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
